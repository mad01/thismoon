# handoff — skill-only recipe

Ships the handoff agent skill (`SKILL.md`: writing a cold-start handoff doc
and persisting session learnings) from the repo-root `skills/handoff/` dir.
Skill-only: there is no companion binary, package, or service.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/handoff`.

## What this recipe does

- **`dotfiles.handoff_skill`** — symlinks `skills/handoff/` into
  `~/.claude/skills/handoff`.
- **`dotfiles.handoff_codex_skill`** — symlinks the same dir into
  `~/.agents/skills/handoff` so Codex discovers it too.

No profile filter — the skill lands on every machine.

## See also

- Skill content: `skills/handoff/SKILL.md`
- Layout rationale: skills live at the repo root so they are discoverable by
  browsing the repo; each recipe links only its own skill directory.
