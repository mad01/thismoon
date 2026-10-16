// Package checkout makes the throwaway clone an eval run works in, and
// reads back the branches the run left behind. A run never touches the
// source repo: the clone is made under a fresh temp dir, detached at the
// pinned commit, and its origin remote is removed so nothing the agents do
// can be pushed back.
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
func Create(ctx context.Context, src, commit, parent string) (*Checkout, error) {
	root, err := os.MkdirTemp(parent, "factory-eval-")
	if err != nil {
		return nil, fmt.Errorf("checkout: temp dir: %w", err)
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
	if _, err := git(ctx, c.Repo, "checkout", "--quiet", "--detach", c.Commit); err != nil {
		return err
	}
	if _, err := git(ctx, c.Repo, "remote", "remove", "origin"); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Worktrees, 0o755); err != nil {
		return fmt.Errorf("checkout: worktrees dir: %w", err)
	}
	return nil
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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("checkout: git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
