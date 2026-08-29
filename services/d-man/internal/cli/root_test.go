package cli

import (
	"os"
	"path/filepath"
	"testing"

	dman "github.com/mad01/thismoon/services/d-man"
)

// none and only build the exists() seam resolveConfig probes the filesystem
// with, so the resolution order is tested without touching real files.
func none(string) bool { return false }

func only(p string) func(string) bool {
	return func(q string) bool { return q == p }
}

// TestResolveConfig pins the routes-file resolution order: DMAN_CONFIG, then
// the per-user path when present, then the system path when present, else the
// per-user path for a clear error message. The system fallback is what lets a
// root launchd daemon (no useful HOME) run `d-man serve` with no flags.
func TestResolveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	userPath := filepath.Join(home, ".config", "d-man", "routes.toml")

	cases := []struct {
		name   string
		env    string
		exists func(string) bool
		want   string
	}{
		{"env wins", "/explicit/routes.toml", none, "/explicit/routes.toml"},
		{"existing user path wins", "", only(userPath), userPath},
		{"system path is the fallback", "", only(dman.SystemRoutesPath), dman.SystemRoutesPath},
		{"nothing on disk defaults to the user path", "", none, userPath},
		{"user path beats system path", "", func(q string) bool {
			return q == userPath || q == dman.SystemRoutesPath
		}, userPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveConfig(tc.env, tc.exists)
			if err != nil {
				t.Fatalf("resolveConfig: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// XDG_CONFIG_HOME relocates the per-user leg of the cascade; the rest of the
// order is unchanged.
func TestResolveConfigHonorsXDG(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "d-man", "routes.toml")

	got, err := resolveConfig("", none)
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want the XDG path %q", got, want)
	}
}

// With no resolvable home there is no per-user path to name. Resolution
// fails rather than degrading to a cwd-relative ".config/d-man/routes.toml",
// and the --config default falls back to the system path instead.
func TestResolveConfigWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if _, err := resolveConfig("", none); err == nil {
		t.Fatal("want an error with no resolvable home, got none")
	}
	if got := defaultConfig(); got != dman.SystemRoutesPath {
		t.Fatalf("defaultConfig = %q, want the system path %q", got, dman.SystemRoutesPath)
	}
}

// TestFileExists covers the one impure helper resolveConfig depends on.
func TestFileExists(t *testing.T) {
	f := filepath.Join(t.TempDir(), "routes.toml")
	if fileExists(f) {
		t.Fatal("missing file reported as existing")
	}
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(f) {
		t.Fatal("existing file reported as missing")
	}
}
