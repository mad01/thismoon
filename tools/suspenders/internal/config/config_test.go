package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeTestFile writes content to path, failing the test on error.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

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

// TestLoadFromIncludesAppendGuardLists: every names file under guard.include
// feeds all three guard lists, after the config's own entries and in include
// order, while the include list itself is kept as written.
func TestLoadFromIncludesAppendGuardLists(t *testing.T) {
	dir := t.TempDir()
	common := filepath.Join(dir, "common.yaml")
	writeTestFile(t, common, `
blocked_words: [acmecorp]
allowlist: [monitoring]
allow_phrases: [dotfiles-acmecorp]
`)
	work := filepath.Join(dir, "work.yaml")
	writeTestFile(t, work, "blocked_words: [acmeinc]\n")
	path := filepath.Join(dir, "config.yaml")
	writeTestFile(t, path, `
guard:
  blocked_words: [ownword]
  include:
    - `+common+`
    - `+work+`
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	g := cfg.Guard
	if want := []string{"ownword", "acmecorp", "acmeinc"}; !slices.Equal(g.BlockedWords, want) {
		t.Errorf("BlockedWords = %v, want %v", g.BlockedWords, want)
	}
	if want := []string{"monitoring"}; !slices.Equal(g.Allowlist, want) {
		t.Errorf("Allowlist = %v, want %v", g.Allowlist, want)
	}
	if want := []string{"dotfiles-acmecorp"}; !slices.Equal(g.AllowPhrases, want) {
		t.Errorf("AllowPhrases = %v, want %v", g.AllowPhrases, want)
	}
	if want := []string{common, work}; !slices.Equal(g.Include, want) {
		t.Errorf("Include = %v, want %v", g.Include, want)
	}
}

// TestLoadFromIncludeMissingIsAnError: a names file the config lists but
// that does not exist fails the load, the same way a broken config does, so
// the guard never runs against a silently shorter list. The error names the
// include and keeps the not-exist cause visible.
func TestLoadFromIncludeMissingIsAnError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.yaml")
	path := filepath.Join(dir, "config.yaml")
	writeTestFile(t, path, "guard:\n  include:\n    - "+missing+"\n")

	cfg, err := LoadFrom(path)
	if err == nil {
		t.Fatal("LoadFrom() with a missing include returned nil error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error = %v, want one wrapping fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "include "+missing) {
		t.Errorf("error = %v, want it to name the include %s", err, missing)
	}
	if cfg != nil {
		t.Errorf("config = %+v, want nil on a load error", cfg)
	}
}

// TestLoadFromIncludeRejectsUnknownKeys: a names file carries three keys and
// nothing else. A stray workspace_dirs would parse cleanly and guard nothing
// if it were tolerated, so it is a load error that names the key.
func TestLoadFromIncludeRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	names := filepath.Join(dir, "names.yaml")
	writeTestFile(t, names, "blocked_words: [acmecorp]\nworkspace_dirs: [~/work]\n")
	path := filepath.Join(dir, "config.yaml")
	writeTestFile(t, path, "guard:\n  include:\n    - "+names+"\n")

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("LoadFrom() with an unknown key in the include returned nil error")
	}
	if !strings.Contains(err.Error(), "workspace_dirs") {
		t.Errorf("error = %v, want it to name the unknown key", err)
	}
}

// TestLoadFromIncludeExpandsTilde: include paths take a leading ~ like every
// other path in the config.
func TestLoadFromIncludeExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, filepath.Join(home, "names.yaml"), "blocked_words: [acmecorp]\n")
	path := filepath.Join(home, "config.yaml")
	writeTestFile(t, path, "guard:\n  include:\n    - ~/names.yaml\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if want := []string{"acmecorp"}; !slices.Equal(cfg.Guard.BlockedWords, want) {
		t.Errorf("BlockedWords = %v, want %v", cfg.Guard.BlockedWords, want)
	}
}
