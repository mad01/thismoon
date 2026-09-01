# loom — skill-only recipe

Ships the loom agent skill (`SKILL.md` + `loom.md`: each agent session
claims its own git worktree under `~/.worktrees/` and commits there; one
weaver session rebases the finished branches and lands them on the default
branch in order) from the repo-root `skills/loom/` dir. Skill-only: there is
no companion binary, package, or service — git itself is the state store.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/loom`.

loom replaces the commit-pipeline recipe (the shared-checkout commit queue
under `~/.commits/`). The item keys are new, so a machine that had
commit-pipeline enabled needs its enable list updated and the old
`~/.claude/skills/commit-pipeline` / `~/.agents/skills/commit-pipeline`
symlinks cleaned up — they dangle once the source dir is gone.

## What this recipe does

- **`dotfiles.loom_skill`** — symlinks `skills/loom/` into
  `~/.claude/skills/loom`.
- **`dotfiles.loom_codex_skill`** — symlinks the same dir into
  `~/.agents/skills/loom` so Codex discovers it too.

No profile filter — the skill lands on every machine.

## See also

- Skill content: `skills/loom/SKILL.md`
- Protocol: `skills/loom/loom.md`
- Decision record: `docs/adr/0014-loom-skill-only-worktree-coordination.md`
- Layout rationale: skills live at the repo root so they are discoverable by
  browsing the repo; each recipe links only its own skill directory.
