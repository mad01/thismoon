package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the csl MCP stdio server for Claude Code and other MCP clients",
	Long: `Start an MCP (Model Context Protocol) stdio server that exposes csl
functionality as native tools for Claude Code and other MCP clients.

The server reads JSON-RPC requests from stdin and writes responses to stdout,
then exits when the client closes stdin. No daemon, no port, just subprocess
IPC spawned per session by the MCP client.

Tools exposed:
  csl_repo_lookup       Resolve a repo name to its local checkout path
  csl_repo_info         Report git health and index staleness for a repo
  csl_repo_health       Fleet-wide sweep for uncommitted or unpushed work
  csl_repo_pull         Fast-forward git pull with safety checks
  csl_repo_reindex      Rebuild the zoekt index for a single repo
  csl_search            Search code across locally checked-out repos
  csl_count             Count matches grouped by repo or language
  csl_query_validate    Validate a zoekt query before running it
  csl_semantic_search   Search by meaning over the vector index
  csl_hybrid_search     Lexical + semantic, fused by RRF
  csl_read              Read a file from a named local repo
  csl_ls                List files and directories in a repo
  csl_show_file         Open a file section for the user in the web UI
  csl_index_info        Index-wide health in one call
  csl_doctor            Run the csl self-checks and return them as JSON

` + agentdoc.RegistrationSnippet(csl.Facts()) + `

Smoke test the stdio transport:
  printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list"}\n' | csl mcp`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	server := mcpserver.New(buildinfo.Get().Version)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "csl mcp: %v\n", err)
		return err
	}
	return nil
}
