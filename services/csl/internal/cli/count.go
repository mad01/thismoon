package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

var (
	countJSONFlag    bool
	countRepoFlag    string
	countLangFlag    string
	countGroupByFlag string
	countNamesFlag   bool
)

var countCmd = &cobra.Command{
	Use:   "count <pattern>",
	Short: "Count matches across indexed repositories",
	Long: `Count matches for a pattern across all indexed repositories.

Results can be grouped by repo or language. Without --group-by,
a single total count is returned. With --group-by and --names, only the
group names print, one per line and no total, so the output is a list a
shell pipe can take as is. Unlike 'csl search', a count has no result
cap: every matching repo is listed.

Examples:
  csl count "TODO"
  csl count "func.*Error" --group-by repo
  csl count "import" --lang go --group-by language
  csl count "uses: .*/deploy" --group-by repo --names
  csl count "FIXME" --repo thismoon --json`,
	Args: cobra.ExactArgs(1),
	RunE: runCount,
}

func init() {
	countCmd.Flags().BoolVar(&countJSONFlag, "json", false, "output as JSON")
	countCmd.Flags().
		StringVarP(&countRepoFlag, "repo", "r", "", "filter to repos matching this name")
	countCmd.Flags().StringVarP(&countLangFlag, "lang", "l", "", "filter to a specific language")
	countCmd.Flags().StringVar(&countGroupByFlag, "group-by", "", "group counts by: repo, language")
	countCmd.Flags().BoolVar(&countNamesFlag, "names", false, "with --group-by: print only the group names, one per line")
	rootCmd.AddCommand(countCmd)
}

type countJSON struct {
	Total   int                  `json:"total"`
	Results []search.CountResult `json:"results,omitempty"`
}

func runCount(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	if countNamesFlag && countGroupByFlag == "" {
		return fmt.Errorf("--names needs --group-by repo or --group-by language")
	}

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return err
	}

	opts := search.CountOptions{
		Pattern:    pattern,
		RepoFilter: countRepoFlag,
		Lang:       countLangFlag,
		GroupBy:    countGroupByFlag,
	}

	// Try daemon first.
	socketPath := daemon.DefaultSocketPath()
	var results []search.CountResult
	var total int
	if err := daemon.EnsureDaemon(indexDir, socketPath); err == nil {
		results, total, err = daemon.CountVia(context.Background(), socketPath, opts)
		if err != nil {
			// Daemon failed — fall through to in-process.
			fmt.Fprintf(cmd.ErrOrStderr(), "daemon count failed, falling back: %v\n", err)
			results, total, err = search.Count(context.Background(), indexDir, opts)
			if err != nil {
				return err
			}
		}
	} else {
		var err error
		results, total, err = search.Count(context.Background(), indexDir, opts)
		if err != nil {
			return err
		}
	}

	return printCount(cmd.OutOrStdout(), results, total)
}

// printCount prints the count the flags ask for: JSON, one group per line
// with its count and a total, only the group names, or a single total.
func printCount(w io.Writer, results []search.CountResult, total int) error {
	if countJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(countJSON{Total: total, Results: results})
	}

	switch {
	case countNamesFlag:
		for _, r := range results {
			fmt.Fprintln(w, r.Group)
		}
	case countGroupByFlag != "":
		for _, r := range results {
			fmt.Fprintf(w, "%-40s %d\n", r.Group, r.Count)
		}
		fmt.Fprintf(w, "\ntotal: %d\n", total)
	default:
		fmt.Fprintf(w, "%d\n", total)
	}

	return nil
}
