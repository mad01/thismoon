// Package mcpserver exposes the kof assertion store as MCP tools. Each tool is
// a thin HTTP call to a running `kof serve`, which owns the store and resolves
// the evidence pins — so the MCP server holds no state and never writes the JSON
// file directly.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	kof "github.com/mad01/thismoon/services/keeper-of-facts"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "kof"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user (e.g. http://kof.this); falls back to the API URL
}

// New builds the kof MCP server. The HTTP client always targets
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
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(kof.Facts())},
	)
	registerTools(s, &handlers{client: client.New(apiURL), webURL: webURL})
	return s, nil
}
