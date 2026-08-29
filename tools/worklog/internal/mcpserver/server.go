// Package mcpserver exposes the worklog store as MCP tools so Claude can
// checkpoint, list, search, show, and re-status work items without shelling out.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/worklog"
)

const Name = "worklog"

// New builds the worklog MCP server with all tools registered. configPath is
// the config file the CLI resolved for this invocation, so a server started
// with --config or $WORKLOG_CONFIG reads the same file its CLI sibling does.
func New(version, configPath string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
	}, &mcp.ServerOptions{Instructions: agentdoc.Instructions(worklog.Facts())})
	registerTools(s, &handlers{configPath: configPath})
	return s
}
