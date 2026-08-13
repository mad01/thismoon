package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/tools/humanizer/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the humanizer MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server that exposes humanizer
functionality as native tools for Claude Code and other MCP clients.

Tools exposed:
  humanizer_detect         Scan text for AI-writing patterns and return findings.
  humanizer_rules_list     List all supported detection rules.
  humanizer_rules_explain  Full metadata + examples for one rule.
  humanizer_voice_profile  Quantitative voice profile of a text sample.
  humanizer_voice_diff     Metric-by-metric delta between two samples.

Register with Claude Code:
  claude mcp add --scope user humanizer -- humanizer mcp`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	server := mcpserver.New(buildinfo.Get().Version)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "humanizer mcp: %v\n", err)
		return err
	}
	return nil
}
