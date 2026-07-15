package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebConfigMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := webConfig()
	if err != nil {
		t.Fatalf("webConfig with no config file: %v, want nil error", err)
	}
	if len(cfg.Dirs) != 0 {
		t.Errorf("got %d dirs, want 0", len(cfg.Dirs))
	}
}

func TestWebConfigMalformedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "csl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("dirs: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := webConfig(); err == nil {
		t.Fatal("webConfig with malformed config: nil error, want parse error")
	}
}
