package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

var (
	semSearchTierFlag     string
	semSearchTagFlag      string
	semSearchTypeFlag     string
	semSearchStatusFlag   string
	semSearchEvidenceFlag string
	semSearchKFlag        int
)

var semanticSearchCmd = &cobra.Command{
	Use:   "semantic-search <query>",
	Short: "Vector search across the vault (semantic; requires Ollama)",
	Long: `Vector search using Ollama embeddings + chromem-go.

Query is converted to an embedding and compared against every chunk in the
semantic index. Results are ranked by cosine similarity. Use brain search for
exact-string lookups; use brain hybrid-search to combine literal + semantic.

Requires semantic.enabled=true in config.yaml and a running Ollama instance.`,
	Args: cobra.ExactArgs(1),
	RunE: runSemanticSearch,
}

func init() {
	semanticSearchCmd.Flags().
		StringVar(&semSearchTierFlag, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	semanticSearchCmd.Flags().
		StringVar(&semSearchTagFlag, "tag", "", "restrict to notes with this frontmatter tag")
	semanticSearchCmd.Flags().
		StringVar(&semSearchTypeFlag, "type", "", "restrict to notes with this frontmatter type")
	semanticSearchCmd.Flags().
		StringVar(&semSearchStatusFlag, "status", "", "restrict to notes with this frontmatter status")
	semanticSearchCmd.Flags().
		StringVar(&semSearchEvidenceFlag, "evidence", "", "restrict to notes with this evidence tier (A/B/C/D/mixed)")
	semanticSearchCmd.Flags().IntVarP(&semSearchKFlag, "k", "k", 20, "number of results to return")
	rootCmd.AddCommand(semanticSearchCmd)
}

func runSemanticSearch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain semantic-search: %w", err)
	}
	if !cfg.Semantic.Enabled {
		return fmt.Errorf(
			"brain semantic-search: semantic.enabled is false in ~/.config/brain/config.yaml",
		)
	}

	idx, err := openSemanticIndex(cfg)
	if err != nil {
		return fmt.Errorf("brain semantic-search: %w", err)
	}

	hits, err := idx.Query(context.Background(), args[0], semantic.QueryOptions{
		K:        semSearchKFlag,
		Tier:     semSearchTierFlag,
		Tag:      semSearchTagFlag,
		Type:     semSearchTypeFlag,
		Status:   semSearchStatusFlag,
		Evidence: semSearchEvidenceFlag,
	})
	if err != nil {
		return fmt.Errorf("brain semantic-search: %w", err)
	}

	w := cmd.OutOrStdout()
	if len(hits) == 0 {
		fmt.Fprintln(w, "no matches")
		return nil
	}
	for _, h := range hits {
		head := h.HeadingPath
		if head == "" {
			head = "(prelude)"
		}
		fmt.Fprintf(w, "%.4f  %s  %s\n", h.Score, h.Path, head)
	}
	return nil
}
