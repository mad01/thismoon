// Package sharing moves knowledge between a private vault and a shared one
// through the shared vault's repo: promote opens a pull request that adds a
// private note, an edit of a shared note becomes a pull request, a flag
// becomes an issue, and sync pulls and pushes each vault and settles the
// promotions whose pull requests merged or closed. The shared clone's own
// checkout is never touched: every pull request is prepared in a temporary
// worktree under brain's state directory.
package sharing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/ghcli"
	"github.com/mad01/thismoon/services/brain/internal/gitvault"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// maxBranchTries bounds the -2, -3, ... suffixes a branch name gets when
// the remote has the name already.
const maxBranchTries = 9

// worktree is a temporary checkout of a shared vault's upstream tip.
type worktree struct {
	// shared is the configured shared vault, its clone the checkout the
	// worktree belongs to.
	shared *vault.Vault
	clone  *gitvault.Repo
	// remote and base are where the pull request goes: the remote the
	// clone tracks and the branch it merges into.
	remote, base string
	// repo and vault are the worktree, as a git checkout and as a vault
	// brain may write to directly.
	repo  *gitvault.Repo
	vault *vault.Vault
}

// openWorktree fetches the shared vault's remote and checks its tip out
// into a fresh directory under the state dir.
func openWorktree(lib *library.Library, shared *vault.Vault) (*worktree, error) {
	clone := lib.Git(shared)
	if !clone.IsRepo() {
		return nil, fmt.Errorf(
			"shared vault %s at %s is not a git clone; brain opens pull requests from a clone of its repo",
			shared.Name,
			shared.Root,
		)
	}
	remote, base, err := target(clone)
	if err != nil {
		return nil, fmt.Errorf("shared vault %s: %w", shared.Name, err)
	}
	if err := clone.Fetch(remote); err != nil {
		return nil, fmt.Errorf("shared vault %s: %w", shared.Name, err)
	}
	root, err := brain.WorktreeRoot()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, shared.Name+"-")
	if err != nil {
		return nil, err
	}
	repo, err := clone.AddWorktree(dir, "refs/remotes/"+remote+"/"+base)
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(dir))
	}
	return &worktree{
		shared: shared, clone: clone, remote: remote, base: base,
		repo: repo, vault: &vault.Vault{Root: dir, Name: shared.Name},
	}, nil
}

// target returns the remote and branch a pull request on the clone goes
// to: what its branch tracks, else origin's default branch.
func target(clone *gitvault.Repo) (remote, base string, err error) {
	remote, base, err = clone.Upstream()
	if err == nil {
		return remote, base, nil
	}
	remotes, rerr := clone.Remotes()
	if rerr != nil {
		return "", "", rerr
	}
	if !slices.Contains(remotes, "origin") {
		return "", "", errors.New("the clone has no remote to open a pull request on")
	}
	head := clone.SymbolicRef("refs/remotes/origin/HEAD")
	if head == "" {
		return "", "", fmt.Errorf(
			"%w; set one with: git branch --set-upstream-to origin/<branch>",
			err,
		)
	}
	return "origin", strings.TrimPrefix(head, "refs/remotes/origin/"), nil
}

// close removes the worktree and its directory.
func (w *worktree) close() error {
	return w.clone.RemoveWorktree(w.vault.Root)
}

// repoName returns the HOST/OWNER/REPO gh is told to use, or "" to let gh
// read it from the clone's remotes.
func (w *worktree) repoName() string {
	url, err := w.clone.RemoteURL(w.remote)
	if err != nil {
		return ""
	}
	return ghcli.RepoOf(url)
}

// freeBranch returns name, or the first of name-2, name-3, ... that the
// remote does not have yet.
func (w *worktree) freeBranch(name string) (string, error) {
	for n := 1; n <= maxBranchTries; n++ {
		cand := name
		if n > 1 {
			cand = fmt.Sprintf("%s-%d", name, n)
		}
		taken, err := w.clone.RemoteBranchExists(w.remote, cand)
		if err != nil {
			return "", err
		}
		if !taken {
			return cand, nil
		}
	}
	return "", fmt.Errorf(
		"branches %s to %s-%d exist on %s already",
		name,
		name,
		maxBranchTries,
		w.remote,
	)
}

// publish commits paths in the worktree with message, pushes the commit to
// branch and opens a pull request from it. It returns the pull request's
// URL.
func (w *worktree) publish(
	ctx context.Context,
	branch, message string,
	paths []string,
	pr ghcli.PR,
) (string, error) {
	if err := w.repo.Commit(message, paths); err != nil {
		return "", err
	}
	if err := w.repo.Push(w.remote, branch); err != nil {
		return "", err
	}
	pr.Repo, pr.Head, pr.Base = w.repoName(), branch, w.base
	url, err := ghcli.CreatePR(ctx, w.shared.Root, pr)
	if err != nil {
		return "", fmt.Errorf(
			"pushed branch %s to %s, but opening the pull request failed: %w "+
				"(open it by hand from that branch)", branch, w.remote, err,
		)
	}
	return url, nil
}
