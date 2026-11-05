package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/search"
)

func newSearchCmd(a *app) *cobra.Command {
	var (
		opts search.SearchOptions
		tier string
		mode string
	)
	c := &cobra.Command{
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
  f:regex          Restrict to vault-relative paths (also file:)
  lang:markdown    Restrict to markdown
  case:yes         Force case-sensitive

Filters:
  --tier           Restrict to inbox, notes, outputs, or MOCs
  --tag            Frontmatter tag filter
  --type           Frontmatter type filter
  --status         Frontmatter status filter
  --mode           files_with_matches (default) | content

Index is built/refreshed automatically if stale. With -o json, prints the
brain_search document.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode == "files" {
				mode = search.ModeFiles
			}
			if mode != search.ModeFiles && mode != search.ModeContent {
				return usageErrorf(
					"--mode must be %s or %s, got %q",
					search.ModeFiles,
					search.ModeContent,
					mode,
				)
			}
			t, err := parseTierFlag(tier)
			if err != nil {
				return err
			}
			cfg, _, err := openVault()
			if err != nil {
				return err
			}
			indexDir, err := ensureIndexCLI(cfg)
			if err != nil {
				return err
			}
			opts.Pattern = args[0]
			opts.Tier = t
			opts.OutputMode = mode
			matches, err := search.Search(context.Background(), indexDir, opts)
			if err != nil {
				return err
			}
			res := search.NewResult(cfg.Vault, mode, matches)
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				writeSearchText(w, res)
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVar(&tier, "tier", "", "restrict to a tier (inbox, notes, outputs, MOCs)")
	f.StringVar(&opts.Tag, "tag", "", "filter by frontmatter tag")
	f.StringVar(&opts.Type, "type", "", "filter by frontmatter type")
	f.StringVar(&opts.Status, "status", "", "filter by frontmatter status")
	f.StringVarP(&opts.File, "file", "f", "", "restrict to file paths matching this regex")
	f.StringVarP(&opts.Lang, "lang", "l", "", "restrict to files of this language")
	f.IntVar(&opts.Limit, "limit", 50, "maximum number of file results")
	f.StringVarP(&mode, "mode", "m", search.ModeFiles, "output mode: files_with_matches | content")
	f.IntVarP(
		&opts.ContextLines,
		"context",
		"C",
		0,
		"context lines around each match (content mode only)",
	)
	f.BoolVar(&opts.CaseSensitive, "case-sensitive", false, "force case-sensitive matching")
	return c
}

func writeSearchText(w io.Writer, res search.Result) {
	if res.Total == 0 {
		fmt.Fprintln(w, "no matches")
		return
	}
	for _, l := range res.Lines {
		fmt.Fprint(w, l.Before)
		fmt.Fprintf(w, "%s:%d:%d: %s\n", l.Path, l.Line, l.Column, l.Text)
		fmt.Fprint(w, l.After)
	}
	for _, f := range res.Files {
		fmt.Fprintln(w, f.Path)
	}
}
