// Package mcpserver exposes the pasteboard as MCP tools so an agent can
// copy and paste without shelling out through pbcopy/pbpaste.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/clipboard"
)

// Name is the MCP server name reported to the host.
const Name = "clipboard"

// New builds the clipboard MCP server with both tools registered.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, &mcp.ServerOptions{Instructions: agentdoc.Instructions(clipboard.Facts())})
	registerTools(s)
	return s
}
