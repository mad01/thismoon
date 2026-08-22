// Package mcpserver exposes reminder management as MCP tools. Each tool is a
// thin HTTP call to a running `reminder serve`, which owns the store — so the
// MCP server holds no state and never writes the JSON file directly.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/reminder"
	"github.com/mad01/thismoon/services/reminder/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "reminder"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user (e.g. http://reminder.this); falls back to the API URL
}

// New builds the reminder MCP server. The HTTP client always targets
// localhost:<Port> (always reachable, no d-man dependency); BaseURL is used only
// for the human-facing link returned in tool responses.
func New(version string, cfg Config) (*mcp.Server, error) {
	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	webURL := cfg.BaseURL
	if webURL == "" {
		webURL = apiURL
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(reminder.Facts())},
	)
	registerTools(s, &handlers{client: client.New(apiURL), webURL: webURL})
	return s, nil
}
