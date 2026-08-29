package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	catalogroot "github.com/mad01/thismoon/services/catalog"
	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

// registryPath is the path to registry.yaml, settable via the persistent
// --registry flag or CATALOG_REGISTRY. It defaults to the registry inside
// catalog's config directory.
var registryPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&registryPath, "registry",
		envdefault.String("CATALOG_REGISTRY", defaultRegistryPath()),
		"path to registry.yaml listing source repos (env CATALOG_REGISTRY)")
	// Expand a leading ~ before any subcommand runs. A flag or env value
	// reaches Go without shell expansion under launchd, and reading a literal
	// "~/..." path is how catalog crash-looped as a supervised agent while the
	// same command worked in a terminal.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		var err error
		registryPath, err = confdir.Expand(registryPath)
		return err
	}
}

// defaultRegistryPath resolves the registry inside catalog's config directory,
// honoring XDG_CONFIG_HOME. A home directory that cannot be resolved leaves
// the ~-prefixed constant in place, so the failure surfaces in
// PersistentPreRunE — which can return an error — rather than here.
func defaultRegistryPath() string {
	if path, err := confdir.Path("catalog", catalogroot.RegistryFileName); err == nil {
		return path
	}
	return catalogroot.DefaultRegistry
}

// loadCatalog loads the catalog from the configured registry, honouring the
// command's context for cancellation.
func loadCatalog(cmd *cobra.Command) (*catalog.Catalog, error) {
	cat, _, err := catalog.Load(cmd.Context(), registryPath)
	return cat, agentdoc.Hint(err, catalogroot.Facts())
}
