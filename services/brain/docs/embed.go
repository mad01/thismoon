// Package docs embeds the agent-facing documentation that the brain binary
// prints with `brain docs [topic]`.
package docs

import _ "embed"

// CLI is docs/cli.md: the agent guide to the brain command line.
//
//go:embed cli.md
var CLI string

// Lifecycle is docs/lifecycle.md: statuses, revocation, brain check and the
// freshness commands, printed by `brain docs lifecycle`.
//
//go:embed lifecycle.md
var Lifecycle string
