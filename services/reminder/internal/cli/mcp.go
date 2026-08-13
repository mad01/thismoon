package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/reminder/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the reminder MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing reminder
management as native tools for Claude Code.

The tools are a thin client over a running ` + "`reminder serve`" + ` — start the
serve agent first (it owns the store and fires notifications).

Tools exposed:
  reminder_create  Create a reminder (absolute or relative time, optional repeat).
  reminder_list    List reminders, soonest due first.
  reminder_get     Get one reminder by id.
  reminder_edit    Edit a reminder by id.
  reminder_cancel  Soft-cancel a reminder by id.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("reminder mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL},
	)
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
