package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/export"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var exportCmd = &cobra.Command{
	Use:   "export <id>",
	Short: "Write a page as one HTML file that opens without present",
	Long: `Write a page as one HTML file that opens in any browser, offline included,
with no present server behind it. The file carries the page's markup, styles,
and scripts, its page data, and its local images, so prose and fixation
reading work offline and with scripts off. Graph, chart, and diagram
libraries load from cdn.jsdelivr.net at the versions the asset cache holds,
each pinned by an integrity hash, and only the ones the page uses. A terminal
block shows a note, since it needs the local serve.

The brief is exported unless --deck asks for the slide deck; a page with only
a deck exports its deck. The file is <id>.html (<id>-deck.html for a deck) in
the current directory unless -o names another path. It needs only the
workdir and its asset cache, so it works with serve running or not.`,
	Args: cobra.ExactArgs(1),
	RunE: runExport,
}

var (
	flagExportOutput string
	flagExportDeck   bool
)

func init() {
	exportCmd.Flags().StringVarP(&flagExportOutput, "output", "o", "",
		"file to write (default <id>.html, or <id>-deck.html for a deck, in the current directory)")
	exportCmd.Flags().BoolVar(&flagExportDeck, "deck", false,
		"export the page's slide deck instead of its brief")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	id := args[0]
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return err
	}
	res, err := export.New(st, flagWorkdir).
		Export(context.Background(), export.Request{ID: id, Deck: flagExportDeck})
	if err != nil {
		return err
	}
	path := flagExportOutput
	if path == "" {
		path = export.FileName(id, res.Rendition)
	}
	if err := export.WriteFile(path, res.HTML); err != nil {
		return err
	}
	for _, note := range res.Notes {
		fmt.Fprintf(cmd.ErrOrStderr(), "present: %s\n", note)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %s, %d bytes, %d libraries from the CDN\n",
		id, res.Rendition, path, len(res.HTML), len(res.Libraries))
	return nil
}
