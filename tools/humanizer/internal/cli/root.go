package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "humanizer",
	Short: "Detect AI-writing patterns and profile voice",
	Long: `humanizer: a local CLI and MCP server that flags AI-writing patterns
and computes quantitative voice profiles for text samples.

Subcommands:
  detect   Scan text for AI-writing patterns.
  profile  Compute a voice profile (sentence stats, punctuation density).
  rules    List or explain the supported detection rules.
  mcp      Start the MCP stdio server for Claude Code.
  version  Print the build this binary came from.

Patterns are derived from Wikipedia's "Signs of AI writing" page via the
/humanizer skill. The detector is purely deterministic: rewrites are left
to the calling agent (Claude).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// pendingExitCode lets a command (e.g. lint) request a non-zero exit without
// returning an error, so findings don't print as an error message.
var pendingExitCode int

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return err
	}
	if pendingExitCode != 0 {
		os.Exit(pendingExitCode)
	}
	return nil
}
