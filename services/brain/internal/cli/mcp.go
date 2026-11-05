package cli

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/brain/internal/mcpserver"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the brain MCP stdio server for Claude Code",
		Long: `Start an MCP (Model Context Protocol) stdio server that exposes brain
functionality as native tools for Claude Code and other MCP clients.

Reads JSON-RPC from stdin, writes responses to stdout, exits when stdin closes.
No daemon, no port — just subprocess IPC spawned per session by the MCP client.

Tools exposed:
  brain_search          Zoekt query across the vault, with tier/tag/status filters.
  brain_read            Read a note's frontmatter + body (sliced by line range).
  brain_peek            Frontmatter + first N lines + backlink count — cheap overview.
  brain_count           Count matches, optionally grouped by tier or tag.
  brain_query_validate  Validate a zoekt query before running it.
  brain_list_tier       List notes in a tier with optional tag/status filter.
  brain_backlinks       Inbound + outbound wikilinks for a note.
  brain_write           Create a new note in a tier with frontmatter.
  brain_edit            Edit body or frontmatter of an existing note.
  brain_promote         Move an inbox note into notes/, rewriting backlinks.

Register with Claude Code:
  claude mcp add --scope user brain -- brain mcp`,
		Args: exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return mcpserver.New(buildinfo.Get().Version).
				Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}
