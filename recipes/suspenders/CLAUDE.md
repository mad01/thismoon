# `suspenders` recipe

Builds and installs the `suspenders` offline git secret scanner. Source lives
in this repo: `tools/suspenders/` (see `tools/suspenders/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/suspenders`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/suspenders`), not a dev checkout.

## What this recipe does

- `wave = 0`: the binary must exist before consuming-repo recipes whose hooks
  run `suspenders hook install --all` against it.
- Package only — public layer per docs/adr/0006. The machine-private config
  overlay (`~/.config/suspenders/config.yaml`, the real blocked words and safe
  references) and the hook install/uninstall wiring live in the consuming
  repo's companion recipe.

## See also

- Source + module notes: `tools/suspenders/CLAUDE.md`
