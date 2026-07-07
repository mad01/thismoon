package scanner

import (
	"fmt"
	"os"
	"strings"

	"github.com/gobwas/glob"
	"gopkg.in/yaml.v3"
)

// IgnoreConfig holds per-repo configuration loaded from .suspenders.yaml.
// Fields overlay the global config: per-repo watch rules and allowlist entries
// are appended to global ones; ignore rules/paths/patterns are repo-only.
type IgnoreConfig struct {
	Rules     []string          `yaml:"rules"`     // rule IDs to suppress
	Paths     []string          `yaml:"paths"`     // file path globs to suppress
	Patterns  []string          `yaml:"patterns"`  // literal match substrings to suppress (for false positives)
	Allowlist []IgnoreAllowItem `yaml:"allowlist"` // exact-match values that are known-safe
	Watch     []WatchEntry      `yaml:"watch"`     // repo-specific custom detection rules
}

// IgnoreAllowItem is an exact value that should be permitted in a per-repo config.
type IgnoreAllowItem struct {
	Match       string   `yaml:"match"`
	Description string   `yaml:"description"`
	Paths       []string `yaml:"paths"`
}

// WatchEntry is a custom detection rule defined in a per-repo config.
type WatchEntry struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`
	Severity    string `yaml:"severity"`
}

// LoadIgnoreConfig reads and parses the ignore file at path.
func LoadIgnoreConfig(path string) (*IgnoreConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg IgnoreConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

// ShouldIgnore returns true when the finding matches any ignore rule.
func (ic *IgnoreConfig) ShouldIgnore(f Finding) bool {
	for _, ruleID := range ic.Rules {
		if strings.EqualFold(ruleID, f.Rule.ID) {
			return true
		}
	}

	for _, pattern := range ic.Paths {
		g, err := glob.Compile(pattern, '/')
		if err != nil {
			continue
		}
		if g.Match(f.File) {
			return true
		}
	}

	for _, p := range ic.Patterns {
		if strings.Contains(f.RawMatch, p) || strings.Contains(f.Match, p) ||
			strings.Contains(f.Context, p) {
			return true
		}
	}

	return false
}
