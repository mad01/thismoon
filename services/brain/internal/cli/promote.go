package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newPromoteCmd(a *app) *cobra.Command {
	var (
		opts vault.PromoteOptions
		tier string
	)
	c := &cobra.Command{
		Use:   "promote <note>",
		Short: "Promote an inbox note into notes/ (or another tier)",
		Long: `Move an inbox note to a validated tier: flips status to "validated",
rewrites [[backlinks]] across the vault, and optionally appends a bullet to
the specified MOC. Frontmatter keys brain does not model are kept. The note
stays in its vault; a shared vault refuses with exit code 6. Fails with exit
code 4 when the target note already exists. With -o json, prints the
brain_promote document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rel := args[0]
			t, err := parseTierFlag(tier)
			if err != nil {
				return err
			}
			opts.TargetTier = t
			_, ref, err := a.locate(rel)
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
	return c
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
