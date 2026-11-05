package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

func newSemanticSearchCmd(a *app) *cobra.Command {
	var opts semantic.QueryOptions
	c := &cobra.Command{
		Use:   "semantic-search <query>",
		Short: "Vector search across the vault (semantic; requires Ollama)",
		Long: `Vector search using Ollama embeddings + chromem-go.

Query is converted to an embedding and compared against every chunk in the
semantic index. Results are ranked by cosine similarity. Use brain search for
exact-string lookups; use brain hybrid-search to combine literal + semantic.

Requires semantic.enabled=true in config.yaml and a running Ollama instance.
With -o json, prints the brain_semantic_search document (snippets mode).`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if !cfg.Semantic.Enabled {
				return errSemanticDisabled
			}
			idx, err := openSemanticIndex(cfg)
			if err != nil {
				return err
			}
			hits, err := idx.Query(context.Background(), args[0], opts)
			if err != nil {
				return err
			}
			res := semantic.NewSearchResult(semantic.ModeSnippets, hits)
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				if res.Total == 0 {
					fmt.Fprintln(w, "no matches")
				}
				for _, h := range res.Hits {
					fmt.Fprintf(
						w,
						"%.4f  %s  %s\n",
						h.Score,
						h.Path,
						headingOrPrelude(h.HeadingPath),
					)
				}
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&opts.Tier, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	f.StringVar(&opts.Tag, "tag", "", "restrict to notes with this frontmatter tag")
	f.StringVar(&opts.Type, "type", "", "restrict to notes with this frontmatter type")
	f.StringVar(&opts.Status, "status", "", "restrict to notes with this frontmatter status")
	f.StringVar(
		&opts.Evidence,
		"evidence",
		"",
		"restrict to notes with this evidence tier (A/B/C/D/mixed)",
	)
	f.IntVarP(&opts.K, "k", "k", 20, "number of results to return")
	return c
}

// errSemanticDisabled is returned by the semantic commands when the config
// leaves the semantic layer off.
var errSemanticDisabled = errors.New(
	"semantic.enabled is false in ~/.config/brain/config.yaml (check with: brain doctor)",
)

func headingOrPrelude(heading string) string {
	if heading == "" {
		return "(prelude)"
	}
	return heading
}
