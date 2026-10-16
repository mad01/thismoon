package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var (
	flagProgressNote    string
	flagProgressSection string
	flagProgressDeck    bool
)

var progressCmd = &cobra.Command{
	Use:   "progress <id> <label> <pct>",
	Short: "Report progress into a page's progress block",
	Long: `Set the progress block labelled <label> on a page to <pct> (0 to 100),
or add one when the page has none with that label. --note shows a short line
after the label under the bar. The block is looked for in every section, or
only in --section (a section's table-of-contents anchor, its heading, or
index:N). A new block goes to --section, else to a Progress section at the
end of the page, created the first time.

The command edits the page in the workdir under the page lock, so it works
with serve running or not, beside the MCP server and other reporters, which
makes it fit for hooks. Open tabs reload on the new version. --deck reports
into the deck instead of the brief.`,
	Args: cobra.ExactArgs(3),
	RunE: runProgress,
}

func init() {
	progressCmd.Flags().StringVar(&flagProgressNote, "note", "", "short line shown after the label")
	progressCmd.Flags().StringVar(&flagProgressSection, "section", "",
		"section to report in: anchor, heading, or index:N")
	progressCmd.Flags().BoolVar(&flagProgressDeck, "deck", false,
		"report into the deck, not the brief")
	rootCmd.AddCommand(progressCmd)
}

func runProgress(cmd *cobra.Command, args []string) error {
	id, label := args[0], args[1]
	pct, err := strconv.ParseFloat(args[2], 64)
	if err != nil {
		return fmt.Errorf("pct %q: want a number from 0 to 100", args[2])
	}
	rendition := docedit.Content
	if flagProgressDeck {
		rendition = docedit.Deck
	}
	p, err := docedit.ParseProgress(docedit.ProgressRequest{
		Label: label, Percent: &pct, Note: flagProgressNote, Section: flagProgressSection,
		Rendition: string(rendition),
	})
	if err != nil {
		return err
	}
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return err
	}
	var res docedit.ProgressResult
	page, err := st.Edit(context.Background(), id, p.Edit(&res))
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%s: %w (workdir %s)", id, err, flagWorkdir)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %s%% in %s, version %d\n",
		id, p.Label, strconv.FormatFloat(p.Percent, 'f', -1, 64), res.Anchor, page.Version)
	return nil
}
