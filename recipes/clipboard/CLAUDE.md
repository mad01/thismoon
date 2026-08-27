# `clipboard` recipe

Builds and installs the `clipboard` CLI + MCP server. Source lives in this
repo: `tools/clipboard/` (see `tools/clipboard/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/clipboard`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/clipboard`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/clipboard` before the consuming repo's MCP
  registration recipe (wave 1) registers `clipboard mcp`.
- Package only — public layer per docs/adr/0006. MCP registration is
  machine-private wiring and lives in the consuming repo's companion recipe.

## See also

- Source + module notes: `tools/clipboard/CLAUDE.md`
