// Package mcpserver exposes brain vault functionality (search, read, write,
// promote, backlinks, doctor) as MCP tools over stdio. Each tool handler is a
// thin adapter over the internal packages (search, vault, markdown, doctor)
// that back the cobra commands.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/brain"
)

// Name is the MCP server name advertised during the initialize handshake.
const Name = brain.Component

// Options configures a server.
type Options struct {
	// ConfigPath is the config file every tool loads, resolved by the caller
	// from --config, BRAIN_CONFIG and the default.
	ConfigPath string
}

// handlers carries what the tool handlers share.
type handlers struct {
	configPath string
}

// New returns a fully wired MCP server with every brain_* tool registered.
// The caller is responsible for running it against a transport, typically
// &mcp.StdioTransport{} from the `brain mcp` cobra subcommand.
func New(version string, opts Options) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, &mcp.ServerOptions{Instructions: agentdoc.Instructions(brain.Facts())})

	h := &handlers{configPath: opts.ConfigPath}
	registerSearchTools(s, h)
	registerSemanticTools(s, h)
	registerReadTools(s, h)
	registerListTools(s, h)
	registerGraphTools(s, h)
	registerWriteTools(s, h)
	registerDoctorTool(s, h)

	return s
}
