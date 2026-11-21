# `cards` recipe

Builds and installs the `cards` CLI. Source lives in this repo:
`tools/cards/` (see `tools/cards/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/cards`. So the package
`working_dir` names the source cache
(`~/.config/ralph/sources/thismoon/tools/cards`), not a dev checkout.

## What this recipe does

- Builds and installs `~/code/bin/cards`, nothing else.
- No `wave`: no recipe in a consuming repo builds against cards or
  registers it, so it needs no place in the order.
- Package only, the public layer per docs/adr/0006. cards has no config
  file, no MCP server and no service, so the consuming repo adds nothing.

## See also

- Source and module notes: `tools/cards/CLAUDE.md`
