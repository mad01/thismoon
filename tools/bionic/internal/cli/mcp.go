package cli

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/bionic/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the bionic MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server that exposes bionic
reading as a native tool for Claude Code and other MCP clients.

Tools exposed:
  bionic_render  Convert markdown text to bionic reading format.

Register with Claude Code:
  claude mcp add --scope user bionic -- bionic mcp`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	server := mcpserver.New(Version)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
