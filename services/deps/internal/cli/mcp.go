package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	deps "github.com/mad01/thismoon/services/deps"
	"github.com/mad01/thismoon/services/deps/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the deps MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server exposing dependency
scanning as native tools for Claude Code.

The tools are a thin client over a running ` + "`deps serve`" + `: start the
serve agent first (it owns the scan store and reaches OSV).

Tools exposed:
  deps_scan          Discover dependencies across all repos (no advisory check).
  deps_check         Discover + check against OSV; return flagged packages.
  deps_list_flagged  Return the flagged packages from the last check.
  deps_scan_repo     Rescan a single repo against OSV and merge the result.
  deps_resolve       Acknowledge advisories by key.
  deps_doctor        Run the doctor checks and return the report.

` + agentdoc.RegistrationSnippet(deps.Facts()),
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("deps mcp: port=%d base-url=%s", flagPort, flagBaseURL)
	srv, err := mcpserver.New(
		buildinfo.Get().Version,
		mcpserver.Config{Port: flagPort, BaseURL: flagBaseURL, Checks: doctorChecks},
	)
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
