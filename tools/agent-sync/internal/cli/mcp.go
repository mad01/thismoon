package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	agentsync "github.com/mad01/thismoon/tools/agent-sync"
	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/mcp"
)

// mcpCmd builds the mcp family; its sync and check also take --force.
func (a *app) mcpCmd() *cobra.Command {
	var opts mcp.Options
	c := a.sectionCommand(section{
		name:  config.SectionMCP,
		short: "Register and remove Claude Code MCP servers through the claude CLI",
		long: `Register the MCP servers the servers.json in mcp.servers names, plus the
*.json drop-ins in mcp.dropins, at user scope through the claude CLI.

A server missing from 'claude mcp list' is added. A server whose 'claude mcp
get' spec differs is removed and added again; a remote (http or sse) server
is compared by url alone, so an OAuth login survives. With mcp.prune, a
registered server the desired set does not name is removed at user scope. A
claude.ai connector, a plugin's server, a server in another scope and a name
'claude mcp add' would reject are skipped, each named in a 'note:' line. An
entry's hosts and profiles lists gate it to matching machines; profiles
come from the TOML file in mcp.profiles_file.

The per-server spec check is slow, so sync records a hash of the desired set
in mcp.stamp. While it matches, sync and check skip the check; plan always
runs it, and --force or mcp.prune makes sync and check run it.

check is the drift gate for 'ralph doctor': its first line is a one-line
summary, and it exits 1 when sync would change anything. A failed claude
call exits 2.`,
		run: func(file *config.File, mode apply.Mode, out apply.Streams) error {
			cfg, err := mcp.ParseConfig(file.MCP)
			if err != nil {
				return agentdoc.Hint(err, agentsync.Facts())
			}
			return mcp.Run(cfg, opts, mode, out)
		},
	})
	for _, sub := range c.Commands() {
		if sub.Name() != apply.Plan.String() {
			sub.Flags().BoolVar(&opts.Force, "force", false,
				"compare every registered server's spec, ignoring the stamp")
		}
	}
	return c
}
