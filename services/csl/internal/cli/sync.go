package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

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
that changed, in a single process with one state.json write.

Hooks are suppressed during sync pulls via core.hooksPath=/dev/null. Any repos
queued by earlier ad-hoc git pulls are also drained and indexed.

Repos on non-default branches, in detached HEAD, with dirty working trees
(tracked uncommitted changes), or without a remote are skipped. Untracked
files don't count as dirty. Repos in hooks.post_merge.exclude are also
skipped.

The run takes the sync lock shared with the background refresher in ` + "`csl web`" + `;
if another sync or refresh is already running the command fails instead of
racing it: retry when it finishes.

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
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	w := cmd.ErrOrStderr()
	isTTY := false
	if f, ok := w.(*os.File); ok {
		isTTY = isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}

	_, err = syncer.Run(cmd.Context(), cfg, syncer.Options{
		Concurrency: syncConcurrencyFlag,
		DryRun:      syncDryRunFlag,
		Out:         cmd.OutOrStdout(),
		Err:         w,
		TTY:         isTTY,
	})
	if errors.Is(err, syncer.ErrLocked) {
		return fmt.Errorf(
			"another sync or background refresh is running — retry when it finishes: %w", err,
		)
	}
	return err
}
