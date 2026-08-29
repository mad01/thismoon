// Package mcpserver exposes wire's channels as MCP tools. Each tool is a thin
// HTTP call to a running `wire serve`, which owns the store and is where a
// blocking read parks — so the MCP server holds no state and never writes the
// JSONL logs directly.
package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
	wire "github.com/mad01/thismoon/services/wire"
	"github.com/mad01/thismoon/services/wire/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "wire"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user (e.g. http://wire.this); falls back to the API URL

	// Checks builds the diagnostics behind the wire_doctor tool, against the
	// same resolved flags the rest of the CLI uses. It is required: the
	// instructions block advertises wire_doctor to every client, so a server
	// that could not register it must not start.
	Checks func(ctx context.Context) []doctor.Check
}

// New builds the wire MCP server. The HTTP client always targets
// localhost:<Port> (always reachable, no d-man dependency); BaseURL is used
// only for the human-facing link returned in tool responses.
func New(version string, cfg Config) (*mcp.Server, error) {
	if cfg.Checks == nil {
		return nil, errors.New("mcpserver: no doctor checks; wire_doctor is advertised to clients and must be registered")
	}
	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	webURL := cfg.BaseURL
	if webURL == "" {
		webURL = apiURL
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(wire.Facts())},
	)
	registerTools(s, &handlers{
		client: client.New(apiURL),
		webURL: webURL,
		port:   cfg.Port,
		checks: cfg.Checks,
	})
	return s, nil
}
