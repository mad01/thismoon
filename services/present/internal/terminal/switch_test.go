package terminal

import (
	"path/filepath"
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
