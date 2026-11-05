package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newInboxCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "inbox",
		Short: "List raw captures, keep them, or archive the expired ones",
		Long: `Work the inbox's raw notes, which expire expires_at days after capture
(30 by default):

  inbox list [--expired]     raw notes, soonest to expire first
  inbox keep <note> --days N extend a raw note's expires_at to N days from today
  inbox archive --expired    move expired raw notes to .brain/archive/

keep is the one way brain extends an expiry; an edit leaves expires_at alone.
archive deletes nothing: it moves each note to the same path under
.brain/archive/ in its vault, which search, check and the link graph never
read. Move a file back under inbox/ to restore it.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(newInboxListCmd(a), newInboxKeepCmd(a), newInboxArchiveCmd(a))
	return c
}

func newInboxListCmd(a *app) *cobra.Command {
	var expired bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List raw inbox notes, soonest to expire first",
		Long: `List the raw notes in every vault's inbox/ (or the --vault ones), soonest
expires_at first. --expired keeps the notes past it. Text output prints
expires_at, the note and its title; -o json prints {"total", "notes":
[{vault, path, title, captured_by, expires_at, expired}]}.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, set, err := a.openVaults()
			if err != nil {
				return err
			}
			listing, err := set.Inbox(time.Now(), expired)
			if err != nil {
				return err
			}
			return a.emit(cmd.OutOrStdout(), listing, func(w io.Writer) error {
				for _, n := range listing.Notes {
					when := n.ExpiresAt
					if when == "" {
						when = "-"
					}
					mark := ""
					if n.Expired {
						mark = "  (expired)"
					}
					fmt.Fprintf(w, "%s  %s  %s%s\n", when, a.show(n.Vault, n.Path), n.Title, mark)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&expired, "expired", false, "only the notes past expires_at")
	return c
}

func newInboxKeepCmd(a *app) *cobra.Command {
	var days int
	c := &cobra.Command{
		Use:   "keep <note>",
		Short: "Extend a raw note's expiry",
		Long: `Keep a raw capture for longer: expires_at becomes --days after today (30 by
default), unless it already expires later. This is the explicit human
action that extends an expiry; an edit, by you or an agent, never resets it.
A note that is not raw refuses with exit code 2, a shared vault with 6. With
-o json, prints {"vault", "path", "sha", "expires_at"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if days <= 0 {
				return usageErrorf("--days must be > 0, got %d", days)
			}
			_, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			res, err := ref.Vault.Keep(ref.Abs(), days, time.Now())
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				fmt.Fprintf(w, "%s expires %s\n", a.show(res.Vault, res.Path), res.ExpiresAt)
				return nil
			})
		},
	}
	c.Flags().IntVar(&days, "days", lifecycle.RawLifetimeDays, "days from today the note expires")
	return c
}

// ArchiveReport is the JSON result of brain inbox archive.
type ArchiveReport struct {
	DryRun   bool             `json:"dry_run"`
	Archived []vault.Archived `json:"archived"`
	Skipped  []vault.IDSkip   `json:"skipped,omitempty"`
	// Error is the failure that stopped the run, after the moves above.
	Error *errorBody `json:"error,omitempty"`
}

func newInboxArchiveCmd(a *app) *cobra.Command {
	var expired, dryRun bool
	c := &cobra.Command{
		Use:   "archive --expired",
		Short: "Move expired raw notes out of search into .brain/archive/",
		Long: `Move every raw inbox note past its expires_at to the same path under
.brain/archive/ in its vault, so search, check and the link graph stop
seeing it; the indexes drop it at once. Nothing is deleted, and a name an
earlier archive took gets a -2, -3, ... suffix. Restore a note by moving it
back under inbox/, then brain inbox keep it. --expired is required, so the
command says what it moves. Shared vaults are read-only here: they are
skipped, and one --vault names refuses with exit code 6. --dry-run reports
the moves and makes none. With -o json, prints {"dry_run", "archived":
[{vault, path, archived_to}], "skipped", "error"}. A run that fails partway
still lists and purges the notes it moved, then exits with the error, which
-o json carries as error {code, message}.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !expired {
				return usageErrorf(
					"pass --expired: archive moves only the raw notes past expires_at",
				)
			}
			lib, set, err := a.openVaults()
			if err != nil {
				return err
			}
			report, runErr := archiveExpired(lib, set, len(a.vaults) > 0, dryRun, time.Now())
			if runErr != nil {
				_, code := classify(runErr)
				report.Error = &errorBody{Code: code, Message: oneLine(runErr.Error())}
			}
			if err := a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				return writeArchiveText(w, a, report)
			}); err != nil {
				return errors.Join(runErr, err)
			}
			if runErr != nil {
				return &reportedError{err: runErr}
			}
			return nil
		},
	}
	c.Flags().
		BoolVar(&expired, "expired", false, "archive the raw notes past expires_at (required)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "report the moves without making them")
	return c
}

// sharedSkipReason is why archive passes over a shared vault no --vault
// names.
const sharedSkipReason = "shared vault; read-only here, its notes change through pull requests"

// archiveExpired archives the expired raw notes of every vault in set,
// skipping the shared ones unless named is set. A vault that fails partway
// still has the notes it moved purged from its indexes and listed in the
// report, which stops at that vault and comes back with the error.
func archiveExpired(
	lib *library.Library,
	set *vault.Set,
	named, dryRun bool,
	now time.Time,
) (ArchiveReport, error) {
	report := ArchiveReport{DryRun: dryRun, Archived: []vault.Archived{}}
	for _, v := range set.Vaults() {
		if v.Shared && !named {
			report.Skipped = append(report.Skipped, vault.IDSkip{
				Vault: v.Name, Reason: sharedSkipReason,
			})
			continue
		}
		moved, err := v.ArchiveExpired(now, dryRun)
		report.Archived = append(report.Archived, moved...)
		if err := errors.Join(err, purgeArchived(lib, v, moved, dryRun)); err != nil {
			return report, err
		}
	}
	return report, nil
}

// purgeArchived drops the archived notes from the vault's indexes.
func purgeArchived(
	lib *library.Library,
	v *vault.Vault,
	moved []vault.Archived,
	dryRun bool,
) error {
	if dryRun || len(moved) == 0 {
		return nil
	}
	rels := make([]string, 0, len(moved))
	for _, m := range moved {
		rels = append(rels, m.Path)
	}
	if err := lib.Purge(context.Background(), v, rels...); err != nil {
		return fmt.Errorf(
			"archived %d note(s) in %s, but purging them from the indexes failed: %w "+
				"(retry with: brain index)", len(moved), v.Name, err,
		)
	}
	return nil
}

func writeArchiveText(w io.Writer, a *app, report ArchiveReport) error {
	verb := "archived"
	if report.DryRun {
		verb = "would archive"
	}
	for _, m := range report.Archived {
		fmt.Fprintf(w, "%s %s -> %s\n", verb, a.show(m.Vault, m.Path), m.ArchivedTo)
	}
	if len(report.Archived) == 0 {
		fmt.Fprintln(w, "no expired raw notes")
	}
	for _, sk := range report.Skipped {
		fmt.Fprintf(w, "skipped %s: %s\n", sk.Vault, sk.Reason)
	}
	return nil
}
