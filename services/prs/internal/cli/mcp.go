package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/prs/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the prs MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing the PR
dashboard as native tools for Claude Code.

The tools are a thin client over a running ` + "`prs serve`" + ` — start the serve
agent first (it owns the cache and the GitHub polling).

Tools exposed:
  prs_list     List the cached open PRs, with repo/author/review/sort filters.
  prs_refresh  Force one synchronous poll cycle and report it.
  prs_status   Report cache freshness, per-repo errors, and the config in effect.`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("prs mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL},
	)
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
