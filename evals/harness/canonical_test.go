package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseWorktreeList(t *testing.T) {
	out := "worktree /src/repo\nHEAD abc\nbranch refs/heads/main\n\n" +
		"worktree /cache/repo/eval-x\nHEAD abc\nbranch refs/heads/eval/x-1\nlocked harness-eval pid 42\n\n" +
		"worktree /tmp/detached\nHEAD abc\ndetached\nlocked\n"
	got := parseWorktreeList(out)
	want := []worktreeEntry{
		{Path: "/src/repo", Branch: "main"},
		{
			Path:   "/cache/repo/eval-x",
			Branch: "eval/x-1",
			Locked: true,
			Reason: "harness-eval pid 42",
		},
		{Path: "/tmp/detached", Locked: true},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestSnapshotChanges(t *testing.T) {
	a := snapshot{Head: "1111111111111111", Branch: "refs/heads/main"}
	if d := a.changes(a); d != "" {
		t.Errorf("same snapshot reports %q", d)
	}
	d := a.changes(
		snapshot{Head: "2222222222222222", Branch: "refs/heads/x", Status: "M README.md"},
	)
	for _, want := range []string{
		"branch went from refs/heads/main to refs/heads/x",
		"HEAD moved from 111111111111 to 222222222222",
		`status went from "" to "M README.md"`,
	} {
		if !strings.Contains(d, want) {
			t.Errorf("changes = %q, missing %q", d, want)
		}
	}
}

// TestSnapshotSeesBranchSwitch covers the change HEAD and status alone
// miss: a new branch at the same commit with a clean tree.
func TestSnapshotSeesBranchSwitch(t *testing.T) {
	ctx := context.Background()
	_, repoDir := newRepo(t, "demo")
	before, err := takeSnapshot(ctx, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "switch", "-q", "-c", "sneaky")
	after, err := takeSnapshot(ctx, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if d := before.changes(after); !strings.Contains(d, "refs/heads/sneaky") {
		t.Errorf("branch switch went unseen: %q", d)
	}
	mustGit(t, repoDir, "switch", "-q", "--detach")
	detached, err := takeSnapshot(ctx, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if detached.Branch != detachedHead {
		t.Errorf("detached HEAD branch = %q", detached.Branch)
	}
}

// deadPID returns the pid of a process that has exited and been reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestSweepStale(t *testing.T) {
	ctx := context.Background()
	_, repoDir := newRepo(t, "demo")
	root := t.TempDir()
	elsewhere := t.TempDir()
	dead := fmt.Sprintf("%s pid %d", lockReason, deadPID(t))
	live := fmt.Sprintf("%s pid %d", lockReason, os.Getpid())
	add := func(parent, name, branch, reason string) string {
		dir := filepath.Join(parent, "demo", name)
		args := []string{"worktree", "add", "-q"}
		if reason != "" {
			args = append(args, "--lock", "--reason", reason)
		}
		mustGit(t, repoDir, append(args, "-b", branch, dir, "HEAD")...)
		return dir
	}
	worktrees := []struct {
		dir      string
		branch   string
		wantGone bool
	}{
		// A dead runner's worktree under the root goes, with its branch.
		{
			add(root, "eval-stale-20261026-120000", "eval/stale-20261026-120000", dead),
			"eval/stale-20261026-120000", true,
		},
		// Same, but a branch name the runner never makes: the branch stays.
		{add(root, "eval-odd", "eval/odd", dead), "eval/odd", true},
		// A live runner's worktree stays.
		{
			add(root, "eval-live-20261026-120000", "eval/live-20261026-120000", live),
			"eval/live-20261026-120000", false,
		},
		// Unlocked under the root: not provably the runner's, so it stays.
		{
			add(root, "eval-unlocked-20261026-120000", "eval/unlocked-20261026-120000", ""),
			"eval/unlocked-20261026-120000", false,
		},
		// Outside the root, even with a dead runner's lock, it stays.
		{
			add(elsewhere, "eval-away-20261026-120000", "eval/away-20261026-120000", dead),
			"eval/away-20261026-120000", false,
		},
		// A loom claim on an eval/ branch elsewhere, unlocked, stays.
		{add(elsewhere, "new-cases", "eval/new-cases", ""), "eval/new-cases", false},
	}
	mustGit(t, repoDir, "branch", "eval/orphan-20261026-120000")

	res, err := sweepStale(ctx, repoDir, root)
	if err != nil {
		t.Fatal(err)
	}
	if res != (sweepResult{Worktrees: 2, Branches: 1}) {
		t.Errorf("result = %+v, want 2 worktrees and 1 branch", res)
	}
	branches := strings.Split(mustGit(t, repoDir, "branch", "--format=%(refname:short)"), "\n")
	for _, wt := range worktrees {
		_, err := os.Stat(wt.dir)
		if gone := os.IsNotExist(err); gone != wt.wantGone {
			t.Errorf("%s gone = %v, want %v", filepath.Base(wt.dir), gone, wt.wantGone)
		}
		branchGone := !slices.Contains(branches, wt.branch)
		wantBranchGone := wt.wantGone && evalBranchRE.MatchString(wt.branch)
		if branchGone != wantBranchGone {
			t.Errorf("branch %s gone = %v, want %v", wt.branch, branchGone, wantBranchGone)
		}
	}
	if !slices.Contains(branches, "eval/orphan-20261026-120000") {
		t.Error("an eval branch with no worktree was deleted")
	}

	if res, err := sweepStale(ctx, repoDir, filepath.Join(root, "missing")); err != nil ||
		res != (sweepResult{}) {
		t.Errorf("missing root: %+v, %v", res, err)
	}
}
