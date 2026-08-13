// Package config loads the config surfaces belt reads: its own guard
// toggles (~/.config/belt/config.yaml, with a legacy config.toml
// fallback), the machine profile from ralph
// (~/.config/ralph/config.local.toml), the internal-name guard section of
// the suspenders config (~/.config/suspenders/config.yaml) so the write-time
// firewall and the git pre-commit guard can never drift apart, and the
// Bash deny patterns from the Claude settings (~/.claude/settings.json +
// settings.local.json) so the script guard and the permission system share
// one deny list.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Config is everything a guard or hint needs to decide.
type Config struct {
	Guards     map[string]Toggle
	Hints      map[string]Toggle
	Profiles   []string
	Suspenders SuspendersGuard
	ClaudeDeny []string // Bash command prefixes from the Claude settings deny lists
}

// Toggle enables or disables a single guard or hint by id, with optional
// path exclusions (substring match on the target file path), extra deny
// patterns beyond the shared sources, and a repository allowlist that lets a
// guard exempt specific repos (canonical host/owner/repo, e.g.
// github.com/mad01/dotfiles).
//
// The list fields are omitempty so `belt config` can print a resolved toggle
// without three empty lists under every guard.
type Toggle struct {
	Enabled       *bool    `toml:"enabled"        yaml:"enabled"`
	ExcludePaths  []string `toml:"exclude_paths"  yaml:"exclude_paths,omitempty"`
	ExtraPatterns []string `toml:"extra_patterns" yaml:"extra_patterns,omitempty"`
	AllowRepos    []string `toml:"allow_repos"    yaml:"allow_repos,omitempty"`
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
	return enabled(c.Guards, id)
}

// HintEnabled reports whether a hint is enabled. Hints default to on for the
// same reason guards do, though the stakes differ: a disabled guard silently
// stops denying, while a disabled hint only stops advising.
func (c Config) HintEnabled(id string) bool {
	return enabled(c.Hints, id)
}

func enabled(toggles map[string]Toggle, id string) bool {
	t, ok := toggles[id]
	if !ok || t.Enabled == nil {
		return true
	}
	return *t.Enabled
}

// HasProfile reports whether the ralph machine profile list contains name.
func (c Config) HasProfile(name string) bool {
	return slices.Contains(c.Profiles, name)
}

// Paths lists the locations of every config surface belt reads. `belt doctor`
// uses it to report where each surface was (or wasn't) found.
type Paths struct {
	BeltYAML       string   // ~/.config/belt/config.yaml
	BeltTOML       string   // legacy fallback, read only when the YAML file is absent
	Ralph          string   // ~/.config/ralph/config.local.toml
	Suspenders     string   // ~/.config/suspenders/config.yaml
	ClaudeSettings []string // ~/.claude/settings.json + settings.local.json
}

// DefaultPaths returns the standard location of every config surface.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("config: resolve home dir: %w", err)
	}
	return Paths{
		BeltYAML:   filepath.Join(home, ".config", "belt", "config.yaml"),
		BeltTOML:   filepath.Join(home, ".config", "belt", "config.toml"),
		Ralph:      filepath.Join(home, ".config", "ralph", "config.local.toml"),
		Suspenders: filepath.Join(home, ".config", "suspenders", "config.yaml"),
		ClaudeSettings: []string{
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude", "settings.local.json"),
		},
	}, nil
}

// Load reads all config surfaces from their default locations. Every file is
// optional: a missing file yields zero values, never an error — a hook must
// not break tool calls because a config is absent.
func Load() Config {
	p, err := DefaultPaths()
	if err != nil {
		return Config{}
	}
	return LoadFrom(p)
}

// LoadFrom reads all config surfaces from the given locations, with the same
// missing-file tolerance as Load.
func LoadFrom(p Paths) Config {
	guards, hints := loadToggles(p.BeltYAML, p.BeltTOML)
	return Config{
		Guards:     guards,
		Hints:      hints,
		Profiles:   LoadProfiles(p.Ralph),
		Suspenders: LoadSuspendersGuard(p.Suspenders),
		ClaudeDeny: LoadClaudeDenyPatterns(p.ClaudeSettings...),
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

// togglesFile is the shape of the belt config file in either format.
type togglesFile struct {
	Guards map[string]Toggle `toml:"guards" yaml:"guards"`
	Hints  map[string]Toggle `toml:"hints"  yaml:"hints"`
}

// loadToggles prefers the YAML config and falls back to the legacy TOML file
// only when the YAML file does not exist. A present-but-broken file yields
// defaults (everything enabled) rather than silently reading the other
// format: fail closed, not stale.
func loadToggles(yamlPath, tomlPath string) (guards, hints map[string]Toggle) {
	guards, hints, err := LoadTogglesYAML(yamlPath)
	if err == nil {
		return guards, hints
	}
	if os.IsNotExist(err) {
		if g, h, tomlErr := LoadTogglesTOML(tomlPath); tomlErr == nil {
			return g, h
		}
	}
	return nil, nil
}

// LoadTogglesYAML reads guard and hint toggles from a YAML belt config. The
// error distinguishes a missing file (os.IsNotExist) from a parse failure so
// doctor can report which one it is.
func LoadTogglesYAML(path string) (guards, hints map[string]Toggle, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg togglesFile
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return cfg.Guards, cfg.Hints, nil
}

// LoadTogglesTOML reads guard and hint toggles from a legacy TOML belt config.
func LoadTogglesTOML(path string) (guards, hints map[string]Toggle, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg togglesFile
	if _, err := toml.Decode(string(raw), &cfg); err != nil {
		return nil, nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return cfg.Guards, cfg.Hints, nil
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
