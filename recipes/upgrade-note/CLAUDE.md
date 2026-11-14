# upgrade-note: skill-only recipe

Ships the upgrade-note agent skill (`SKILL.md`) from the repo-root
`skills/upgrade-note/` dir. The skill fixes what to collect before writing an
entry in `docs/UPGRADING.md`, when an entry is needed, and the form of the
entry. Skill-only: there is no companion binary, package, or service. The form
is checked by the Go test in `kit/upgradenotes`, which reads the real page.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/upgrade-note`.

## What it links

- `~/.claude/skills/upgrade-note` for Claude Code.
- `~/.agents/skills/upgrade-note` for Codex.

## Changing the form

The form lives in three places that must agree: the introduction of
`docs/UPGRADING.md`, the rules in `skills/upgrade-note/SKILL.md`, and
`kit/upgradenotes` (`Check`). Change all three in one pull request.
