package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/mad01/thismoon/kit/repofind"
)

// evalBranchPrefix starts every branch a worktree workspace creates.
const evalBranchPrefix = "eval/"

// evalBranchRE matches the whole name addWorktree gives a branch:
// eval/<id>-<YYYYMMDD-HHMMSS>.
var evalBranchRE = regexp.MustCompile(`^eval/.+-[0-9]{8}-[0-9]{6}$`)

// detachedHead stands in for the branch when HEAD points at no branch.
const detachedHead = "(detached)"

// snapshot is the state of a canonical checkout the runner compares before
// and after a case: the commit HEAD points at, the branch it is on, and
// the porcelain status.
type snapshot struct {
	Head   string
	Branch string
	Status string
}

// takeSnapshot reads dir's HEAD commit, its branch, and `git status
// --porcelain`. An empty dir (a tmpdir workspace has no canonical
// checkout) gives the zero snapshot.
func takeSnapshot(ctx context.Context, dir string) (snapshot, error) {
	if dir == "" {
		return snapshot{}, nil
	}
	head, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return snapshot{}, err
	}
	branch, err := headBranch(ctx, dir)
	if err != nil {
		return snapshot{}, err
	}
	status, err := git(ctx, dir, "status", "--porcelain")
	if err != nil {
		return snapshot{}, err
	}
	return snapshot{Head: head, Branch: branch, Status: status}, nil
}

// headBranch returns the ref HEAD points at, or detachedHead. `git
// symbolic-ref -q` exits 1, silently, exactly when HEAD is detached.
func headBranch(ctx context.Context, dir string) (string, error) {
	ref, err := git(ctx, dir, "symbolic-ref", "-q", "HEAD")
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return detachedHead, nil
	}
	return ref, err
}

// changes describes how after differs from s, or returns "" when it does
// not.
func (s snapshot) changes(after snapshot) string {
	var out []string
	if s.Branch != after.Branch {
		out = append(out, fmt.Sprintf("branch went from %s to %s", s.Branch, after.Branch))
	}
	if s.Head != after.Head {
		out = append(
			out,
			fmt.Sprintf("HEAD moved from %s to %s", shortSHA(s.Head), shortSHA(after.Head)),
		)
	}
	if s.Status != after.Status {
		out = append(out, fmt.Sprintf("status went from %q to %q",
			clip(s.Status, 200), clip(after.Status, 200)))
	}
	return strings.Join(out, "; ")
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// worktreeEntry is one block of `git worktree list --porcelain`. Branch is
// the short name, "" for a detached HEAD.
type worktreeEntry struct {
	Path   string
	Branch string
	Locked bool
	Reason string
}

func parseWorktreeList(out string) []worktreeEntry {
	var entries []worktreeEntry
	for line := range strings.SplitSeq(out, "\n") {
		key, value, _ := strings.Cut(line, " ")
		if key == "worktree" {
			entries = append(entries, worktreeEntry{Path: value})
			continue
		}
		if len(entries) == 0 {
			continue
		}
		cur := &entries[len(entries)-1]
		switch key {
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "locked":
			cur.Locked, cur.Reason = true, value
		}
	}
	return entries
}

// liveRunner reports whether a lock reason names a harness-eval runner
// whose process is still alive.
func liveRunner(reason string) bool {
	rest, ok := strings.CutPrefix(reason, lockReason+" pid ")
	if !ok {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil || pid <= 0 {
		return false
	}
	err = syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// sweepResult counts what a sweep removed.
type sweepResult struct {
	Worktrees int
	Branches  int
}

// staleEvalWorktree reports whether wt is a worktree a runner created and
// no live runner holds: it sits in an eval-* directory under root, and its
// lock reason names a harness-eval runner whose process has exited. root
// must be symlink-resolved, because git reports resolved paths.
func staleEvalWorktree(wt worktreeEntry, root string) bool {
	rel, err := filepath.Rel(root, wt.Path)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	if !strings.HasPrefix(filepath.Base(wt.Path), "eval-") {
		return false
	}
	return strings.HasPrefix(wt.Reason, lockReason+" pid ") && !liveRunner(wt.Reason)
}

// sweepStale removes what earlier runs left in repoDir when they died
// before Cleanup. It touches only stale eval worktrees under root (see
// staleEvalWorktree), and deletes a swept worktree's branch only when the
// name is exactly one addWorktree makes. Every other worktree and branch,
// eval/ ones included, is left alone.
func sweepStale(ctx context.Context, repoDir, root string) (sweepResult, error) {
	var res sweepResult
	root, err := filepath.EvalSymlinks(repofind.ExpandHome(root))
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	out, err := git(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return res, err
	}
	var errs []error
	for _, wt := range parseWorktreeList(out) {
		if !staleEvalWorktree(wt, root) {
			continue
		}
		if _, err := git(ctx, repoDir, "worktree", "remove", "--force", "--force", wt.Path); err != nil {
			errs = append(errs, err)
			continue
		}
		res.Worktrees++
		if !evalBranchRE.MatchString(wt.Branch) {
			continue
		}
		if _, err := git(ctx, repoDir, "branch", "-D", wt.Branch); err != nil {
			errs = append(errs, err)
			continue
		}
		res.Branches++
	}
	return res, errors.Join(errs...)
}

// sweepWorktreeRepos runs sweepStale on every repo a worktree case in
// cases names, once each, and logs one line per repo. A failed sweep is
// logged and does not stop the run.
func sweepWorktreeRepos(ctx context.Context, cases []Case, roots workspaceRoots, log io.Writer) {
	seen := map[string]bool{}
	for _, c := range cases {
		if c.Workspace.Kind != workspaceWorktree || seen[c.Workspace.Repo] {
			continue
		}
		seen[c.Workspace.Repo] = true
		repoDir, err := resolveRepo(ctx, c.Workspace.Repo, roots.SrcRoot)
		if err != nil {
			fmt.Fprintf(log, "sweep %s: %v\n", c.Workspace.Repo, err)
			continue
		}
		res, err := sweepStale(ctx, repoDir, roots.WorktreeRoot)
		fmt.Fprintf(log, "sweep %s: removed %d stale eval worktrees and %d eval branches\n",
			repoDir, res.Worktrees, res.Branches)
		if err != nil {
			fmt.Fprintf(log, "sweep %s: %v\n", repoDir, err)
		}
	}
}
