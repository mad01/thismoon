// Package suspenders holds the suspenders component identity: the embedded
// operating doc and the mechanical facts it renders with. The docs command
// reads from here, so the doc cannot drift from the defaults the code uses.
package suspenders

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
