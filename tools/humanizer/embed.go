// Package humanizer holds the humanizer component identity: the embedded
// operating doc and the mechanical facts it renders with. internal/cli,
// internal/mcpserver, and internal/rules all read from here, so the doc, the
// MCP instructions, and the error hints cannot drift from the defaults the
// code actually uses.
package humanizer

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
