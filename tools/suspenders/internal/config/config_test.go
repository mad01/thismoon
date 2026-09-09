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

// TestLoadDefaultsWithoutWriting pins the no-surprise-write contract:
// suspenders runs under git pre-commit hooks, where a load that creates a
// file puts it wherever git happened to leave the process.
func TestLoadDefaultsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Dirs) == 0 {
		t.Error("loaded config has no dirs")
	}

	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("Load must not create %s (stat err = %v)", p, err)
	}
}

// TestWithDefaultsFillsOmittedDirs: a config file that configures the
// scanner but never mentions dirs still discovers repos, instead of leaving
// `hook install --all` with nothing to walk.
func TestWithDefaultsFillsOmittedDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("guard:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if len(cfg.Dirs) != len(defaultDirs) {
		t.Errorf("Dirs = %v, want the defaults %v", cfg.Dirs, defaultDirs)
	}
}

// TestExplicitEmptyDirsStaysEmpty: an explicit `dirs: []` is a machine
// saying it discovers nothing, and must not be overwritten by the defaults.
func TestExplicitEmptyDirsStaysEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("dirs: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if len(cfg.Dirs) != 0 {
		t.Errorf("Dirs = %v, want an empty list", cfg.Dirs)
	}
}

func TestPathForPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	tests := []struct {
		name string
		env  string
		flag string
		want string
	}{
		{
			"neither set uses the default",
			"",
			"",
			filepath.Join(home, ".config", "suspenders", "config.yaml"),
		},
		{"env relocates", "/tmp/from-env.yaml", "", "/tmp/from-env.yaml"},
		{"flag wins over env", "/tmp/from-env.yaml", "/tmp/from-flag.yaml", "/tmp/from-flag.yaml"},
		{"tilde is expanded", "~/from-env.yaml", "", filepath.Join(home, "from-env.yaml")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvConfig, tc.env)
			got, err := PathFor(tc.flag)
			if err != nil {
				t.Fatalf("PathFor() error: %v", err)
			}
			if got != tc.want {
				t.Errorf("PathFor(%q) = %q, want %q", tc.flag, got, tc.want)
			}
		})
	}
}

// TestPathNeedsAResolvableHome: the old fallback returned "./.config/..."
// with no HOME, which under a pre-commit hook resolves inside the repository
// being committed to — reading a config nobody wrote, and writing one there.
func TestPathNeedsAResolvableHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if p, err := Path(); err == nil {
		t.Errorf("Path() = %q with no home, want an error", p)
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
// with no error, covered by TestLoadDefaultsWithoutWriting).
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
	p, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	want := "/tmp/xdg/suspenders/config.yaml"
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}
