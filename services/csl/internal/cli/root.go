package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// configFlag holds --config: the config file every surface in this process
// reads, pinned before any subcommand runs.
var configFlag string

var rootCmd = &cobra.Command{
	Use:   "csl",
	Short: "Local code search + MCP server (zoekt)",
	Long: `csl — local code search daemon and MCP server.

Indexes your local git repositories with zoekt and exposes them as an MCP
stdio server (for Claude Code and other MCP clients) plus a CLI for direct
use.

Run ` + "`csl mcp`" + ` to start the MCP stdio server.
Run ` + "`csl search <pattern>`" + ` to search from the command line.
Run ` + "`csl repo`" + ` to pick a repo interactively or list them as JSON/TOON.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Pin the config file ahead of every subcommand, so the CLI, the MCP
	// server, and the search daemon started from this process all read the
	// same one.
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		config.SetPath(configFlag)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFlag, "config", "",
		"path to the YAML config (env CSL_CONFIG; default ~/.config/csl/config.yaml)")
}

// Execute runs the root cobra command.
func Execute() error {
	return rootCmd.Execute()
}
