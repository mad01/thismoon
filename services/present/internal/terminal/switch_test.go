package terminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSwitchOnOff(t *testing.T) {
	path := SwitchPath(filepath.Join(t.TempDir(), "fresh"))
	assertEnabled(t, path, false)
	if err := Enable(path); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	assertEnabled(t, path, true)
	if err := Disable(path); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	assertEnabled(t, path, false)
	if err := Disable(path); err != nil {
		t.Errorf("Disable on an off switch: %v", err)
	}
}

func assertEnabled(t *testing.T, path string, want bool) {
	t.Helper()
	got, err := Enabled(path)
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if got != want {
		t.Errorf("Enabled = %v, want %v", got, want)
	}
}

func TestWatchSwitchKillsSessionsOnceOff(t *testing.T) {
	m, home := newManager(t, Config{})
	path := SwitchPath(home)
	if err := Enable(path); err != nil {
		t.Fatal(err)
	}
	sub := attach(t, m, "demo", Spec{Cmd: "sleep 60"})
	go m.WatchSwitch(t.Context(), path, 10*time.Millisecond, t.Logf)

	time.Sleep(100 * time.Millisecond)
	if len(m.List()) != 1 {
		t.Fatal("the session must run while the switch is on")
	}
	if err := Disable(path); err != nil {
		t.Fatal(err)
	}
	readAll(t, sub)
	if len(m.List()) != 0 {
		t.Errorf("List() after off = %+v, want none", m.List())
	}
}

// A switch it cannot read is logged once when that starts and once when it
// reads again, not at every check.
func TestWatchSwitchLogsOnlyWhenReadabilityChanges(t *testing.T) {
	m, home := newManager(t, Config{})
	dir := filepath.Join(home, "workdir")
	path := SwitchPath(dir)
	if err := Enable(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := Enabled(path); err == nil {
		t.Skip("the switch still reads in a directory without permissions (running as root?)")
	}

	var mu sync.Mutex
	var lines []string
	logf := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	logged := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(lines)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.WatchSwitch(ctx, path, 5*time.Millisecond, logf)
	}()
	t.Cleanup(func() { cancel(); <-done })

	time.Sleep(150 * time.Millisecond)
	if got := logged(); len(got) != 1 {
		t.Fatalf("logged %q over many failed checks, want one line", got)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the recovery line", func() bool { return len(logged()) >= 2 })
	time.Sleep(50 * time.Millisecond)
	if got := logged(); len(got) != 2 || !strings.Contains(got[1], "reads again") {
		t.Errorf("logged %q, want the failure then one recovery line", got)
	}
}
