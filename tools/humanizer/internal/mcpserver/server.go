// Package mcpserver exposes humanizer functionality (pattern detection,
// rule metadata, voice profiling) as MCP tools over stdio. Each tool
// handler is a thin adapter over the internal rules and voice packages.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/humanizer"
)

// Name is the MCP server name advertised during the initialize handshake.
const Name = "humanizer"

// New returns a fully wired MCP server with every humanizer_* tool registered.
// The caller is responsible for running it against a transport, typically
// &mcp.StdioTransport{} from the `humanizer mcp` cobra subcommand.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, &mcp.ServerOptions{Instructions: agentdoc.Instructions(humanizer.Facts())})

	registerStatusTools(s)
	registerDetectTools(s)
	registerStatisticalTools(s)
	registerRulesTools(s)
	registerVoiceTools(s)
	registerScrubTools(s)
	registerRewriteTools(s)

	return s
}
