package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no tilde", "/abs/path", "/abs/path"},
		{"tilde only", "~", home},
		{"tilde with subdir", "~/foo/bar", filepath.Join(home, "foo/bar")},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExpandPath(tc.input)
			if got != tc.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Dirs) == 0 {
		t.Fatal("DefaultConfig().Dirs is empty")
	}
}

func TestLoadCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Dirs) == 0 {
		t.Error("loaded config has no dirs")
	}

	// File should now exist.
	p := Path()
	if _, err := os.Stat(p); err != nil {
		t.Errorf("config file not created at %s: %v", p, err)
	}
}

func TestLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// First load creates default.
	first, err := Load()
	if err != nil {
		t.Fatalf("first Load() error: %v", err)
	}

	// Second load reads from file.
	second, err := Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}

	if len(first.Dirs) != len(second.Dirs) {
		t.Errorf("dirs mismatch: %v vs %v", first.Dirs, second.Dirs)
	}
}

func TestLoadScanExcludeRules(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "suspenders")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(
		"dirs: [~/code]\nscan:\n  exclude_rules:\n    - high-entropy-string\n    - generic-api-key\n",
	)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Scan.ExcludeRules) != 2 {
		t.Fatalf("ExcludeRules len = %d, want 2", len(cfg.Scan.ExcludeRules))
	}
	if cfg.Scan.ExcludeRules[0] != "high-entropy-string" {
		t.Errorf("ExcludeRules[0] = %q, want %q", cfg.Scan.ExcludeRules[0], "high-entropy-string")
	}
}

// TestLoad_malformedYAMLErrors pins the contract the hook run and scan commands
// rely on for fail-closed behavior: a present-but-unparseable config yields an
// error and a nil config, distinct from a missing config (which yields defaults
// with no error, covered by TestLoadCreatesDefault).
func TestLoad_malformedYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "suspenders")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// scan.enabled is a *bool; a string value makes yaml.Unmarshal fail.
	bad := []byte("scan:\n  enabled: not-a-bool\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), bad, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected Load to error on malformed config, got nil")
	}
	if cfg != nil {
		t.Errorf("expected nil config on parse error, got %+v", cfg)
	}
}

func TestPath_XDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	p := Path()
	want := "/tmp/xdg/suspenders/config.yaml"
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}
