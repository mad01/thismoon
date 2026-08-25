// Package opener holds the opener component identity: the embedded
// operating doc and the mechanical facts it renders with. internal/sysopen,
// internal/cli, and internal/mcpserver all read from here, so the doc, the
// MCP instructions, and the error hints cannot drift from the code.
package opener

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
