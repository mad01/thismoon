package cli

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/mcpserver"
)

func newMCPCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the brain MCP stdio server for Claude Code",
		Long: `Start an MCP (Model Context Protocol) stdio server that exposes brain
functionality as native tools for Claude Code and other MCP clients.

Reads JSON-RPC from stdin, writes responses to stdout, exits when stdin closes.
No daemon, no port: the MCP client spawns it per session. Every tool loads the
config this command resolved (--config, then BRAIN_CONFIG, then the default).

Tools exposed:
  brain_search           Zoekt query across the vault, with tier/tag/status filters.
  brain_semantic_search  Vector search over note chunks (needs Ollama).
  brain_hybrid_search    Literal and semantic results fused by reciprocal rank.
  brain_read             Read a note's frontmatter + body (sliced by line range).
  brain_peek             Frontmatter + first N lines + backlink count, a cheap overview.
  brain_count            Count matches, optionally grouped by tier, tag or vault.
  brain_query_validate   Validate a zoekt query before running it.
  brain_list_tier        List notes in a tier with optional tag/status filter.
  brain_backlinks        Inbound + outbound wikilinks for a note.
  brain_graph            The wikilink graph as nodes and edges, optionally around one note.
  brain_write            Create a new note in a tier with frontmatter.
  brain_edit             Edit body or frontmatter of an existing note.
  brain_promote          Move an inbox note into notes/, rewriting backlinks.
  brain_doctor           Run the doctor checks and return the report.

` + agentdoc.RegistrationSnippet(brain.Facts()),
		Args: exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			server := mcpserver.New(
				buildinfo.Get().Version,
				mcpserver.Options{ConfigPath: a.configPath},
			)
			return server.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}
