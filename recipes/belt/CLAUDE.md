# `belt` recipe

Builds and installs the `belt` CLI. Source lives in this repo: `tools/belt/`
(see `tools/belt/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/belt`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/belt`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/belt` before the consuming repo's claude
  recipe (wave 1) merges the settings template that registers the PreToolUse
  hooks pointing at it.
- Package only — public layer per docs/adr/0006. The guard-toggle config
  overlay (`~/.config/belt/config.toml`, carries internal-name
  `exclude_paths`) and the hook registration are machine-private wiring and
  live in the consuming repo's companion recipe.

The item key `packages.belt` is the same one the dotfiles recipe used before
the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
