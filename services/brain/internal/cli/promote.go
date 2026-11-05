package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var (
	promoteTitleFlag string
	promoteTierFlag  string
	promoteMOCFlag   string
)

var promoteCmd = &cobra.Command{
	Use:   "promote <path>",
	Short: "Promote an inbox note into notes/ (or another tier)",
	Long: `Move an inbox note to a validated tier: flips status to "validated",
rewrites [[backlinks]] across the vault, and optionally appends a bullet to
the specified MOC.`,
	Args: cobra.ExactArgs(1),
	RunE: runPromoteCmd,
}

func init() {
	promoteCmd.Flags().
		StringVar(&promoteTitleFlag, "title", "", "new title (re-slugs the filename)")
	promoteCmd.Flags().StringVar(&promoteTierFlag, "tier", "", "target tier (default notes)")
	promoteCmd.Flags().
		StringVar(&promoteMOCFlag, "moc", "", "append a bullet to this MOC (vault-relative or absolute)")
	rootCmd.AddCommand(promoteCmd)
}

func runPromoteCmd(cmd *cobra.Command, args []string) error {
	rel := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain promote: %w", err)
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return fmt.Errorf("brain promote: %w", err)
	}
	if filepath.IsAbs(rel) || strings.Contains(filepath.Clean(rel), "..") {
		return fmt.Errorf("brain promote: path must be vault-relative: %s", rel)
	}
	full := filepath.Join(cfg.Vault, filepath.Clean(rel))

	opts := vault.PromoteOptions{
		NewTitle: promoteTitleFlag,
		MOC:      promoteMOCFlag,
	}
	if promoteTierFlag != "" {
		t, err := vault.ParseTier(promoteTierFlag)
		if err != nil {
			return fmt.Errorf("brain promote: %w", err)
		}
		opts.TargetTier = t
	}

	res, err := v.Promote(full, opts)
	if err != nil {
		return fmt.Errorf("brain promote: %w", err)
	}

	w := cmd.OutOrStdout()
	newRel, _ := filepath.Rel(cfg.Vault, res.NewPath)
	oldRel, _ := filepath.Rel(cfg.Vault, res.OldPath)
	fmt.Fprintf(w, "%s -> %s\n", filepath.ToSlash(oldRel), filepath.ToSlash(newRel))
	if len(res.BacklinkRewrites) > 0 {
		fmt.Fprintf(w, "rewrote %d backlink(s):\n", len(res.BacklinkRewrites))
		for _, r := range res.BacklinkRewrites {
			if p, err := filepath.Rel(cfg.Vault, r); err == nil {
				fmt.Fprintf(w, "  %s\n", filepath.ToSlash(p))
			} else {
				fmt.Fprintf(w, "  %s\n", r)
			}
		}
	}
	if res.MOCAppended != "" {
		if p, err := filepath.Rel(cfg.Vault, res.MOCAppended); err == nil {
			fmt.Fprintf(w, "appended to MOC: %s\n", filepath.ToSlash(p))
		} else {
			fmt.Fprintf(w, "appended to MOC: %s\n", res.MOCAppended)
		}
	}
	return nil
}
