package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/events/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the events MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing the event log
as native tools for Claude Code.

The tools are a thin client over a running ` + "`events serve`" + ` — start the
serve agent first (it owns the store).

Tools exposed:
  events_query    Query the event log, newest first.
  events_sources  List event sources with counts.
  events_emit     Record a single event.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("events mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL},
	)
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
