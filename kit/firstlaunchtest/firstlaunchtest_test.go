package firstlaunchtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// recordingTB stands in for the test Warm fails, so a failed warm-up can be
// checked instead of ending this test.
type recordingTB struct {
	testing.TB
	fatal string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestWarmPassesArgsAndReplacesEnv: the script exits 0 only when it sees the
// argument and the variable and not the test's own environment, so a pass
// shows Warm handed both over and replaced the environment instead of
// merging into it.
func TestWarmPassesArgsAndReplacesEnv(t *testing.T) {
	t.Setenv("FIRSTLAUNCH_LEAK", "x")
	path := writeScript(t,
		`[ "$1" = warm ] && [ "$FIRSTLAUNCH" = set ] && [ -z "$FIRSTLAUNCH_LEAK" ]`+"\n")
	tb := &recordingTB{TB: t}
	Warm(tb, path, []string{"warm"}, []string{"FIRSTLAUNCH=set"})
	if tb.fatal != "" {
		t.Errorf("Warm failed a script that exits 0: %s", tb.fatal)
	}
}

// TestWarmInheritsEnvWhenNil: with a nil env the script sees the variable
// the test set, so a pass shows Warm kept the test's environment.
func TestWarmInheritsEnvWhenNil(t *testing.T) {
	t.Setenv("FIRSTLAUNCH", "set")
	path := writeScript(t, `[ "$1" = warm ] && [ "$FIRSTLAUNCH" = set ]`+"\n")
	tb := &recordingTB{TB: t}
	Warm(tb, path, []string{"warm"}, nil)
	if tb.fatal != "" {
		t.Errorf("Warm failed a script that exits 0: %s", tb.fatal)
	}
}

func TestWarmFailsOnNonZeroExit(t *testing.T) {
	path := writeScript(t, "echo boom\nexit 3\n")
	tb := &recordingTB{TB: t}
	Warm(tb, path, nil, nil)
	if !strings.Contains(tb.fatal, "boom") {
		t.Errorf("Warm on a failing script: fatal = %q, want it to carry the output", tb.fatal)
	}
}

// TestWarmFailsOnLingeringChild: the script exits 0 but leaves a child
// holding its output. Warm must fail it within the budget rather than wait
// for the child and pass. The child is killed afterwards so none outlives
// the test.
func TestWarmFailsOnLingeringChild(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "child.pid")
	path := writeScript(t, "sleep 60 &\necho $! > "+pidfile+"\nexit 0\n")
	t.Cleanup(func() { killChild(t, pidfile) })
	tb := &recordingTB{TB: t}

	start := time.Now()
	Warm(tb, path, nil, nil)
	elapsed := time.Since(start)

	if tb.fatal == "" {
		t.Error("Warm passed a script whose child still held the output")
	}
	if elapsed > budget {
		t.Errorf("Warm took %s with a lingering child, want under %s", elapsed, budget)
	}
}

// killChild ends the process whose pid the script wrote to pidfile.
func killChild(t *testing.T, pidfile string) {
	t.Helper()
	raw, err := os.ReadFile(pidfile)
	if err != nil {
		t.Errorf("read child pid: %v", err)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Errorf("parse child pid %q: %v", raw, err)
		return
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Errorf("kill child %d: %v", pid, err)
	}
}
