package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/repofind"
)

// gitTimeout bounds every git call the runner makes around a case.
const gitTimeout = 60 * time.Second

// workspaceRoots says where repo names resolve and where throwaway
// workspaces land. TmpRoot "" means the system temp directory.
type workspaceRoots struct {
	SrcRoot      string
	WorktreeRoot string
	TmpRoot      string
}

// lockReason starts the lock reason of every eval worktree. The runner's
// pid follows it, so a sweep can tell a live run's worktree from a stale one.
const lockReason = "harness-eval"

// Workspace is a prepared directory a case runs in. Cleanup removes what
// prepareWorkspace created; a repo workspace is the canonical checkout and
// is left alone.
type Workspace struct {
	Dir     string
	Kind    string
	repoDir string
	branch  string
	base    string
}

// prepareWorkspace sets up the directory a case runs in. A worktree
// workspace is a fresh, locked git worktree on a throwaway eval/ branch,
// cut from the canonical checkout's HEAD, at
// <WorktreeRoot>/<repo>/eval-<id>-<stamp>.
func prepareWorkspace(
	ctx context.Context,
	spec WorkspaceSpec,
	roots workspaceRoots,
	caseID string,
	now time.Time,
) (*Workspace, error) {
	switch spec.Kind {
	case workspaceRepo:
		dir, err := resolveRepo(ctx, spec.Repo, roots.SrcRoot)
		if err != nil {
			return nil, err
		}
		return &Workspace{Dir: dir, Kind: spec.Kind}, nil
	case workspaceWorktree:
		repoDir, err := resolveRepo(ctx, spec.Repo, roots.SrcRoot)
		if err != nil {
			return nil, err
		}
		return addWorktree(ctx, repoDir, roots.WorktreeRoot, caseID, now)
	case workspaceTmpdir:
		dir, err := os.MkdirTemp(roots.TmpRoot, "harness-eval-"+caseID+"-")
		if err != nil {
			return nil, fmt.Errorf("tmpdir workspace: %w", err)
		}
		return &Workspace{Dir: dir, Kind: spec.Kind}, nil
	}
	return nil, fmt.Errorf("unknown workspace kind %q", spec.Kind)
}

// resolveRepo turns a case's repo reference into the checkout's top-level
// directory. A bare name resolves under srcRoot; anything with a slash or a
// leading ~ is a path.
func resolveRepo(ctx context.Context, ref, srcRoot string) (string, error) {
	dir := repofind.ExpandHome(ref)
	if !strings.ContainsRune(ref, '/') && !strings.HasPrefix(ref, "~") {
		dir = filepath.Join(repofind.ExpandHome(srcRoot), ref)
	}
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo %q at %s: %w", ref, dir, err)
	}
	return top, nil
}

func addWorktree(
	ctx context.Context,
	repoDir, root, caseID string,
	now time.Time,
) (*Workspace, error) {
	stamp := now.Format("20060102-150405")
	dir := filepath.Join(
		repofind.ExpandHome(root), filepath.Base(repoDir), "eval-"+caseID+"-"+stamp)
	branch := evalBranchPrefix + caseID + "-" + stamp
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, fmt.Errorf("worktree workspace: %w", err)
	}
	reason := fmt.Sprintf("%s pid %d", lockReason, os.Getpid())
	_, err := git(ctx, repoDir,
		"worktree", "add", "--lock", "--reason", reason, "-b", branch, dir, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("worktree workspace: %w", err)
	}
	ws := &Workspace{Dir: dir, Kind: workspaceWorktree, repoDir: repoDir, branch: branch}
	base, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("worktree workspace: %w", err), ws.Cleanup())
	}
	ws.base = base
	return ws, nil
}

// Canonical is the canonical checkout a workspace hangs off: the checkout
// itself for a repo workspace, the main checkout for a worktree, and ""
// for a tmpdir.
func (w *Workspace) Canonical() string {
	switch w.Kind {
	case workspaceRepo:
		return w.Dir
	case workspaceWorktree:
		return w.repoDir
	}
	return ""
}

// Commits returns the subjects of the commits made in a worktree workspace
// since it was cut, oldest first. Other kinds have none.
func (w *Workspace) Commits(ctx context.Context) ([]string, error) {
	if w.Kind != workspaceWorktree {
		return nil, nil
	}
	out, err := git(ctx, w.Dir, "log", "--reverse", "--format=%s", w.base+"..HEAD")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Cleanup removes a worktree workspace and its branch, or a tmpdir
// workspace's directory. It runs on its own deadline so an interrupted run
// still tears down.
func (w *Workspace) Cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	switch w.Kind {
	case workspaceWorktree:
		return removeEvalWorktree(ctx, w.repoDir, w.Dir, w.branch)
	case workspaceTmpdir:
		return os.RemoveAll(w.Dir)
	}
	return nil
}

// removeEvalWorktree removes a locked eval worktree (the doubled --force
// is what removes a locked one) and deletes its branch.
func removeEvalWorktree(ctx context.Context, repoDir, dir, branch string) error {
	_, rmErr := git(ctx, repoDir, "worktree", "remove", "--force", "--force", dir)
	_, brErr := git(ctx, repoDir, "branch", "-D", branch)
	return errors.Join(rmErr, brErr)
}

// git runs one git command in dir and returns its trimmed stdout; the
// error carries stderr.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
