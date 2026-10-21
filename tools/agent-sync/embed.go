// Package agentsync holds the agent-sync component identity: the embedded
// operating doc, the mechanical facts it renders with, and the defaults
// every section shares. internal/cli, internal/config and the section
// packages read from here, so the doc, the help text and the code cannot
// disagree about a default.
package agentsync

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
