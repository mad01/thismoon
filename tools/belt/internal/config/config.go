// Package config loads the config surfaces belt reads. Belt-owned settings
// live in ~/.config/belt/config.yaml (guard and hint toggles, machine
// profiles, the internal-name list), with a legacy config.toml fallback.
// Two settings fall back to the tool that originated them when the belt
// config does not set them: profiles fall back to the ralph machine config
// (~/.config/ralph/config.local.toml) and the internal-name list falls back
// to the guard: section of the suspenders config
// (~/.config/suspenders/config.yaml). The Bash deny patterns always come
// from the Claude settings (~/.claude/settings.json + settings.local.json)
// so the script guard and the permission system share one deny list.
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

// Sources of a fallback-resolved setting: which config file supplied the
// value belt runs with. doctor and config report them so a resolved setting
// is always attributable to a file.
const (
	SourceBelt       = "belt"
	SourceRalph      = "ralph"
	SourceSuspenders = "suspenders"
)

// Config is everything a guard or hint needs to decide.
type Config struct {
	Guards        map[string]Toggle
	Hints         map[string]Toggle
	Profiles      []string
	ProfileSource string // SourceBelt when the belt config sets profiles, else SourceRalph
	Names         InternalNames
	NamesSource   string   // SourceBelt when the belt config sets internal_names, else SourceSuspenders
	ClaudeDeny    []string // Bash command prefixes from the Claude settings deny lists
}

// Toggle enables or disables a single guard or hint by id, with optional
// path exclusions, extra deny patterns beyond the shared sources, and a
// repository allowlist that lets a guard exempt specific repos (canonical
// host/owner/repo, e.g. github.com/mad01/dotfiles). An exclude_paths entry
// starting with ~ or / is expanded and matched as a path prefix; any other
// entry matches as a substring of the target path.
//
// The list fields are omitempty so `belt config` can print a resolved toggle
// without three empty lists under every guard.
type Toggle struct {
	Enabled       *bool    `toml:"enabled"        yaml:"enabled"`
	ExcludePaths  []string `toml:"exclude_paths"  yaml:"exclude_paths,omitempty"`
	ExtraPatterns []string `toml:"extra_patterns" yaml:"extra_patterns,omitempty"`
	AllowRepos    []string `toml:"allow_repos"    yaml:"allow_repos,omitempty"`
}

// ExcludesPath reports whether the toggle's exclude_paths cover the given
// file path: prefix match for absolute and ~-prefixed entries, substring
// match otherwise.
func (t Toggle) ExcludesPath(path string) bool {
	for _, excl := range t.ExcludePaths {
		if excl == "" {
			continue
		}
		if strings.HasPrefix(excl, "~") || strings.HasPrefix(excl, "/") {
			dir := strings.TrimSuffix(ExpandHome(excl), "/")
			if path == dir || strings.HasPrefix(path, dir+"/") {
				return true
			}
			continue
		}
		if strings.Contains(path, excl) {
			return true
		}
	}
	return false
}

// InternalNames is the name-derivation config for the write-internal-names
// guard: where to discover internal repos, extra always-blocked words, and
// safe references to drop. Shape-compatible with the guard: section of the
// suspenders config so the same block works in either file.
type InternalNames struct {
	WorkspaceDirs []string `toml:"workspace_dirs" yaml:"workspace_dirs"`
	BlockedWords  []string `toml:"blocked_words"  yaml:"blocked_words"`
	Allowlist     []string `toml:"allowlist"      yaml:"allowlist"`
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

// HasProfile reports whether the resolved machine profile list contains name.
func (c Config) HasProfile(name string) bool {
	return slices.Contains(c.Profiles, name)
}

// Paths lists the locations of every config surface belt reads. `belt doctor`
// uses it to report where each surface was (or wasn't) found.
type Paths struct {
	BeltYAML       string   // ~/.config/belt/config.yaml
	BeltTOML       string   // legacy fallback, read only when the YAML file is absent
	Ralph          string   // ~/.config/ralph/config.local.toml (profiles fallback)
	Suspenders     string   // ~/.config/suspenders/config.yaml (internal-name fallback)
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
// missing-file tolerance as Load. Profiles and the internal-name list come
// from the belt config when it sets them; otherwise they fall back to the
// ralph and suspenders configs, and the Source fields record which one won.
func LoadFrom(p Paths) Config {
	f := loadFile(p.BeltYAML, p.BeltTOML)
	cfg := Config{
		Guards:     f.Guards,
		Hints:      f.Hints,
		ClaudeDeny: LoadClaudeDenyPatterns(p.ClaudeSettings...),
	}
	cfg.Profiles, cfg.ProfileSource = f.Profiles, SourceBelt
	if len(cfg.Profiles) == 0 {
		cfg.Profiles, cfg.ProfileSource = LoadProfiles(p.Ralph), SourceRalph
	}
	if f.InternalNames != nil {
		cfg.Names, cfg.NamesSource = *f.InternalNames, SourceBelt
	} else {
		cfg.Names, cfg.NamesSource = LoadSuspendersGuard(p.Suspenders), SourceSuspenders
	}
	return cfg
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

// File is the parsed shape of the belt config file in either format.
// InternalNames is a pointer so an absent internal_names section (fall back
// to the suspenders config) is distinguishable from a present-but-empty one
// (belt owns the list, and it is empty).
type File struct {
	Guards        map[string]Toggle `toml:"guards"         yaml:"guards"`
	Hints         map[string]Toggle `toml:"hints"          yaml:"hints"`
	Profiles      []string          `toml:"profiles"       yaml:"profiles"`
	InternalNames *InternalNames    `toml:"internal_names" yaml:"internal_names"`
}

// loadFile prefers the YAML config and falls back to the legacy TOML file
// only when the YAML file does not exist. A present-but-broken file yields
// defaults (everything enabled) rather than silently reading the other
// format: fail closed, not stale.
func loadFile(yamlPath, tomlPath string) File {
	f, err := LoadFileYAML(yamlPath)
	if err == nil {
		return f
	}
	if os.IsNotExist(err) {
		if f, tomlErr := LoadFileTOML(tomlPath); tomlErr == nil {
			return f
		}
	}
	return File{}
}

// LoadFileYAML reads the belt config from a YAML file. The error
// distinguishes a missing file (os.IsNotExist) from a parse failure so
// doctor can report which one it is.
func LoadFileYAML(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return f, nil
}

// LoadFileTOML reads the belt config from a legacy TOML file.
func LoadFileTOML(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if _, err := toml.Decode(string(raw), &f); err != nil {
		return File{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return f, nil
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

// LoadSuspendersGuard reads the `guard:` section of a suspenders config.yaml
// into the shared InternalNames shape.
func LoadSuspendersGuard(path string) InternalNames {
	raw, err := os.ReadFile(path)
	if err != nil {
		return InternalNames{}
	}
	var cfg struct {
		Guard InternalNames `yaml:"guard"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return InternalNames{}
	}
	return cfg.Guard
}

// ExpandHome expands a leading ~ to the user's home directory.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
