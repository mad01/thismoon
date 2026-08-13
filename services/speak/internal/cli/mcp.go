package cli

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/speak/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the speak MCP stdio server for Claude Code",
	Long: `Start an MCP (Model Context Protocol) stdio server that speaks text and
markdown aloud on this machine's speakers.

Unlike ` + "`speak serve`" + ` (which plays audio in the browser), the MCP tools drive
server-side playback with afplay, so an agent can make the machine talk. The
tools synthesize against the same local Kokoro engine as serve (--tts-url); the
speak-tts agent must be running, but ` + "`speak serve`" + ` need not be.

Tools exposed:
  speak_text    Speak given text aloud; returns a session id.
  speak_file    Read a markdown file aloud section by section.
  speak_pause   Pause playback and release the lock.
  speak_resume  Resume after pause or stop.
  speak_stop    Stop playback, saving the position.
  speak_voices  List the Kokoro voices.
  speak_status  Report engine reachability and playback state.`,
	RunE: runMCP,
}

func init() {
	mcpCmd.Flags().StringVar(&flagTTSURL, "tts-url", resolvedTTSURL(),
		"base URL of the mlx-audio TTS server (env SPEAK_TTS_URL)")
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	// stdout is the MCP protocol channel; log the resolved target to stderr.
	log.Printf("speak mcp: tts-url=%s", flagTTSURL)
	srv, err := mcpserver.New(buildinfo.Get().Version, mcpserver.Config{TTSURL: flagTTSURL})
	if err != nil {
		return err
	}
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
