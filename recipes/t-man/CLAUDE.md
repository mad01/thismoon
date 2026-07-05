# `t-man` recipe

Builds and installs the `t-man` CLI. Source lives in this repo: `tools/t-man/`
(see `tools/t-man/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/t-man`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/t-man`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/t-man` like the other Go tools.
- Package only — no service block, no `t-man add` hook. t-man is the tool the
  other recipes register their services *with*; it is not a service itself.
  Already-registered launchd services keep running across a binary rebuild.

## Relationship to the rest of the fleet

Every service recipe in this repo (and several in the consuming dotfiles repo)
carries `depends_on = ["packages.t_man"]` so registration hooks order after
this package builds. That hard dep on a platform foundation is deliberate —
docs/adr/0006.

The item key `packages.t_man` is the same one the dotfiles `packages` recipe
used before the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
