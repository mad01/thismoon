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
	"time"

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
	NamesSource   string                 // SourceBelt when the belt config sets internal_names, else SourceSuspenders
	ClaudeDeny    []string               // Bash command prefixes from the Claude settings deny lists
	GitIdentity   []GitIdentity          // git-identity rules, first matching rule wins
	CommitGuards  []CommitGuard          // commit-guard rules, every rule is checked
	CustomGuards  map[string]CustomGuard // external-command guards keyed by guard name
}

// GitIdentity is one git-identity rule: the git user.email expected for
// commits in repos matching the rule. Rules are repo-scoped rather than
// profile-scoped because the repo decides the identity — a work machine
// committing to a personal repo in the evening must still use the personal
// email, whatever profile the machine carries. Profile is an optional extra
// gate for rules that should only exist on some machines.
type GitIdentity struct {
	Repos   []string `toml:"repos"   yaml:"repos,omitempty"`   // repo patterns (exact or trailing /*); empty = every repo
	Profile string   `toml:"profile" yaml:"profile,omitempty"` // optional: rule active only on machines with this profile
	Email   string   `toml:"email"   yaml:"email"`
	Mode    string   `toml:"mode"    yaml:"mode,omitempty"` // "hard" (default) blocks, "soft" warns via events
}

// Soft reports whether the rule warns instead of denying. Identity mismatches
// default to hard: a wrong email is painful to fix after push.
func (g GitIdentity) Soft() bool { return g.Mode == "soft" }

// CommitGuard is one work-hours commit rule: commits to matching repos are
// blocked (or warned about) inside the configured local-time window on
// machines carrying the rule's profile.
type CommitGuard struct {
	Repos       []string `toml:"repos"        yaml:"repos"`                  // repo patterns (exact or trailing /*)
	AlwaysAllow []string `toml:"always_allow" yaml:"always_allow,omitempty"` // repos exempt from the hours check
	Profile     string   `toml:"profile"      yaml:"profile,omitempty"`      // rule active only on machines with this profile
	BlockHours  string   `toml:"block_hours"  yaml:"block_hours"`            // "HH:MM-HH:MM" local time
	BlockDays   []string `toml:"block_days"   yaml:"block_days,omitempty"`   // mon..sun; empty = weekdays
	Mode        string   `toml:"mode"         yaml:"mode,omitempty"`         // "soft" (default) warns, "hard" blocks
	Override    string   `toml:"override"     yaml:"override,omitempty"`     // override name that disables the rule
}

// Hard reports whether the rule denies instead of warning. Hours guards
// default to soft: the point is a nudge toward evenings, not lost work.
func (g CommitGuard) Hard() bool { return g.Mode == "hard" }

// CustomGuard is one externally-implemented guard: a named command belt execs
// with the tool-call payload on stdin. Exit 0 allows, exit 1 denies with
// stdout as the reason, anything else (or a timeout) allows with a warn event
// so a broken external never blocks work.
type CustomGuard struct {
	Enabled *bool    `toml:"enabled" yaml:"enabled,omitempty"`
	Event   string   `toml:"event"   yaml:"event"`           // "bash" or "write"
	Command []string `toml:"command" yaml:"command"`         // external tool + args
	Mode    string   `toml:"mode"    yaml:"mode,omitempty"`  // "hard" (default) blocks, "soft" warns via events
	Match   string   `toml:"match"   yaml:"match,omitempty"` // substring gate on the command (bash) or file path (write)
}

// Soft reports whether the guard warns instead of denying.
func (g CustomGuard) Soft() bool { return g.Mode == "soft" }

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
	Enabled *bool `toml:"enabled" yaml:"enabled"`
	// Mode downgrades a guard's denials to warn events when set to "soft";
	// "hard" (the default) blocks. Read by script-deny-list only.
	Mode          string   `toml:"mode"           yaml:"mode,omitempty"`
	ExcludePaths  []string `toml:"exclude_paths"  yaml:"exclude_paths,omitempty"`
	ExtraPatterns []string `toml:"extra_patterns" yaml:"extra_patterns,omitempty"`
	AllowRepos    []string `toml:"allow_repos"    yaml:"allow_repos,omitempty"`
	// AllowReposByProfile scopes an allowlist entry to machines carrying a
	// ralph profile: profile name -> repos allowlisted only there. Lets one
	// fleet-shared config allow a repo on personal machines while work
	// machines stay fail-closed (e.g. the personal agent-memory store may
	// carry internal names on personal Macs, where no work store exists to
	// route them to).
	AllowReposByProfile map[string][]string `toml:"allow_repos_by_profile" yaml:"allow_repos_by_profile,omitempty"`
}

// Soft reports whether the toggle downgrades denials to warn events instead
// of blocking.
func (t Toggle) Soft() bool { return t.Mode == "soft" }

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
// missing or partial config file fails closed, not silent. A custom guard's
// own enabled field wins over a guards: toggle of the same name — the
// custom_guards entry is where the guard is defined, so it is where it is
// switched off.
func (c Config) GuardEnabled(id string) bool {
	if cg, ok := c.CustomGuards[id]; ok && cg.Enabled != nil {
		return *cg.Enabled
	}
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

// RepoAllowed reports whether a guard's allowlist covers the canonical repo
// on this machine: the flat allow_repos list, or an allow_repos_by_profile
// bucket whose profile this machine carries. An empty repo never matches.
func (c Config) RepoAllowed(guardID, repo string) bool {
	if repo == "" {
		return false
	}
	t := c.Guards[guardID]
	if slices.Contains(t.AllowRepos, repo) {
		return true
	}
	for profile, repos := range t.AllowReposByProfile {
		if c.HasProfile(profile) && slices.Contains(repos, repo) {
			return true
		}
	}
	return false
}

// RepoMatches reports whether the canonical host/owner/repo matches any of
// the patterns: an exact match, or a prefix match for a pattern ending in
// "/*" (github.com/mad01/* covers the whole org). An empty repo never
// matches — an unresolved remote must not trip a repo-scoped rule.
func RepoMatches(patterns []string, repo string) bool {
	if repo == "" {
		return false
	}
	for _, p := range patterns {
		if prefix, ok := strings.CutSuffix(p, "/*"); ok {
			if strings.HasPrefix(repo, prefix+"/") {
				return true
			}
			continue
		}
		if p == repo {
			return true
		}
	}
	return false
}

// OverridesDir is where guard overrides live: one file per override name,
// managed by `belt override set|extend|clear`. The file's content is the
// RFC 3339 expiry the override runs until; an empty file is a legacy
// untimed override, active until cleared.
func OverridesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "belt", "overrides")
}

// Override is one override file's parsed state.
type Override struct {
	Name string
	// Expiry is when the override stops applying. Zero for a legacy untimed
	// file and for a malformed one.
	Expiry time.Time
	// Legacy marks an empty pre-timed-overrides file: active until cleared.
	Legacy bool
	// Malformed marks a file whose content did not parse as RFC 3339. A
	// malformed override is inactive — the guard stays armed.
	Malformed bool
}

// Active reports whether the override applies at t.
func (o Override) Active(t time.Time) bool {
	if o.Malformed {
		return false
	}
	if o.Legacy {
		return true
	}
	return t.Before(o.Expiry)
}

// Remaining is how long the override still applies at t: zero when it is
// legacy (no expiry to count down), malformed, or already expired.
func (o Override) Remaining(t time.Time) time.Duration {
	if o.Legacy || o.Malformed || !t.Before(o.Expiry) {
		return 0
	}
	return o.Expiry.Sub(t)
}

// ReadOverride parses the named override file; ok is false when no such
// override is set.
func ReadOverride(name string) (Override, bool) {
	dir := OverridesDir()
	if dir == "" || name == "" {
		return Override{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return Override{}, false
	}
	return parseOverride(name, data), true
}

// parseOverride interprets one override file's bytes.
func parseOverride(name string, data []byte) Override {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return Override{Name: name, Legacy: true}
	}
	expiry, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return Override{Name: name, Malformed: true}
	}
	return Override{Name: name, Expiry: expiry}
}

// OverrideActive reports whether the named override applies right now: its
// file exists and is unexpired (or legacy untimed). A missing overrides dir
// means no override is active.
func OverrideActive(name string) bool {
	o, ok := ReadOverride(name)
	return ok && o.Active(time.Now())
}

// Overrides lists every override file, parsed and sorted by name, for the
// doctor report and `belt override`. Expired and malformed entries are
// included so the listing can say why a switch is not working.
func Overrides() []Override {
	entries, err := os.ReadDir(OverridesDir())
	if err != nil {
		return nil
	}
	var out []Override
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if o, ok := ReadOverride(e.Name()); ok {
			out = append(out, o)
		}
	}
	slices.SortFunc(out, func(a, b Override) int { return strings.Compare(a.Name, b.Name) })
	return out
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
		Guards:       f.Guards,
		Hints:        f.Hints,
		ClaudeDeny:   LoadClaudeDenyPatterns(p.ClaudeSettings...),
		GitIdentity:  f.GitIdentity,
		CommitGuards: f.CommitGuards,
		CustomGuards: f.CustomGuards,
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
	Guards        map[string]Toggle      `toml:"guards"         yaml:"guards"`
	Hints         map[string]Toggle      `toml:"hints"          yaml:"hints"`
	Profiles      []string               `toml:"profiles"       yaml:"profiles"`
	InternalNames *InternalNames         `toml:"internal_names" yaml:"internal_names"`
	GitIdentity   []GitIdentity          `toml:"git_identity"   yaml:"git_identity"`
	CommitGuards  []CommitGuard          `toml:"commit_guards"  yaml:"commit_guards"`
	CustomGuards  map[string]CustomGuard `toml:"custom_guards"  yaml:"custom_guards"`
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
