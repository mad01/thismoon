# `factory-eval` recipe

Builds and installs the `factory-eval` CLI, an experimental eval of the factory.
Source lives in this repo: `tools/factory-eval/` (see
`tools/factory-eval/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/factory-eval`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/factory-eval`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/factory-eval`. Nothing depends on it.
- Package only, the public layer per docs/adr/0006. The recipe never runs a
  batch: a paid batch starts only by hand, against a signed-off cases file.

## See also

- Source and module notes: `tools/factory-eval/CLAUDE.md`
