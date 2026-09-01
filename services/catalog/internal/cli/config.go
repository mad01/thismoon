package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Print which registry file catalog resolved and how many sources it lists.

The registry is resolved in this order, first match winning:

  --registry <path>
  $CATALOG_REGISTRY
  $XDG_CONFIG_HOME/catalog/registry.yaml   (when that variable is an absolute path)
  ~/.config/catalog/registry.yaml

A leading ~ is expanded wherever it comes from: the flag, the environment
variable, the default, and each sources[].path below, so a value handed over
by a launchd agent, which never runs through a shell, resolves the same as one
typed in a terminal.

catalog has no settings file. Its only input is the registry: a list of source
roots, one per repo, that every command walks for service-info.yaml files.

  sources:                                        # the only top-level key
    - path: ~/code/src/github.com/mad01/dotfiles  # one repo root per entry;
    - path: ~/code/src/github.com/mad01/thismoon  # a leading ~ is expanded

Each source root is walked recursively for files named service-info.yaml
(.git, node_modules, vendor, .idea, dist and build are never descended into).
Every such file holds one or more entities: a System for the repo, a Component
per tool, several separated by --- in one file. A source that isn't checked out
is skipped without complaint, so a registry may list more repos than a machine
has. Nothing is cached: each run rescans from disk.

Because the registry is a data file rather than settings, this command prints
only where it lives and how big it is. Use 'catalog list' to see the entities it
produces, and 'catalog validate' to schema-check them and enforce the
globally-unique name rule.

Pair it with doctor: doctor checks that the resolved registry reads and loads,
config shows which file and key to change.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		writeRegistryStatus(cmd.OutOrStdout(), registryPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}

// writeRegistryStatus writes the one-line registry header for path. An absent
// or unreadable registry is reported in the status rather than returned as an
// error: naming the path catalog looked at is the point of the command.
func writeRegistryStatus(w io.Writer, path string) {
	fmt.Fprintf(w, "registry: %s (%s)\n", path, registryStatus(path))
}

// registryStatus describes how the registry at path resolved: loaded with its
// source count, missing, or the error that stopped it parsing.
func registryStatus(path string) string {
	reg, err := catalog.LoadRegistry(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "missing"
	case err != nil:
		return "parse error: " + err.Error()
	}
	if len(reg.Sources) == 1 {
		return "loaded, 1 source"
	}
	return fmt.Sprintf("loaded, %d sources", len(reg.Sources))
}
