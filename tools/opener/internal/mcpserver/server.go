// Package mcpserver exposes the macOS open command as MCP tools so an agent
// can open URLs, files, and apps, and reveal paths in Finder, without
// shelling out.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/opener"
)

// Name is the MCP server name reported to the host.
const Name = "opener"

// New builds the opener MCP server with all tools registered.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, &mcp.ServerOptions{Instructions: agentdoc.Instructions(opener.Facts())})
	registerTools(s)
	return s
}
