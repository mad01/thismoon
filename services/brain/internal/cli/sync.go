package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/sharing"
)

func newSyncCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull and push each vault, and settle promotions into shared vaults",
		Long: `Sync every vault (or the --vault ones) with its git remote, in three steps:

  1. Pull each vault that has a remote, fast-forward only (git pull --ff-only).
  2. Settle each promoting note of the private vaults: gh pr view on its
     shared_pr. MERGED removes the private copy with a commit, so links
     resolve to the shared copy, which has the same id and path. CLOSED
     returns the note to validated and drops shared_pr. OPEN leaves it.
  3. Push each private vault that is ahead, so the commits of step 2 go out
     too. A shared vault is never pushed: shared notes change through pull
     requests.

A vault with no remote (a local-only private vault) is never pulled or
pushed; sync says so in one line and still settles its promotions. A vault
with uncommitted changes to tracked files, one diverged from its upstream,
or one whose branch tracks no remote branch is refused with a hint and left
as it is: brain never stashes, resets or merges. A merged note whose shared
copy is not in a pulled shared vault yet stays until a later sync.

Exits 1 when a vault was refused or failed, or a pull request could not be
read; the report prints either way. Needs gh (the GitHub CLI) for step 2.
With -o json, prints {"ok", "vaults": [{vault, shared, state, message,
pulled, pushed, promotions: [{path, id, pr, state, action, detail}]}]};
state is synced, local_only, not_git, refused or failed, and action is
removed, validated, waiting or failed.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			lib, set, err := a.openVaults()
			if err != nil {
				return err
			}
			report := sharing.Sync(context.Background(), lib, set, time.Now())
			if err := a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				a.writeSyncText(w, report)
				return nil
			}); err != nil {
				return err
			}
			if err := report.Err(); err != nil {
				return &reportedError{err: err}
			}
			return nil
		},
	}
}

func (a *app) writeSyncText(w io.Writer, r sharing.SyncReport) {
	for _, v := range r.Vaults {
		fmt.Fprintf(w, "%s: %s\n", v.Vault, v.Message)
		for _, p := range v.Promotions {
			fmt.Fprintf(w, "  %s: %s (%s)\n", a.show(v.Vault, p.Path), p.Detail, p.PR)
		}
	}
}
