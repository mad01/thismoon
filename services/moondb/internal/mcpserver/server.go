// Package mcpserver exposes the moondb history as MCP tools. Each tool is a
// thin HTTP call to a running `moondb serve`, which owns the store and runs
// the queries, so the MCP server holds no state and never opens the store.
// Every tool is read-only: moondb records through hooks, never through MCP.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "moondb"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user; falls back to the API URL

	// API, when set, is the serve's own handler, which the tools call in
	// process instead of localhost:<Port>: the MCP server a shared
	// instance mounts at /mcp.
	API http.Handler

	// Checks builds the diagnostics behind the moondb_doctor tool, against
	// the same resolved flags the rest of the CLI uses. It is required: the
	// instructions block advertises moondb_doctor to every client, so a
	// server that could not register it must not start.
	Checks func(ctx context.Context) []doctor.Check
}

// New builds the moondb MCP server. The HTTP client targets
// localhost:<Port>, which is reachable without any proxy, or calls cfg.API
// in process when it is set; BaseURL is used only for the human-facing link
// returned in tool responses.
func New(version string, cfg Config) (*mcp.Server, error) {
	if cfg.Checks == nil {
		return nil, errors.New(
			"mcpserver: no doctor checks; moondb_doctor is advertised to clients and must be registered",
		)
	}
	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	webURL := cfg.BaseURL
	if webURL == "" {
		webURL = apiURL
	}
	c := client.New(apiURL)
	if cfg.API != nil {
		c = client.NewForHandler(cfg.API)
	}
	return newServer(version, &handlers{
		client: c,
		webURL: webURL,
		checks: cfg.Checks,
	}), nil
}

// newServer registers the tools over h; tests call it with a client pointed
// at an httptest server.
func newServer(version string, h *handlers) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(moondb.Facts())},
	)
	registerTools(s, h)
	return s
}
