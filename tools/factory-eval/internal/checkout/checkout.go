// Package checkout makes the throwaway clone an eval run works in, and
// reads back the branches the run left behind. The clone is made under a
// fresh temp dir and detached at the pinned commit. Its origin remote, its
// branches, its tags and its reflogs are removed and unreachable objects
// are pruned, so it holds nothing newer than the pin and nothing the
// agents do can be pushed back. Fingerprint detects whether a run reached
// the source checkout anyway.
package checkout

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The git identity every commit in a clone carries. The harness sets it in
// the clone's local config, which the worktrees share, so no commit
// prompts and the user's own identity never lands on an eval branch.
const (
	IdentityName  = "factory-eval"
	IdentityEmail = "factory-eval@localhost"
)

// Checkout is one throwaway clone. Root holds the clone and the directory
// factory workers claim their worktrees under; Remove deletes all of it.
type Checkout struct {
	Root      string
	Repo      string
	Worktrees string
	Commit    string
}

// Create clones src under a new temp dir inside parent (the system temp
// dir when parent is empty) and detaches the clone at commit. On failure
// the partial dir is removed.
//
// Root is the resolved path, symlinks included. claude records a session's
// working directory in resolved form, and on macOS the system temp dir is
// a symlink (/var/folders is /private/var/folders), so a path the harness
// hands to claude under the unresolved name is, to claude, a different
// directory from the one the session runs in. The first paid run lost
// every cd into the clone to that difference.
func Create(ctx context.Context, src, commit, parent string) (*Checkout, error) {
	made, err := os.MkdirTemp(parent, "factory-eval-")
	if err != nil {
		return nil, fmt.Errorf("checkout: temp dir: %w", err)
	}
	root, err := filepath.EvalSymlinks(made)
	if err != nil {
		_ = os.RemoveAll(made)
		return nil, fmt.Errorf("checkout: resolve temp dir: %w", err)
	}
	c := &Checkout{
		Root:      root,
		Repo:      filepath.Join(root, "thismoon"),
		Worktrees: filepath.Join(root, "worktrees"),
		Commit:    commit,
	}
	if err := c.clone(ctx, src); err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	return c, nil
}

func (c *Checkout) clone(ctx context.Context, src string) error {
	if _, err := git(ctx, "", "clone", "--quiet", "--no-checkout", src, c.Repo); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"checkout", "--quiet", "--detach", c.Commit},
		{"remote", "remove", "origin"},
		{"config", "user.name", IdentityName},
		{"config", "user.email", IdentityEmail},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := git(ctx, c.Repo, args...); err != nil {
			return err
		}
	}
	if err := c.dropHistoryPastPin(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Worktrees, 0o755); err != nil {
		return fmt.Errorf("checkout: worktrees dir: %w", err)
	}
	return nil
}

// dropHistoryPastPin deletes every local branch and tag, expires the
// reflogs and prunes what that left unreachable. The clone then has no
// main that moves with the source and no object newer than the pin, and
// with origin gone git fetch has no remote to bring them back from.
func (c *Checkout) dropHistoryPastPin(ctx context.Context) error {
	refs, err := c.LocalRefs(ctx)
	if err != nil {
		return err
	}
	var deletes strings.Builder
	for _, ref := range refs {
		deletes.WriteString("delete " + ref + "\n")
	}
	if _, err := run(ctx, c.Repo, deletes.String(), "update-ref", "--stdin"); err != nil {
		return err
	}
	if _, err := git(ctx, c.Repo, "reflog", "expire", "--expire=now", "--all"); err != nil {
		return err
	}
	_, err = git(ctx, c.Repo, "-c", "gc.auto=0", "gc", "--quiet", "--prune=now")
	return err
}

// LocalRefs lists the clone's branches and tags.
func (c *Checkout) LocalRefs(ctx context.Context) ([]string, error) {
	out, err := git(ctx, c.Repo, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// Remove deletes the clone and every worktree under Root.
func (c *Checkout) Remove() error {
	if err := os.RemoveAll(c.Root); err != nil {
		return fmt.Errorf("checkout: remove %s: %w", c.Root, err)
	}
	return nil
}

// BranchExists reports whether branch names a commit in the clone.
func (c *Checkout) BranchExists(ctx context.Context, branch string) bool {
	_, err := git(ctx, c.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// ChangedFiles lists the paths that differ between the pinned commit and
// the tip of branch, sorted as git prints them.
func (c *Checkout) ChangedFiles(ctx context.Context, branch string) ([]string, error) {
	out, err := git(ctx, c.Repo, "diff", "--name-only", c.Commit, "refs/heads/"+branch)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// Original returns a file's content at the pinned commit, or "" when the
// file did not exist there.
func (c *Checkout) Original(ctx context.Context, path string) string {
	out, err := git(ctx, c.Repo, "show", c.Commit+":"+path)
	if err != nil {
		return ""
	}
	return string(out)
}

// AtBranch returns a file's content at the tip of branch, or "" when the
// branch or the file is missing.
func (c *Checkout) AtBranch(ctx context.Context, branch, path string) string {
	out, err := git(ctx, c.Repo, "show", "refs/heads/"+branch+":"+path)
	if err != nil {
		return ""
	}
	return string(out)
}

// Worktree checks branch out, detached, into a fresh directory under Root
// for the deterministic checks, and returns that directory and a func that
// removes the worktree again.
func (c *Checkout) Worktree(ctx context.Context, branch string) (string, func(), error) {
	dir, err := os.MkdirTemp(c.Root, "check-")
	if err != nil {
		return "", nil, fmt.Errorf("checkout: check dir: %w", err)
	}
	_, err = git(ctx, c.Repo, "worktree", "add", "--quiet", "--detach", "--force", dir,
		"refs/heads/"+branch)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	remove := func() {
		_, _ = git(context.Background(), c.Repo, "worktree", "remove", "--force", dir)
	}
	return dir, remove, nil
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return run(ctx, dir, "", args...)
}

// run runs git in dir with stdin as its input.
func run(ctx context.Context, dir, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("checkout: git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
