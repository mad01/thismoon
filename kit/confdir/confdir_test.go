package confdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir(t *testing.T) {
	tests := []struct {
		name       string
		home       string
		xdgConfig  string
		component  string
		want       string
		wantErr    bool
		wantErrHas string
	}{
		{
			name:      "home fallback",
			home:      "/Users/tester",
			component: "csl",
			want:      "/Users/tester/.config/csl",
		},
		{
			name:      "xdg wins when absolute",
			home:      "/Users/tester",
			xdgConfig: "/tmp/xdg-config",
			component: "csl",
			want:      "/tmp/xdg-config/csl",
		},
		{
			name:      "relative xdg is ignored",
			home:      "/Users/tester",
			xdgConfig: "relative/config",
			component: "csl",
			want:      "/Users/tester/.config/csl",
		},
		{
			name:       "unresolvable home is an error",
			home:       "",
			component:  "csl",
			wantErr:    true,
			wantErrHas: "confdir: resolve directory for \"csl\"",
		},
		{
			name:      "xdg still resolves without a home",
			home:      "",
			xdgConfig: "/tmp/xdg-config",
			component: "csl",
			want:      "/tmp/xdg-config/csl",
		},
		{
			name:       "empty component is an error",
			home:       "/Users/tester",
			component:  "",
			wantErr:    true,
			wantErrHas: "empty component name",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			t.Setenv(envConfigHome, tc.xdgConfig)

			got, err := Dir(tc.component)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Dir(%q) = %q, want error", tc.component, got)
				}
				if !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Errorf("Dir(%q) error = %v, want it to mention %q",
						tc.component, err, tc.wantErrHas)
				}
				if got != "" {
					t.Errorf("Dir(%q) = %q on error, want an empty path", tc.component, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Dir(%q) error: %v", tc.component, err)
			}
			if got != tc.want {
				t.Errorf("Dir(%q) = %q, want %q", tc.component, got, tc.want)
			}
		})
	}
}

func TestPath(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	t.Setenv(envConfigHome, "")

	got, err := Path("prs", "config.yaml")
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if want := "/Users/tester/.config/prs/config.yaml"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}

	if _, err := Path("", "config.yaml"); err == nil {
		t.Error("Path with an empty component = nil error, want one")
	}
}

func TestStateDir(t *testing.T) {
	// A real directory to stand in for an install that predates the XDG
	// state location.
	existing := t.TempDir()

	tests := []struct {
		name      string
		home      string
		xdgState  string
		component string
		legacy    string
		want      string
		wantErr   bool
	}{
		{
			name:      "home fallback",
			home:      "/Users/tester",
			component: "wire",
			want:      "/Users/tester/.local/state/wire",
		},
		{
			name:      "xdg wins when absolute",
			home:      "/Users/tester",
			xdgState:  "/tmp/xdg-state",
			component: "wire",
			want:      "/tmp/xdg-state/wire",
		},
		{
			name:      "existing legacy directory wins",
			home:      "/Users/tester",
			xdgState:  "/tmp/xdg-state",
			component: "csl",
			legacy:    existing,
			want:      existing,
		},
		{
			name:      "missing legacy directory falls through",
			home:      "/Users/tester",
			component: "csl",
			legacy:    filepath.Join(existing, "absent"),
			want:      "/Users/tester/.local/state/csl",
		},
		{
			name:      "legacy pointing at a file falls through",
			home:      "/Users/tester",
			component: "csl",
			legacy:    writeFile(t, existing, "not-a-dir"),
			want:      "/Users/tester/.local/state/csl",
		},
		{
			name:      "tilde legacy is expanded only to test it",
			home:      existing,
			component: "present",
			legacy:    "~",
			want:      "~",
		},
		{
			name:      "tilde legacy without a home is an error",
			home:      "",
			component: "present",
			legacy:    "~/.config/present",
			wantErr:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			t.Setenv(envStateHome, tc.xdgState)

			got, err := StateDir(tc.component, tc.legacy)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("StateDir(%q, %q) = %q, want error", tc.component, tc.legacy, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("StateDir(%q, %q) error: %v", tc.component, tc.legacy, err)
			}
			if got != tc.want {
				t.Errorf("StateDir(%q, %q) = %q, want %q", tc.component, tc.legacy, got, tc.want)
			}
		})
	}
}

func TestExpand(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "bare tilde", path: "~", want: "/Users/tester"},
		{name: "tilde prefix", path: "~/.config/csl", want: "/Users/tester/.config/csl"},
		{name: "absolute path untouched", path: "/etc/d-man", want: "/etc/d-man"},
		{name: "relative path untouched", path: "config.yaml", want: "config.yaml"},
		{name: "tilde user is not expanded", path: "~other/x", want: "~other/x"},
		{name: "empty path", path: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Expand(tc.path)
			if err != nil {
				t.Fatalf("Expand(%q) error: %v", tc.path, err)
			}
			if got != tc.want {
				t.Errorf("Expand(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}

	t.Run("unresolvable home is an error", func(t *testing.T) {
		t.Setenv("HOME", "")
		got, err := Expand("~/.config/csl")
		if err == nil {
			t.Fatalf("Expand() = %q, want error", got)
		}
		if got != "" {
			t.Errorf("Expand() = %q on error, want an empty path", got)
		}
	})
}

// writeFile creates a regular file under dir and returns its path.
func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
