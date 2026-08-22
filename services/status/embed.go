// Package status holds the status component identity: the embedded operating
// doc and the mechanical facts it renders with. internal/cli reads from here,
// so the doc and the error hints cannot drift from the defaults the code
// actually uses.
package status

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
