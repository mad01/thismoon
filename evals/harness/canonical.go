package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"syscall"
)

// evalBranchPrefix starts every branch a worktree workspace creates.
const evalBranchPrefix = "eval/"

// snapshot is the state of a canonical checkout the runner compares before
// and after a case: the commit HEAD points at and the porcelain status.
type snapshot struct {
	Head   string
	Status string
}

// takeSnapshot reads dir's HEAD and `git status --porcelain`. An empty dir
// (a tmpdir workspace has no canonical checkout) gives the zero snapshot.
func takeSnapshot(ctx context.Context, dir string) (snapshot, error) {
	if dir == "" {
		return snapshot{}, nil
	}
	head, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return snapshot{}, err
	}
	status, err := git(ctx, dir, "status", "--porcelain")
	if err != nil {
		return snapshot{}, err
	}
	return snapshot{Head: head, Status: status}, nil
}

// changes describes how after differs from s, or returns "" when it does
// not.
func (s snapshot) changes(after snapshot) string {
	var out []string
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

// sweepStale removes what earlier runs left in repoDir when they died
// before Cleanup: eval/ worktrees whose lock does not name a live runner,
// then eval/ branches that no remaining worktree has checked out. The main
// checkout and every other worktree and branch are left alone.
func sweepStale(ctx context.Context, repoDir string) (sweepResult, error) {
	var res sweepResult
	out, err := git(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return res, err
	}
	inUse := map[string]bool{}
	var errs []error
	for i, wt := range parseWorktreeList(out) {
		if i == 0 || !strings.HasPrefix(wt.Branch, evalBranchPrefix) {
			inUse[wt.Branch] = true
			continue
		}
		if liveRunner(wt.Reason) {
			inUse[wt.Branch] = true
			continue
		}
		if _, err := git(ctx, repoDir, "worktree", "remove", "--force", "--force", wt.Path); err != nil {
			inUse[wt.Branch] = true
			errs = append(errs, err)
			continue
		}
		res.Worktrees++
	}
	refs, err := git(ctx, repoDir,
		"for-each-ref", "--format=%(refname:short)", "refs/heads/"+evalBranchPrefix)
	if err != nil {
		return res, errors.Join(append(errs, err)...)
	}
	for branch := range strings.SplitSeq(refs, "\n") {
		if branch == "" || inUse[branch] {
			continue
		}
		if _, err := git(ctx, repoDir, "branch", "-D", branch); err != nil {
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
		res, err := sweepStale(ctx, repoDir)
		fmt.Fprintf(log, "sweep %s: removed %d stale eval worktrees and %d eval branches\n",
			repoDir, res.Worktrees, res.Branches)
		if err != nil {
			fmt.Fprintf(log, "sweep %s: %v\n", repoDir, err)
		}
	}
}
