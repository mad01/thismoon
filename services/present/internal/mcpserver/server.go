// Package mcpserver wires the present store to MCP tools that create, read,
// update, and list presentations (no delete), plus open one in the browser.
package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// Name is the MCP server name advertised to clients.
const Name = "present"

// Config holds the runtime settings shared with the HTTP server so the URLs
// returned by tools match where pages are served.
type Config struct {
	Workdir string
	Port    int
	BaseURL string // override for URL prefix (e.g. "http://present.this"); falls back to http://localhost:<Port>

	// Checks builds the diagnostics behind the present_doctor tool, against
	// the same resolved flags the rest of the CLI uses. It is required: the
	// instructions block advertises present_doctor to every client, so a
	// server that could not register it must not start.
	Checks func(ctx context.Context) []doctor.Check
}

// New builds the present MCP server.
func New(version string, cfg Config) (*mcp.Server, error) {
	if cfg.Checks == nil {
		return nil, errors.New("mcpserver: no doctor checks; present_doctor is advertised to clients and must be registered")
	}
	st, err := store.New(cfg.Workdir)
	if err != nil {
		return nil, hint(err)
	}
	base := cfg.BaseURL
	if base == "" {
		base = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(present.Facts())},
	)
	registerTools(s, &handlers{store: st, baseURL: base, open: openURL, checks: cfg.Checks})
	return s, nil
}
