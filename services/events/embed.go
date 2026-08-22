// Package events holds the events component identity: the embedded operating
// doc and the mechanical facts it renders with. internal/cli and
// internal/mcpserver both read from here, so the doc, the MCP instructions,
// and the error hints cannot drift from the defaults the code actually uses.
package events

import (
	_ "embed"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string

// DefaultPort is the port `events serve` listens on when EVENTS_PORT and
// --port are both unset. The server binds to loopback only.
const DefaultPort = 7430

// DefaultWorkdir is the store directory when EVENTS_WORKDIR is unset. The
// leading ~ is expanded at runtime by the CLI, never at build time.
const DefaultWorkdir = "~/.local/share/events"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "events",
		Bin:       "events",
		Purpose:   "local event/audit log producers post to and agents query",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultWorkdir,
	}
}
