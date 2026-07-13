package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/keep/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the keep MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing the keep
assertion store as native tools for Claude Code.

The tools are a thin client over a running ` + "`keep serve`" + ` — start the serve
agent first (it owns the store and resolves the evidence pins).

Tools exposed:
  keep_assert   Record an assertion pinned to evidence.
  keep_query    List assertions, newest first.
  keep_get      Get one assertion by id.
  keep_retract  Withdraw an assertion with a counter-evidence note.
  keep_check    Re-hash pins and report fresh/stale/flipped counts.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("keep mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(Version, mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL})
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
