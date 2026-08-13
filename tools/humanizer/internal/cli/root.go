package cli

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "humanizer",
	Short: "Detect AI-writing patterns and profile voice",
	Long: `humanizer — a local CLI and MCP server that flags AI-writing patterns
and computes quantitative voice profiles for text samples.

Subcommands:
  detect   Scan text for AI-writing patterns.
  profile  Compute a voice profile (sentence stats, punctuation density).
  rules    List or explain the supported detection rules.
  mcp      Start the MCP stdio server for Claude Code.
  version  Print the build this binary came from.

Patterns are derived from Wikipedia's "Signs of AI writing" page via the
/humanizer skill. The detector is purely deterministic — rewrites are left
to the calling agent (Claude).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}
