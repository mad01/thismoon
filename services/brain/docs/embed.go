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

// Sharing is docs/sharing.md: git per vault, brain sync and the commands that
// change a shared vault through its repo, printed by `brain docs sharing`.
//
//go:embed sharing.md
var Sharing string

// Serve is docs/serve.md: the brain serve HTTP API and UI contract, printed
// by `brain docs serve`.
//
//go:embed serve.md
var Serve string
