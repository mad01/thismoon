# `belt` recipe

Builds and installs the `belt` CLI. Source lives in this repo: `tools/belt/`
(see `tools/belt/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/belt`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/belt`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/belt` before the consuming repo's settings
  recipe (wave 1, `examples/dotfiles/recipes/agent-config`) has agent-sync
  merge the settings template that registers the hooks pointing at it.
- Package only, the public layer per docs/adr/0006. The guard-toggle config
  overlay (`~/.config/belt/config.yaml`, carries internal-name
  `exclude_paths`; a legacy `config.toml` beside it is read only when the
  YAML file is absent) and the hook registration are machine-private wiring and
  live in the consuming repo's companion recipe.
- The pi package `tools/belt/pi` gets no item. pi loads it in place from
  `~/.config/ralph/sources/thismoon/tools/belt/pi`, so the sources cache ralph
  refreshes on every `ralph up` is the install, and the package calls the
  binary this recipe builds. The consuming repo declares the package in pi's
  `settings.json`, renders `~/.pi/agent/belt.json` when a machine needs more
  than the defaults, and checks that the package path exists.

The item key `packages.belt` is the same one the dotfiles recipe used before
the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
