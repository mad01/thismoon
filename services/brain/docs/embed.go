// Package docs embeds the agent-facing documentation that the brain binary
// prints with `brain docs`.
package docs

import _ "embed"

// CLI is docs/cli.md: the agent guide to the brain command line.
//
//go:embed cli.md
var CLI string
