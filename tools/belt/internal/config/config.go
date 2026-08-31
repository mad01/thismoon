// Package config loads the config surfaces belt reads. Belt-owned settings
// live in ~/.config/belt/config.yaml (guard and hint toggles, the
// internal-name list, guard rules), with a legacy config.toml fallback.
// The config is standalone (docs/adr/0010): belt reads no other tool's
// config file and has no machine-profile concept — the provisioning layer
// renders each machine class's values into belt's own file. The one
// exception is the Bash deny patterns, which come from the Claude settings
// (~/.claude/settings.json + settings.local.json) behind the
// claude_settings gate so the script guard and the permission system share
// one deny list.
//
// Absent and invalid are different answers here. A missing config file
// means the built-in defaults (every guard armed, no rules), because a
// machine that never wrote one still gets the guardrails. A config file
// that is present but unparseable or invalid is an error every caller must
// surface: belt is a guard, and a guard that cannot read its own rules must
// not decide it has none.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
)

// Component is belt's config directory name under the XDG config root.
const Component = "belt"

// EnvConfig names the config file, overriding the default location. The
// --config flag wins over it.
const EnvConfig = "BELT_CONFIG"

// Guard event names. They live here rather than in the guard package
// because the config file names them (custom_guards.<name>.event) and this
// package validates what it reads; internal/guard aliases these constants.
const (
	EventBash  = "bash"  // matcher: Bash
	EventWrite = "write" // matcher: Write|Edit
)

// Guard and rule modes. "hard" denies, "soft" downgrades the denial to a
// warn event on the events service; which one is the default depends on the
// rule (identity mismatches default to hard, work-hours nudges to soft).
const (
	ModeHard = "hard"
	ModeSoft = "soft"
)

// SoftModeGuards lists the built-in guards that read a `mode:` toggle.
// Every other guard ignores it, so setting mode: soft on one of them is a
// config error rather than a quietly ignored key — the shape that lets an
// operator believe a guard was downgraded while it still blocks (or, worse,
// believe it blocks while they meant to soften it). A guard package test
// pins this list to the ids that actually call Toggle.Soft.
var SoftModeGuards = []string{"script-deny-list"}

// Config is everything a guard or hint needs to decide.
type Config struct {
	Guards map[string]Toggle
	Hints  map[string]Toggle
	// DirectMainRepos names the repos whose workflow is direct-to-main:
	// single-writer store clones and config repos that never take PRs.
	// Read by exactly the two checks whose subject is that workflow —
	// git-push-main (the push is allowed) and the commit-policy hint (the
	// branch + PR advice is silenced). Purpose-named on the reversibility
	// criterion (docs/adr/0013): a generic "exclude" list gets edited to
	// quiet an advisory and silently disarms a guard; a list that states a
	// workflow fact cannot reach checks the fact is irrelevant to.
	DirectMainRepos []string
	Names           InternalNames
	ClaudeSettings  ClaudeSettings         // gate on reading the Claude settings files
	ClaudeDeny      []string               // Bash deny prefixes from the Claude settings; empty when the read is disabled
	GitIdentity     []GitIdentity          // git-identity rules, first matching rule wins
	CommitGuards    []CommitGuard          // commit-guard rules, every rule is checked
	CustomGuards    map[string]CustomGuard // external-command guards keyed by guard name
}

// GitIdentity is one git-identity rule: the git user.email expected for
// commits in repos matching the rule. Rules are repo-scoped because the
// repo decides the identity — a work machine committing to a personal repo
// in the evening must still use the personal email. A rule that should
// exist only on some machines belongs in that machine class's rendered
// config, not behind a runtime gate (docs/adr/0010).
type GitIdentity struct {
	Repos []string `toml:"repos" yaml:"repos,omitempty"` // repo patterns (exact or trailing /*); empty = every repo
	Email string   `toml:"email" yaml:"email"`
	Mode  string   `toml:"mode"  yaml:"mode,omitempty"` // "hard" (default) blocks, "soft" warns via events
}

// Soft reports whether the rule warns instead of denying. Identity mismatches
// default to hard: a wrong email is painful to fix after push.
func (g GitIdentity) Soft() bool { return g.Mode == "soft" }

// CommitGuard is one work-hours commit rule: commits to matching repos are
// blocked (or warned about) inside the configured local-time window. A rule
// meant for one machine class lives in that class's rendered config
// (docs/adr/0010).
type CommitGuard struct {
	Repos       []string `toml:"repos"        yaml:"repos"`                  // repo patterns (exact or trailing /*)
	AlwaysAllow []string `toml:"always_allow" yaml:"always_allow,omitempty"` // repos exempt from the hours check
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
	Event   string   `toml:"event"   yaml:"event"`           // "bash" or "write"; required, validated at load
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
	Enabled *bool `toml:"enabled"        yaml:"enabled"`
	// Mode downgrades a guard's denials to warn events when set to "soft";
	// "hard" (the default) blocks. Read by the guards in SoftModeGuards
	// only, and setting it on any other id is a config error rather than a
	// key that quietly does nothing.
	Mode          string   `toml:"mode"           yaml:"mode,omitempty"`
	ExcludePaths  []string `toml:"exclude_paths"  yaml:"exclude_paths,omitempty"`
	ExtraPatterns []string `toml:"extra_patterns" yaml:"extra_patterns,omitempty"`
	AllowRepos    []string `toml:"allow_repos"    yaml:"allow_repos,omitempty"`
	// ExcludeRepos opts repos out of a hint (same canonical host/owner/repo
	// patterns as AllowRepos). The two fields are kind-scoped: guards exempt
	// repos with allow_repos ("the guarded action is allowed there"), hints
	// opt them out with exclude_repos, and validate() rejects each on the
	// wrong kind — the panel review found exclude-on-a-deny-guard reads
	// inverted, so the split stays (docs/adr/0013).
	ExcludeRepos []string `toml:"exclude_repos"  yaml:"exclude_repos,omitempty"`
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
// guard: where to discover internal repos, extra always-blocked words, safe
// references to drop, and sanctioned compound phrases that may carry a
// blocked name. Deliberately shape-compatible with the guard: section of the
// suspenders config — the tools stay standalone and neither reads the
// other's file (docs/adr/0010), but one authored block can be rendered into
// both configs by the provisioning layer.
type InternalNames struct {
	WorkspaceDirs []string `toml:"workspace_dirs" yaml:"workspace_dirs"`
	BlockedWords  []string `toml:"blocked_words"  yaml:"blocked_words"`
	Allowlist     []string `toml:"allowlist"      yaml:"allowlist"`
	// AllowPhrases are exact phrases neutralized in content before name
	// matching, so a sanctioned compound that contains a blocked name (a
	// private companion repo's own name, say) passes while the bare name
	// anywhere else still denies. Unlike Allowlist, nothing leaves the
	// blocked set.
	AllowPhrases []string `toml:"allow_phrases"  yaml:"allow_phrases"`
}

// ClaudeSettings gates belt's read of the Claude Code settings files, the
// only non-belt config surface belt reads (docs/adr/0010). The only thing read is the permissions.deny Bash
// entries, which the script-deny-list guard enforces inside scripts so the
// guard and the permission system share one deny list.
type ClaudeSettings struct {
	Enabled *bool `toml:"enabled" yaml:"enabled"`
}

// ReadEnabled reports whether belt may read the Claude settings files.
// Defaults to true so script-deny-list keeps enforcing the deny list on
// machines that never wrote the key; enabled: false stops belt opening the
// Claude settings at all, leaving the guard with extra_patterns only.
func (c ClaudeSettings) ReadEnabled() bool { return c.Enabled == nil || *c.Enabled }

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

// RepoAllowed reports whether a guard's allow_repos list covers the
// canonical repo. Patterns match the same way as git_identity[].repos and
// commit_guards[].repos — exactly, or by trailing "/*" org wildcard — so one
// spelling of a repo pattern works everywhere in the file. An empty repo
// never matches.
func (c Config) RepoAllowed(guardID, repo string) bool {
	return RepoMatches(c.Guards[guardID].AllowRepos, repo)
}

// HintRepoExcluded reports whether the canonical repo is opted out of a
// hint by that hint's own exclude_repos list. Same patterns as RepoAllowed;
// an empty repo is never excluded. The shared direct_main_repos list is
// deliberately not consulted here — only commit-policy reads it, via
// DirectMain, because the workflow fact it states is about commits and
// pushes, not about the other hints.
func (c Config) HintRepoExcluded(hintID, repo string) bool {
	return RepoMatches(c.Hints[hintID].ExcludeRepos, repo)
}

// HintRepoExcludedTail is HintRepoExcluded for the one hint that knows a
// repo only as org/name (kof-assertions works on index-side search results
// with no filesystem path to resolve): the host segment of each canonical
// pattern is ignored and the tail matched, case-insensitively. Every hint
// that can reach the working tree resolves the canonical identity instead.
func (c Config) HintRepoExcludedTail(hintID, orgName string) bool {
	return RepoTailMatches(c.Hints[hintID].ExcludeRepos, orgName)
}

// DirectMain reports whether the canonical repo is on the top-level
// direct_main_repos list. Read by git-push-main and the commit-policy hint
// only (see the Config field comment and docs/adr/0013).
func (c Config) DirectMain(repo string) bool {
	return RepoMatches(c.DirectMainRepos, repo)
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

// RepoTailMatches matches the owner/name tail of canonical patterns against
// an org/name identity, case-insensitively (kof lowercases its identities).
// Dropping the host erases the public-vs-internal axis, so this is reserved
// for per-hint lists on hints with no path to resolve (kof-assertions) —
// never for guard exemption or the direct_main_repos list.
func RepoTailMatches(patterns []string, orgName string) bool {
	var tails []string
	for _, p := range patterns {
		if _, tail, ok := strings.Cut(p, "/"); ok && tail != "" {
			tails = append(tails, strings.ToLower(tail))
		}
	}
	return RepoMatches(tails, strings.ToLower(orgName))
}

// OverridesDir is where guard overrides live: one file per override name,
// managed by `belt override set|extend|clear`. The file's content is the
// RFC 3339 expiry the override runs until; an empty file is a legacy
// untimed override, active until cleared.
//
// Overrides always sit in belt's config directory, even when --config or
// BELT_CONFIG points the config file somewhere else: an override is machine
// state a person sets from the CLI, not part of the rendered config, and
// following a relocated file would hide the overrides already set.
func OverridesDir() string {
	dir, err := confdir.Dir(Component)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "overrides")
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
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			// Dotfiles are never overrides: `belt override set` refuses a
			// leading dot, so anything with one belongs to something else
			// (.DS_Store) and would otherwise list as a malformed override.
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
	ClaudeSettings []string // ~/.claude/settings.json + settings.local.json (claude_settings gate)
}

// DefaultPaths returns the standard location of every config surface,
// honoring XDG_CONFIG_HOME for belt's own files. The Claude settings stay
// under ~/.claude: they belong to Claude Code, which looks for them there.
func DefaultPaths() (Paths, error) {
	dir, err := confdir.Dir(Component)
	if err != nil {
		return Paths{}, fmt.Errorf("config: resolve config dir: %w", err)
	}
	home, err := confdir.Expand("~")
	if err != nil {
		return Paths{}, fmt.Errorf("config: resolve home dir: %w", err)
	}
	return Paths{
		BeltYAML: filepath.Join(dir, "config.yaml"),
		BeltTOML: filepath.Join(dir, "config.toml"),
		ClaudeSettings: []string{
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude", "settings.local.json"),
		},
	}, nil
}

// PathsFor returns the config surfaces with belt's own config file
// relocated: the flag value when given, else $BELT_CONFIG, else the default
// path. A relocated file is read as YAML with no legacy TOML fallback — the
// fallback exists for installs that still have the old file at the default
// location, not for a path someone chose today.
func PathsFor(flagValue string) (Paths, error) {
	p, err := DefaultPaths()
	if err != nil {
		return Paths{}, err
	}
	override := flagValue
	if override == "" {
		override = os.Getenv(EnvConfig)
	}
	if override == "" {
		return p, nil
	}
	expanded, err := confdir.Expand(override)
	if err != nil {
		return Paths{}, fmt.Errorf("config: %w", err)
	}
	p.BeltYAML, p.BeltTOML = expanded, ""
	return p, nil
}

// Load reads all config surfaces from their default locations.
func Load() (Config, error) {
	p, err := DefaultPaths()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(p)
}

// LoadFrom reads all config surfaces from the given locations. A missing
// belt config yields the defaults with a nil error; a present one that fails
// to parse or validate yields an error, and callers that keep going anyway
// (doctor, config) get the same defaults alongside it. The belt config owns
// every setting; the only non-belt surface read is the Claude settings deny
// lists, when the claude_settings gate allows it (docs/adr/0010).
func LoadFrom(p Paths) (Config, error) {
	f, src := ReadFile(p)
	cfg := Config{
		Guards:          f.Guards,
		Hints:           f.Hints,
		DirectMainRepos: f.DirectMainRepos,
		Names:           f.InternalNames,
		ClaudeSettings:  f.ClaudeSettings,
		GitIdentity:     f.GitIdentity,
		CommitGuards:    f.CommitGuards,
		CustomGuards:    f.CustomGuards,
	}
	if cfg.ClaudeSettings.ReadEnabled() {
		cfg.ClaudeDeny = LoadClaudeDenyPatterns(p.ClaudeSettings...)
	}
	if src.Broken() {
		return cfg, src.Err
	}
	return cfg, nil
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

// File is the parsed shape of the belt config file in either format. An
// absent internal_names section means an empty name set — belt reads no
// other tool's config to fill it (docs/adr/0010).
type File struct {
	Guards          map[string]Toggle      `toml:"guards"          yaml:"guards"`
	Hints           map[string]Toggle      `toml:"hints"           yaml:"hints"`
	DirectMainRepos []string               `toml:"direct_main_repos" yaml:"direct_main_repos"`
	InternalNames   InternalNames          `toml:"internal_names"  yaml:"internal_names"`
	ClaudeSettings  ClaudeSettings         `toml:"claude_settings" yaml:"claude_settings"`
	GitIdentity     []GitIdentity          `toml:"git_identity"    yaml:"git_identity"`
	CommitGuards    []CommitGuard          `toml:"commit_guards"   yaml:"commit_guards"`
	CustomGuards    map[string]CustomGuard `toml:"custom_guards"   yaml:"custom_guards"`
}

// Source reports which belt config file was read and what happened.
type Source struct {
	// Path is the file belt read, or the one it looked for when none exists.
	Path string
	// Legacy marks the deprecated TOML file as the one that loaded.
	Legacy bool
	// Err is nil when the file loaded and validated, an os.IsNotExist error
	// when no config file exists at all (the defaults case), and a parse or
	// validation failure otherwise.
	Err error
}

// Broken reports whether the config file exists but cannot be used. It is
// the state every guard entrypoint has to fail closed on: belt cannot tell
// an empty rule set from an unreadable one, and guessing "no rules" would
// turn a typo into a silently disarmed guard.
func (s Source) Broken() bool { return s.Err != nil && !os.IsNotExist(s.Err) }

// ReadFile reads the belt config file, preferring the YAML config and
// falling back to the legacy TOML file only when the YAML file does not
// exist. A present-but-broken file of either format is reported as an error
// rather than silently read from the other one: fail closed, not stale.
func ReadFile(p Paths) (File, Source) {
	f, err := LoadFileYAML(p.BeltYAML)
	switch {
	case err == nil:
		return validated(f, Source{Path: p.BeltYAML})
	case !os.IsNotExist(err):
		return File{}, Source{Path: p.BeltYAML, Err: err}
	case p.BeltTOML == "":
		return File{}, Source{Path: p.BeltYAML, Err: os.ErrNotExist}
	}
	f, err = LoadFileTOML(p.BeltTOML)
	switch {
	case err == nil:
		return validated(f, Source{Path: p.BeltTOML, Legacy: true})
	case os.IsNotExist(err):
		// Neither file exists: report the YAML path, the one to create.
		return File{}, Source{Path: p.BeltYAML, Err: os.ErrNotExist}
	default:
		return File{}, Source{Path: p.BeltTOML, Legacy: true, Err: err}
	}
}

// validated runs the file's own checks and drops the parsed values when they
// fail, so no caller can accidentally use a half-understood config.
func validated(f File, src Source) (File, Source) {
	if err := f.validate(); err != nil {
		return File{}, Source{Path: src.Path, Legacy: src.Legacy, Err: fmt.Errorf("config: %s: %w", src.Path, err)}
	}
	return f, src
}

// validate checks the values belt cannot interpret at use time. Every rule
// here covers a key that parses as valid YAML and then does nothing: a
// custom guard registered on a misspelled event never fires, and a mode:
// soft on a guard that ignores modes keeps blocking. Both look like a
// working config from the outside, which is exactly what a guard tool must
// not ship.
func (f File) validate() error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(f.CustomGuards)) {
		cg := f.CustomGuards[name]
		if cg.Event != EventBash && cg.Event != EventWrite {
			errs = append(errs, fmt.Errorf(
				"custom_guards.%s.event is %q (must be %q or %q)", name, cg.Event, EventBash, EventWrite))
		}
		if err := validMode(cg.Mode); err != nil {
			errs = append(errs, fmt.Errorf("custom_guards.%s.%w", name, err))
		}
	}
	errs = append(errs, toggleModeErrors("guards", f.Guards)...)
	errs = append(errs, toggleModeErrors("hints", f.Hints)...)
	errs = append(errs, toggleFieldErrors("guards", f.Guards, GuardFields)...)
	errs = append(errs, toggleFieldErrors("hints", f.Hints, HintFields)...)
	for i, r := range f.GitIdentity {
		if err := validMode(r.Mode); err != nil {
			errs = append(errs, fmt.Errorf("git_identity[%d].%w", i, err))
		}
	}
	for i, r := range f.CommitGuards {
		if err := validMode(r.Mode); err != nil {
			errs = append(errs, fmt.Errorf("commit_guards[%d].%w", i, err))
		}
	}
	return errors.Join(errs...)
}

// GuardFields and HintFields declare which optional Toggle keys each
// built-in id reads (`enabled` is universal, `mode` is owned by
// SoftModeGuards). validate() rejects a set key missing from the id's row —
// the key would parse and then do nothing. Ids absent from the tables stay
// lax on purpose: custom guard names are user-defined, and a rendered
// config may target a newer belt than this binary (config and binary ship
// from different repos), so rejecting keys this binary does not know would
// turn ordinary rollout skew into a machine-wide deny.
var (
	GuardFields = map[string][]string{
		"git-push-main":        {"allow_repos"},
		"git-identity":         {},
		"commit-guard":         {},
		"script-deny-list":     {"extra_patterns", "exclude_paths"},
		"write-internal-names": {"allow_repos", "exclude_paths"},
	}
	HintFields = map[string][]string{
		"agent-memory":    {},
		"commit-policy":   {"exclude_repos"},
		"kof-assertions":  {"exclude_repos"},
		"kof-consult":     {"exclude_repos"},
		"kof-deposit":     {},
		"lint-policy":     {"exclude_repos"},
		"prefer-csl":      {"exclude_repos"},
		"humanizer-check": {},
	}
)

// toggleFieldErrors reports toggles on known ids carrying a list key the id
// does not read, naming the keys it does read so the fix is in the message.
func toggleFieldErrors(kind string, toggles map[string]Toggle, schema map[string][]string) []error {
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(toggles)) {
		allowed, known := schema[id]
		if !known {
			continue
		}
		t := toggles[id]
		set := map[string]bool{
			"exclude_paths":  len(t.ExcludePaths) > 0,
			"extra_patterns": len(t.ExtraPatterns) > 0,
			"allow_repos":    len(t.AllowRepos) > 0,
			"exclude_repos":  len(t.ExcludeRepos) > 0,
		}
		for _, field := range []string{"exclude_paths", "extra_patterns", "allow_repos", "exclude_repos"} {
			if !set[field] || slices.Contains(allowed, field) {
				continue
			}
			reads := "only enabled"
			if len(allowed) > 0 {
				reads = strings.Join(allowed, ", ")
			}
			errs = append(errs, fmt.Errorf(
				"%s.%s: %s has no effect (%s reads: %s)", kind, id, field, id, reads))
		}
	}
	return errs
}

// toggleModeErrors reports guard and hint toggles carrying a mode the guard
// does not read. Only the guards in SoftModeGuards act on one; everywhere
// else the key is a belief about behavior that is not happening.
func toggleModeErrors(kind string, toggles map[string]Toggle) []error {
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(toggles)) {
		mode := toggles[id].Mode
		if err := validMode(mode); err != nil {
			errs = append(errs, fmt.Errorf("%s.%s.%w", kind, id, err))
			continue
		}
		if mode == ModeSoft && !slices.Contains(SoftModeGuards, id) {
			errs = append(errs, fmt.Errorf(
				"%s.%s: mode: soft has no effect (only %s reads a mode; use enabled: false to switch this one off)",
				kind, id, strings.Join(SoftModeGuards, ", ")))
		}
	}
	return errs
}

// validMode rejects anything but the two modes and the empty per-rule
// default. A typo'd mode reads as the default, which is the opposite of the
// intent half the time.
func validMode(mode string) error {
	if mode == "" || mode == ModeHard || mode == ModeSoft {
		return nil
	}
	return fmt.Errorf("mode is %q (must be %q or %q)", mode, ModeHard, ModeSoft)
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

// ExpandHome expands a leading ~ to the user's home directory, leaving the
// path untouched when the home directory cannot be resolved. Its callers
// match paths (exclude_paths prefixes, script locations) rather than open a
// config, and an unexpanded ~ simply fails to match — the guard stays armed.
// Belt cannot get this far with an unresolvable home anyway: config loading
// resolves one first and fails closed when it cannot.
func ExpandHome(path string) string {
	expanded, err := confdir.Expand(path)
	if err != nil {
		return path
	}
	return expanded
}
