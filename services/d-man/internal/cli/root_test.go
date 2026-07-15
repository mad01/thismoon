package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveConfig pins the routes-file resolution order: DMAN_CONFIG, then
// the per-user path when present, then the system path when present, else the
// per-user path for a clear error message. The system fallback is what lets a
// root launchd daemon (no useful HOME) run `d-man serve` with no flags.
func TestResolveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "d-man", "routes.toml")

	none := func(string) bool { return false }
	only := func(p string) func(string) bool {
		return func(q string) bool { return q == p }
	}

	if got := resolveConfig("/explicit/routes.toml", none); got != "/explicit/routes.toml" {
		t.Fatalf("env should win, got %q", got)
	}
	if got := resolveConfig("", only(userPath)); got != userPath {
		t.Fatalf("existing user path should win, got %q", got)
	}
	if got := resolveConfig("", only(systemConfig)); got != systemConfig {
		t.Fatalf("system path should be the fallback, got %q", got)
	}
	if got := resolveConfig("", none); got != userPath {
		t.Fatalf("with nothing on disk the user path is the default, got %q", got)
	}
	both := func(q string) bool { return q == userPath || q == systemConfig }
	if got := resolveConfig("", both); got != userPath {
		t.Fatalf("user path should take precedence over system path, got %q", got)
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
