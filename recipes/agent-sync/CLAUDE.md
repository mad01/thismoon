# `agent-sync` recipe

Builds and installs the `agent-sync` CLI. Source lives in this repo:
`tools/agent-sync/` (see `tools/agent-sync/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/agent-sync`. So the package
`working_dir` names the source cache
(`~/.config/ralph/sources/thismoon/tools/agent-sync`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/agent-sync` before the consuming repo's
  sync hooks (wave 1) call it.
- Package only, the public layer per docs/adr/0006. It never runs a sync.
  The consuming repo ships the data and the config, and adds one build
  hook per section it uses (`settings`, `mods`, `mcp`) that runs
  `agent-sync <section> sync` with `verify = "agent-sync <section> check"`.

## See also

- Source and module notes: `tools/agent-sync/CLAUDE.md`
