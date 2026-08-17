# `humanizer` recipe

Builds and installs the `humanizer` CLI + MCP server. Source lives in this
repo: `tools/humanizer/` (see `tools/humanizer/CLAUDE.md`).

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/humanizer`. The package
`working_dir` therefore names the source cache
(`~/.config/ralph/sources/thismoon/tools/humanizer`), not a dev checkout.

## What this recipe does

- `wave = 0`: builds `~/code/bin/humanizer` before the consuming repo's MCP
  registration recipe (wave 1) points the MCP host at it.
- **`dotfiles.humanizer_skill` / `dotfiles.humanizer_codex_skill`** — symlink
  the repo-root `skills/humanizer/` dir into `~/.claude/skills/` and
  `~/.agents/skills/` so the skill ships with the tool on every machine.
- Public layer per docs/adr/0006. MCP registration, the sandbox wrapper +
  seatbelt profile, and the `config.yaml` symlink are machine-private wiring
  and live in the consuming repo's companion recipe.

The item key `packages.humanizer` is the same one the dotfiles recipe used
before the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
