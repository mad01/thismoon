package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var (
	countTierFlag    string
	countGroupByFlag string
)

var countCmd = &cobra.Command{
	Use:   "count <query>",
	Short: "Count matches of a zoekt query across the vault",
	Long: `Count matches of a zoekt query across the vault.

Use --group-by=tier or --group-by=tag for a per-group breakdown; omit for a
single total.`,
	Args: cobra.ExactArgs(1),
	RunE: runCountCmd,
}

func init() {
	countCmd.Flags().
		StringVar(&countTierFlag, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	countCmd.Flags().StringVar(&countGroupByFlag, "group-by", "", "group counts by: tier or tag")
	rootCmd.AddCommand(countCmd)
}

func runCountCmd(cmd *cobra.Command, args []string) error {
	pattern := args[0]
	if countGroupByFlag != "" && countGroupByFlag != "tier" && countGroupByFlag != "tag" {
		return fmt.Errorf(
			"brain count: --group-by must be 'tier', 'tag', or empty, got %q",
			countGroupByFlag,
		)
	}

	var tier vault.Tier
	if countTierFlag != "" {
		t, err := vault.ParseTier(countTierFlag)
		if err != nil {
			return fmt.Errorf("brain count: %w", err)
		}
		tier = t
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain count: %w", err)
	}
	indexDir, err := ensureIndexCLI(cfg)
	if err != nil {
		return fmt.Errorf("brain count: %w", err)
	}

	opts := search.CountOptions{
		Pattern: pattern,
		Tier:    tier,
		GroupBy: countGroupByFlag,
	}
	results, total, err := search.Count(context.Background(), indexDir, opts)
	if err != nil {
		return fmt.Errorf("brain count: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "total: %d\n", total)
	if len(results) > 0 {
		for _, r := range results {
			fmt.Fprintf(w, "  %-20s %d\n", r.Group, r.Count)
		}
	}
	return nil
}
