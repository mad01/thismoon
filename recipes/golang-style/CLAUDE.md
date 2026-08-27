# `golang-style` recipe

Ships the golang-style agent skill (the Go style guide: `SKILL.md` plus its
`references/` pages) from the repo-root `skills/golang-style/` dir. Skill-only:
there is no companion binary, package, or service.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/golang-style`.

## What this recipe does

- **`dotfiles.golang_style_skill`** — symlinks `skills/golang-style/` into
  `~/.claude/skills/golang-style`.
- **`dotfiles.golang_style_codex_skill`** — symlinks the same dir into
  `~/.agents/skills/golang-style` so Codex and other agents discover it too.

No profile filter — the skill lands on every machine, same as when it shipped
from the consuming repo's vendored skill set.

## See also

- Skill content: `skills/golang-style/SKILL.md`
- Layout rationale: skills live at the repo root so they are discoverable by
  browsing the repo; each recipe links only its own skill directory.
