// Package mcpserver exposes brain vault functionality (search, read, write,
// promote, backlinks) as MCP tools over stdio. Each tool handler is a thin
// adapter over the internal packages (search, vault, markdown) that back the
// cobra commands.
package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Name is the MCP server name advertised during the initialize handshake.
const Name = "brain"

// New returns a fully wired MCP server with every brain_* tool registered.
// The caller is responsible for running it against a transport, typically
// &mcp.StdioTransport{} from the `brain mcp` cobra subcommand.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, nil)

	registerSearchTools(s)
	registerSemanticTools(s)
	registerReadTools(s)
	registerListTools(s)
	registerGraphTools(s)
	registerWriteTools(s)

	return s
}
