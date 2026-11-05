package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/search"
)

func newCountCmd(a *app) *cobra.Command {
	var tier, groupBy string
	c := &cobra.Command{
		Use:   "count <query>",
		Short: "Count matches of a zoekt query across the vault",
		Long: `Count matches of a zoekt query across the vault.

Use --group-by=tier or --group-by=tag for a per-group breakdown; omit for a
single total. With -o json, prints the brain_count document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if groupBy != "" && groupBy != "tier" && groupBy != "tag" {
				return usageErrorf("--group-by must be tier, tag, or empty, got %q", groupBy)
			}
			t, err := parseTierFlag(tier)
			if err != nil {
				return err
			}
			cfg, _, err := a.openVault()
			if err != nil {
				return err
			}
			indexDir, err := ensureIndexCLI(cfg)
			if err != nil {
				return err
			}
			groups, total, err := search.Count(context.Background(), indexDir, search.CountOptions{
				Pattern: args[0],
				Tier:    t,
				GroupBy: groupBy,
			})
			if err != nil {
				return err
			}
			res := search.Counts{Total: total, Groups: groups}
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				fmt.Fprintf(w, "total: %d\n", res.Total)
				for _, g := range res.Groups {
					fmt.Fprintf(w, "  %-20s %d\n", g.Group, g.Count)
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&tier, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	c.Flags().StringVar(&groupBy, "group-by", "", "group counts by: tier or tag")
	return c
}
