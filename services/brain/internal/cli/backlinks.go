package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func newBacklinksCmd(a *app) *cobra.Command {
	var title string
	c := &cobra.Command{
		Use:   "backlinks [note]",
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
			ref, err := a.backlinksTarget(args, title)
			if err != nil {
				return err
			}
			links, err := ref.Vault.Backlinks(ref.Path)
			if err != nil {
				return withNoteHint(err, ref.Path)
			}
			return a.emit(cmd.OutOrStdout(), links, func(w io.Writer) error {
				fmt.Fprintf(w, "inbound (%d):\n", len(links.Inbound))
				for _, in := range links.Inbound {
					fmt.Fprintf(w, "  %s:%d: %s\n", a.show(in.Vault, in.Path), in.Line, in.Excerpt)
				}
				fmt.Fprintf(w, "outbound (%d):\n", len(links.Outbound))
				for _, out := range links.Outbound {
					target := a.show(out.Vault, out.Path)
					if out.Path == "" {
						target = "(dangling)"
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

// backlinksTarget locates the note named by the argument or by --title.
func (a *app) backlinksTarget(args []string, title string) (vault.Ref, error) {
	if len(args) == 1 {
		_, ref, err := a.locate(args[0])
		return ref, err
	}
	_, set, err := a.openVaults()
	if err != nil {
		return vault.Ref{}, err
	}
	return set.FindByTitle(title)
}
