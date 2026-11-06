package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

func newSemanticSearchCmd(a *app) *cobra.Command {
	var (
		opts           semantic.QueryOptions
		includeRevoked bool
	)
	c := &cobra.Command{
		Use:   "semantic-search <query>",
		Short: "Vector search across the vault (semantic; requires Ollama)",
		Long: `Vector search using Ollama embeddings + chromem-go.

Query is converted to an embedding and compared against every chunk in the
semantic index of each vault (every vault unless --vault narrows it). Results
are ranked by cosine similarity. A stale semantic index is refreshed first;
when Ollama does not answer, the refresh is skipped with a warning on stderr.
Use brain search for exact-string lookups; use brain hybrid-search to combine
literal + semantic.

Requires semantic.enabled=true in config.yaml and a running Ollama instance.
With -o json, prints the brain_semantic_search document (snippets mode).`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, set, err := a.openVaults()
			if err != nil {
				return err
			}
			res, err := lib.SemanticSearch(context.Background(), set, library.SemanticQuery{
				Text:           args[0],
				Mode:           semantic.ModeSnippets,
				Filter:         opts,
				IncludeRevoked: includeRevoked,
			})
			if err != nil {
				return semanticErr(err)
			}
			warn(cmd, res.Warnings)
			return a.emit(cmd.OutOrStdout(), res, func(w io.Writer) error {
				if res.Total == 0 {
					fmt.Fprintln(w, "no matches")
				}
				for _, h := range res.Hits {
					fmt.Fprintf(
						w,
						"%.4f  %s  %s\n",
						h.Score,
						a.show(h.Vault, h.Path),
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
	f.BoolVar(&includeRevoked, "include-revoked", false, includeRevokedHelp)
	return c
}

// semanticErr points a disabled semantic layer at brain doctor.
func semanticErr(err error) error {
	if errors.Is(err, library.ErrSemanticDisabled) {
		return fmt.Errorf("%w (check with: brain doctor)", err)
	}
	return err
}

func headingOrPrelude(heading string) string {
	if heading == "" {
		return "(prelude)"
	}
	return heading
}
