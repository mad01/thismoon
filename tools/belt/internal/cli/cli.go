// Package cli wires the belt commands: `hook <event>` (the Claude Code
// PreToolUse guard entrypoint), `hint <event>` (the PostToolUse advisory
// entrypoint), `record <event>` (the lifecycle ledger entrypoint), `watch`
// (the asyncRewake inbox watcher), `check` (manual dry-run), `doctor`
// (resolved-config report), `config` (config locations + annotated setting
// reference), `docs` (the embedded operating doc), and `version`.
package cli

import (
	"encoding/json"
	"errors"
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
	"github.com/mad01/thismoon/tools/belt/internal/ledger"
)

// Execute runs the root command. A watcher that delivered items exits with
// the wake code and prints nothing more; any other error prints once and
// exits 1.
func Execute() {
	err := rootCmd().Execute()
	switch {
	case err == nil:
	case errors.Is(err, errWake):
		os.Exit(wakeExitCode)
	default:
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:           "belt",
		Short:         "Claude Code guard and hint hooks (pairs with suspenders)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "",
		"belt config file (default ~/.config/belt/config.yaml; overrides $"+config.EnvConfig+")")
	paths := func() (config.Paths, error) { return config.PathsFor(configPath) }
	root.AddCommand(
		hookCmd(paths),
		hintCmd(paths),
		recordCmd(paths),
		watchCmd(paths),
		checkCmd(paths),
		doctorCmd(paths),
		configDocCmd(paths),
		overrideCmd(),
		agentcli.DocsCommand(belt.OperatingDoc, belt.Facts()),
		versionCmd(),
	)
	return root
}

// pathsFunc resolves the config surfaces for one invocation, applying the
// --config flag and $BELT_CONFIG. Commands take it rather than calling
// config.PathsFor themselves so every command answers to the same override.
type pathsFunc func() (config.Paths, error)

// loader turns resolved paths into the config load the hook entrypoints
// take. Resolution and parse failures arrive as one error, because the hook
// path treats them the same way: it has no config, so it denies.
func loader(paths pathsFunc) hook.Load {
	return func() (config.Config, error) {
		p, err := paths()
		if err != nil {
			return config.Config{}, err
		}
		return config.LoadFrom(p)
	}
}

func hintCmd(paths pathsFunc) *cobra.Command {
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
session-start hints once when a session opens (a SessionStart hook),
subagent-start hints once per subagent before its first prompt (a
SubagentStart hook: the advice lands in the subagent's context, and only
custom hints run there), and prompt hints on each user prompt (a
UserPromptSubmit hook: advice goes to stdout as plain text, the form that
event adds to context). Wire them in ~/.claude/settings.json:

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
      "SubagentStart": [
        {"matcher": "", "hooks": [{"type": "command", "command": "belt hint subagent-start"}]}
      ],
      "UserPromptSubmit": [
        {"matcher": "", "hooks": [{"type": "command", "command": "belt hint prompt"}]}
      ]
    }
  }`,
		Args: eventArg("hint event", hint.Events),
		RunE: func(cmd *cobra.Command, args []string) error {
			hook.RunHint(
				strings.ToLower(args[0]),
				loader(paths),
				cmd.InOrStdin(),
				cmd.OutOrStdout(),
			)
			return nil // always exit 0: advice travels in the JSON, not the exit code
		},
	}
}

func recordCmd(paths pathsFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "record <event>",
		Short: "Record a Claude Code lifecycle event to the events ledger (payload on stdin, no output)",
		Long: `Record a Claude Code lifecycle event to the events ledger. Reads the hook
payload on stdin and posts one entry to the local events service under the
` + ledger.Source + ` source (ledger.source replaces it), tagged with the
session, the agent, and the repo, worktree and branch the event happened in.

For a valid event it never writes to stdout or stderr and always exits 0.
On SubagentStop any output keeps the subagent running, TaskCompleted,
TeammateIdle and PreCompact treat exit 2 as a block, and PermissionDenied
reads its output for a retry flag, so a ledger entry must never be able to
steer the session. The config decides only whether entries post
(ledger.enabled, default on) and where; a broken config file records with
the defaults. An unreadable payload is still recorded, at warn level.

The event argument is belt's record event, not the Claude Code hook name.
An unknown event is a wiring mistake and fails loud like every belt
entrypoint: an error on stderr and exit 1. Wire them in
~/.claude/settings.json:

  {
    "hooks": {
      "SubagentStart":    [{"matcher": "", "hooks": [{"type": "command", "command": "belt record subagent-start"}]}],
      "SubagentStop":     [{"matcher": "", "hooks": [{"type": "command", "command": "belt record subagent-stop"}]}],
      "TaskCompleted":    [{"matcher": "", "hooks": [{"type": "command", "command": "belt record task-completed"}]}],
      "TeammateIdle":     [{"matcher": "", "hooks": [{"type": "command", "command": "belt record teammate-idle"}]}],
      "PermissionDenied": [{"matcher": "", "hooks": [{"type": "command", "command": "belt record permission-denied"}]}],
      "Notification":     [{"matcher": "", "hooks": [{"type": "command", "command": "belt record notification"}]}],
      "PreCompact":       [{"matcher": "", "hooks": [{"type": "command", "command": "belt record pre-compact"}]}],
      "PostCompact":      [{"matcher": "", "hooks": [{"type": "command", "command": "belt record post-compact"}]}],
      "SessionEnd":       [{"matcher": "", "hooks": [{"type": "command", "command": "belt record session-end"}]}],
      "StopFailure":      [{"matcher": "", "hooks": [{"type": "command", "command": "belt record stop-failure"}]}]
    }
  }`,
		Args: eventArg("record event", ledger.Events),
		RunE: func(cmd *cobra.Command, args []string) error {
			hook.RunRecord(strings.ToLower(args[0]), loader(paths), cmd.InOrStdin())
			return nil // always exit 0: a non-zero exit would block some of these events
		},
	}
}

// eventArg builds the parse-time check every hook entrypoint shares:
// exactly one event, matched case-insensitively against the entrypoint's
// list. A miswired settings entry must fail loud. An unmatched event would
// run nothing: zero guards silently allow everything the hook was wired to
// deny, zero hints look identical to "nothing to say", and zero records
// leave a timeline that only looks quiet.
func eventArg(kind string, events func() []string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		if slices.Contains(events(), strings.ToLower(args[0])) {
			return nil
		}
		return fmt.Errorf("unknown %s %q (valid: %s)", kind, args[0], strings.Join(events(), ", "))
	}
}

func hookCmd(paths pathsFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "hook <event>",
		Short: "Run as a Claude Code PreToolUse hook (payload on stdin, deny JSON on stdout)",
		Long: `Run as a Claude Code PreToolUse hook. Reads the tool-call payload on stdin
and writes any deny decision as JSON on stdout; a valid event always exits 0
(the deny travels in the JSON, not the exit code).

A config file belt can't parse denies every tool call with that as the
reason. Belt is a guard: it can't tell "no rules configured" from "the rules
didn't load", so it blocks instead of guessing the permissive one. A payload
belt can't read is denied the same way, with a belt[payload] reason: stdin
that is not a JSON object, or a field the guards read (tool_name,
tool_input, cwd, and the command, file_path, content or new_string inside
tool_input) holding another JSON type. The session and agent fields never
deny. See belt docs.

The event argument is belt's guard event, not the Claude Code tool name:
bash guards Bash commands, write guards Write and Edit, external-text guards
the MCP tools that publish to a public code host (the matcher names them;
only the github.com server belongs there). Wire all three in
~/.claude/settings.json:

  {
    "hooks": {
      "PreToolUse": [
        {"matcher": "Bash", "hooks": [{"type": "command", "command": "belt hook bash"}]},
        {"matcher": "Write|Edit", "hooks": [{"type": "command", "command": "belt hook write"}]},
        {"matcher": "^mcp__gh_com__.*", "hooks": [{"type": "command", "command": "belt hook external-text"}]}
      ]
    }
  }`,
		Args: eventArg("event", guard.Events),
		RunE: func(cmd *cobra.Command, args []string) error {
			hook.Run(strings.ToLower(args[0]), loader(paths), cmd.InOrStdin(), cmd.OutOrStdout())
			return nil // always exit 0 for a valid event: the deny travels in the JSON
		},
	}
}

func checkCmd(paths pathsFunc) *cobra.Command {
	var cwd, file, content, tool, input string
	cmd := &cobra.Command{
		Use:   "check <event> [command]",
		Short: "Dry-run the guards against a command, write, or MCP call and print each verdict",
		Long: "Examples:\n" +
			"  belt check bash \"git push origin main\"\n" +
			"  belt check write --file ~/code/src/github.com/mad01/thismoon/README.md --content \"some text\"\n" +
			"  belt check external-text --tool mcp__gh_com__create_pull_request" +
			" --input '{\"owner\":\"o\",\"repo\":\"r\",\"title\":\"t\",\"body\":\"text\"}'",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// The guards treat an empty cwd as unknown and fail closed, so a
			// dry-run names the process cwd explicitly rather than leaving it
			// to be guessed.
			if cwd == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve working directory: %w", err)
				}
				cwd = wd
			}
			in := guard.Input{
				Event:    args[0],
				Cwd:      cwd,
				FilePath: file,
				Content:  content,
				ToolName: tool,
			}
			if len(args) == 2 {
				in.Command = args[1]
			}
			if input != "" {
				if err := json.Unmarshal([]byte(input), &in.ToolInput); err != nil {
					return fmt.Errorf("--input is not a JSON object: %w", err)
				}
			}
			p, err := paths()
			if err != nil {
				return err
			}
			// A dry-run reports the config failure as an error: nothing is
			// being guarded here, so there is no call to fail closed on.
			cfg, err := config.LoadFrom(p)
			if err != nil {
				return err
			}
			guards := guard.ForEvent(in.Event, cfg)
			if len(guards) == 0 {
				return fmt.Errorf("no guards for event %q (valid: %s)",
					in.Event, strings.Join(guard.Events(), ", "))
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
	cmd.Flags().
		StringVar(&cwd, "cwd", "", "working directory for bare git push resolution (default: current dir)")
	cmd.Flags().StringVar(&file, "file", "", "target file path (write event)")
	cmd.Flags().StringVar(&content, "content", "", "content to scan (write event)")
	cmd.Flags().StringVar(&tool, "tool", "", "MCP tool name (external-text event)")
	cmd.Flags().
		StringVar(&input, "input", "", "tool call input as a JSON object (external-text event)")
	return cmd
}

func versionCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the belt version",
		Long: `Print the belt version (the git commit it was built from).

Plain output is the bare version token: the cross-tool convention sibling
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
