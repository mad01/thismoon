// Package config manages the suspenders global configuration file.
//
// Loading never writes. suspenders runs inside git pre-commit hooks, where
// the process environment is whatever git handed it and the working
// directory is the repository being committed to; a load that creates files
// puts them somewhere nobody asked for. Defaults live in memory, and
// `suspenders config init` is the one command that puts a file on disk.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
)

// Component is the config directory name suspenders owns under the XDG
// config root.
const Component = "suspenders"

// EnvConfig names the config file, overriding the default location. The
// --config flag wins over it.
const EnvConfig = "SUSPENDERS_CONFIG"

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

// defaultDirs are the repo-discovery roots `hook install --all` walks when
// the config names none. They are a guess about one machine's layout, which
// is why they stay in memory: writing them into a file makes them look like
// a decision someone made.
var defaultDirs = []string{"~/code/src", "~/workspace"}

// DefaultConfig returns the config suspenders runs with when there is no
// config file.
func DefaultConfig() *Config {
	return (&Config{}).WithDefaults()
}

// WithDefaults fills the fields a config file left out and returns c.
//
// Nil and empty mean different things for Dirs: an absent `dirs` key takes
// the defaults, while an explicit `dirs: []` is a machine saying it
// discovers nothing, and the commands that need dirs report that rather than
// quietly walking two directories the operator did not ask for.
func (c *Config) WithDefaults() *Config {
	if c.Dirs == nil {
		c.Dirs = slices.Clone(defaultDirs)
	}
	return c
}

// Path returns the default path of the config file:
// $XDG_CONFIG_HOME/suspenders/config.yaml, else
// ~/.config/suspenders/config.yaml. An unresolvable home directory is an
// error, never a path relative to the working directory — under a
// pre-commit hook that working directory is the repository being committed
// to, so the old fallback both read a config nobody wrote and wrote one into
// somebody's repo.
func Path() (string, error) {
	return confdir.Path(Component, "config.yaml")
}

// PathFor returns the config file to use: the flag value when given, else
// $SUSPENDERS_CONFIG, else the default path.
func PathFor(flagValue string) (string, error) {
	override := flagValue
	if override == "" {
		override = os.Getenv(EnvConfig)
	}
	if override == "" {
		return Path()
	}
	expanded, err := confdir.Expand(override)
	if err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	return expanded, nil
}

// ExpandPath expands a leading ~ to the user's home directory, returning the
// path unchanged when the home directory cannot be resolved. Its callers
// pass the result to repo discovery, where an unexpanded ~ finds nothing;
// the commands that need a resolvable home fail on Path first.
func ExpandPath(p string) string {
	expanded, err := confdir.Expand(p)
	if err != nil {
		return p
	}
	return expanded
}

// Load reads the config from the default path (or the one --config and
// $SUSPENDERS_CONFIG select, via PathFor).
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(p)
}

// LoadFrom reads the config from path. A missing file yields the defaults
// with a nil error; a file that exists but cannot be read or parsed is an
// error, which the guard entrypoints turn into a failed hook rather than a
// commit checked against nothing.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg.WithDefaults(), nil
}

// Write creates path's parent directory and writes cfg to it. It is the
// explicit counterpart to Load's read-only behavior: `suspenders config
// init` calls it, nothing else does.
func Write(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}
