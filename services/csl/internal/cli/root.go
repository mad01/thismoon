package cli

import (
	"github.com/spf13/cobra"
)

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
}

// Execute runs the root cobra command.
func Execute() error {
	return rootCmd.Execute()
}
