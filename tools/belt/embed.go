// Package belt holds the belt component identity: the embedded operating doc
// and the mechanical facts it renders with. internal/cli reads from here, so
// the doc `belt docs` prints cannot drift from the defaults the code runs
// with.
package belt

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
