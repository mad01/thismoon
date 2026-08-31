package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// configReference documents every setting belt reads from its own config
// file. Shipped in the binary so an agent (or human) staring at a deny can go
// from `belt doctor` (what is the state) to `belt config --help` (which key
// changes it) without hunting for repo docs.
const configReference = `# ~/.config/belt/config.yaml — every key optional; guards and hints default
# to enabled when the file or their entry is missing (fail closed, not silent).
# Absent and invalid differ: no file means these defaults, while a file that
# fails to parse or validate makes every belt hook call deny until it is fixed.

# There is no machine-profile concept in this file (docs/adr/0010): the
# provisioning layer renders a per-machine-class config, so a guard or rule
# that should exist only on some machines simply is not present in the
# others' files. git-push-main, for example, ships enabled and is switched
# off in the rendered config of machines where direct pushes are fine.

# The internal-name list for write-internal-names, owned by belt and
# standalone: no other file is consulted, and an unset or empty section
# means an empty name set (the guard then has nothing to match). Every repo
# found under workspace_dirs contributes its org name, repo name, and
# checkout dir name as separate blocked names.
internal_names:
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - internal-brand
  allowlist:
    - some-safe-name

# The switch on belt reading the Claude settings files (~/.claude/
# settings.json + settings.local.json), the source of the permissions.deny
# Bash entries script-deny-list enforces inside scripts. Defaults to true;
# false stops belt opening those files, leaving the guard with
# extra_patterns only.
claude_settings:
  enabled: true

# Git identity enforcement for git commit, first matching rule wins. Rules
# are repo-scoped: the repo decides the expected email, whatever machine the
# commit happens on. repos entries are canonical host/owner/repo, exact or
# with a trailing /* org wildcard; an empty repos list covers every repo.
# mode hard (default) blocks a mismatched commit; soft warns via the events
# service and allows.
git_identity:
  - repos:
      - github.com/you/*
    email: you@personal.example
    mode: hard

# Work-hours commit guard: commits to matching repos inside the local-time
# window are blocked (hard) or warned about (soft, the default).
# always_allow exempts repos needed at any hour; block_days defaults to
# mon-fri; override names a timed switch (belt override set <name> --reason
# "...", 10m default, --for to size it) that suppresses the rule with a
# warn event until it expires or is cleared.
commit_guards:
  - repos:
      - github.com/you/*
    always_allow:
      - github.com/you/essential-tooling
    block_hours: "09:00-17:00"
    block_days: [mon, tue, wed, thu, fri]
    mode: soft
    override: vacation

# Named external guards: belt execs the command with the tool-call fields as
# JSON on stdin ({"command","cwd"} for bash, {"file_path","content","cwd"}
# for write). Exit 0 allows; exit 1 denies with stdout line one as the
# reason (soft mode downgrades it to a warn event); exit 2+, a timeout (5s),
# or a start failure allow with a warn event — a broken external fails open.
# match gates when the external is exec'd at all: a substring of the bash
# command (bash event) or target file path (write event). event is required
# and must be bash or write — a misspelled one is a config error, since the
# guard would register on an event that never fires.
custom_guards:
  check-branch-naming:
    enabled: true
    event: bash
    command: [branch-lint, check]
    match: git commit
    mode: hard

guards:
  git-push-main:
    enabled: true
    # Exempt whole repos from this guard by canonical host/owner/repo (or a
    # trailing /* org wildcard), matched against the push working dir's
    # origin remote. This is how one repo gets to push to its default branch
    # while every other repo stays fail-closed. An unresolved remote always
    # denies.
    allow_repos:
      - github.com/you/yourrepo

  script-deny-list:
    enabled: true
    # "soft" downgrades every denial to a warn event on the events service
    # and lets the command proceed — the rollout setting for tuning new
    # patterns before they block. "hard" (the default) blocks. This is the
    # only guard that reads a mode here; setting it elsewhere is an error.
    mode: hard
    # Extra patterns denied inside scripts, beyond the Claude settings
    # permissions.deny Bash(...) entries (which are read live, never copied).
    # A re: prefix compiles the rest as a case-insensitive regex, for flag
    # reordering and wildcards the literal form cannot express.
    extra_patterns:
      - rm -rf
      - "re:git\\s+push\\s.*--force"
    # Skip trusted script locations. A ~/ or absolute entry is matched as a
    # directory prefix; anything else as a substring of the script path.
    exclude_paths:
      - ~/trusted/scripts

  write-internal-names:
    enabled: true
    # Repos allowed to carry internal names despite a github.com remote
    # (canonical host/owner/repo, matched against the target file's origin
    # remote) — a private companion repo whose purpose is internal config.
    allow_repos:
      - github.com/you/private-companion
    # Paths where internal references are deliberate. A ~/ or absolute entry
    # is matched as a directory prefix; anything else as a substring of the
    # target file path.
    exclude_paths:
      - ~/notes
    # The blocked-name list itself lives in the top-level internal_names
    # section, not here.

hints:
  agent-memory:
    enabled: true
  prefer-csl:
    enabled: true
  commit-policy:
    enabled: true
    # Repos opted out because committing straight to main is their norm;
    # everywhere else a commit on main/master draws the branch + PR advice.
    # Same patterns as guards.git-push-main.allow_repos — keep the two
    # lists in step. (allow_repos under hints: is a validation error; the
    # exempt-vs-opt-out key is scoped per kind.)
    exclude_repos:
      - github.com/you/dotfiles
  kof-assertions:
    enabled: true
  kof-consult:
    enabled: true
  kof-deposit:
    enabled: true
  humanizer-check:
    enabled: true`

func configDocCmd(paths pathsFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the current effective config",
		Long: `Show which config file belt loaded and every setting in effect after
defaults and fallbacks are applied — what this binary runs with, not what
the file happens to spell out.

belt reads ~/.config/belt/config.yaml (or $XDG_CONFIG_HOME/belt/config.yaml);
--config and $BELT_CONFIG relocate it. A legacy config.toml beside the default
file is read only when the YAML file is absent. A present-but-broken file of
either format does not fall back to the other one and does not fall back to
the defaults either: the hooks deny every guarded tool call until it parses,
and this command prints the defaults only so the reference stays readable
while the file is broken. Every value prints resolved, defaults included.
claude_deny is the one section that is not a config.yaml key: the Bash deny
patterns are read live from the Claude settings and shown here because the
script-deny-list guard enforces them.

Every setting:

` + configReference + `

Pair it with doctor: doctor shows the state belt resolved, config shows which
file and key to change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := paths()
			if err != nil {
				return err
			}
			return runConfigDoc(cmd.OutOrStdout(), p)
		},
	}
}

// runConfigDoc prints where belt's config surfaces live and the settings in
// effect. A broken config file is reported in the header status and still
// prints the built-in defaults: the command has to stay useful when the file
// it describes is the thing that is wrong. Those defaults are not what belt
// is enforcing in that state — the hooks are denying everything — so the
// header says so.
func runConfigDoc(w io.Writer, p config.Paths) error {
	cfg, _ := config.LoadFrom(p)
	_, src := config.ReadFile(p)
	fmt.Fprintf(w, "config file:  %s (%s)\n", src.Path, loadStatus(src.Err))
	switch {
	case src.Legacy:
		fmt.Fprintf(w, "              (legacy TOML format — rename it to %s)\n", p.BeltYAML)
	case p.BeltTOML != "":
		fmt.Fprintf(w, "              (legacy fallback %s, read only when this file is absent)\n", p.BeltTOML)
	default:
		fmt.Fprintln(w, "              (relocated by --config or $"+config.EnvConfig+
			"; no legacy TOML fallback applies)")
	}
	if src.Broken() {
		fmt.Fprintln(w, "              belt hook DENIES every guarded tool call until this parses;"+
			" the values below are the defaults, not what is being enforced")
	}
	if cfg.ClaudeSettings.ReadEnabled() {
		fmt.Fprintf(w, "also read:    %s  (%s — permissions.deny Bash entries)\n",
			strings.Join(p.ClaudeSettings, " + "), pathStatus(p.ClaudeSettings...))
	} else {
		fmt.Fprintln(w, "also read:    nothing — claude_settings.enabled: false skips the Claude settings files")
	}
	for _, warn := range unknownToggleWarnings(cfg) {
		fmt.Fprintf(w, "warning:      %s\n", warn)
	}

	fmt.Fprintln(w)
	return encodeYAML(w, resolveEffective(cfg))
}

// encodeYAML writes v at the 2-space indent the config files themselves use,
// so the output can go straight back into one.
func encodeYAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode effective config: %w", err)
	}
	return enc.Close()
}

// effectiveConfig is the full resolved config in the shape of
// ~/.config/belt/config.yaml with every default made explicit:
// claude_settings prints its resolved enabled state. ClaudeDeny is the one
// section that is not a config.yaml key — the Bash deny patterns read live
// from the Claude settings — included because the script-deny-list guard
// enforces them and no other command lists them.
type effectiveConfig struct {
	InternalNames  config.InternalNames          `yaml:"internal_names"`
	ClaudeSettings config.ClaudeSettings         `yaml:"claude_settings"`
	GitIdentity    []config.GitIdentity          `yaml:"git_identity,omitempty"`
	CommitGuards   []config.CommitGuard          `yaml:"commit_guards,omitempty"`
	CustomGuards   map[string]config.CustomGuard `yaml:"custom_guards,omitempty"`
	Guards         map[string]config.Toggle      `yaml:"guards"`
	Hints          map[string]config.Toggle      `yaml:"hints"`
	ClaudeDeny     []string                      `yaml:"claude_deny"`
}

// resolveEffective materializes every registered guard and hint with the
// enabled state belt would actually use, so a missing config file prints the
// armed defaults instead of an empty document.
func resolveEffective(cfg config.Config) effectiveConfig {
	guards := guard.All(cfg)
	hints := hint.All(cfg)
	claudeRead := cfg.ClaudeSettings.ReadEnabled()
	e := effectiveConfig{
		InternalNames:  cfg.Names,
		ClaudeSettings: config.ClaudeSettings{Enabled: &claudeRead},
		GitIdentity:    cfg.GitIdentity,
		CommitGuards:   cfg.CommitGuards,
		CustomGuards:   cfg.CustomGuards,
		Guards:         make(map[string]config.Toggle, len(guards)),
		Hints:          make(map[string]config.Toggle, len(hints)),
		ClaudeDeny:     cfg.ClaudeDeny,
	}
	for _, g := range guards {
		if _, ok := g.(*guard.Custom); ok {
			continue // rendered under custom_guards with their full definition
		}
		e.Guards[g.ID()] = withEnabled(cfg.Guards[g.ID()], cfg.GuardEnabled(g.ID()))
	}
	for _, h := range hints {
		e.Hints[h.ID()] = withEnabled(cfg.Hints[h.ID()], cfg.HintEnabled(h.ID()))
	}
	return e
}

// unknownToggleWarnings lists guard and hint keys the config file sets that
// no registered guard or hint answers to. A typo'd id configures nothing,
// and resolveEffective keys off the registry — without the warning the entry
// would vanish from the output with the guard still armed.
func unknownToggleWarnings(cfg config.Config) []string {
	warns := unknownKeys(
		"guard",
		cfg.Guards,
		guard.All(cfg),
		func(g guard.Guard) string { return g.ID() },
	)
	warns = append(
		warns,
		unknownKeys(
			"hint",
			cfg.Hints,
			hint.All(cfg),
			func(h hint.Hint) string { return h.ID() },
		)...,
	)
	return warns
}

// unknownKeys reports the toggle keys with no registered counterpart, sorted
// so the warnings print in a stable order.
func unknownKeys[T any](
	kind string,
	toggles map[string]config.Toggle,
	registered []T,
	id func(T) string,
) []string {
	known := make(map[string]bool, len(registered))
	for _, r := range registered {
		known[id(r)] = true
	}
	var warns []string
	for key := range toggles {
		if !known[key] {
			warns = append(
				warns,
				fmt.Sprintf(
					"unknown %s %q in config file — no registered %s answers to it",
					kind,
					key,
					kind,
				),
			)
		}
	}
	sort.Strings(warns)
	return warns
}

// withEnabled pins a toggle's implicit default to an explicit value, so an
// omitted `enabled` prints as the true/false belt resolved it to.
func withEnabled(t config.Toggle, on bool) config.Toggle {
	t.Enabled = &on
	return t
}

// loadStatus renders what happened when the config file was read, in the
// vocabulary every `<tool> config` header uses.
func loadStatus(err error) string {
	switch {
	case err == nil:
		return "loaded"
	case os.IsNotExist(err):
		return "missing, defaults in use"
	default:
		return "parse error: " + err.Error()
	}
}

// pathStatus reports whether a surface belt only reads is there. Presence is
// all this check performs, so "present" is all it claims.
func pathStatus(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return "present"
		}
	}
	return "missing"
}
