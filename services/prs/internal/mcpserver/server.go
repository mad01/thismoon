// Package mcpserver exposes the PR cache as MCP tools. Each tool is a thin
// HTTP call to a running `prs serve`, which owns the cache and the GitHub
// polling — so the MCP server holds no state and never talks to GitHub
// directly.
package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
	prs "github.com/mad01/thismoon/services/prs"
	"github.com/mad01/thismoon/services/prs/internal/client"
)

// Name is the MCP server name advertised to clients.
const Name = "prs"

// Config locates the serve instance the tools talk to.
type Config struct {
	Port    int
	BaseURL string // display/link URL shown to the user (e.g. http://prs.this); falls back to the API URL

	// Checks builds the diagnostics behind the prs_doctor tool, against the
	// same resolved flags the rest of the CLI uses. It is required: the
	// instructions block advertises prs_doctor to every client, so a server
	// that could not register it must not start.
	Checks func(ctx context.Context) []doctor.Check
}

// New builds the prs MCP server. The HTTP client always targets
// localhost:<Port> (always reachable, no d-man dependency); BaseURL is used
// only for the human-facing link returned in tool responses.
func New(version string, cfg Config) (*mcp.Server, error) {
	if cfg.Checks == nil {
		return nil, errors.New(
			"mcpserver: no doctor checks; prs_doctor is advertised to clients and must be registered",
		)
	}
	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	webURL := cfg.BaseURL
	if webURL == "" {
		webURL = apiURL
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version},
		&mcp.ServerOptions{Instructions: agentdoc.Instructions(prs.Facts())},
	)
	registerTools(s, &handlers{client: client.New(apiURL), webURL: webURL, checks: cfg.Checks})
	return s, nil
}
