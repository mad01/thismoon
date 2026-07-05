package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/mcpserver"
	"github.com/mad01/thismoon/services/present/internal/render"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the present MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing presentation
management as native tools for Claude Code.

Tools exposed:
  present_create  Create a page, return its id and URL.
  present_read    Read a page's current content by id.
  present_update  Update a page by id (auto-reloads open tabs).
  present_list    List all pages.
  present_open    Open a page in the browser (once per page).

Register with Claude Code:
  claude mcp add --scope user present -- present mcp`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// Seed the editable template so it exists for the serve daemon and for
	// users who want to customize the chrome before the first request.
	if err := render.EnsureTemplate(flagWorkdir); err != nil {
		return err
	}
	// Log the resolved workdir/port to stderr (stdout is the MCP protocol
	// channel). If this diverges from what `present serve` uses, updates land
	// where nothing serves them — this line makes the divergence visible.
	log.Printf("present mcp: workdir=%s port=%d base-url=%s", flagWorkdir, flagPort, flagBaseURL)
	server, err := mcpserver.New(
		Version,
		mcpserver.Config{Workdir: flagWorkdir, Port: flagPort, BaseURL: flagBaseURL},
	)
	if err != nil {
		return err
	}
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
