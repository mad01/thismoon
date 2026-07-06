package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Backstage-style systems & components catalog",
	Long: `catalog — a minimal systems catalog for your tools.

Reads Backstage-aligned service-info.yaml files from the repos listed in your
registry, builds an in-memory catalog of Systems and Components, and serves a
localhost web UI to browse and search them by name or owner.

Run ` + "`catalog web`" + ` to serve the web UI on localhost.
Run ` + "`catalog list`" + ` to print entities from the command line.
Run ` + "`catalog validate`" + ` to check service-info.yaml files.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root cobra command.
func Execute() error {
	return rootCmd.Execute()
}
