// Package mcpserver exposes csl search/repo/read functionality as MCP tools
// over stdio. Each tool handler is a thin adapter over the same internal
// packages that back the cobra commands — see internal/cli/{search,count,
// query,read,repo}.go for the canonical implementations.
//
// The handlers reuse the existing search daemon (via the daemon.*Via
// helpers) so zoekt index shards stay mmap'd across MCP calls, exactly
// as they do for the cobra commands.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/csl"
)

// Name is the MCP server name advertised during the initialize handshake.
const Name = "csl"

// Options carries the parts of csl's config the tool set depends on, so the
// caller resolves config once and New stays free of file I/O.
type Options struct {
	// SemanticEnabled mirrors semantic.enabled from config.yaml. When it is
	// false the semantic and hybrid tools are not registered at all, so a
	// client never sees a tool that would answer available=false on a machine
	// with no embedding backend. The web UI gates its modes the same way.
	SemanticEnabled bool
}

// New returns a fully wired MCP server with every csl_* tool registered that
// opts allows. The caller is responsible for running it against a transport,
// typically &mcp.StdioTransport{} from the `csl mcp` cobra subcommand.
func New(version string, opts Options) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, &mcp.ServerOptions{Instructions: agentdoc.Instructions(csl.Facts())})

	registerRepoTools(s)
	registerSearchTools(s)
	if opts.SemanticEnabled {
		registerSemanticTools(s)
		registerHybridTools(s)
	}
	registerReadTools(s)
	registerOutlineTools(s)
	registerShowTools(s)
	registerInfoTools(s)
	registerDoctorTools(s)

	return s
}
