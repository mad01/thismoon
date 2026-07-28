package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/wire/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the wire MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing wire's channels
as native tools for Claude Code.

The tools are a thin client over a running ` + "`wire serve`" + ` — start the serve
agent first (it owns the store and is where a blocking read waits).

Tools exposed:
  wire_open   Open a channel and get the handle to pass to another session.
  wire_post   Post a message to a channel.
  wire_read   Read after a cursor, optionally blocking for the next message.
  wire_list   List channels, most recently active first.
  wire_close  End a conversation and wake everyone waiting on it.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("wire mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(Version, mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL})
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
