package commands

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/internalnames"
	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/guard"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

func init() {
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor [path]",
	Short: "Explain the guard: the build, the config in effect, and the blocked names it derives",
	Long: `Show why the guard decides what it decides for a repo: which build is
installed, the global config, any per-repo overrides, whether the repo is
guard-exempt, and the full blocked-name set derived from the workspace dirs,
blocked words, and included names files. The list is derived fresh on every
run, exactly as a pre-commit check derives it; nothing here is persisted (see
why.md: a file enumerating the names would itself be the leak the guard
prevents). Defaults to the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	configFile, err := configFilePath()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	printBuild(out)
	fmt.Fprintf(out, "config: %s\n", configFile)

	// Like scan and hook run, a present-but-broken config is an error, not a
	// silent fallback to defaults: doctor's whole job is showing the truth.
	// The build and config path are already out by now, and the error names
	// the file at fault (the config itself, or a names file it includes).
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "scan:  %s\n", onOff(cfg.Scan.ScanEnabled()))
	fmt.Fprintf(out, "guard: %s\n", onOff(cfg.Guard.Enabled))
	fmt.Fprintf(out,
		"  workspace dirs: %d, blocked words: %d, safe references: %d, allow phrases: %d\n",
		len(cfg.Guard.WorkspaceDirs), len(cfg.Guard.BlockedWords),
		len(cfg.Guard.Allowlist), len(cfg.Guard.AllowPhrases))
	printIncludes(out, cfg.Guard.Include)

	printRepoStatus(out, root, cfg)

	names, err := guard.New(guardConfigFor(root, cfg)).CollectNames()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nblocked names (%d):\n", len(names))
	if len(names) == 0 {
		fmt.Fprintln(out, "  (none — the guard matches nothing)")
		return nil
	}
	for _, n := range names {
		fmt.Fprintf(out, "  %s\n", n)
	}
	return nil
}

// printBuild reports which build of suspenders is answering. It comes first
// because every line after it is what *this* binary derived — a doctor run
// against a stale binary is the failure mode the section exists to expose.
func printBuild(out io.Writer) {
	info := buildinfo.Get()
	fmt.Fprintln(out, "build:")
	fmt.Fprintf(out, "  %-12s %s\n", "version", info.Version)
	fmt.Fprintf(out, "  %-12s %s\n", "commit", orUnknown(info.Commit))
	fmt.Fprintf(out, "  %-12s %s\n", "tag", orUnknown(info.Tag))
	fmt.Fprintf(out, "  %-12s %s\n", "build time", orUnknown(info.BuildTime))
	fmt.Fprintln(out)
}

// orUnknown renders a build metadata field the build did not inject.
func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

// printIncludes reports each shared names file the config lists with the
// counts it contributed to the guard lists. The counts above already fold
// these in; this shows where they came from. Loading has already failed on a
// missing or broken include by the time doctor gets here, so a state other
// than loaded means the file changed under the running process; it is
// reported rather than assumed.
func printIncludes(out io.Writer, includes []string) {
	for _, p := range includes {
		names, err := internalnames.Read(config.ExpandPath(p))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			fmt.Fprintf(out, "  include: %s (MISSING)\n", p)
		case err != nil:
			fmt.Fprintf(out, "  include: %s (BROKEN: %v)\n", p, err)
		default:
			fmt.Fprintf(out,
				"  include: %s (loaded: blocked words %d, safe references %d, allow phrases %d)\n",
				p, len(names.BlockedWords), len(names.Allowlist), len(names.AllowPhrases))
		}
	}
}

// printRepoStatus reports the guard's view of one repo: per-repo overrides
// and whether the repo is exempt, with the reason — the two things that make
// the same content pass in one repo and block in another.
func printRepoStatus(out io.Writer, root string, cfg *config.Config) {
	fmt.Fprintf(out, "\nrepo: %s\n", root)

	if p := repoConfigPath(root); p != "" {
		if ic, err := scanner.LoadIgnoreConfig(p); err != nil {
			fmt.Fprintf(out, "  per-repo config: %s (unreadable: %v)\n", p, err)
		} else {
			fmt.Fprintf(out, "  per-repo config: %s (+%d safe references, +%d blocked words)\n",
				p, len(ic.Guard.Allowlist), len(ic.Guard.BlockedWords))
		}
	} else {
		fmt.Fprintln(out, "  per-repo config: none")
	}

	switch {
	case isInsideWorkspaceDirs(root, cfg.Guard.WorkspaceDirs):
		fmt.Fprintln(out, "  guard exempt: yes (inside a workspace dir — internal by definition)")
	case len(cfg.Exclude) > 0 && matchesRepo(repoNameFromPath(root), cfg.Exclude):
		fmt.Fprintln(out, "  guard exempt: yes (repo matches a top-level exclude pattern)")
	default:
		fmt.Fprintln(out, "  guard exempt: no")
	}
}

func onOff(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
