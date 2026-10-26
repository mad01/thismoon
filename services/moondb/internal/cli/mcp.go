package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the moondb MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing the moondb
edit history as native tools for Claude Code.

The tools are a thin client over a running ` + "`moondb serve`" + `: start the serve
agent first (it owns the store and drains the spool the hooks write).

Tools exposed:
  moondb_why       Explain one line of one file: the edit, turn, prompt, commit.
  moondb_history   List the recorded edits of a path, newest first.
  moondb_inflight  List a checkout's files other sessions edited and no commit took.
  moondb_turn      Show one turn with its edits and the prompt that drove it.
  moondb_sessions  List recorded sessions, newest first.
  moondb_doctor    Run the doctor checks and return the report.

` + agentdoc.RegistrationSnippet(moondb.Facts()),
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("moondb mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL, Checks: doctorChecks},
	)
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
