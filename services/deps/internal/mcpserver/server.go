// Package mcpserver exposes dependency scanning as MCP tools. Each tool is a
// thin HTTP call to a running `deps serve`, which owns the store and reaches OSV
// — so the MCP server holds no state and never scans or writes directly.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	deps "github.com/mad01/thismoon/services/deps"
	"github.com/mad01/thismoon/services/deps/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "deps"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user (e.g. http://deps.this)
}

// New builds the deps MCP server. The HTTP client always targets localhost:<Port>
// (always reachable, no d-man dependency); BaseURL is used only for the
// human-facing link returned in tool responses.
func New(version string, cfg Config) (*mcp.Server, error) {
	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	webURL := cfg.BaseURL
	if webURL == "" {
		webURL = apiURL
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(deps.Facts())},
	)
	registerTools(s, &handlers{client: client.New(apiURL), webURL: webURL})
	return s, nil
}
