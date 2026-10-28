package syncer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/firstlaunchtest"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

const hungRemoteURL = "git@localhost:fake/repo.git"

// fakeSSH stands in for ssh on a dead socket: every invocation but the
// warm-up logs its pid, then sleeps far past any timeout under test.
type fakeSSH struct{ pidLog string }

// installHungSSH routes every git ssh connection in this test to a fakeSSH and
// isolates git config, so a user's url rewrites or sshCommand cannot route
// around it. The git subprocesses inherit the environment.
func installHungSSH(t *testing.T) fakeSSH {
	t.Helper()
	bin := t.TempDir()
	pidLog := filepath.Join(bin, "ssh.pids")
	script := filepath.Join(bin, "fake-ssh")
	body := "#!/bin/sh\n[ \"$1\" = warm ] && exit 0\necho $$ >> '" + pidLog + "'\nexec sleep 60\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	// A fresh script's first launch can be slow under load; launch it once
	// untimed, before it logs a pid, so the pull timeout measures git.
	firstlaunchtest.Warm(t, script, []string{"warm"}, nil)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_SSH_COMMAND", script)
	// Skip git's `ssh -G` variant probe so the fetch is the only ssh call.
	t.Setenv("GIT_SSH_VARIANT", "ssh")
	return fakeSSH{pidLog: pidLog}
}

// pids returns the pid of every fake ssh started so far, in start order.
func (f fakeSSH) pids(t *testing.T) []int {
	t.Helper()
	data, err := os.ReadFile(f.pidLog)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read pid log: %v", err)
	}
	var pids []int
	for line := range strings.FieldsSeq(string(data)) {
		pid, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("pid log line %q: %v", line, err)
		}
		pids = append(pids, pid)
	}
	return pids
}

// waitStarted blocks until the first fake ssh has started and returns its pid.
func (f fakeSSH) waitStarted(t *testing.T) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pids := f.pids(t); len(pids) > 0 {
			return pids[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fake ssh never started")
	return 0
}

// hungRepo returns a clean repo on its default branch whose origin is an ssh
// remote, so a real pull dials the fake ssh.
func hungRepo(t *testing.T, name string) finder.Repo {
	t.Helper()
	setURL := func(t *testing.T, _ string, run func(args ...string)) {
		t.Helper()
		run("remote", "set-url", "origin", hungRemoteURL)
	}
	dir := setupSyncRepo(t, name, "main", false, setURL)
	return finder.Repo{Name: "fake/" + name, Path: dir, Remote: hungRemoteURL}
}

func setPullTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := pullTimeout
	pullTimeout = d
	t.Cleanup(func() { pullTimeout = old })
}

// waitProcessGone fails the test unless pid exits within a few seconds. A
// killed orphan lingers as a zombie until init reaps it, hence the poll.
func waitProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Errorf("fake ssh (pid %d) still running; the pull's process tree was not killed", pid)
}

func TestPullRepo_TimeoutKillsHungPull(t *testing.T) {
	ssh := installHungSSH(t)
	repo := hungRepo(t, "hung")
	setPullTimeout(t, time.Second)

	start := time.Now()
	r := pullRepo(context.Background(), repo, false)
	elapsed := time.Since(start)

	if r.Status != "fail" || r.Message != "pull timed out after 1s" {
		t.Errorf(
			"got status %q message %q, want fail / pull timed out after 1s",
			r.Status,
			r.Message,
		)
	}
	if elapsed > 6*time.Second {
		t.Errorf("pullRepo took %s, want it bounded by the 1s pull timeout", elapsed)
	}
	pids := ssh.pids(t)
	if len(pids) != 1 {
		t.Fatalf("fake ssh ran %d times, want 1: a timed-out pull must not be retried", len(pids))
	}
	waitProcessGone(t, pids[0])
}

func TestPullRepo_CancelStopsHungPull(t *testing.T) {
	ssh := installHungSSH(t)
	repo := hungRepo(t, "cancel")
	setPullTimeout(t, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan PullResult, 1)
	go func() { done <- pullRepo(ctx, repo, false) }()

	pid := ssh.waitStarted(t)
	cancel()
	select {
	case r := <-done:
		if r.Status != "fail" || r.Message != "pull cancelled" {
			t.Errorf("got status %q message %q, want fail / pull cancelled", r.Status, r.Message)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("pullRepo did not return after its context was cancelled")
	}
	if n := len(ssh.pids(t)); n != 1 {
		t.Errorf("fake ssh ran %d times, want 1: a cancelled pull must not be retried", n)
	}
	waitProcessGone(t, pid)
}

func TestPullRepo_AlreadyCancelledFailsFast(t *testing.T) {
	ssh := installHungSSH(t)
	repo := hungRepo(t, "precancelled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := pullRepo(ctx, repo, false)

	if r.Status != "fail" || r.Message != "pull cancelled" {
		t.Errorf("got status %q message %q, want fail / pull cancelled", r.Status, r.Message)
	}
	if n := len(ssh.pids(t)); n != 0 {
		t.Errorf("fake ssh ran %d times, want 0: a cancelled context must not dial the remote", n)
	}
}

func TestPullAll_CancelledContextFailsEveryRepo(t *testing.T) {
	ssh := installHungSSH(t)
	repos := []finder.Repo{hungRepo(t, "one"), hungRepo(t, "two"), hungRepo(t, "three")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := pullAll(ctx, repos, 1, false, nil)

	if len(results) != len(repos) {
		t.Fatalf("got %d results, want %d", len(results), len(repos))
	}
	for _, r := range results {
		if r.Status != "fail" || r.Message != "pull cancelled" {
			t.Errorf("%s: got status %q message %q, want fail / pull cancelled",
				r.Repo.Name, r.Status, r.Message)
		}
	}
	if n := len(ssh.pids(t)); n != 0 {
		t.Errorf("fake ssh ran %d times, want 0", n)
	}
}
