# ask: skill-only recipe

Ships the ask agent skill (`SKILL.md`: answering a question from the brain
vault and citing the notes behind the answer) from the repo-root `skills/ask/`
dir. Skill-only: there is no companion binary, package, or service.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/ask`.

## What this recipe does

- **`dotfiles.ask_skill`**: symlinks `skills/ask/` into
  `~/.claude/skills/ask`.
- **`dotfiles.ask_codex_skill`**: symlinks the same dir into
  `~/.agents/skills/ask` so Codex discovers it too.

No profile filter: the skill lands on every machine.

## What it needs

The skill reads the vault through the brain MCP, with the `brain` CLI as
fallback. The `brain` recipe builds the binary; registering the MCP server is
machine-private and lives in the consuming repo. The recipe declares no
`depends_on`: without brain the skill still loads, and its tool-backed steps
are unavailable.

## See also

- Skill content: `skills/ask/SKILL.md`
- The vault tools: `services/brain/docs/mcp.md`
- Layout rationale: skills live at the repo root so they are discoverable by
  browsing the repo; each recipe links only its own skill directory.
