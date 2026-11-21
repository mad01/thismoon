// Package cards holds the cards component identity: the embedded operating
// doc and the mechanical facts it renders with. internal/cli reads both, so
// the doc printed by `cards docs` and the doctor's pointers cannot drift from
// the code.
package cards

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
