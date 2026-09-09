package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/mcpserver"
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
  present_doctor  Run the doctor checks and return the report.

` + agentdoc.RegistrationSnippet(present.Facts()),
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// Log the resolved workdir/port to stderr (stdout is the MCP protocol
	// channel). If this diverges from what `present serve` uses, updates land
	// where nothing serves them — this line makes the divergence visible.
	log.Printf("present mcp: workdir=%s port=%d base-url=%s", flagWorkdir, flagPort, flagBaseURL)
	server, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{
			Workdir: flagWorkdir,
			Port:    flagPort,
			BaseURL: flagBaseURL,
			Checks:  doctorChecks,
		},
	)
	if err != nil {
		return err
	}
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
