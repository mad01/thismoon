package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/record"
)

// recordCeiling bounds the whole record run, budget overruns included. It
// sits under the 3 s timeout the Edit/Write hook is registered with, so the
// hook never reports moondb as hung, and it holds the turn-edge snapshot
// budget and the Bash budget with room to spare.
const recordCeiling = 2500 * time.Millisecond

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Spool one hook payload from stdin (run by the Claude Code hooks)",
	Long: `Read one Claude Code hook payload on stdin and write what it says to the
spool under --workdir. serve drains the spool; record itself never touches
the store and never waits for serve.

Three hooks pipe into it. UserPromptSubmit opens a turn. PostToolUse for
Edit, Write, MultiEdit and NotebookEdit records the file a tool changed;
PostToolUse for Bash records the files a command changed, when the user
setting bashEditDiffEnabled makes Claude Code report them; a file git
reports clean after the command (a pull or checkout brought it back to its
committed state) is skipped: counted on the turn, with no edit. Stop closes the
turn and reports commits made since. UserPromptSubmit and Stop also take a
snapshot of each worktree in play: the content of its dirty files and of
the files the session touched. record prints nothing on stdout and exits 0
whatever happens, so a failure here can never block the session; problems go
to stderr.

Per-machine switches live in ` + configFile + ` in the moondb config directory:
  [capture]
  enabled = false          # turn recording off on this machine
  exclude = ["*.secret"]   # globs added to the built-in exclude list`,
	RunE: runRecord,
}

func init() {
	rootCmd.AddCommand(recordCmd)
}

// runRecord is the hook entry point. It returns nil on every path: the exit
// code belongs to the harness contract, not to the outcome.
func runRecord(cmd *cobra.Command, _ []string) error {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "moondb record: panic: %v\n", r)
		}
	}()
	// A broken config fails closed (ADR-0011): capture nothing, say so once.
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "moondb record: config:", cfgErr)
		return nil
	}
	if !cfg.Capture.enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), recordCeiling)
	defer cancel()
	opts := record.Options{
		Workdir:        flagWorkdir,
		Excludes:       cfg.Capture.excludes(),
		Budget:         moondb.RecordBudget,
		BashBudget:     moondb.BashBudget,
		SnapshotBudget: moondb.SnapshotBudget,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "moondb record: "+format+"\n", args...)
		},
	}
	if err := record.Run(ctx, cmd.InOrStdin(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "moondb record:", err)
	}
	return nil
}
