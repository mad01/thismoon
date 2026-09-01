package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	kof "github.com/mad01/thismoon/services/keeper-of-facts"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the kof MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing the kof
assertion store as native tools for Claude Code.

The tools are a thin client over a running ` + "`kof serve`" + `: start the serve
agent first (it owns the store and resolves the evidence pins).

Tools exposed:
  kof_assert   Record an assertion pinned to evidence.
  kof_query    List assertions, newest first.
  kof_get      Get one assertion by id.
  kof_retract  Withdraw an assertion with a counter-evidence note.
  kof_check    Re-hash pins and report fresh/stale/flipped counts.
  kof_doctor   Run the doctor checks and return the report.

` + agentdoc.RegistrationSnippet(kof.Facts()),
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("kof mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL, Checks: doctorChecks},
	)
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
