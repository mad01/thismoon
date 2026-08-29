package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseOverride(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	future := now.Add(10 * time.Minute).Format(time.RFC3339)
	past := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name          string
		content       string
		wantActive    bool
		wantLegacy    bool
		wantMalformed bool
		wantRemaining time.Duration
	}{
		{"empty file is legacy and active", "", true, true, false, 0},
		{"whitespace-only file is legacy", "  \n", true, true, false, 0},
		{"future expiry is active", future + "\n", true, false, false, 10 * time.Minute},
		{"past expiry is inactive", past + "\n", false, false, false, 0},
		{"garbage is malformed and inactive", "tomorrow-ish\n", false, false, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := parseOverride("vacation", []byte(tt.content))
			if got := o.Active(now); got != tt.wantActive {
				t.Errorf("Active = %v, want %v", got, tt.wantActive)
			}
			if o.Legacy != tt.wantLegacy {
				t.Errorf("Legacy = %v, want %v", o.Legacy, tt.wantLegacy)
			}
			if o.Malformed != tt.wantMalformed {
				t.Errorf("Malformed = %v, want %v", o.Malformed, tt.wantMalformed)
			}
			if got := o.Remaining(now); got != tt.wantRemaining {
				t.Errorf("Remaining = %v, want %v", got, tt.wantRemaining)
			}
		})
	}
}

// isolateOverrides points OverridesDir at a private temp directory and
// returns it, created.
//
// Both HOME and XDG_CONFIG_HOME have to be pinned. OverridesDir resolves
// through kit/confdir, which prefers an absolute XDG_CONFIG_HOME over HOME,
// so setting HOME alone leaves the test reading a shared directory on any
// machine that exports XDG_CONFIG_HOME. GitHub's Linux runners do and macOS
// does not, which is how this passed locally while CI saw six overrides: the
// cli package's override tests write three files of their own, and the two
// package binaries run at the same time under `go test ./tools/belt/...`.
func isolateOverrides(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := OverridesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOverrideFiles(t *testing.T) {
	dir := isolateOverrides(t)
	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("expired", time.Now().Add(-time.Hour).Format(time.RFC3339))
	writeFile("running", time.Now().Add(time.Hour).Format(time.RFC3339))
	writeFile("vacation", "") // legacy untimed

	if !OverrideActive("running") {
		t.Error("OverrideActive(running) = false, want true for unexpired override")
	}
	if OverrideActive("expired") {
		t.Error("OverrideActive(expired) = true, want false past expiry")
	}
	if !OverrideActive("vacation") {
		t.Error("OverrideActive(vacation) = false, want true for legacy empty file")
	}
	if OverrideActive("missing") {
		t.Error("OverrideActive(missing) = true, want false for absent file")
	}

	got := Overrides()
	want := []string{"expired", "running", "vacation"}
	if len(got) != len(want) {
		t.Fatalf("Overrides() returned %d entries, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("Overrides()[%d].Name = %q, want %q (sorted)", i, got[i].Name, name)
		}
	}
}
