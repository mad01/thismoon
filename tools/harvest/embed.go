// Package harvest holds the harvest component identity: the embedded
// operating doc, the mechanical facts it renders with, and the defaults the
// internal packages share. internal/settings and internal/cli read from
// here, so the doc and the code cannot disagree about a default.
package harvest

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
