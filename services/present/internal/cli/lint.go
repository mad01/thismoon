package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/limits"
	"github.com/mad01/thismoon/services/present/internal/lint"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var lintCmd = &cobra.Command{
	Use:   "lint [<id>] [--file <args.json>]",
	Short: "Check a page against the warnings of the limits table",
	Long: `Check a page against the warn rules of the limits table and print every
finding, one per line: where, the rule, and the message. A clean page prints
nothing. The lint only warns: present_create and present_update store a page
whatever it finds, and return the same findings in their result.

Give a stored page's id, or --file with a present_create argument object
(title, content, deck; content and deck as a Doc JSON string or the Doc
object itself; - reads stdin). Nothing is compiled, so --file judges the Doc
as written and works on a Doc the renderer would refuse. A page stored as
legacy HTML has no Doc and raises nothing.

--json prints {warnings, counts} with a count for every warn rule. --strict
exits 1 when anything was found. ` + "`present limits`" + ` prints every rule.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLint,
}

var (
	flagLintFile   string
	flagLintJSON   bool
	flagLintStrict bool
)

func init() {
	lintCmd.Flags().StringVar(&flagLintFile, "file", "",
		"a present_create argument object to lint instead of a stored page (- for stdin)")
	lintCmd.Flags().BoolVar(&flagLintJSON, "json", false,
		"print {warnings, counts} as JSON")
	lintCmd.Flags().BoolVar(&flagLintStrict, "strict", false,
		"exit 1 when the lint finds anything")
	rootCmd.AddCommand(lintCmd)
}

// errLintTarget is a lint call that names no page or two.
var errLintTarget = errors.New("lint: give a page id or --file <args.json>, not both")

func runLint(cmd *cobra.Command, args []string) error {
	page, err := lintPage(cmd.InOrStdin(), args)
	if err != nil {
		return err
	}
	findings := lint.Check(page)
	out := cmd.OutOrStdout()
	if flagLintJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(lint.Summarize(findings)); err != nil {
			return err
		}
	} else {
		for _, f := range findings {
			fmt.Fprintf(out, "%s [%s] %s\n", f.Where, f.Rule, f.Message)
		}
	}
	if flagLintStrict && len(findings) > 0 {
		return fmt.Errorf("lint: %d %s", len(findings), warningWord(len(findings)))
	}
	return nil
}

// lintPage reads the page the call names: an argument file, stdin, or a
// stored page by id.
func lintPage(stdin io.Reader, args []string) (lint.Page, error) {
	switch {
	case flagLintFile != "" && len(args) > 0, flagLintFile == "" && len(args) == 0:
		return lint.Page{}, errLintTarget
	case flagLintFile == "-":
		data, err := io.ReadAll(stdin)
		if err != nil {
			return lint.Page{}, fmt.Errorf("lint: read stdin: %w", err)
		}
		return lint.ParseArgs(data)
	case flagLintFile != "":
		data, err := os.ReadFile(flagLintFile)
		if err != nil {
			return lint.Page{}, fmt.Errorf("lint: %w", err)
		}
		return lint.ParseArgs(data)
	}
	return storedPage(args[0])
}

// storedPage reads a page and its Doc sources from the workdir. A missing
// source is a rendition the page lacks or holds as legacy HTML.
func storedPage(id string) (lint.Page, error) {
	st, err := store.NewFS(flagWorkdir)
	if err != nil {
		return lint.Page{}, err
	}
	ctx := context.Background()
	p, err := st.Get(ctx, id)
	if err != nil {
		return lint.Page{}, err
	}
	doc, err := st.LoadDoc(ctx, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return lint.Page{}, err
	}
	deck, err := st.LoadDeckSource(ctx, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return lint.Page{}, err
	}
	return lint.FromSources(p.Title, doc, deck)
}

func warningWord(n int) string {
	if n == 1 {
		return "warning"
	}
	return "warnings"
}

var limitsCmd = &cobra.Command{
	Use:   "limits",
	Short: "Print every limit present holds a page to",
	Long: `Print the limits reference: every warning the lint raises and every refusal
the renderer makes with a number, with its number, where it applies, its
source, the rule, and the reason, then what the lint does not see. It is the
text the skill carries as references/limits.md. --json prints the table.`,
	Args: cobra.NoArgs,
	RunE: runLimits,
}

var flagLimitsJSON bool

func init() {
	limitsCmd.Flags().BoolVar(&flagLimitsJSON, "json", false, "print the limits table as JSON")
	rootCmd.AddCommand(limitsCmd)
}

func runLimits(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	if !flagLimitsJSON {
		_, err := io.WriteString(out, lint.Reference())
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(limits.All())
}
