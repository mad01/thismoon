package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/library"
)

func newHybridSearchCmd(a *app) *cobra.Command {
	var (
		q      library.HybridQuery
		rerank bool
	)
	c := &cobra.Command{
		Use:   "hybrid-search <query>",
		Short: "Hybrid (literal + semantic) search with RRF fusion",
		Long: `Hybrid search combining the literal (zoekt) and semantic (vector) layers
via Reciprocal Rank Fusion (RRF, k=60), across every vault unless --vault
narrows it. Use --rerank to additionally pass the top candidates through a
local LLM scorer (slower; default off). A stale semantic index is refreshed
first; when Ollama does not answer, the hits are literal only and a warning
says so on stderr.

Requires semantic.enabled=true in config.yaml and a running Ollama instance.
With -o json, prints the brain_hybrid_search document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, set, err := a.openVaults()
			if err != nil {
				return err
			}
			q.Text = args[0]
			q.Rerank = rerank || lib.Config().Semantic.RerankDefault
			res, err := lib.HybridSearch(context.Background(), set, q)
			if err != nil {
				return semanticErr(err)
			}
			warn(cmd, res.Warnings)
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				a.writeHybridText(w, res)
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&q.Tier, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	f.StringVar(&q.Tag, "tag", "", "restrict to notes with this frontmatter tag")
	f.StringVar(&q.Type, "type", "", "restrict to notes with this frontmatter type")
	f.StringVar(&q.Status, "status", "", "restrict to notes with this frontmatter status")
	f.StringVar(
		&q.Evidence,
		"evidence",
		"",
		"restrict to notes with this evidence tier (A/B/C/D/mixed)",
	)
	f.IntVarP(&q.K, "k", "k", 20, "final number of results to return")
	f.IntVar(&q.KZoekt, "k-zoekt", 30, "candidates pulled from the literal index before fusion")
	f.IntVar(
		&q.KVector,
		"k-vector",
		30,
		"candidates pulled from the semantic index before fusion",
	)
	f.BoolVar(&rerank, "rerank", false, "rerank fused candidates with the LLM (slower)")
	return c
}

func (a *app) writeHybridText(w io.Writer, res hybrid.Result) {
	if res.Total == 0 {
		fmt.Fprintln(w, "no matches")
	}
	for _, h := range res.Hits {
		fmt.Fprintf(
			w,
			"%.4f  [%s]  %s  %s\n",
			h.Score,
			strings.Join(h.Sources, "+"),
			a.show(h.Vault, h.Path),
			headingOrPrelude(h.HeadingPath),
		)
	}
}
