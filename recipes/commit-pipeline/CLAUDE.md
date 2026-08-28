# commit-pipeline — skill-only recipe

Ships the commit-pipeline agent skill (`SKILL.md` + `pipeline.md`: one
committer session serializes the commits that requester sessions queue as
files under `~/.commits/`) from the repo-root `skills/commit-pipeline/` dir.
Skill-only: there is no companion binary, package, or service.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`), and
ralph merges the recipe with the identity `thismoon/commit-pipeline`.

## What this recipe does

- **`dotfiles.commit_pipeline_skill`** — symlinks `skills/commit-pipeline/`
  into `~/.claude/skills/commit-pipeline`.
- **`dotfiles.commit_pipeline_codex_skill`** — symlinks the same dir into
  `~/.agents/skills/commit-pipeline` so Codex discovers it too.

No profile filter — the skill lands on every machine.

## See also

- Skill content: `skills/commit-pipeline/SKILL.md`
- Protocol: `skills/commit-pipeline/pipeline.md`
- Layout rationale: skills live at the repo root so they are discoverable by
  browsing the repo; each recipe links only its own skill directory.
