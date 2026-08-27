# `opener` recipe

Builds and installs the `opener` CLI + MCP server. Source lives in this
repo: `tools/opener/` (see `tools/opener/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/opener`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/opener`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/opener` before the consuming repo's MCP
  registration recipe (wave 1) registers `opener mcp`.
- Package only — public layer per docs/adr/0006. MCP registration is
  machine-private wiring and lives in the consuming repo's companion recipe.

## See also

- Source + module notes: `tools/opener/CLAUDE.md`
