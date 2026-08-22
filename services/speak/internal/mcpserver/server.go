// Package mcpserver exposes the speak read-aloud engine as MCP tools. Unlike
// reminder's MCP server (a thin client to a running serve), this one owns
// playback itself: the tools drive an in-process playback.Engine that fetches
// audio from the TTS engine and plays it with afplay. It needs the speak-tts
// engine reachable, but not `speak serve`.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/playback"
	"github.com/mad01/thismoon/services/speak/internal/ttsclient"
)

// Name is the MCP server name advertised to clients.
const Name = "speak-aloud"

// DefaultVoice is the Kokoro voice used when a tool omits one.
const DefaultVoice = "af_heart"

// Config locates the TTS engine the playback tools synthesize against.
type Config struct {
	TTSURL string // base URL of the mlx-audio engine (e.g. http://127.0.0.1:8765)
}

// New builds the speak MCP server with a fresh playback engine.
func New(version string, cfg Config) (*mcp.Server, error) {
	engine := playback.New(ttsclient.New(cfg.TTSURL), DefaultVoice)
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(speak.Facts())},
	)
	registerTools(s, &handlers{engine: engine})
	return s, nil
}
