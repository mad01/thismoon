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
	searchTierFlag          string
	searchTagFlag           string
	searchTypeFlag          string
	searchStatusFlag        string
	searchFileFlag          string
	searchLangFlag          string
	searchLimitFlag         int
	searchOutputModeFlag    string
	searchContextLinesFlag  int
	searchCaseSensitiveFlag bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Zoekt query across the vault",
	Long: `Search across the vault using zoekt query syntax.

Query syntax:
  literal           Substring (smart-case by default)
  regex.*          Regex
  "exact phrase"   Phrase
  foo bar          AND
  foo|bar          OR
  -test            NOT
  file:regex       Restrict to vault-relative paths
  lang:markdown    Restrict to markdown
  case:yes         Force case-sensitive

Filters:
  --tier           Restrict to inbox, notes, outputs, or MOCs
  --tag            Frontmatter tag filter
  --type           Frontmatter type filter
  --status         Frontmatter status filter
  --mode           files_with_matches (default) | content

Index is built/refreshed automatically if stale.`,
	Args: cobra.ExactArgs(1),
	RunE: runSearchCmd,
}

func init() {
	searchCmd.Flags().
		StringVar(&searchTierFlag, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	searchCmd.Flags().StringVar(&searchTagFlag, "tag", "", "filter by frontmatter tag")
	searchCmd.Flags().StringVar(&searchTypeFlag, "type", "", "filter by frontmatter type")
	searchCmd.Flags().StringVar(&searchStatusFlag, "status", "", "filter by frontmatter status")
	searchCmd.Flags().
		StringVarP(&searchFileFlag, "file", "f", "", "restrict to file paths matching this regex")
	searchCmd.Flags().
		StringVarP(&searchLangFlag, "lang", "l", "", "restrict to files of this language")
	searchCmd.Flags().IntVar(&searchLimitFlag, "limit", 50, "maximum number of file results")
	searchCmd.Flags().
		StringVarP(&searchOutputModeFlag, "mode", "m", "files_with_matches", "output mode: files_with_matches | content")
	searchCmd.Flags().
		IntVarP(&searchContextLinesFlag, "context", "C", 0, "context lines around each match (content mode only)")
	searchCmd.Flags().
		BoolVar(&searchCaseSensitiveFlag, "case-sensitive", false, "force case-sensitive matching")
	rootCmd.AddCommand(searchCmd)
}

func runSearchCmd(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	mode := searchOutputModeFlag
	if mode == "files" {
		mode = "files_with_matches"
	}
	if mode != "files_with_matches" && mode != "content" {
		return fmt.Errorf(
			"brain search: --mode must be files_with_matches or content, got %q",
			mode,
		)
	}

	var tier vault.Tier
	if searchTierFlag != "" {
		t, err := vault.ParseTier(searchTierFlag)
		if err != nil {
			return fmt.Errorf("brain search: %w", err)
		}
		tier = t
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain search: %w", err)
	}
	indexDir, err := ensureIndexCLI(cfg)
	if err != nil {
		return fmt.Errorf("brain search: %w", err)
	}

	opts := search.SearchOptions{
		Pattern:       pattern,
		File:          searchFileFlag,
		Tag:           searchTagFlag,
		Type:          searchTypeFlag,
		Status:        searchStatusFlag,
		Tier:          tier,
		Lang:          searchLangFlag,
		CaseSensitive: searchCaseSensitiveFlag,
		Limit:         searchLimitFlag,
		ContextLines:  searchContextLinesFlag,
		OutputMode:    mode,
	}

	matches, err := search.Search(context.Background(), indexDir, opts)
	if err != nil {
		return fmt.Errorf("brain search: %w", err)
	}

	w := cmd.OutOrStdout()
	if len(matches) == 0 {
		fmt.Fprintln(w, "no matches")
		return nil
	}

	if mode == "content" {
		for _, m := range matches {
			if m.Before != "" {
				fmt.Fprint(w, m.Before)
			}
			fmt.Fprintf(w, "%s:%d:%d: %s\n", m.Path, m.Line, m.Column, m.Text)
			if m.After != "" {
				fmt.Fprint(w, m.After)
			}
		}
		return nil
	}

	seen := make(map[string]bool)
	for _, m := range matches {
		if seen[m.Path] {
			continue
		}
		seen[m.Path] = true
		fmt.Fprintln(w, m.Path)
	}
	return nil
}
