package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show the build and resolved config: surfaces loaded, guard/hint state, kof reachability, blocked names",
		Long: `Show what belt is actually running with: which build is installed, which
config surface loaded (or didn't), which guards and hints are enabled,
whether the kof serve instance the kof-* hints query is reachable, and
the resolved blocked-name set the write-internal-names guard matches
against. Use it to answer "why did that check fire" — a deny names the
guard, doctor names the build and the config behind it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			runDoctor(cmd.OutOrStdout(), p, liveKofProbe)
			return nil
		},
	}
}

// kofProbe reports kof serve reachability for the doctor report: the base URL
// the hints query, the stored assertion count, and the connection error when
// unreachable. Injected so tests never dial the real port.
type kofProbe func() (base string, count int, err error)

// liveKofProbe probes the same kof serve instance the kof-* hints query.
func liveKofProbe() (string, int, error) {
	base := hint.KofBaseURL()
	count, err := hint.KofProbe(base)
	return base, count, err
}

func runDoctor(w io.Writer, p config.Paths, probe kofProbe) {
	cfg := config.LoadFrom(p)

	printBuild(w)

	fmt.Fprintln(w, "config surfaces:")
	fmt.Fprintf(w, "  belt         %s\n", beltConfigLine(p))
	fmt.Fprintf(w, "  profiles     %s\n", profilesNote(cfg, p))
	fmt.Fprintf(w, "  names        %s\n", namesNote(cfg, p))
	fmt.Fprintf(w, "  claude deny  %s  %d bash deny patterns (belt config lists them)\n",
		strings.Join(p.ClaudeSettings, " + "), len(cfg.ClaudeDeny))
	for _, warn := range unknownToggleWarnings(cfg) {
		fmt.Fprintf(w, "  warning      %s\n", warn)
	}

	fmt.Fprintln(w, "\nguards:")
	var customs []*guard.Custom
	for _, g := range guard.All(cfg) {
		if c, ok := g.(*guard.Custom); ok {
			customs = append(customs, c)
			continue
		}
		fmt.Fprintf(
			w,
			"  %-22s %-7s %s%s%s\n",
			g.ID(),
			g.Event(),
			enabledWord(cfg.GuardEnabled(g.ID())),
			toggleNote(cfg.Guards[g.ID()]),
			ruleNote(cfg, g.ID()),
		)
	}

	printCustomGuards(w, cfg, customs)

	fmt.Fprintln(w, "\noverrides — rules naming one stop applying while it is active (belt override set|extend|clear <name>):")
	if overrides := config.Overrides(); len(overrides) > 0 {
		now := time.Now()
		for _, o := range overrides {
			fmt.Fprintf(w, "  %s  %s\n", o.Name, overrideStatus(o, now))
		}
	} else {
		fmt.Fprintln(w, "  (none set)")
	}

	fmt.Fprintln(w, "\nhints:")
	for _, h := range hint.All(cfg) {
		fmt.Fprintf(w, "  %-22s %-7s %s%s\n",
			h.ID(), h.Event(), enabledWord(cfg.HintEnabled(h.ID())), toggleNote(cfg.Hints[h.ID()]))
	}

	base, count, probeErr := probe()
	fmt.Fprintln(w, "\nkof serve — backs the kof-* hints:")
	fmt.Fprintf(w, "  %s  %s\n", base, kofNote(count, probeErr))

	names := guard.BlockedNames(cfg.Names)
	sort.Strings(names)
	fmt.Fprintf(
		w,
		"\nblocked names (%d) — write-internal-names denies these in github.com repos:\n",
		len(names),
	)
	if len(names) == 0 {
		fmt.Fprintln(w, "  (none — without an internal_names section in the belt config or a"+
			" suspenders guard config the guard allows every write)")
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

// profilesNote reports the resolved machine profiles and which config file
// supplied them: the belt config's own profiles list, or the ralph machine
// config it falls back to.
func profilesNote(cfg config.Config, p config.Paths) string {
	if len(cfg.Profiles) == 0 {
		return fmt.Sprintf(
			"none (belt config profiles unset, ralph fallback %s empty or missing) — git-push-main fails closed (denies every push to main)",
			p.Ralph,
		)
	}
	source := "belt config"
	if cfg.ProfileSource == config.SourceRalph {
		source = "ralph fallback " + p.Ralph
	}
	return fmt.Sprintf("%s  (from %s)", strings.Join(cfg.Profiles, ", "), source)
}

// namesNote reports where the internal-name list came from: the belt
// config's own internal_names section, or the suspenders guard config it
// falls back to.
func namesNote(cfg config.Config, p config.Paths) string {
	source := "belt config internal_names"
	if cfg.NamesSource == config.SourceSuspenders {
		source = "suspenders fallback " + p.Suspenders
		if _, err := os.Stat(p.Suspenders); err != nil {
			return fmt.Sprintf(
				"none (belt config internal_names unset, %s missing) — write-internal-names has no names to match",
				p.Suspenders,
			)
		}
	}
	return fmt.Sprintf("workspace dirs: %d, blocked words: %d, allowlist: %d  (from %s)",
		len(cfg.Names.WorkspaceDirs), len(cfg.Names.BlockedWords), len(cfg.Names.Allowlist),
		source)
}

// kofNote renders the kof reachability line. It exists to split the three
// states the hints render identically (as silence): kof down, store empty,
// and store populated.
func kofNote(count int, err error) string {
	switch {
	case err != nil:
		return fmt.Sprintf("UNREACHABLE (%v) — kof-* hints stay silent", err)
	case count == 0:
		return "reachable, 0 assertions stored — kof-* hints stay silent until something deposits (kof_assert)"
	default:
		return fmt.Sprintf("reachable, %d assertions stored", count)
	}
}

// ruleNote reports how many config rules feed the rule-driven guards — a
// rule-less git-identity or commit-guard is enabled yet checks nothing, and
// that must be visible.
func ruleNote(cfg config.Config, id string) string {
	switch id {
	case guard.GitIdentityID:
		return countNote("git_identity rules", len(cfg.GitIdentity))
	case guard.CommitGuardID:
		return countNote("commit_guards rules", len(cfg.CommitGuards))
	}
	return ""
}

func countNote(what string, n int) string {
	if n == 0 {
		return fmt.Sprintf("  (no %s — guard is a no-op)", what)
	}
	return fmt.Sprintf("  (%s: %d)", what, n)
}

// printCustomGuards renders the config-registered external guards with the
// one health fact belt can check for them: whether the command resolves on
// PATH.
func printCustomGuards(w io.Writer, cfg config.Config, customs []*guard.Custom) {
	fmt.Fprintln(w, "\ncustom guards — external commands from custom_guards in the belt config:")
	if len(customs) == 0 {
		fmt.Fprintln(w, "  (none configured)")
		return
	}
	for _, c := range customs {
		cg := c.Config()
		mode := "hard"
		if cg.Soft() {
			mode = "soft"
		}
		fmt.Fprintf(w, "  %-22s %-7s %-5s %-9s command: %s%s%s\n",
			c.ID(), cg.Event, mode, enabledWord(cfg.GuardEnabled(c.ID())),
			strings.Join(cg.Command, " "), matchNote(cg.Match), reachabilityNote(cg.Command))
	}
}

func matchNote(match string) string {
	if match == "" {
		return ""
	}
	return fmt.Sprintf("  match: %q", match)
}

// reachabilityNote flags a command PATH cannot resolve: the guard would warn
// and allow on every call, i.e. check nothing.
func reachabilityNote(command []string) string {
	if len(command) == 0 {
		return "  MISCONFIGURED (empty command — guard checks nothing)"
	}
	if _, err := exec.LookPath(command[0]); err != nil {
		return fmt.Sprintf("  UNREACHABLE (%q not on PATH — guard allows with a warn event)", command[0])
	}
	return ""
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
