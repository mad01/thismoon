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

# Machine profiles for profile-gated guards (git-push-main allows pushes to
# main on the personal profile). When unset, belt falls back to the profiles
# list in the ralph machine config (~/.config/ralph/config.local.toml).
profiles:
  - personal

# The internal-name list for write-internal-names, owned by belt. When this
# whole section is unset, belt falls back to the guard: section of the
# suspenders config (~/.config/suspenders/config.yaml); when set, it is the
# only source. Every repo found under workspace_dirs contributes its org
# name, repo name, and checkout dir name as separate blocked names.
internal_names:
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - internal-brand
  allowlist:
    - some-safe-name

guards:
  git-push-main:
    enabled: true
    # Exempt whole repos from this guard by canonical host/owner/repo,
    # matched against the push working dir's origin remote. This is how one
    # repo gets to push to its default branch while every other repo stays
    # fail-closed. Unknown profile or unresolved remote always denies.
    allow_repos:
      - github.com/you/yourrepo

  script-deny-list:
    enabled: true
    # Extra patterns denied inside scripts, beyond the Claude settings
    # permissions.deny Bash(...) entries (which are read live, never copied).
    extra_patterns:
      - rm -rf
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
    # section (or its suspenders fallback), not here.

hints:
  agent-memory:
    enabled: true
  prefer-csl:
    enabled: true
  keep-assertions:
    enabled: true
  keep-consult:
    enabled: true
  keep-deposit:
    enabled: true`

func configDocCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the current effective config",
		Long: `Show which config file belt loaded and every setting in effect after
defaults and fallbacks are applied — what this binary runs with, not what
the file happens to spell out.

belt reads ~/.config/belt/config.yaml; a legacy config.toml beside it is read
only when the YAML file is absent, and a present-but-broken file of either
format means defaults rather than a fall back to the other one. Profiles and
internal_names print with their resolved values whichever file supplied them;
the header names the winning source. claude_deny is the one section that is
not a config.yaml key: the Bash deny patterns are read live from the Claude
settings and shown here because the script-deny-list guard enforces them.

Every setting:

` + configReference + `

Pair it with doctor: doctor shows the state belt resolved, config shows which
file and key to change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			return runConfigDoc(cmd.OutOrStdout(), p)
		},
	}
}

// runConfigDoc prints where belt's config surfaces live and the settings in
// effect. A broken config file is reported in the header status and still
// prints the defaults belt fell back to: the command has to stay useful when
// the file it describes is the thing that is wrong.
func runConfigDoc(w io.Writer, p config.Paths) error {
	cfg := config.LoadFrom(p)
	path, legacy, loadErr := resolveBeltConfig(p)
	fmt.Fprintf(w, "config file:  %s (%s)\n", path, loadStatus(loadErr))
	if legacy {
		fmt.Fprintf(w, "              (legacy TOML format — rename it to %s)\n", p.BeltYAML)
	} else {
		fmt.Fprintf(w, "              (legacy fallback %s, read only when this file is absent)\n", p.BeltTOML)
	}
	fmt.Fprintf(
		w,
		"fallbacks:    %s  (%s — guard: section, used only when internal_names is unset here)\n",
		p.Suspenders,
		fallbackStatus(p.Suspenders, cfg.NamesSource == config.SourceSuspenders),
	)
	fmt.Fprintf(
		w,
		"              %s  (%s — profiles list, used only when profiles is unset here)\n",
		p.Ralph,
		fallbackStatus(p.Ralph, cfg.ProfileSource == config.SourceRalph),
	)
	fmt.Fprintf(w, "also read:    %s  (%s — permissions.deny Bash entries)\n",
		strings.Join(p.ClaudeSettings, " + "), pathStatus(p.ClaudeSettings...))
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
// ~/.config/belt/config.yaml with every default and fallback made explicit:
// profiles and internal_names carry the values belt resolved whichever file
// supplied them (the header names the source). ClaudeDeny is the one section
// that is not a config.yaml key — the Bash deny patterns read live from the
// Claude settings — included because the script-deny-list guard enforces
// them and no other command lists them.
type effectiveConfig struct {
	Profiles      []string                 `yaml:"profiles"`
	InternalNames config.InternalNames     `yaml:"internal_names"`
	Guards        map[string]config.Toggle `yaml:"guards"`
	Hints         map[string]config.Toggle `yaml:"hints"`
	ClaudeDeny    []string                 `yaml:"claude_deny"`
}

// resolveEffective materializes every registered guard and hint with the
// enabled state belt would actually use, so a missing config file prints the
// armed defaults instead of an empty document.
func resolveEffective(cfg config.Config) effectiveConfig {
	guards := guard.All(cfg)
	hints := hint.All(cfg)
	e := effectiveConfig{
		Profiles:      cfg.Profiles,
		InternalNames: cfg.Names,
		Guards:        make(map[string]config.Toggle, len(guards)),
		Hints:         make(map[string]config.Toggle, len(hints)),
		ClaudeDeny:    cfg.ClaudeDeny,
	}
	for _, g := range guards {
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

// resolveBeltConfig reports which belt config file is in effect and what
// happened reading it, mirroring the YAML-first, legacy-TOML-fallback order of
// config.LoadFrom: a nil error means the file loaded, an os.IsNotExist error
// means no config file exists at all, and anything else is a parse failure
// that leaves belt running on defaults.
func resolveBeltConfig(p config.Paths) (path string, legacy bool, err error) {
	if _, err := config.LoadFileYAML(p.BeltYAML); err == nil {
		return p.BeltYAML, false, nil
	} else if !os.IsNotExist(err) {
		return p.BeltYAML, false, err
	}
	_, err = config.LoadFileTOML(p.BeltTOML)
	switch {
	case err == nil:
		return p.BeltTOML, true, nil
	case os.IsNotExist(err):
		return p.BeltYAML, false, os.ErrNotExist
	default:
		return p.BeltTOML, true, err
	}
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

// fallbackStatus reports a fallback file's role in the resolved config, not
// just its presence: whether this run actually read it, and when the
// fallback was selected but the file is absent, that the setting is empty.
func fallbackStatus(path string, inUse bool) string {
	_, err := os.Stat(path)
	present := err == nil
	switch {
	case inUse && present:
		return "in use"
	case inUse:
		return "missing, fallback empty"
	case present:
		return "present, unused — set in belt config"
	default:
		return "missing, unused — set in belt config"
	}
}
