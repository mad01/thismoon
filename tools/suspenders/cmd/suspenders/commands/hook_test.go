package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunHookRun_malformedConfigFails covers finding 8: a present-but-broken
// config.yaml must fail the hook run (fail-closed) rather than being swallowed,
// which would silently disable the guard and external hooks on every commit.
// config.Load errors before any git work, so no repo is needed.
func TestRunHookRun_malformedConfigFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "suspenders")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// scan.enabled is a *bool; a string value makes config parsing fail.
	bad := []byte("scan:\n  enabled: not-a-bool\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), bad, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runHookRun(nil, []string{"pre-commit"}); err == nil {
		t.Fatal("expected hook run to fail on malformed config, got nil")
	}
}
