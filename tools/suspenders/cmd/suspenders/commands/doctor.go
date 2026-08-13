package commands

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/guard"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

func init() {
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor [path]",
	Short: "Explain the guard: the config in effect and the blocked names it derives",
	Long: `Show why the guard decides what it decides for a repo: the global config,
any per-repo overrides, whether the repo is guard-exempt, and the full
blocked-name set derived from the workspace dirs and blocked words. The list
is derived fresh on every run, exactly as a pre-commit check derives it —
nothing here is persisted (see why.md: a file enumerating the names would
itself be the leak the guard prevents). Defaults to the current directory.`,
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

	// Like scan and hook run, a present-but-broken config is an error, not a
	// silent fallback to defaults — doctor's whole job is showing the truth.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "config: %s\n", config.Path())
	fmt.Fprintf(out, "scan:  %s\n", onOff(cfg.Scan.ScanEnabled()))
	fmt.Fprintf(out, "guard: %s\n", onOff(cfg.Guard.Enabled))
	fmt.Fprintf(out, "  workspace dirs: %d, blocked words: %d, safe references: %d\n",
		len(cfg.Guard.WorkspaceDirs), len(cfg.Guard.BlockedWords), len(cfg.Guard.Allowlist))

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
