package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/sharing"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newPromoteCmd(a *app) *cobra.Command {
	var (
		opts  vault.PromoteOptions
		tier  string
		to    string
		force bool
	)
	c := &cobra.Command{
		Use:   "promote <note>",
		Short: "Promote an inbox note into notes/, or a note into a shared vault",
		Long: `Move an inbox note to a validated tier: flips status to "validated",
rewrites [[backlinks]] across the vault, and optionally appends a bullet to
the specified MOC. Frontmatter keys brain does not model are kept. The note
stays in its vault; a shared vault refuses with exit code 6. Fails with exit
code 4 when the target note already exists. With -o json, prints the
brain_promote document.

--to <shared vault> shares a validated private note instead: brain checks
it against the shared vault's .brain/policy.yaml (errors refuse it), scans
it for secrets (exit 8), and opens a pull request on the shared vault's repo
that adds it under the same path and id, without private-only fields such as
status_reason. The branch is brain/promote/<stem>-<short id>, prepared in a
temporary worktree, so the shared clone's checkout is never touched. The
private copy becomes promoting, with shared_pr naming the pull request;
brain sync removes it once the pull request merges, or returns it to
validated if it closes. brain never edits a shared MOC: the note's mocs
field lists the MOCs it belongs in, and the pull request says so. A target
that is not shared exits 6, a note that is not validated 2, a note with an
open shared_pr 2 unless --force. Needs gh (the GitHub CLI) logged in. With
-o json, prints {"vault", "path", "id", "shared_vault", "branch", "pr",
"checks", "mocs", "warnings"}.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if to != "" {
				return a.promoteTo(cmd, args[0], to, tier, opts, force)
			}
			if force {
				return usageErrorf("--force goes with --to")
			}
			t, err := parseTierFlag(tier)
			if err != nil {
				return err
			}
			opts.TargetTier = t
			_, ref, err := a.locate(args[0])
			if err != nil {
				return err
			}
			res, err := ref.Vault.Promote(ref.Abs(), opts)
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			report := ref.Vault.Report(res)
			return a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				a.writePromoteText(w, report)
				return nil
			})
		},
	}
	c.Flags().StringVar(&opts.NewTitle, "title", "", "new title (re-slugs the filename)")
	c.Flags().StringVar(&tier, "tier", "", "target tier (default notes)")
	c.Flags().
		StringVar(&opts.MOC, "moc", "", "append a bullet to this MOC (vault-relative or absolute)")
	c.Flags().
		StringVar(&to, "to", "", "open a pull request that adds the note to this shared vault")
	c.Flags().BoolVar(&force, "force", false,
		"with --to: open another pull request for a note promoting through one already")
	return c
}

// promoteTo runs brain promote --to.
func (a *app) promoteTo(
	cmd *cobra.Command,
	ref, to, tier string,
	opts vault.PromoteOptions,
	force bool,
) error {
	if tier != "" || opts.NewTitle != "" || opts.MOC != "" {
		return usageErrorf("--to takes no --tier, --title or --moc: the shared copy keeps the " +
			"note's path, and brain never edits a shared MOC (list them in the note's mocs field)")
	}
	lib, note, err := a.locate(ref)
	if err != nil {
		return err
	}
	target, err := lib.Set().Get(to)
	if err != nil {
		return err
	}
	res, err := sharing.Promote(context.Background(), lib, note, target,
		sharing.PromoteOptions{Force: force})
	if err != nil {
		return withNoteHint(err, note.Path)
	}
	warn(cmd, res.Warnings)
	return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
		fmt.Fprintf(w, "%s -> %s:%s\n", a.show(res.Vault, res.Path), res.SharedVault, res.Path)
		fmt.Fprintf(w, "opened %s (branch %s)\n", res.PR, res.Branch)
		for _, m := range res.MOCs {
			fmt.Fprintf(w, "list it in MOC %s: brain never edits a shared MOC\n", m)
		}
		fmt.Fprintf(
			w,
			"%s is promoting until the pull request merges or closes; brain sync settles it\n",
			a.show(res.Vault, res.Path),
		)
		return nil
	})
}

func (a *app) writePromoteText(w io.Writer, r vault.PromoteReport) {
	fmt.Fprintf(w, "%s -> %s\n", a.show(r.Vault, r.OldPath), a.show(r.Vault, r.NewPath))
	if len(r.BacklinkRewrites) > 0 {
		fmt.Fprintf(w, "rewrote %d backlink(s):\n", len(r.BacklinkRewrites))
		for _, p := range r.BacklinkRewrites {
			fmt.Fprintf(w, "  %s\n", a.show(r.Vault, p))
		}
	}
	if r.MOCAppended != "" {
		fmt.Fprintf(w, "appended to MOC: %s\n", a.show(r.Vault, r.MOCAppended))
	}
}
