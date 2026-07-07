// Package cli builds the bionic cobra command tree (render + mcp subcommands).
package cli

import "github.com/spf13/cobra"

// Version is set via -ldflags at build time; defaults to "dev".
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:           "bionic",
	Short:         "Bionic reading — bold the first half of each word for speed-reading",
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Errors are returned to main, which prints them once.
func Execute() error {
	return rootCmd.Execute()
}
