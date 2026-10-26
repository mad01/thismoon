package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	a := snapshot{Head: "1111111111111111", Status: ""}
	if d := a.changes(a); d != "" {
		t.Errorf("same snapshot reports %q", d)
	}
	d := a.changes(snapshot{Head: "2222222222222222", Status: "M README.md"})
	if !strings.Contains(d, "HEAD moved from 111111111111 to 222222222222") ||
		!strings.Contains(d, `status went from "" to "M README.md"`) {
		t.Errorf("changes = %q", d)
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
	add := func(name, branch, reason string) string {
		dir := filepath.Join(root, name)
		args := []string{"worktree", "add", "-q"}
		if reason != "" {
			args = append(args, "--lock", "--reason", reason)
		}
		mustGit(t, repoDir, append(args, "-b", branch, dir, "HEAD")...)
		return dir
	}
	stale := add("stale", "eval/stale-1", fmt.Sprintf("%s pid %d", lockReason, deadPID(t)))
	live := add("live", "eval/live-1", fmt.Sprintf("%s pid %d", lockReason, os.Getpid()))
	mine := add("mine", "feature/mine", "")
	mustGit(t, repoDir, "branch", "eval/orphan-1")
	mustGit(t, repoDir, "branch", "keep-me")

	res, err := sweepStale(ctx, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if res != (sweepResult{Worktrees: 1, Branches: 2}) {
		t.Errorf("result = %+v, want 1 worktree and 2 branches (stale and orphan)", res)
	}
	for dir, wantGone := range map[string]bool{stale: true, live: false, mine: false} {
		_, err := os.Stat(dir)
		if gone := os.IsNotExist(err); gone != wantGone {
			t.Errorf("%s gone = %v, want %v", filepath.Base(dir), gone, wantGone)
		}
	}
	branches := mustGit(t, repoDir, "branch", "--format=%(refname:short)")
	for _, b := range []string{"eval/live-1", "feature/mine", "keep-me"} {
		if !strings.Contains(branches, b) {
			t.Errorf("branch %s was swept:\n%s", b, branches)
		}
	}
	for _, b := range []string{"eval/stale-1", "eval/orphan-1"} {
		if strings.Contains(branches, b) {
			t.Errorf("branch %s survived the sweep", b)
		}
	}
}
