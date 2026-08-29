package csl

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultBaseURLMatchesDefaultPort keeps the documented constant and the
// derivation in sync: DefaultBaseURL is what BaseURLForPort must produce for
// DefaultPort, or the two answers drift apart.
func TestDefaultBaseURLMatchesDefaultPort(t *testing.T) {
	if got := BaseURLForPort(DefaultPort); got != DefaultBaseURL {
		t.Errorf("BaseURLForPort(%d) = %q, want DefaultBaseURL %q", DefaultPort, got, DefaultBaseURL)
	}
}

func TestResolvedPort(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int
	}{
		{name: "unset falls back", want: DefaultPort},
		{name: "env wins", env: "9424", want: 9424},
		{name: "unparseable falls back", env: "nope", want: DefaultPort},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(PortEnv, tc.env)
			if got := ResolvedPort(); got != tc.want {
				t.Errorf("ResolvedPort() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestStateDirKeepsExistingInstall is the migration contract: a machine that
// already has ~/.config/csl keeps writing there, so upgrading costs no
// re-index. A fresh machine lands under the XDG state root.
func TestStateDirKeepsExistingInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	legacy := filepath.Join(home, ".config", "csl")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	got, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir() error: %v", err)
	}
	if got != legacy {
		t.Errorf("StateDir() = %q, want the existing %q", got, legacy)
	}

	if err := os.RemoveAll(legacy); err != nil {
		t.Fatalf("remove legacy dir: %v", err)
	}
	got, err = StateDir()
	if err != nil {
		t.Fatalf("StateDir() error: %v", err)
	}
	if want := filepath.Join(home, ".local", "state", "csl"); got != want {
		t.Errorf("StateDir() = %q on a fresh install, want %q", got, want)
	}
}

func TestStatePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	got, err := StatePath("search-index")
	if err != nil {
		t.Fatalf("StatePath() error: %v", err)
	}
	if want := filepath.Join(home, ".local", "state", "csl", "search-index"); got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}

// TestStateDirForDocs checks the doc rendering collapses the home prefix;
// operating.md reads better with ~ than with a machine's real home.
func TestStateDirForDocs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	if got, want := stateDirForDocs(), "~/.local/state/csl"; got != want {
		t.Errorf("stateDirForDocs() = %q, want %q", got, want)
	}

	outside := t.TempDir()
	t.Setenv("XDG_STATE_HOME", outside)
	if got, want := stateDirForDocs(), filepath.Join(outside, "csl"); got != want {
		t.Errorf("stateDirForDocs() = %q outside home, want the absolute %q", got, want)
	}
}
