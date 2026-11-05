package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newBacklinksCmd(a *app) *cobra.Command {
	var title string
	c := &cobra.Command{
		Use:   "backlinks [path]",
		Short: "Inbound and outbound wikilinks of a note",
		Long: `List the notes that link to a note ([[stem]], [[title]] or [[alias]]) and
the wikilinks the note itself contains, resolved to vault paths where a note
with that name exists. Name the note by vault-relative path, or by --title
(matched against frontmatter title and aliases). With -o json, prints the
brain_backlinks document.`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (len(args) == 1) == (title != "") {
				return usageErrorf("name the note by <path> or by --title, not both or neither")
			}
			_, v, err := openVault()
			if err != nil {
				return err
			}
			var rel string
			if len(args) == 1 {
				rel = args[0]
			} else if rel, err = v.FindByTitle(title); err != nil {
				return err
			}
			links, err := v.Backlinks(rel)
			if err != nil {
				return withNoteHint(err, rel)
			}
			return a.emit(cmd.OutOrStdout(), links, func(w io.Writer) error {
				fmt.Fprintf(w, "inbound (%d):\n", len(links.Inbound))
				for _, in := range links.Inbound {
					fmt.Fprintf(w, "  %s:%d: %s\n", in.Path, in.Line, in.Excerpt)
				}
				fmt.Fprintf(w, "outbound (%d):\n", len(links.Outbound))
				for _, out := range links.Outbound {
					target := out.Path
					if target == "" {
						target = "(unresolved)"
					}
					fmt.Fprintf(w, "  %d: [[%s]] -> %s\n", out.Line, out.Target, target)
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&title, "title", "", "find the note by frontmatter title or alias")
	return c
}
