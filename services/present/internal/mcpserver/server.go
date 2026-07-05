// Package mcpserver wires the present store to MCP tools that create, read,
// update, and list presentations (no delete), plus open one in the browser.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
}

// New builds the present MCP server.
func New(version string, cfg Config) (*mcp.Server, error) {
	st, err := store.New(cfg.Workdir)
	if err != nil {
		return nil, err
	}
	base := cfg.BaseURL
	if base == "" {
		base = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	s := mcp.NewServer(&mcp.Implementation{Name: Name, Version: version}, nil)
	registerTools(s, &handlers{store: st, baseURL: base, open: openURL})
	return s, nil
}
