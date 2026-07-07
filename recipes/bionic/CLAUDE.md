# `bionic` recipe

Builds and installs the `bionic` CLI + MCP server. Source lives in this repo:
`tools/bionic/` (see `tools/bionic/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/bionic`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/bionic`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/bionic` before the consuming repo's MCP
  registration recipe (wave 1) points the MCP host at it.
- Package only — public layer per docs/adr/0006. MCP registration and the
  sandbox wrapper + seatbelt profile are machine-private wiring and live in
  the consuming repo's companion recipe.

The item key `packages.bionic` is the same one the dotfiles recipe used
before the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
