package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	pb "github.com/mad01/thismoon/services/csl/internal/daemon/proto"
	"github.com/mad01/thismoon/services/csl/internal/hybrid"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

var (
	hybridRepoFlag   string
	hybridLangFlag   string
	hybridLimitFlag  int
	hybridRRFKFlag   int
	hybridExpandFlag int
	hybridJSONFlag   bool
)

// hybridJSONHit is the JSON shape for one fused result.
type hybridJSONHit struct {
	Repo     string  `json:"repo"`
	Path     string  `json:"path"`
	Score    float64 `json:"score"`
	LexRank  int     `json:"lex_rank"`
	LexLine  int     `json:"lex_line,omitempty"`
	LexText  string  `json:"lex_text,omitempty"`
	SemRank  int     `json:"sem_rank"`
	SemStart int     `json:"sem_start,omitempty"`
	SemEnd   int     `json:"sem_end,omitempty"`
	SemScore float32 `json:"sem_score,omitempty"`
	Snippet  string  `json:"snippet,omitempty"`
}

var hybridCmd = &cobra.Command{
	Use:   "hybrid <query>",
	Short: "Search code by combining lexical and semantic ranking (RRF)",
	Long: `Hybrid search: run both lexical (zoekt) and semantic (vector) search for the
same query and fuse the two rankings with Reciprocal Rank Fusion (RRF).

A file that ranks well in BOTH backends rises above a file that only tops one:
this catches exact-match cases that pure semantic misses and meaning-based
matches that pure lexical misses. Build the semantic index first with:

  csl index --semantic-all

If the semantic index isn't built, results degrade to lexical-only (a note is
printed to stderr).

Examples:
  csl hybrid "where do we retry failed HTTP requests"
  csl hybrid "parse configuration file" --lang go --limit 10
  csl hybrid "auth middleware" --repo thismoon --rrf-k 40 --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHybrid,
}

func init() {
	hybridCmd.Flags().
		StringVarP(&hybridRepoFlag, "repo", "r", "", "restrict to repos whose name contains this substring")
	hybridCmd.Flags().
		StringVarP(&hybridLangFlag, "lang", "l", "", "restrict to a single language (e.g. go, typescript, python)")
	hybridCmd.Flags().IntVar(&hybridLimitFlag, "limit", 50, "maximum number of fused results")
	hybridCmd.Flags().
		IntVar(&hybridRRFKFlag, "rrf-k", hybrid.DefaultK, "RRF smoothing constant (lower favors top-ranked outliers, higithostr favors consensus)")
	hybridCmd.Flags().
		IntVar(&hybridExpandFlag, "expand", 0, "extra context lines around each semantic chunk")
	hybridCmd.Flags().BoolVar(&hybridJSONFlag, "json", false, "output as JSON")
	rootCmd.AddCommand(hybridCmd)
}

func runHybrid(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"query is required\n\nUsage: csl hybrid <query>\nRun 'csl hybrid --help' for examples",
		)
	}
	query := args[0]
	if hybridLimitFlag <= 0 {
		hybridLimitFlag = 50
	}

	lexMatches, err := hybridLexicalMatches(cmd, query)
	if err != nil {
		return err
	}

	semResults, available, err := hybridSemanticResults(cmd, query)
	if err != nil {
		return err
	}
	if !available {
		fmt.Fprintln(
			cmd.ErrOrStderr(),
			"semantic index unavailable — showing lexical-only results. Build it with:\n  csl index --semantic-all",
		)
	}

	fused := hybrid.Fuse(lexMatches, semResults, hybridRRFKFlag, hybridLimitFlag)
	return outputHybridResults(cmd, fused)
}

// hybridLexicalMatches runs the lexical backend in content mode (daemon-first,
// in-process fallback) so fusion has a ranked list of per-line matches.
func hybridLexicalMatches(cmd *cobra.Command, query string) ([]search.Match, error) {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, nil
	}
	repoNames := make(map[string]string, len(repos))
	for _, r := range repos {
		repoNames[r.Name] = r.Path
	}

	opts := search.SearchOptions{
		Pattern:    query,
		RepoFilter: hybridRepoFlag,
		Lang:       hybridLangFlag,
		Limit:      hybridLimitFlag,
		OutputMode: "content",
	}

	socketPath := daemon.DefaultSocketPath()
	if err := daemon.EnsureDaemon(indexDir, socketPath); err == nil {
		matches, derr := daemon.SearchVia(context.Background(), socketPath, opts, repoNames)
		if derr == nil {
			return matches, nil
		}
		fmt.Fprintf(
			cmd.ErrOrStderr(),
			"daemon lexical search failed, falling back to in-process: %v\n",
			derr,
		)
	}
	return search.Search(context.Background(), indexDir, opts, repoNames)
}

// hybridSemanticResults runs the semantic backend (daemon-first, in-process
// fallback). The bool reports whether the semantic index/model is ready.
func hybridSemanticResults(cmd *cobra.Command, query string) ([]semantic.Result, bool, error) {
	indexDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		return nil, false, err
	}
	filter := semantic.Filter{}
	if hybridRepoFlag != "" {
		filter.Repos = []string{hybridRepoFlag}
	}
	if hybridLangFlag != "" {
		filter.Langs = []string{hybridLangFlag}
	}

	socketPath := daemon.DefaultSocketPath()
	if lexDir, lexErr := search.DefaultIndexDir(); lexErr == nil {
		if err := daemon.EnsureDaemon(lexDir, socketPath); err == nil {
			resp, derr := daemon.SemanticSearchVia(socketPath, &pb.SemanticSearchRequest{
				Query:  query,
				K:      int32(hybridLimitFlag),
				Repos:  filter.Repos,
				Langs:  filter.Langs,
				Expand: int32(hybridExpandFlag),
			})
			if derr == nil {
				if !resp.Available {
					return nil, false, nil
				}
				return hybridResultsFromProto(resp.Hits), true, nil
			}
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"daemon semantic search failed, falling back to in-process: %v\n",
				derr,
			)
		}
	}

	emb := semantic.NewDefaultEmbedder()
	results, err := semantic.SearchInProcess(
		context.Background(), indexDir, emb, query, hybridLimitFlag, filter, hybridExpandFlag,
	)
	if err != nil {
		return nil, false, err
	}
	if results == nil {
		return nil, false, nil
	}
	return results, true, nil
}

func hybridResultsFromProto(hits []*pb.SemanticHit) []semantic.Result {
	out := make([]semantic.Result, len(hits))
	for i, h := range hits {
		out[i] = semantic.Result{
			Hit: semantic.Hit{
				Repo:      h.Repo,
				Path:      h.File,
				Lang:      h.Lang,
				Kind:      h.Kind,
				StartLine: int(h.StartLine),
				EndLine:   int(h.EndLine),
				Score:     h.Score,
			},
			Snippet: h.Snippet,
		}
	}
	return out
}

func outputHybridResults(cmd *cobra.Command, fused []hybrid.FusedHit) error {
	w := cmd.OutOrStdout()

	if hybridJSONFlag {
		hits := make([]hybridJSONHit, len(fused))
		for i, f := range fused {
			hits[i] = hybridJSONHit{
				Repo: f.Repo, Path: f.Path, Score: f.Score,
				LexRank: f.LexRank, LexLine: f.LexLine, LexText: f.LexText,
				SemRank: f.SemRank, SemStart: f.SemStart, SemEnd: f.SemEnd,
				SemScore: f.SemScore, Snippet: f.Snippet,
			}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(hits)
	}

	if len(fused) == 0 {
		fmt.Fprintln(w, "no matches")
		return nil
	}

	for _, f := range fused {
		fmt.Fprintf(w, "%s/%s  score=%.4f  (lex#%s sem#%s)\n",
			f.Repo, f.Path, f.Score, rankStr(f.LexRank), rankStr(f.SemRank))
		if f.LexRank != 0 && f.LexText != "" {
			fmt.Fprintf(w, "    lex L%d: %s\n", f.LexLine, f.LexText)
		}
		if f.SemRank != 0 && f.Snippet != "" {
			fmt.Fprintf(w, "    sem L%d-%d (score=%.2f):\n", f.SemStart, f.SemEnd, f.SemScore)
			for _, line := range splitLines(f.Snippet) {
				fmt.Fprintf(w, "      %s\n", line)
			}
		}
	}
	return nil
}

// rankStr renders a 1-based rank, or "-" when the file was absent from that
// backend's results.
func rankStr(rank int) string {
	if rank == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", rank)
}
