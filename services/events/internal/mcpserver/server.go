// Package mcpserver exposes the event log as MCP tools. Each tool is a thin HTTP
// call to a running `events serve`, which owns the store — so the MCP server
// holds no state and never writes the JSONL files directly.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/events/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "events"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user (e.g. http://events.this); falls back to the API URL
}

// New builds the events MCP server. The HTTP client always targets
// localhost:<Port> (always reachable, no d-man dependency); BaseURL is used only
// for the human-facing link returned in tool responses.
func New(version string, cfg Config) (*mcp.Server, error) {
	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	webURL := cfg.BaseURL
	if webURL == "" {
		webURL = apiURL
	}
	s := mcp.NewServer(&mcp.Implementation{Name: Name, Version: version}, nil)
	registerTools(s, &handlers{client: client.New(apiURL), webURL: webURL})
	return s, nil
}
