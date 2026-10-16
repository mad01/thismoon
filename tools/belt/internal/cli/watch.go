package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hook"
)

// errWake is what `belt watch` returns after printing the items it found.
// Execute maps it to wakeExitCode without printing anything further: the
// items on stderr are the whole message.
var errWake = errors.New("belt watch: items delivered")

// wakeExitCode is the exit code an asyncRewake hook uses to wake the
// session; Claude Code shows the hook's stderr to it as a system reminder.
// watch is the one belt command that exits with it on purpose
// (docs/adr/0024).
const wakeExitCode = 2

func watchCmd(paths pathsFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Wait for an item addressed to this session and wake it (a Claude Code asyncRewake hook)",
		Long: `Wait for an item addressed to this session and wake the session with it.
Runs as a Claude Code asyncRewake hook on Stop and SessionStart: Claude Code
starts it in the background each time the session goes idle, and it polls
the events service for entries under the ` + config.DefaultWatchSource + ` source (watch.source
replaces it) whose to tag is this session's id.

When items arrive it moves its cursor past them, prints them on stderr and
exits 2, which wakes the session with the text as a system reminder. Each
item is delivered once. With nothing after watch.max_wait_s (default 540)
it exits 0 silently, and a newer watcher for the same session makes an
older one exit 0. Watch disabled (the default: set watch.enabled), a config
belt cannot read, a payload without a session id, and an events service
that does not answer all end in exit 0 with no output.

Producers address an item by posting to the events service with the source
and a to tag, for example:

  curl -s -X POST http://127.0.0.1:7430/api/events -H 'Content-Type: application/json' \
    -d '{"source":"` + config.DefaultWatchSource + `","title":"rebase done","message":"feat/x is ready",
         "tags":{"to":"<session id>","from":"orchestrator"}}'

Wire it in ~/.claude/settings.json with a timeout above watch.max_wait_s:

  {
    "hooks": {
      "Stop":         [{"hooks": [{"type": "command", "command": "belt watch", "asyncRewake": true, "timeout": 590}]}],
      "SessionStart": [{"hooks": [{"type": "command", "command": "belt watch", "asyncRewake": true, "timeout": 590}]}]
    }
  }`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hook.RunWatch(cmd.Context(), loader(paths), cmd.InOrStdin(), cmd.ErrOrStderr()) {
				return errWake
			}
			return nil
		},
	}
}
