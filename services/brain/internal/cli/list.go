package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newListCmd(a *app) *cobra.Command {
	var (
		opts vault.ListOptions
		tier string
	)
	c := &cobra.Command{
		Use:   "list [tier]",
		Short: "List notes in a tier with optional filters",
		Long: `List notes in a single vault tier (inbox, notes, outputs, or MOCs), named
as the argument or with --tier.

Prints a simple table: updated | tier/path | title, newest first, across
every vault unless --vault narrows it. Optionally filter by frontmatter tag,
status, or type. With -o json, prints the brain_list_tier document.`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := listTierName(args, tier)
			if err != nil {
				return err
			}
			if opts.Limit <= 0 {
				return usageErrorf("--limit must be > 0")
			}
			if opts.Tier, err = vault.ParseTier(name); err != nil {
				return err
			}
			_, set, err := a.openVaults()
			if err != nil {
				return err
			}
			listing, err := set.List(opts)
			if err != nil {
				return err
			}
			return a.emit(cmd.OutOrStdout(), listing, func(w io.Writer) error {
				for _, n := range listing.Notes {
					updated := n.Updated
					if updated == "" {
						updated = "-"
					}
					fmt.Fprintf(w, "%s  %s  %s\n",
						printable(updated), printable(a.show(n.Vault, n.Path)), printable(n.Title))
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&tier, "tier", "", "tier to list (inbox, notes, outputs, MOCs)")
	c.Flags().StringVar(&opts.Tag, "tag", "", "filter by frontmatter tag")
	c.Flags().StringVar(&opts.Status, "status", "", "filter by frontmatter status")
	c.Flags().StringVar(&opts.Type, "type", "", "filter by frontmatter type")
	c.Flags().IntVar(&opts.Limit, "limit", 100, "maximum number of notes to print")
	return c
}

// listTierName returns the tier named by the argument or --tier; exactly one
// of the two must be given, or both must agree.
func listTierName(args []string, flag string) (string, error) {
	switch {
	case len(args) == 0 && flag == "":
		return "", usageErrorf(
			"name a tier: brain list <inbox|notes|outputs|MOCs> or --tier <tier>",
		)
	case len(args) == 0:
		return flag, nil
	case flag != "" && flag != args[0]:
		return "", usageErrorf("tier given twice: %q and --tier %q", args[0], flag)
	}
	return args[0], nil
}

// printable returns s with every control character but tab written as its
// Go escape (\n, \x1b), so a title or a body line printed as text can
// neither break the output's lines nor send an escape sequence to the
// terminal.
func printable(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if isControl(r) {
			b.WriteString(strings.Trim(strconv.QuoteRune(r), "'"))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isControl(r rune) bool { return r != '\t' && unicode.IsControl(r) }

// writePrintable writes text line by line through printable, ending each
// line with "\n".
func writePrintable(w io.Writer, text string) {
	for line := range strings.Lines(text) {
		fmt.Fprintln(w, printable(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")))
	}
}
