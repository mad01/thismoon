// Package config manages the suspenders global configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the global suspenders configuration.
type Config struct {
	Dirs      []string      `yaml:"dirs"`      // directories to scan for git repos
	Exclude   []string      `yaml:"exclude"`   // repo name patterns to exclude (glob)
	Watch     []WatchRule   `yaml:"watch"`     // custom patterns to watch for
	Allowlist []AllowEntry  `yaml:"allowlist"` // exact-match values that are known-safe
	Scan      ScanConfig    `yaml:"scan"`      // built-in secret scanner settings
	Guard     GuardConfig   `yaml:"guard"`     // built-in internal-reference guard
	Hooks     HooksConfig   `yaml:"hooks"`     // external hook scripts grouped by event
	History   HistoryConfig `yaml:"history"`   // history clean settings
}

// ScanConfig controls the built-in secret scanner.
type ScanConfig struct {
	Enabled      *bool    `yaml:"enabled"`       // nil = true (backwards compat)
	ExcludeRules []string `yaml:"exclude_rules"` // rule IDs to suppress globally
}

// ScanEnabled returns whether the built-in scan is enabled (default true).
func (s ScanConfig) ScanEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// GuardConfig controls the built-in internal-reference guard.
type GuardConfig struct {
	Enabled       bool     `yaml:"enabled"`
	WorkspaceDirs []string `yaml:"workspace_dirs"`
	BlockedWords  []string `yaml:"blocked_words"`
	Allowlist     []string `yaml:"allowlist"`
	FilePatterns  []string `yaml:"file_patterns"`
}

// HistoryConfig controls history clean behaviour.
type HistoryConfig struct {
	ReplaceTable map[string]string `yaml:"replace_table"` // old string -> new string
	RedactFiles  []string          `yaml:"redact_files"`  // path globs whose blob content is fully redacted
}

// ExternalHook defines a user-configured hook script.
type ExternalHook struct {
	Name         string   `yaml:"name"`
	Command      string   `yaml:"command"`
	FilePatterns []string `yaml:"file_patterns"`
	Repos        []string `yaml:"repos"`
	Enabled      *bool    `yaml:"enabled"` // nil = true
}

// IsEnabled returns whether this external hook is enabled (default true).
func (h ExternalHook) IsEnabled() bool {
	return h.Enabled == nil || *h.Enabled
}

// HooksConfig holds external hook scripts grouped by git event type.
type HooksConfig struct {
	PreCommit []ExternalHook `yaml:"pre_commit"`
	PostMerge []ExternalHook `yaml:"post_merge"`
}

// WatchRule is a user-defined detection pattern.
type WatchRule struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`  // regex
	Severity    string `yaml:"severity"` // high, medium, low
}

// AllowEntry is an exact-match value that should be permitted. If a finding's
// raw (unredacted) match equals Value, it is suppressed. If Paths is set, the
// allowance only applies to files matching those globs.
type AllowEntry struct {
	Match       string   `yaml:"match"`
	Description string   `yaml:"description"`
	Paths       []string `yaml:"paths"` // optional file glob restriction
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Dirs: []string{"~/code/src", "~/workspace"},
	}
}

// Path returns the path to the config file, respecting XDG_CONFIG_HOME.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			base = filepath.Join(".", ".config")
		} else {
			base = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(base, "suspenders", "config.yaml")
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(p string) string {
	if len(p) == 0 || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// Load reads the config from the default path, creating a default config file
// if none exists.
func Load() (*Config, error) {
	p := Path()

	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		cfg := DefaultConfig()
		if writeErr := writeDefault(p, cfg); writeErr != nil {
			// Non-fatal: return the default even if we can't persist it.
			return cfg, nil
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", p, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", p, err)
	}
	return &cfg, nil
}

func writeDefault(p string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("write default config %s: %w", p, err)
	}
	return nil
}
