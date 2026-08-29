package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

func TestLoadFrom(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	content := []byte("dirs:\n  - /tmp/repos\n  - /tmp/other\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(cfg.Dirs))
	}
	if cfg.Dirs[0] != "/tmp/repos" {
		t.Errorf("expected /tmp/repos, got %s", cfg.Dirs[0])
	}
	if cfg.Dirs[1] != "/tmp/other" {
		t.Errorf("expected /tmp/other, got %s", cfg.Dirs[1])
	}
}

func TestLoadFromTildeExpansion(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	content := []byte("dirs:\n  - ~/code/repos\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "code/repos")
	if cfg.Dirs[0] != expected {
		t.Errorf("expected %s, got %s", expected, cfg.Dirs[0])
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	_, err := LoadFrom("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFromInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	content := []byte("not: [valid: yaml: {{{\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(cfgPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// TestLoadFromIgnoresRetiredKeys pins the compatibility promise made when
// layout/summary/tmpdir were dropped: a config file that still sets them
// loads unchanged, because the decoder is not in strict mode.
func TestLoadFromIgnoresRetiredKeys(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("dirs:\n  - /tmp/repos\nlayout: tab\nsummary: true\ntmpdir: /tmp/scratch\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom() with retired keys = %v, want nil", err)
	}
	if len(cfg.Dirs) != 1 || cfg.Dirs[0] != "/tmp/repos" {
		t.Errorf("dirs = %v, want the live key to survive the retired ones", cfg.Dirs)
	}
}

func TestPathPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Cleanup(func() { SetPath("") })

	t.Run("default location", func(t *testing.T) {
		SetPath("")
		t.Setenv(PathEnv, "")
		got, err := Path()
		if err != nil {
			t.Fatalf("Path() error: %v", err)
		}
		if want := filepath.Join(home, ".config", "csl", "config.yaml"); got != want {
			t.Errorf("Path() = %q, want %q", got, want)
		}
	})

	t.Run("env moves it", func(t *testing.T) {
		SetPath("")
		t.Setenv(PathEnv, "~/elsewhere.yaml")
		got, err := Path()
		if err != nil {
			t.Fatalf("Path() error: %v", err)
		}
		if want := filepath.Join(home, "elsewhere.yaml"); got != want {
			t.Errorf("Path() = %q, want the expanded %q", got, want)
		}
	})

	t.Run("pinned path beats env", func(t *testing.T) {
		t.Setenv(PathEnv, "/from/env.yaml")
		SetPath("/from/flag.yaml")
		got, err := Path()
		if err != nil {
			t.Fatalf("Path() error: %v", err)
		}
		if got != "/from/flag.yaml" {
			t.Errorf("Path() = %q, want the pinned path", got)
		}
	})
}

func TestEffectiveWebBaseURL(t *testing.T) {
	t.Run("configured value wins", func(t *testing.T) {
		t.Setenv("CSL_PORT", "9424")
		cfg := &Config{Web: WebConfig{BaseURL: "http://csl.this/"}}
		if got, want := cfg.EffectiveWebBaseURL(), "http://csl.this"; got != want {
			t.Errorf("EffectiveWebBaseURL() = %q, want %q", got, want)
		}
	})

	t.Run("unset follows the port", func(t *testing.T) {
		t.Setenv("CSL_PORT", "9424")
		cfg := &Config{}
		if got, want := cfg.EffectiveWebBaseURL(), "http://127.0.0.1:9424"; got != want {
			t.Errorf("EffectiveWebBaseURL() = %q, want %q", got, want)
		}
	})

	t.Run("nil config falls back to the default port", func(t *testing.T) {
		t.Setenv("CSL_PORT", "")
		var cfg *Config
		if got, want := cfg.EffectiveWebBaseURL(), csl.DefaultBaseURL; got != want {
			t.Errorf("EffectiveWebBaseURL() = %q, want %q", got, want)
		}
	})
}

func TestPostMergeHookExclude(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	content := []byte(`dirs:
  - /tmp/repos
hooks:
  post_merge:
    enabled: true
    exclude:
      - /Users/me/workspace/large-monorepo
      - org/big-monorepo
      - ~/code/skip-this
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Hooks.PostMerge.Enabled {
		t.Fatal("expected enabled=true")
	}

	home, _ := os.UserHomeDir()
	cases := []struct {
		path, name string
		want       bool
	}{
		{"/Users/me/workspace/large-monorepo", "me/large-monorepo", true},
		{"/anywhere", "org/big-monorepo", true},
		{filepath.Join(home, "code/skip-this"), "me/skip-this", true},
		{"/Users/me/workspace/other", "me/other", false},
	}
	for _, c := range cases {
		if got := cfg.Hooks.PostMerge.IsExcluded(c.path, c.name); got != c.want {
			t.Errorf("IsExcluded(%q, %q) = %v, want %v", c.path, c.name, got, c.want)
		}
	}
}

func TestFilterExcluded(t *testing.T) {
	h := &PostMergeHook{Exclude: []string{"/repos/skip-by-path", "org/skip-by-name"}}
	repos := []finder.Repo{
		{Name: "org/keep", Path: "/repos/keep"},
		{Name: "org/skipped", Path: "/repos/skip-by-path"},
		{Name: "org/skip-by-name", Path: "/repos/elsewhere"},
	}

	got := h.FilterExcluded(repos)
	if len(got) != 1 || got[0].Name != "org/keep" {
		t.Fatalf("FilterExcluded = %+v, want only org/keep", got)
	}

	// Nil receiver and empty exclude list pass repos through untouched.
	var nilHook *PostMergeHook
	if out := nilHook.FilterExcluded(repos); len(out) != len(repos) {
		t.Fatalf("nil hook filtered repos: %+v", out)
	}
	if out := (&PostMergeHook{}).FilterExcluded(repos); len(out) != len(repos) {
		t.Fatalf("empty exclude filtered repos: %+v", out)
	}
}

func TestPostMergeHookDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("dirs:\n  - /tmp/repos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks.PostMerge.Enabled {
		t.Error("expected enabled=false when hooks block is omitted")
	}
	if cfg.Hooks.PostMerge.IsExcluded("/x", "x/y") {
		t.Error("expected no exclusion when list is empty")
	}
}

func TestSemanticConfig(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantEnabled bool
		wantSync    bool
	}{
		{"default (no semantic block)", "dirs:\n  - /tmp\n", false, false},
		{"enabled only", "dirs:\n  - /tmp\nsemantic:\n  enabled: true\n", true, false},
		{"enabled and sync", "dirs:\n  - /tmp\nsemantic:\n  enabled: true\n  sync: true\n", true, true},
		{"explicit false", "dirs:\n  - /tmp\nsemantic:\n  enabled: false\n", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			cfgPath := filepath.Join(tmp, "config.yaml")
			if err := os.WriteFile(cfgPath, []byte(tt.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadFrom(cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.SemanticEnabled(); got != tt.wantEnabled {
				t.Errorf("SemanticEnabled() = %v, want %v", got, tt.wantEnabled)
			}
			if got := cfg.SemanticSyncEnabled(); got != tt.wantSync {
				t.Errorf("SemanticSyncEnabled() = %v, want %v", got, tt.wantSync)
			}
		})
	}
}

func TestSemanticConfigNilReceiver(t *testing.T) {
	var cfg *Config
	if cfg.SemanticEnabled() {
		t.Error("expected SemanticEnabled() = false for nil config")
	}
	if cfg.SemanticSyncEnabled() {
		t.Error("expected SemanticSyncEnabled() = false for nil config")
	}
}

func TestLoadFromGlobalConfig(t *testing.T) {
	tmp := t.TempDir()

	// Set HOME to tmp so Load() looks in tmp/.config/csl/
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmp)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	cfgDir := filepath.Join(tmp, ".config", "csl")
	_ = os.MkdirAll(cfgDir, 0o755)
	cfgPath := filepath.Join(cfgDir, "config.yaml")

	content := []byte("dirs:\n  - /global/repos\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Dirs[0] != "/global/repos" {
		t.Errorf("expected /global/repos from global config, got %s", cfg.Dirs[0])
	}
}

func TestDaemonIdleTimeout(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	content := []byte("dirs:\n  - /tmp/repos\ndaemon:\n  idle_timeout_minutes: 45\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.DaemonIdleTimeout(); got != 45*time.Minute {
		t.Errorf("DaemonIdleTimeout() = %v, want 45m", got)
	}
}

func TestDaemonIdleTimeoutDefault(t *testing.T) {
	var nilCfg *Config
	if got := nilCfg.DaemonIdleTimeout(); got != 10*time.Minute {
		t.Errorf("nil receiver: DaemonIdleTimeout() = %v, want 10m", got)
	}
	zero := &Config{}
	if got := zero.DaemonIdleTimeout(); got != 10*time.Minute {
		t.Errorf("zero config: DaemonIdleTimeout() = %v, want 10m", got)
	}
	neg := &Config{Daemon: DaemonConfig{IdleTimeoutMinutes: -5}}
	if got := neg.DaemonIdleTimeout(); got != 10*time.Minute {
		t.Errorf("negative: DaemonIdleTimeout() = %v, want 10m", got)
	}
}
