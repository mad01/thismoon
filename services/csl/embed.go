// Package csl holds the csl component identity: the embedded operating doc
// and the mechanical facts it renders with. internal/cli, internal/daemon,
// internal/mcpserver, and internal/repo/config read from here, so the doc,
// the MCP instructions, and the error hints cannot drift from the defaults
// the code actually uses.
package csl

import _ "embed"

// OperatingDoc is the operating doc template (operating.md), embedded at
// build time. Render it with agentdoc.Render and Facts().
//
//go:embed operating.md
var OperatingDoc string

// ClaudeMD is the csl rules fragment (claude-md.md) that tells an agent when
// to reach for csl and in which form, the MCP tools or the CLI, embedded at
// build time. It is the one copy of those rules: `csl docs --claude-md`
// prints it verbatim, and the csl recipe links the file to
// ~/.claude/CLAUDE.csl.md for a global CLAUDE.md to import. Unlike
// OperatingDoc it holds no template placeholders. The excerpt in README.md
// under "Add this to your CLAUDE.md" is gated against it by
// TestREADMEExcerptIsPartOfClaudeMD, and its CLI block against the command
// tree by TestClaudeMDCLIBlockMatchesCommands in internal/cli.
//
//go:embed claude-md.md
var ClaudeMD string

// ClaudeMDCLI is the CLI-only form of the fragment (claude-md-cli.md) for a
// machine where csl is on PATH but `csl mcp` is not registered, so every
// call is a shell command. `csl docs --claude-md --cli` prints it. Its CLI
// and query-syntax sections are byte for byte those of ClaudeMD, gated by
// TestClaudeMDCLISharesSections, and it names no csl_* tool at all.
//
//go:embed claude-md-cli.md
var ClaudeMDCLI string
