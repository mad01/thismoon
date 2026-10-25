// Package srtspec holds the srtspec component identity: the embedded
// operating doc and the mechanical facts it renders with, so the doc and
// the CLI's help cannot drift from the code.
package srtspec

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string
