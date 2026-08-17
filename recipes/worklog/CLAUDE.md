# `worklog` recipe

Builds and installs the `worklog` CLI + MCP server. Source lives in this
repo: `tools/worklog/` (see `tools/worklog/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/worklog`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/worklog`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/worklog` before the consuming repo's MCP
  registration recipe (wave 1) registers `worklog mcp`.
- **`dotfiles.worklog_skill` / `dotfiles.worklog_codex_skill`** — symlink the
  repo-root `skills/worklog/` dir into `~/.claude/skills/` and
  `~/.agents/skills/` so the skill ships with the tool on every machine.
- Public layer per docs/adr/0006. MCP registration and the ticket-firewall
  config overlay (`~/.config/worklog/config.yaml`) are machine-private wiring
  and live in the consuming repo's companion recipe.

The item key `packages.worklog` is the same one the dotfiles recipe used
before the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
