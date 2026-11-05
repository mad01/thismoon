package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/hybrid"
	"github.com/mad01/thismoon/services/brain/internal/rerank"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newHybridSearchCmd(a *app) *cobra.Command {
	var (
		opts   hybridQueryOptions
		rerank bool
	)
	c := &cobra.Command{
		Use:   "hybrid-search <query>",
		Short: "Hybrid (literal + semantic) search with RRF fusion",
		Long: `Hybrid search combining the literal (zoekt) and semantic (vector) layers
via Reciprocal Rank Fusion (RRF, k=60). Use --rerank to additionally pass the
top candidates through a local LLM scorer (slower; default off).

Requires semantic.enabled=true in config.yaml and a running Ollama instance.
With -o json, prints the brain_hybrid_search document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if !cfg.Semantic.Enabled {
				return errSemanticDisabled
			}
			opts.Query = args[0]
			opts.Rerank = rerank || cfg.Semantic.RerankDefault
			hits, err := runHybridQuery(context.Background(), cfg, opts)
			if err != nil {
				return err
			}
			res := hybrid.NewResult(hits)
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				if res.Total == 0 {
					fmt.Fprintln(w, "no matches")
				}
				for _, h := range res.Hits {
					fmt.Fprintf(
						w,
						"%.4f  [%s]  %s  %s\n",
						h.Score,
						strings.Join(h.Sources, "+"),
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
	f.IntVarP(&opts.K, "k", "k", 20, "final number of results to return")
	f.IntVar(&opts.KZoekt, "k-zoekt", 30, "candidates pulled from the literal index before fusion")
	f.IntVar(
		&opts.KVector,
		"k-vector",
		30,
		"candidates pulled from the semantic index before fusion",
	)
	f.BoolVar(&rerank, "rerank", false, "rerank fused candidates with the LLM (slower)")
	return c
}

// hybridQueryOptions is the shared input shape for both the CLI and the MCP
// handler, so the two surfaces never drift in default behaviour.
type hybridQueryOptions struct {
	Query    string
	K        int
	KZoekt   int
	KVector  int
	Tier     string
	Tag      string
	Type     string
	Status   string
	Evidence string
	Rerank   bool
}

// runHybridQuery executes the literal + semantic pull, RRF-fuses them, and
// optionally reranks.
func runHybridQuery(
	ctx context.Context,
	cfg *config.Config,
	in hybridQueryOptions,
) ([]hybrid.ResultHit, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if in.K <= 0 {
		in.K = 20
	}
	if in.KZoekt <= 0 {
		in.KZoekt = 30
	}
	if in.KVector <= 0 {
		in.KVector = 30
	}

	// --- semantic pull ---
	idx, err := openSemanticIndex(cfg)
	if err != nil {
		return nil, fmt.Errorf("semantic open: %w", err)
	}
	semHits, err := idx.Query(ctx, in.Query, semantic.QueryOptions{
		K:        in.KVector,
		Tier:     in.Tier,
		Tag:      in.Tag,
		Type:     in.Type,
		Status:   in.Status,
		Evidence: in.Evidence,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic query: %w", err)
	}

	// --- literal pull (path-keyed, since zoekt doesn't chunk) ---
	indexDir, err := ensureIndexCLI(cfg)
	if err != nil {
		return nil, fmt.Errorf("ensure literal index: %w", err)
	}
	var tier vault.Tier
	if in.Tier != "" {
		t, perr := vault.ParseTier(in.Tier)
		if perr != nil {
			return nil, fmt.Errorf("tier: %w", perr)
		}
		tier = t
	}
	zMatches, err := search.Search(ctx, indexDir, search.SearchOptions{
		Pattern: in.Query,
		Tag:     in.Tag,
		Type:    in.Type,
		Status:  in.Status,
		Tier:    tier,
		Limit:   in.KZoekt,
	})
	if err != nil {
		return nil, fmt.Errorf("literal query: %w", err)
	}
	zoektPaths := dedupePaths(zMatches)

	// --- fuse: identify by path so chunked semantic hits and path-level
	//     literal hits land in the same bucket. ---
	semByPath := make(map[string]semantic.Hit)
	semOrder := make([]string, 0, len(semHits))
	for _, h := range semHits {
		// Keep the highest-scoring chunk per path as the representative.
		if cur, ok := semByPath[h.Path]; !ok || h.Score > cur.Score {
			if !ok {
				semOrder = append(semOrder, h.Path)
			}
			semByPath[h.Path] = h
		}
	}
	// Re-sort semOrder by best-chunk score so RRF input ranks reflect the
	// per-path best signal.
	rebuiltOrder := append(make([]string, 0, len(semByPath)), semOrder...)
	// Stable sort by descending score.
	for i := 1; i < len(rebuiltOrder); i++ {
		for j := i; j > 0 && semByPath[rebuiltOrder[j]].Score > semByPath[rebuiltOrder[j-1]].Score; j-- {
			rebuiltOrder[j], rebuiltOrder[j-1] = rebuiltOrder[j-1], rebuiltOrder[j]
		}
	}

	fused := hybrid.Fuse([]hybrid.Input{
		{Source: "zoekt", IDs: zoektPaths},
		{Source: "vector", IDs: rebuiltOrder},
	}, hybrid.DefaultK)

	// Walk fused list; emit hybrid.ResultHit per path with semantic metadata when
	// available, falling back to literal-only paths.
	out := make([]hybrid.ResultHit, 0, in.K)
	for _, f := range fused {
		if len(out) >= in.K {
			break
		}
		hh := hybrid.ResultHit{
			Path:    f.ID,
			Score:   f.Score,
			Sources: f.Sources,
		}
		if sh, ok := semByPath[f.ID]; ok {
			hh.Title = sh.Title
			hh.HeadingPath = sh.HeadingPath
			hh.Snippet = sh.Snippet
			hh.Tier = sh.Tier
			hh.Tags = sh.Tags
			hh.Type = sh.Type
			hh.Status = sh.Status
			hh.Evidence = sh.Evidence
		}
		out = append(out, hh)
	}

	// --- optional reranker ---
	if in.Rerank && len(out) > 3 {
		r := rerank.New(cfg.Semantic.OllamaURL, cfg.Semantic.ChatModel)
		cands := make([]rerank.Candidate, 0, len(out))
		for i, h := range out {
			snippet := h.Snippet
			if snippet == "" {
				snippet = h.Path
			}
			cands = append(cands, rerank.Candidate{
				ID:            h.Path,
				Snippet:       snippet,
				RetrievalRank: i + 1,
				Score:         h.Score,
			})
		}
		scored, err := r.Score(ctx, in.Query, cands)
		if err != nil {
			// Reranker failures are non-fatal — return the fused list.
			return out, nil //nolint:nilerr // preserve fused list on rerank failure
		}
		// Reorder out by scored order.
		byID := make(map[string]hybrid.ResultHit, len(out))
		for _, h := range out {
			byID[h.Path] = h
		}
		reordered := make([]hybrid.ResultHit, 0, len(scored))
		for _, s := range scored {
			if h, ok := byID[s.ID]; ok {
				h.Score = s.Final
				reordered = append(reordered, h)
			}
		}
		out = reordered
	}

	return out, nil
}

// dedupePaths returns unique paths from match list in input order.
func dedupePaths(ms []search.Match) []string {
	seen := make(map[string]struct{}, len(ms))
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if _, ok := seen[m.Path]; ok {
			continue
		}
		seen[m.Path] = struct{}{}
		out = append(out, m.Path)
	}
	return out
}
