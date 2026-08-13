package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show the build and resolved config: surfaces loaded, guard/hint state, blocked names",
		Long: `Show what belt is actually running with: which build is installed, which
config surface loaded (or didn't), which guards and hints are enabled, and
the resolved blocked-name set the write-internal-names guard matches
against. Use it to answer "why did that check fire" — a deny names the
guard, doctor names the build and the config behind it.`,
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

	printBuild(w)

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

// printBuild reports which build of belt is answering. It comes first because
// every line after it describes what *this* binary resolved — a doctor run
// against a stale binary is the failure mode the section exists to expose.
func printBuild(w io.Writer) {
	info := buildinfo.Get()
	fmt.Fprintln(w, "build:")
	fmt.Fprintf(w, "  %-12s %s\n", "version", info.Version)
	fmt.Fprintf(w, "  %-12s %s\n", "commit", orUnknown(info.Commit))
	fmt.Fprintf(w, "  %-12s %s\n", "tag", orUnknown(info.Tag))
	fmt.Fprintf(w, "  %-12s %s\n", "build time", orUnknown(info.BuildTime))
	fmt.Fprintln(w)
}

// orUnknown renders a build metadata field the build did not inject.
func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

// beltConfigLine reports which belt config file is in effect, spelling out
// what `belt config` puts in its header status.
func beltConfigLine(p config.Paths) string {
	path, legacy, err := resolveBeltConfig(p)
	switch {
	case err == nil && legacy:
		return path + "  loaded (legacy TOML — rename to config.yaml)"
	case err == nil:
		return path + "  loaded"
	case os.IsNotExist(err):
		return path + "  missing — defaults, everything enabled"
	default:
		return fmt.Sprintf(
			"%s  PARSE ERROR (%v) — running with defaults, everything enabled",
			path,
			err,
		)
	}
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
