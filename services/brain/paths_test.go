package brain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

func TestStateDir(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares home and returns the state directory brain must pick.
		setup func(t *testing.T, home string) string
	}{
		{
			name: "fresh install lands in ~/.local/state",
			setup: func(t *testing.T, home string) string {
				return filepath.Join(home, ".local", "state", "brain")
			},
		},
		{
			name: "legacy index keeps ~/.config/brain",
			setup: func(t *testing.T, home string) string {
				legacy := filepath.Join(home, ".config", "brain")
				writeFile(t, filepath.Join(legacy, brain.LegacyStateProbe))
				return legacy
			},
		},
		{
			name: "config file alone does not pin the legacy directory",
			setup: func(t *testing.T, home string) string {
				writeFile(t, filepath.Join(home, ".config", "brain", brain.ConfigFile))
				return filepath.Join(home, ".local", "state", "brain")
			},
		},
		{
			name: "empty legacy index directory does not pin it either",
			setup: func(t *testing.T, home string) string {
				if err := os.MkdirAll(filepath.Join(home, ".config", "brain", "index"), 0o755); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(home, ".local", "state", "brain")
			},
		},
		{
			name: "XDG_STATE_HOME wins over a legacy index",
			setup: func(t *testing.T, home string) string {
				writeFile(t, filepath.Join(home, ".config", "brain", brain.LegacyStateProbe))
				xdg := filepath.Join(home, "xdg-state")
				t.Setenv("XDG_STATE_HOME", xdg)
				return filepath.Join(xdg, "brain")
			},
		},
		{
			name: "relative XDG_STATE_HOME is ignored",
			setup: func(t *testing.T, home string) string {
				t.Setenv("XDG_STATE_HOME", "relative/state")
				return filepath.Join(home, ".local", "state", "brain")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := testhome.Isolate(t)
			want := tc.setup(t, home)
			got, err := brain.StateDir()
			if err != nil {
				t.Fatalf("StateDir: %v", err)
			}
			if got != want {
				t.Errorf("StateDir = %q, want %q", got, want)
			}
			index, err := brain.IndexDir()
			if err != nil {
				t.Fatal(err)
			}
			if index != filepath.Join(want, "index") {
				t.Errorf("IndexDir = %q, want it under %q", index, want)
			}
		})
	}
}

func TestDefaultConfigPath(t *testing.T) {
	home := testhome.Isolate(t)
	got, err := brain.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "brain", "config.yaml"); got != want {
		t.Errorf("DefaultConfigPath = %q, want %q", got, want)
	}
	xdg := filepath.Join(home, "xdg-config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err = brain.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "brain", "config.yaml"); got != want {
		t.Errorf("DefaultConfigPath under XDG_CONFIG_HOME = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}
