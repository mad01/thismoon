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

// ClaudeMD is the CLAUDE.md snippet (claude-md.md) that teaches an agent when
// to reach for the csl_* MCP tools, embedded at build time. It is printed
// verbatim by `csl docs --claude-md`; unlike OperatingDoc it holds no template
// placeholders. The copy in README.md under "Add this to your CLAUDE.md" is
// gated against it by TestClaudeMDMatchesREADME.
//
//go:embed claude-md.md
var ClaudeMD string
