package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/sharing"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newFlagCmd(a *app) *cobra.Command {
	var (
		reason string
		dryRun bool
	)
	c := &cobra.Command{
		Use:   "flag <note> --reason <text>",
		Short: "Mark a note disputed, or open an issue on a shared note",
		Long: `Flag a note whose claims you doubt: status becomes disputed, --reason is
stored as status_reason and status_at records the day. read and peek then
show a DISPUTED banner until someone settles the note (edit --status
validated, or retract it). Flagging a disputed note again replaces the
reason. A superseded, retracted or withdrawn note refuses with exit code 2.
With -o json, prints {"vault", "path", "sha"}.

A note in a shared vault changes through its repo, so flag opens an issue
there with gh issue create instead: the title names the note, the body its
ref, id and the reason. --dry-run prints the issue and opens nothing. A
shared clone with no remote refuses with exit code 6. With -o json, prints
{"vault", "path", "id", "repo", "title", "body", "url", "dry_run"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reason) == "" {
				return usageErrorf("--reason <text> is required")
			}
			lib, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			if ref.Vault.Shared {
				return a.flagShared(cmd, lib, ref, reason, dryRun)
			}
			if dryRun {
				return usageErrorf("--dry-run goes with a shared note, where flag opens an issue")
			}
			sha, err := ref.Vault.Flag(ref.Abs(), reason, time.Now())
			return a.emitRevoked(cmd, ref, sha, err)
		},
	}
	c.Flags().StringVar(&reason, "reason", "", "why the note is in doubt (required)")
	c.Flags().BoolVar(&dryRun, "dry-run", false,
		"for a shared note: print the issue instead of opening it")
	return c
}

// flagShared opens, or with dryRun prints, the issue a flag on a shared
// note becomes.
func (a *app) flagShared(
	cmd *cobra.Command,
	lib *library.Library,
	ref vault.Ref,
	reason string,
	dryRun bool,
) error {
	res, err := sharing.FlagIssue(context.Background(), lib, ref, reason, dryRun)
	if err != nil {
		return withNoteHint(err, ref.Path)
	}
	return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
		if !res.DryRun {
			fmt.Fprintf(w, "opened %s\n", res.URL)
			return nil
		}
		repo := res.Repo
		if repo == "" {
			repo = "the shared vault's repo"
		}
		fmt.Fprintf(w, "would open an issue on %s:\ntitle: %s\n\n%s", repo, res.Title, res.Body)
		return nil
	})
}
