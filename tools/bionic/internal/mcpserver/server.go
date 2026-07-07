// Package mcpserver wires the bionic reading transform as an MCP stdio server.
package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Name is the MCP server name reported to clients.
const Name = "bionic"

// New builds the bionic MCP server with its tools registered.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, nil)

	registerTools(s)

	return s
}
