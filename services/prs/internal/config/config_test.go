package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Loaded {
		t.Error("missing file should report Loaded=false")
	}
	if cfg.PollInterval != DefaultPollInterval {
		t.Errorf("PollInterval = %s, want %s", cfg.PollInterval, DefaultPollInterval)
	}
	if len(cfg.Dirs) != 0 {
		t.Errorf("Dirs = %v, want empty", cfg.Dirs)
	}
}

func TestLoadFullConfig(t *testing.T) {
	path := writeConfig(t, `
dirs:
  - /tmp/code
exclude:
  - org/skip-*
hosts:
  - github.com
poll_interval: 90s
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Loaded {
		t.Error("Loaded = false, want true")
	}
	if len(cfg.Dirs) != 1 || cfg.Dirs[0] != "/tmp/code" {
		t.Errorf("Dirs = %v", cfg.Dirs)
	}
	if len(cfg.Exclude) != 1 || cfg.Exclude[0] != "org/skip-*" {
		t.Errorf("Exclude = %v", cfg.Exclude)
	}
	if cfg.PollInterval != 90*time.Second {
		t.Errorf("PollInterval = %s, want 90s", cfg.PollInterval)
	}
}

func TestLoadBadIntervalFails(t *testing.T) {
	path := writeConfig(t, "poll_interval: soonish\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unparseable poll_interval")
	}
}

func TestLoadNegativeIntervalFails(t *testing.T) {
	path := writeConfig(t, "poll_interval: -5m\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for negative poll_interval")
	}
}

func TestLoadBadYAMLFails(t *testing.T) {
	path := writeConfig(t, "dirs: [unclosed\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestHostAllowed(t *testing.T) {
	open := Config{}
	if !open.HostAllowed("anything.example.com") {
		t.Error("empty allowlist should allow every host")
	}
	limited := Config{Hosts: []string{"github.com"}}
	if !limited.HostAllowed("github.com") {
		t.Error("listed host should be allowed")
	}
	if limited.HostAllowed("other.example.com") {
		t.Error("unlisted host should be rejected")
	}
}
