package cli

import (
	"fmt"
	"io"
	"os"
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
    # Skip trusted script locations (substring match on the script path).
    exclude_paths:
      - /trusted/scripts/

  write-internal-names:
    enabled: true
    # Repos allowed to carry internal names despite a github.com remote
    # (canonical host/owner/repo, matched against the target file's origin
    # remote) — a private companion repo whose purpose is internal config.
    allow_repos:
      - github.com/you/private-companion
    # Paths where internal references are deliberate (substring match on the
    # target file path).
    exclude_paths:
      - /notes/
    # The blocked-name list itself is NOT configured here: it comes from the
    # guard: section of the suspenders config, shared with the pre-commit
    # guard. See suspenders config.

hints:
  prefer-csl:
    enabled: true
  keep-assertions:
    enabled: true`

func configDocCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the current effective config",
		Long: `Show which config file belt loaded and the guard and hint settings in effect
after defaults are applied — what this binary runs with, not what the file
happens to spell out.

belt reads ~/.config/belt/config.yaml; a legacy config.toml beside it is read
only when the YAML file is absent, and a present-but-broken file of either
format means defaults rather than a fall back to the other one. The surfaces
belt shares with other tools (ralph, suspenders, the Claude settings) are
listed as paths only — doctor is where their resolved state lives.

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
	path, legacy, loadErr := resolveBeltConfig(p)
	fmt.Fprintf(w, "config file:  %s (%s)\n", path, loadStatus(loadErr))
	if legacy {
		fmt.Fprintf(w, "              (legacy TOML format — rename it to %s)\n", p.BeltYAML)
	} else {
		fmt.Fprintf(w, "              (legacy fallback %s, read only when this file is absent)\n", p.BeltTOML)
	}
	fmt.Fprintf(w, "also read:    %s  (%s — guard: section = the blocked-name source)\n",
		p.Suspenders, pathStatus(p.Suspenders))
	fmt.Fprintf(w, "              %s  (%s — profiles list = machine profile)\n",
		p.Ralph, pathStatus(p.Ralph))
	fmt.Fprintf(w, "              %s  (%s — permissions.deny Bash entries)\n",
		strings.Join(p.ClaudeSettings, " + "), pathStatus(p.ClaudeSettings...))

	fmt.Fprintln(w)
	return encodeYAML(w, resolveEffective(config.LoadFrom(p)))
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

// effectiveConfig is the belt-owned half of the resolved config: the shape of
// ~/.config/belt/config.yaml with every default made explicit. The surfaces
// belt only reads (ralph, suspenders, the Claude settings) stay out of it —
// they are other tools' config, and `belt doctor` already reports what they
// resolved to.
type effectiveConfig struct {
	Guards map[string]config.Toggle `yaml:"guards"`
	Hints  map[string]config.Toggle `yaml:"hints"`
}

// resolveEffective materializes every registered guard and hint with the
// enabled state belt would actually use, so a missing config file prints the
// armed defaults instead of an empty document.
func resolveEffective(cfg config.Config) effectiveConfig {
	guards := guard.All(cfg)
	hints := hint.All(cfg)
	e := effectiveConfig{
		Guards: make(map[string]config.Toggle, len(guards)),
		Hints:  make(map[string]config.Toggle, len(hints)),
	}
	for _, g := range guards {
		e.Guards[g.ID()] = withEnabled(cfg.Guards[g.ID()], cfg.GuardEnabled(g.ID()))
	}
	for _, h := range hints {
		e.Hints[h.ID()] = withEnabled(cfg.Hints[h.ID()], cfg.HintEnabled(h.ID()))
	}
	return e
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
	if _, _, err := config.LoadTogglesYAML(p.BeltYAML); err == nil {
		return p.BeltYAML, false, nil
	} else if !os.IsNotExist(err) {
		return p.BeltYAML, false, err
	}
	_, _, err = config.LoadTogglesTOML(p.BeltTOML)
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
// all this command promises for those files; doctor reports what each one
// resolved to.
func pathStatus(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return "loaded"
		}
	}
	return "missing"
}
