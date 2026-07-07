// Package config loads the config surfaces belt reads: its own guard
// toggles (~/.config/belt/config.toml), the machine profile from ralph
// (~/.config/ralph/config.local.toml), the internal-name guard section of
// the suspenders config (~/.config/suspenders/config.yaml) so the write-time
// firewall and the git pre-commit guard can never drift apart, and the
// Bash deny patterns from the Claude settings (~/.claude/settings.json +
// settings.local.json) so the script guard and the permission system share
// one deny list.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Config is everything a guard needs to decide.
type Config struct {
	Guards     map[string]GuardToggle
	Profiles   []string
	Suspenders SuspendersGuard
	ClaudeDeny []string // Bash command prefixes from the Claude settings deny lists
}

// GuardToggle enables or disables a single guard by id, with optional
// per-guard path exclusions (substring match on the target file path) and
// extra deny patterns beyond the shared sources.
type GuardToggle struct {
	Enabled       *bool    `toml:"enabled"`
	ExcludePaths  []string `toml:"exclude_paths"`
	ExtraPatterns []string `toml:"extra_patterns"`
}

// SuspendersGuard mirrors the `guard:` section of the suspenders config.
type SuspendersGuard struct {
	WorkspaceDirs []string `yaml:"workspace_dirs"`
	BlockedWords  []string `yaml:"blocked_words"`
	Allowlist     []string `yaml:"allowlist"`
}

// GuardEnabled reports whether a guard is enabled; guards default to on so a
// missing or partial config file fails closed, not silent.
func (c Config) GuardEnabled(id string) bool {
	t, ok := c.Guards[id]
	if !ok || t.Enabled == nil {
		return true
	}
	return *t.Enabled
}

// HasProfile reports whether the ralph machine profile list contains name.
func (c Config) HasProfile(name string) bool {
	for _, p := range c.Profiles {
		if p == name {
			return true
		}
	}
	return false
}

// Load reads all config surfaces from their default locations. Every file is
// optional: a missing file yields zero values, never an error — a hook must
// not break tool calls because a config is absent.
func Load() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}
	}
	return Config{
		Guards:     loadToggles(filepath.Join(home, ".config", "belt", "config.toml")),
		Profiles:   LoadProfiles(filepath.Join(home, ".config", "ralph", "config.local.toml")),
		Suspenders: LoadSuspendersGuard(filepath.Join(home, ".config", "suspenders", "config.yaml")),
		ClaudeDeny: LoadClaudeDenyPatterns(
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude", "settings.local.json"),
		),
	}
}

// LoadClaudeDenyPatterns extracts Bash command prefixes from the
// permissions.deny lists of the given Claude settings files. An entry like
// `Bash(kubectl delete:*)` yields `kubectl delete`; non-Bash entries are
// ignored. Missing or malformed files contribute nothing.
func LoadClaudeDenyPatterns(paths ...string) []string {
	seen := map[string]bool{}
	var patterns []string
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			Permissions struct {
				Deny []string `json:"deny"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			continue
		}
		for _, entry := range cfg.Permissions.Deny {
			inner, ok := strings.CutPrefix(entry, "Bash(")
			if !ok {
				continue
			}
			inner, ok = strings.CutSuffix(inner, ")")
			if !ok {
				continue
			}
			inner = strings.TrimSuffix(inner, ":*")
			inner = strings.TrimSpace(inner)
			if inner == "" || inner == "*" || seen[inner] {
				continue
			}
			seen[inner] = true
			patterns = append(patterns, inner)
		}
	}
	return patterns
}

func loadToggles(path string) map[string]GuardToggle {
	var cfg struct {
		Guards map[string]GuardToggle `toml:"guards"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil
	}
	return cfg.Guards
}

// LoadProfiles reads the `profiles` list from a ralph config.local.toml.
func LoadProfiles(path string) []string {
	var cfg struct {
		Profiles []string `toml:"profiles"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil
	}
	return cfg.Profiles
}

// LoadSuspendersGuard reads the `guard:` section of a suspenders config.yaml.
func LoadSuspendersGuard(path string) SuspendersGuard {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SuspendersGuard{}
	}
	var cfg struct {
		Guard SuspendersGuard `yaml:"guard"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return SuspendersGuard{}
	}
	return cfg.Guard
}
