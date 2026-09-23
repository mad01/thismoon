// Package mcpserver exposes the speak read-aloud engine as MCP tools. Unlike
// reminder's MCP server (a thin client to a running serve), this one owns
// playback itself: the tools drive an in-process playback.Engine that fetches
// audio from the active TTS provider and plays it with afplay. It needs the
// provider reachable, but not `speak serve`.
package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/playback"
	"github.com/mad01/thismoon/services/speak/internal/provider"
)

// Name is the MCP server name advertised to clients.
const Name = "speak-aloud"

// Config locates what the tools need: the provider they synthesize through
// and the directory playback writes its audio and lock to.
type Config struct {
	Provider *provider.Provider
	StateDir string // playback state: per-sentence audio files and playback.lock

	// Checks builds the diagnostics behind the speak_doctor tool, against
	// the same resolved flags the rest of the CLI uses. It is required: the
	// instructions block advertises speak_doctor to every client, so a
	// server that could not register it must not start.
	Checks func(ctx context.Context) []doctor.Check
}

// New builds the speak MCP server with a fresh playback engine.
func New(version string, cfg Config) (*mcp.Server, error) {
	if cfg.Checks == nil {
		return nil, errors.New(
			"mcpserver: no doctor checks; speak_doctor is advertised to clients and must be registered",
		)
	}
	if cfg.Provider == nil {
		return nil, errors.New("mcpserver: no TTS provider")
	}
	engine := playback.New(playback.Config{
		Speaker:      cfg.Provider,
		Health:       cfg.Provider.NewHealth(),
		DefaultVoice: cfg.Provider.Voices().Default,
		StateDir:     cfg.StateDir,
		Ping:         cfg.Provider.Ping,
	})
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(speak.Facts())},
	)
	h := &handlers{engine: engine, provider: cfg.Provider, checks: cfg.Checks}
	if err := registerTools(s, h); err != nil {
		return nil, err
	}
	return s, nil
}
