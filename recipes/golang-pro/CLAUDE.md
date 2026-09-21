# `golang-pro` recipe

Ships the golang-pro agent skill (`SKILL.md` plus its `references/` pages on
concurrency, generics, interfaces, profiling, project structure, and testing)
from the repo-root `skills/golang-pro/` dir. Skill-only: there is no companion
binary, package, or service.

The skill is vendored, not written here. It comes from
[jeffallan/claude-skills](https://github.com/jeffallan/claude-skills) under
the MIT license; `skills/golang-pro/LICENSE` carries the upstream notice and
the skill's frontmatter names the author. Refresh it by copying the upstream
`skills/golang-pro/` directory over ours and recording the upstream commit in
the commit message.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/golang-pro`.

## What this recipe does

- **`dotfiles.golang_pro_skill`** — symlinks `skills/golang-pro/` into
  `~/.claude/skills/golang-pro`.
- **`dotfiles.golang_pro_codex_skill`** — symlinks the same dir into
  `~/.agents/skills/golang-pro` so Codex and other agents discover it too.

No profile filter — the skill lands on every machine, same as when it shipped
from the consuming repo's vendored skill set.

## See also

- Skill content: `skills/golang-pro/SKILL.md`
- The house-style half: `skills/golang-style/SKILL.md` says which of this
  skill's generic advice the codebase overrides.
