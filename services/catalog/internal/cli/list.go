package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

var (
	listOwner  string
	listSystem string
	listKind   string
	listJSON   bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List catalog entities, optionally filtered",
	Long: `List Systems and Components from the catalog.

Filter by owner, system or kind, and search names/descriptions/tags with the
positional query argument:

  catalog list                      # everything
  catalog list --owner mad01        # entities owned by mad01
  catalog list --kind Component     # only components
  catalog list --system dotfiles    # components in the dotfiles system
  catalog list pres                 # name/description/tag substring match`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVar(&listOwner, "owner", "", "filter by owner")
	listCmd.Flags().StringVar(&listSystem, "system", "", "filter components by system")
	listCmd.Flags().StringVar(&listKind, "kind", "", "filter by kind (System|Component)")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output JSON")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cat, err := loadCatalog(cmd)
	if err != nil {
		return err
	}

	q := catalog.Query{Owner: listOwner, Kind: catalog.Kind(listKind)}
	if len(args) == 1 {
		q.Text = args[0]
	}
	entities := cat.Search(q)

	// --system narrows to components in a given system (Search has no system
	// field, so apply it here).
	if listSystem != "" {
		filtered := entities[:0]
		for _, e := range entities {
			if e.Spec.System == listSystem {
				filtered = append(filtered, e)
			}
		}
		entities = filtered
	}

	if listJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entities)
	}

	// Writes to the tabwriter are buffered; the real error surfaces on Flush.
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tNAME\tOWNER\tSYSTEM\tDESCRIPTION")
	for _, e := range entities {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			e.Kind, e.Metadata.Name, e.Spec.Owner, e.Spec.System, e.Metadata.Description)
	}
	return w.Flush()
}
