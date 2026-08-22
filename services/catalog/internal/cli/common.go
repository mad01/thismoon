package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	catalogroot "github.com/mad01/thismoon/services/catalog"
	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

// registryPath is the path to registry.yaml, settable via the persistent
// --registry flag. It defaults to ~/.config/catalog/registry.yaml.
var registryPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&registryPath, "registry",
		catalog.ExpandPath(catalogroot.DefaultRegistry),
		"path to registry.yaml listing source repos")
}

// loadCatalog loads the catalog from the configured registry, honouring the
// command's context for cancellation.
func loadCatalog(cmd *cobra.Command) (*catalog.Catalog, error) {
	cat, _, err := catalog.Load(cmd.Context(), registryPath)
	return cat, agentdoc.Hint(err, catalogroot.Facts())
}
