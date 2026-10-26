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
- **`humanizer-mcp-sandbox.sh`**, kept executable by
  `hooks.builds.humanizer_chmod_mcp_wrapper`: the spawn wrapper the consuming
  repo's MCP registration names instead of the bare binary. It pulls only
  the judge backend vars (`OPENROUTER_API_KEY`, `LITELLM_BASE_URL`,
  `LITELLM_API_KEY`, `HUMANIZER_MODEL`) from the ralph-managed secrets file,
  rebuilds the environment with an `env -i` allow-list, `cd`-s to `/`, derives
  the judge host from those vars (the host of `LITELLM_BASE_URL`, else
  `openrouter.ai`) and passes it as `SRT_RUN_ALLOW_DOMAINS`, then execs
  `~/code/bin/srt-run ~/.config/srt/humanizer.json ~/code/bin/humanizer mcp`.
  The `vale` subprocess inherits the sandbox. Without the settings file srt
  exits 1 (fail closed).
- Public layer per docs/adr/0006. MCP registration, the srt settings and the
  `config.yaml` symlink are machine-private wiring and live in the consuming
  repo's companion recipe. The code roots the server may read differ per
  machine, so the access manifest is the consuming repo's;
  `examples/dotfiles/recipes/humanizer/` is one to start from.
  `docs/SANDBOXING.md` has the triage guide.

The item key `packages.humanizer` is the same one the dotfiles recipe used
before the migration, so ralph state carries across the cutover with no
uninstall/reinstall churn.
