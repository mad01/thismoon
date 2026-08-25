// Package cli wires the belt commands: `hook <event>` (the Claude Code
// PreToolUse guard entrypoint), `hint <event>` (the PostToolUse advisory
// entrypoint), `check` (manual dry-run), `doctor` (resolved-config report),
// `config` (config locations + annotated setting reference), `docs` (the
// embedded operating doc), and `version`.
package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	belt "github.com/mad01/thismoon/tools/belt"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
	"github.com/mad01/thismoon/tools/belt/internal/hook"
)

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
		Short:         "Claude Code guard and hint hooks (pairs with suspenders)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(hookCmd(), hintCmd(), checkCmd(), doctorCmd(), configDocCmd(), overrideCmd(),
		agentcli.DocsCommand(belt.OperatingDoc, belt.Facts()), versionCmd())
	return root
}

func hintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hint <event>",
		Short: "Run as a Claude Code PostToolUse hint hook (payload on stdin, advice JSON on stdout)",
		Long: `Run as a Claude Code PostToolUse hook. Reads the tool-call payload on stdin
and writes any advice as hookSpecificOutput.additionalContext on stdout. A
hint never blocks: the tool has already run and its result stands, so silence
and advice are the only two outcomes.

The event argument is belt's hint event, not the Claude Code tool name:
search hints on the csl search tools, bash hints on Bash commands,
external-text hints on the MCP tools that publish text off this machine,
session-start hints once when a session opens (a SessionStart hook), and
prompt hints on each user prompt (a UserPromptSubmit hook — advice goes to
stdout as plain text, the form that event adds to context). Wire them in
~/.claude/settings.json:

  {
    "hooks": {
      "PostToolUse": [
        {"matcher": "mcp__csl__csl_(search|semantic_search|hybrid_search)",
         "hooks": [{"type": "command", "command": "belt hint search"}]},
        {"matcher": "Bash", "hooks": [{"type": "command", "command": "belt hint bash"}]},
        {"matcher": "mcp__(github|slack)__.*",
         "hooks": [{"type": "command", "command": "belt hint external-text"}]}
      ],
      "SessionStart": [
        {"matcher": "", "hooks": [{"type": "command", "command": "belt hint session-start"}]}
      ],
      "UserPromptSubmit": [
        {"matcher": "", "hooks": [{"type": "command", "command": "belt hint prompt"}]}
      ]
    }
  }`,
		Args: validHintEventArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			hook.RunHint(strings.ToLower(args[0]), cmd.InOrStdin(), cmd.OutOrStdout())
			return nil // always exit 0: advice travels in the JSON, not the exit code
		},
	}
}

// validHintEventArg rejects unknown hint events at parse time. A miswired
// entry would run zero hints and silently advise nothing, which looks
// identical to "there was nothing to say" — so it must fail loud instead.
func validHintEventArg(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	if slices.Contains(hint.Events(), strings.ToLower(args[0])) {
		return nil
	}
	return fmt.Errorf("unknown hint event %q (valid: %s)", args[0], strings.Join(hint.Events(), ", "))
}

func hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook <event>",
		Short: "Run as a Claude Code PreToolUse hook (payload on stdin, deny JSON on stdout)",
		Long: `Run as a Claude Code PreToolUse hook. Reads the tool-call payload on stdin
and writes any deny decision as JSON on stdout; a valid event always exits 0
(the deny travels in the JSON, not the exit code).

The event argument is belt's guard event, not the Claude Code tool name:
bash guards Bash commands, write guards Write and Edit. Wire both in
~/.claude/settings.json:

  {
    "hooks": {
      "PreToolUse": [
        {"matcher": "Bash", "hooks": [{"type": "command", "command": "belt hook bash"}]},
        {"matcher": "Write|Edit", "hooks": [{"type": "command", "command": "belt hook write"}]}
      ]
    }
  }`,
		Args: validEventArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			hook.Run(strings.ToLower(args[0]), cmd.InOrStdin(), cmd.OutOrStdout())
			return nil // always exit 0 for a valid event: the deny travels in the JSON
		},
	}
}

// validEventArg rejects unknown events at parse time, case-insensitively. An
// unmatched event would run zero guards and silently allow everything the
// hook was wired to deny, so a miswired settings entry must fail loud.
func validEventArg(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	switch strings.ToLower(args[0]) {
	case guard.EventBash, guard.EventWrite:
		return nil
	default:
		return fmt.Errorf("unknown event %q (valid: %s, %s)", args[0], guard.EventBash, guard.EventWrite)
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
	var output string
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the belt version",
		Long: `Print the belt version (the git commit it was built from).

Plain output is the bare version token — the cross-tool convention sibling
tools follow so ralph and status can probe any of them for the build they are
running. With -o json, prints the full build metadata object: version, commit,
tag, build_time, with every key present and "" for anything unknown.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Get()
			if output == "json" {
				fmt.Fprint(cmd.OutOrStdout(), info.PrettyJSON())
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), info.Version)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "Output format: text or json")
	return cmd
}
