package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show the resolved config: surfaces loaded, guard/hint state, blocked names",
		Long: `Show what belt is actually running with: which config surface loaded (or
didn't), which guards and hints are enabled, and the resolved blocked-name
set the write-internal-names guard matches against. Use it to answer "why
did that check fire" — a deny names the guard, doctor names the config
behind it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			runDoctor(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

func runDoctor(w io.Writer, p config.Paths) {
	cfg := config.LoadFrom(p)

	fmt.Fprintln(w, "config surfaces:")
	fmt.Fprintf(w, "  belt         %s\n", beltConfigLine(p))
	fmt.Fprintf(w, "  ralph        %s  %s\n", p.Ralph, profilesNote(cfg.Profiles))
	fmt.Fprintf(w, "  suspenders   %s  %s\n", p.Suspenders, suspendersNote(p.Suspenders, cfg.Suspenders))
	fmt.Fprintf(w, "  claude deny  %s  %d bash deny patterns\n",
		strings.Join(p.ClaudeSettings, " + "), len(cfg.ClaudeDeny))

	fmt.Fprintln(w, "\nguards:")
	for _, g := range guard.All(cfg) {
		fmt.Fprintf(w, "  %-22s %-7s %s%s\n",
			g.ID(), g.Event(), enabledWord(cfg.GuardEnabled(g.ID())), toggleNote(cfg.Guards[g.ID()]))
	}

	fmt.Fprintln(w, "\nhints:")
	for _, h := range hint.All(cfg) {
		fmt.Fprintf(w, "  %-22s %-7s %s%s\n",
			h.ID(), h.Event(), enabledWord(cfg.HintEnabled(h.ID())), toggleNote(cfg.Hints[h.ID()]))
	}

	names := guard.BlockedNames(cfg.Suspenders)
	sort.Strings(names)
	fmt.Fprintf(w, "\nblocked names (%d) — write-internal-names denies these in github.com repos:\n",
		len(names))
	if len(names) == 0 {
		fmt.Fprintln(w, "  (none — without a suspenders guard config the guard allows every write)")
		return
	}
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

// beltConfigLine reports which belt config file is in effect, mirroring the
// YAML-first, legacy-TOML-fallback order of config.LoadFrom.
func beltConfigLine(p config.Paths) string {
	if _, _, err := config.LoadTogglesYAML(p.BeltYAML); err == nil {
		return p.BeltYAML + "  loaded"
	} else if !os.IsNotExist(err) {
		return fmt.Sprintf("%s  PARSE ERROR (%v) — running with defaults, everything enabled", p.BeltYAML, err)
	}
	if _, _, err := config.LoadTogglesTOML(p.BeltTOML); err == nil {
		return p.BeltTOML + "  loaded (legacy TOML — rename to config.yaml)"
	} else if !os.IsNotExist(err) {
		return fmt.Sprintf("%s  PARSE ERROR (%v) — running with defaults, everything enabled", p.BeltTOML, err)
	}
	return p.BeltYAML + "  missing — defaults, everything enabled"
}

func profilesNote(profiles []string) string {
	if len(profiles) == 0 {
		return "no profiles — git-push-main fails closed (denies every push to main)"
	}
	return "profiles: " + strings.Join(profiles, ", ")
}

func suspendersNote(path string, s config.SuspendersGuard) string {
	if _, err := os.Stat(path); err != nil {
		return "missing — write-internal-names has no names to match"
	}
	return fmt.Sprintf("workspace dirs: %d, blocked words: %d, safe references: %d",
		len(s.WorkspaceDirs), len(s.BlockedWords), len(s.Allowlist))
}

func enabledWord(on bool) string {
	if on {
		return "enabled"
	}
	return "DISABLED"
}

// toggleNote summarizes the non-empty knobs of a toggle, so doctor shows at a
// glance which guard carries an allowlist or exclusions.
func toggleNote(t config.Toggle) string {
	var parts []string
	if n := len(t.AllowRepos); n > 0 {
		parts = append(parts, fmt.Sprintf("allow_repos: %d", n))
	}
	if n := len(t.ExcludePaths); n > 0 {
		parts = append(parts, fmt.Sprintf("exclude_paths: %d", n))
	}
	if n := len(t.ExtraPatterns); n > 0 {
		parts = append(parts, fmt.Sprintf("extra_patterns: %d", n))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}
