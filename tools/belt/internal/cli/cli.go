// Package cli wires the belt commands: `hook <event>` (the Claude Code
// PreToolUse entrypoint), `check` (manual dry-run), and `version`.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hook"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Execute runs the root command.
func Execute() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "belt",
		Short:         "Claude Code PreToolUse guard hooks (pairs with suspenders)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(hookCmd(), checkCmd(), versionCmd())
	return root
}

func hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "hook <event>",
		Short:     "Run as a Claude Code PreToolUse hook (payload on stdin, deny JSON on stdout)",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{guard.EventBash, guard.EventWrite},
		RunE: func(cmd *cobra.Command, args []string) error {
			hook.Run(args[0], cmd.InOrStdin(), cmd.OutOrStdout())
			return nil // always exit 0: the deny travels in the JSON
		},
	}
}

func checkCmd() *cobra.Command {
	var cwd, file, content string
	cmd := &cobra.Command{
		Use:   "check <event> [command]",
		Short: "Dry-run the guards against a command or write and print each verdict",
		Long: "Examples:\n" +
			"  belt check bash \"git push origin main\"\n" +
			"  belt check write --file ~/code/src/github.com/mad01/thismoon/README.md --content \"some text\"",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := guard.Input{Event: args[0], Cwd: cwd, FilePath: file, Content: content}
			if len(args) == 2 {
				in.Command = args[1]
			}
			cfg := config.Load()
			guards := guard.ForEvent(in.Event, cfg)
			if len(guards) == 0 {
				return fmt.Errorf("no guards for event %q (valid: %s, %s)", in.Event, guard.EventBash, guard.EventWrite)
			}
			denied := false
			for _, g := range guards {
				if d := g.Check(in); d != nil {
					denied = true
					fmt.Fprintf(cmd.OutOrStdout(), "DENY  %-22s %s\n", g.ID(), d.Reason)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "allow %-22s\n", g.ID())
				}
			}
			if denied {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "working directory for bare git push resolution (default: current dir)")
	cmd.Flags().StringVar(&file, "file", "", "target file path (write event)")
	cmd.Flags().StringVar(&content, "content", "", "content to scan (write event)")
	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), Version)
		},
	}
}
