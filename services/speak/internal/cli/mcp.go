package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the speak MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server that speaks text and
markdown aloud on this machine's speakers.

Unlike ` + "`speak serve`" + ` (which plays audio in the browser), the MCP tools drive
server-side playback with afplay, so an agent can make the machine talk. The
tools synthesize through the same provider as serve (the config's active
block, --provider to override); that provider must be reachable, but
` + "`speak serve`" + ` need not be running. The voice parameter lists the
provider's voices.

Tools exposed:
  speak_text    Speak given text aloud; returns a session id.
  speak_file    Read a markdown file aloud section by section.
  speak_pause   Pause playback and release the lock.
  speak_resume  Resume after pause or stop.
  speak_stop    Stop playback, saving the position.
  speak_voices  List the provider's voices, default marked.
  speak_status  Report provider health and playback state.
  speak_doctor  Run the same checks as ` + "`speak doctor`" + `, as JSON.

` + agentdoc.RegistrationSnippet(speak.Facts()),
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, _ []string) error {
	p := activeProvider(cmd.Context())
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("speak mcp: provider=%s model=%s remote=%t state-dir=%s",
		p.Name(), p.Model(), p.Config().Remote, flagStateDir)
	srv, err := mcpserver.New(buildinfo.Get().Version, mcpserver.Config{
		Provider: p,
		StateDir: flagStateDir,
		Checks:   doctorChecks,
	})
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
