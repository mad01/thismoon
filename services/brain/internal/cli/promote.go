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
		Use:   "promote <path>",
		Short: "Promote an inbox note into notes/ (or another tier)",
		Long: `Move an inbox note to a validated tier: flips status to "validated",
rewrites [[backlinks]] across the vault, and optionally appends a bullet to
the specified MOC. Frontmatter keys brain does not model are kept. Fails with
exit code 4 when the target note already exists. With -o json, prints the
brain_promote document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rel := args[0]
			t, err := parseTierFlag(tier)
			if err != nil {
				return err
			}
			opts.TargetTier = t
			_, v, err := openVault()
			if err != nil {
				return err
			}
			full, err := v.Resolve(rel)
			if err != nil {
				return err
			}
			res, err := v.Promote(full, opts)
			if err != nil {
				return withNoteHint(err, rel)
			}
			report := v.Report(res)
			return a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				writePromoteText(w, report)
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

func writePromoteText(w io.Writer, r vault.PromoteReport) {
	fmt.Fprintf(w, "%s -> %s\n", r.OldPath, r.NewPath)
	if len(r.BacklinkRewrites) > 0 {
		fmt.Fprintf(w, "rewrote %d backlink(s):\n", len(r.BacklinkRewrites))
		for _, p := range r.BacklinkRewrites {
			fmt.Fprintf(w, "  %s\n", p)
		}
	}
	if r.MOCAppended != "" {
		fmt.Fprintf(w, "appended to MOC: %s\n", r.MOCAppended)
	}
}
