package k8sstore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// logCollector records what the sweeper logs. RunSweeper logs from its own
// goroutine while the test reads, so the lines are guarded.
type logCollector struct {
	mu    sync.Mutex
	lines []string
}

func (c *logCollector) logf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}

func (c *logCollector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

// TestRunSweeperAnnouncesItself pins the one line an operator looks for in
// pod logs to know the sweeper is running, and its interval.
func TestRunSweeperAnnouncesItself(t *testing.T) {
	f := fakeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exp := f.now.Add(-time.Hour)
	if _, err := f.st.Create(ctx, store.Draft{
		Title: "Gone", Content: "<p>x</p>", Ephemeral: true, ExpiresAt: &exp,
	}); err != nil {
		t.Fatal(err)
	}

	var c logCollector
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunSweeper(ctx, f.st, time.Hour, c.logf)
	}()
	waitFor(t, func() bool {
		lines := c.snapshot()
		return len(lines) == 2 && strings.Contains(lines[1], "deleted=1")
	})
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunSweeper did not return after its context was cancelled")
	}

	lines := c.snapshot()
	if want := "present: sweeping expired pages every 1h0m0s"; lines[0] != want {
		t.Errorf("first log line = %q, want %q", lines[0], want)
	}
	if want := "present: sweep deleted=1 next in 1h0m0s"; lines[1] != want {
		t.Errorf("sweep log line = %q, want %q", lines[1], want)
	}
}

// TestRunSweeperLogsAnEmptySweep pins the heartbeat: a sweep that deleted
// nothing still logs, because a silent sweeper is indistinguishable from a
// dead one at a ten-minute interval.
func TestRunSweeperLogsAnEmptySweep(t *testing.T) {
	f := fakeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := f.st.Create(ctx, store.Draft{Title: "Kept", Content: "<p>x</p>"}); err != nil {
		t.Fatal(err)
	}

	var c logCollector
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunSweeper(ctx, f.st, time.Hour, c.logf)
	}()
	waitFor(t, func() bool { return len(c.snapshot()) == 2 })
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunSweeper did not return after its context was cancelled")
	}

	lines := c.snapshot()
	if want := "present: sweep deleted=0 next in 1h0m0s"; lines[1] != want {
		t.Errorf("empty sweep log line = %q, want %q", lines[1], want)
	}
}

// TestRunSweeperStopsWithoutSweeping guards the shutdown path: a context
// that is already done must not produce a sweep, so a pod that is going
// away never logs a sweep that failed on the way out.
func TestRunSweeperStopsWithoutSweeping(t *testing.T) {
	f := fakeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var c logCollector
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunSweeper(ctx, f.st, time.Hour, c.logf)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunSweeper did not return for a cancelled context")
	}
	if lines := c.snapshot(); len(lines) != 1 {
		t.Errorf("logged %v, want only the start line", lines)
	}
}

// waitFor polls cond until it holds or the test gives up.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never held")
}
