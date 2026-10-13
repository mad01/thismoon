package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/progstatus"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

var (
	syncConcurrencyFlag int
	syncDryRunFlag      bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull all repos and batch-reindex updated ones",
	Long: `Pull all discovered repos (parallel, ff-only) and batch-reindex the ones
that changed, in a single process with one state.json write. Repos whose index
fingerprint is stale are reindexed too, pulled or not, so an index-format bump
heals on the next sync.

Hooks are suppressed during sync pulls via core.hooksPath=/dev/null. Any repos
queued by earlier ad-hoc git pulls are also drained and indexed.

Repos on non-default branches, in detached HEAD, with dirty working trees
(tracked uncommitted changes), or without a remote are not pulled. Untracked
files don't count as dirty. Repos in hooks.post_merge.exclude are also
skipped.

The run takes the sync lock shared with the background refresher in ` + "`csl web`" + `;
if another sync or refresh is already running the command fails instead of
racing it: retry when it finishes. Each pull is bounded by a 5-minute timeout,
so a hung remote fails that repo instead of stalling the run and the lock.

Use --concurrency to control parallel pulls (default: from config, fallback 8).
Use --dry-run to preview what would happen without pulling or indexing.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().
		IntVar(&syncConcurrencyFlag, "concurrency", 0, "parallel pull workers (0 = use config default)")
	syncCmd.Flags().
		BoolVar(&syncDryRunFlag, "dry-run", false, "preview without pulling or indexing")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, _ []string) error {
	status := progstatus.New("csl")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	w := cmd.ErrOrStderr()
	isTTY := false
	if f, ok := w.(*os.File); ok {
		isTTY = progstatus.IsTerminal(f)
	}

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := interruptContext(parent)
	defer stop()

	msg := "syncing repos"
	if syncDryRunFlag {
		msg = "previewing sync"
	}
	status.Working(msg)
	rep, err := syncer.Run(ctx, cfg, syncer.Options{
		Concurrency: syncConcurrencyFlag,
		DryRun:      syncDryRunFlag,
		Out:         cmd.OutOrStdout(),
		Err:         w,
		TTY:         isTTY,
	})
	if err != nil {
		err = lockedHint(err)
		if ctx.Err() != nil { // the user interrupted the run: nothing to show
			status.Idle()
		} else {
			status.Error(err.Error())
		}
		return err
	}
	status.Done(syncDone(rep, syncDryRunFlag))
	return nil
}

// syncDone words the terminal's final report. A dry run evaluated repos and
// pulled nothing, so it must not read as a sync.
func syncDone(rep *syncer.Report, dryRun bool) string {
	if dryRun {
		return fmt.Sprintf("dry run: %d repos evaluated", len(rep.Results))
	}
	return fmt.Sprintf("%d repos evaluated, %d indexed", len(rep.Results), len(rep.Indexed))
}

// interruptContext returns a context that the first SIGINT or SIGTERM cancels,
// which kills each in-flight pull's process group. git runs outside the
// terminal's foreground group, so without this a Ctrl-C would orphan the pulls.
// Once ctx is done the handler is removed: a second Ctrl-C kills the process
// with the default behavior, even during the index phase, which ignores ctx.
func interruptContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx, stop
}
