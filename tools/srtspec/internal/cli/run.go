package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/srtspec/internal/launch"
)

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <settings-file> [--] <command> [args...]",
		Short: "Launch a command inside srt under a settings file, the fleet way",
		Long: `Launch a command inside srt (anthropics/sandbox-runtime) under a settings
file, the way every sandboxed MCP server and t-man service starts.
~/code/bin/srt-run execs this.

The settings file is ~/.config/srt/<label>.json, rendered from the process's
access manifest; a leading ~ is expanded. What this machine adds is merged
in first: ~/.config/srt/<label>.local.json, which may only widen, and the
hosts in SRT_RUN_ALLOW_DOMAINS. The merged copy goes to
~/.config/srt/merged/<label>.json (mode 600) and srt reads that. A layer that
does more than widen, or a bare * host, refuses the launch (exit 1).

The label is the file's name without .json, or SRT_RUN_LABEL. The command
runs as env SRT_RUN=<label> <command>, the marker sandbox-watch attributes
denials by. CLAUDE_CODE_TMPDIR is set to TMPDIR (else /private/tmp) so the
child has a temp dir srt allows. SRT_RUN_RUNTIME picks bun or node; unset,
bun when one is installed. Exit 127 when srt or the runtime is missing, 2 on
a usage error. One line, "srt-run: <label> runtime <bun|node>", goes to
stderr; then srt replaces this process.`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			if len(args) >= 2 && args[1] == "--" {
				args = append([]string{args[0]}, args[2:]...)
			}
			if len(args) < 2 {
				return &launch.ExitError{
					Code: 2,
					Msg:  "usage: srtspec run <settings-file> [--] <command> [args...]",
				}
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			c, err := launch.Plan(launch.Input{
				Settings: args[0],
				Argv:     args[1:],
				Home:     home,
				Env:      environ(),
				Log:      cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "srt-run: %s runtime %s\n", c.Label, c.Runtime)
			return syscall.Exec(c.Path, c.Argv, c.Env)
		},
	}
}

// environ returns the process environment as a map; a later duplicate wins,
// as it does for a shell.
func environ() map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	return env
}

// ExitCode is the status main should exit with for err: the launch
// contract's code when err carries one, else 1.
func ExitCode(err error) int {
	var exit *launch.ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	return 1
}
