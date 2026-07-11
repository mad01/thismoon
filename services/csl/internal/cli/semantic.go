package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	pb "github.com/mad01/thismoon/services/csl/internal/daemon/proto"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

var (
	semanticRepoFlag   string
	semanticLangFlag   string
	semanticKFlag      int
	semanticExpandFlag int
	semanticJSONFlag   bool
)

// semanticResult is the JSON/human shape for one semantic hit, unifying the
// daemon and in-process result types.
type semanticResult struct {
	Repo      string  `json:"repo"`
	Path      string  `json:"path"`
	Lang      string  `json:"lang,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"`
	Snippet   string  `json:"snippet,omitempty"`
}

var semanticCmd = &cobra.Command{
	Use:   "semantic <query>",
	Short: "Search code by meaning using vector embeddings",
	Long: `Search code semantically (by meaning/intent) across all indexed repositories.

Unlike 'csl search' (lexical/regex), this matches natural-language queries and
synonyms against an embedding index. Build the index first with:

  csl index --semantic-all

Examples:
  csl semantic "where do we retry failed HTTP requests"
  csl semantic "parse configuration file" --lang go --k 5
  csl semantic "auth middleware" --repo thismoon --expand 3`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSemantic,
}

func init() {
	semanticCmd.Flags().StringVar(&semanticRepoFlag, "repo", "", "restrict to repos whose name contains this substring")
	semanticCmd.Flags().StringVar(&semanticLangFlag, "lang", "", "restrict to a single language (e.g. go, typescript, python)")
	semanticCmd.Flags().IntVar(&semanticKFlag, "k", 10, "maximum number of results")
	semanticCmd.Flags().IntVar(&semanticExpandFlag, "expand", 0, "extra context lines around each matched chunk")
	semanticCmd.Flags().BoolVar(&semanticJSONFlag, "json", false, "output as JSON")
	rootCmd.AddCommand(semanticCmd)
}

func runSemantic(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"query is required\n\nUsage: csl semantic <query>\nRun 'csl semantic --help' for examples",
		)
	}
	query := args[0]
	if semanticKFlag <= 0 {
		semanticKFlag = 10
	}

	indexDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		return err
	}
	filter := semantic.Filter{}
	if semanticRepoFlag != "" {
		filter.Repos = []string{semanticRepoFlag}
	}
	if semanticLangFlag != "" {
		filter.Langs = []string{semanticLangFlag}
	}

	results, available, err := semanticQuery(cmd, indexDir, query, filter)
	if err != nil {
		return err
	}
	if !available {
		fmt.Fprintln(
			cmd.ErrOrStderr(),
			"semantic search unavailable — build the index first:\n  csl index --semantic-all",
		)
		return nil
	}
	return outputSemanticResults(cmd, results)
}

// semanticQuery tries the daemon first (it holds both indexes) then falls back
// to in-process. The bool reports whether the semantic index/model is ready.
func semanticQuery(
	cmd *cobra.Command,
	indexDir, query string,
	filter semantic.Filter,
) ([]semanticResult, bool, error) {
	socketPath := daemon.DefaultSocketPath()
	if lexDir, lexErr := search.DefaultIndexDir(); lexErr == nil {
		if err := daemon.EnsureDaemon(lexDir, socketPath); err == nil {
			resp, derr := daemon.SemanticSearchVia(socketPath, &pb.SemanticSearchRequest{
				Query:  query,
				K:      int32(semanticKFlag),
				Repos:  filter.Repos,
				Langs:  filter.Langs,
				Expand: int32(semanticExpandFlag),
			})
			if derr == nil {
				if !resp.Available {
					return nil, false, nil
				}
				return resultsFromProto(resp.Hits), true, nil
			}
			fmt.Fprintf(
				cmd.ErrOrStderr(),
				"daemon semantic search failed, falling back to in-process: %v\n",
				derr,
			)
		}
	}
	return semanticInProcess(indexDir, query, filter)
}

func semanticInProcess(
	indexDir, query string,
	filter semantic.Filter,
) ([]semanticResult, bool, error) {
	emb := semantic.NewDefaultEmbedder()
	results, err := semantic.SearchInProcess(
		context.Background(), indexDir, emb, query, semanticKFlag, filter, semanticExpandFlag,
	)
	if err != nil {
		return nil, false, err
	}
	if results == nil {
		return nil, false, nil
	}
	return resultsFromInProcess(results), true, nil
}

func resultsFromProto(hits []*pb.SemanticHit) []semanticResult {
	out := make([]semanticResult, len(hits))
	for i, h := range hits {
		out[i] = semanticResult{
			Repo:      h.Repo,
			Path:      h.File,
			Lang:      h.Lang,
			Kind:      h.Kind,
			StartLine: int(h.StartLine),
			EndLine:   int(h.EndLine),
			Score:     h.Score,
			Snippet:   h.Snippet,
		}
	}
	return out
}

func resultsFromInProcess(results []semantic.Result) []semanticResult {
	out := make([]semanticResult, len(results))
	for i, r := range results {
		out[i] = semanticResult{
			Repo:      r.Repo,
			Path:      r.Path,
			Lang:      r.Lang,
			Kind:      r.Kind,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Score:     r.Score,
			Snippet:   r.Snippet,
		}
	}
	return out
}

func outputSemanticResults(cmd *cobra.Command, results []semanticResult) error {
	w := cmd.OutOrStdout()

	if semanticJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	if len(results) == 0 {
		fmt.Fprintln(w, "no matches")
		return nil
	}

	for _, r := range results {
		fmt.Fprintf(
			w, "%s %s:%d-%d  score=%.2f  [%s]\n",
			r.Repo, r.Path, r.StartLine, r.EndLine, r.Score, r.Kind,
		)
		if r.Snippet != "" {
			for _, line := range splitLines(r.Snippet) {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
	}
	return nil
}

// splitLines splits s on newlines without dropping a meaningful empty result.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
