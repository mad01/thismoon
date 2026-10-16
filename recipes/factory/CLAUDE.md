# Recipe for the factory skill (skill-only)

Ships the factory agent skill from the repo-root `skills/factory/` dir: the
procedure (`SKILL.md`), the message contract, the routing table, the worker
brief and the worker definitions, plus the eval suites under `evals/`. One
orchestrator session runs a plan through worker subagents, one loom worktree
per unit. Skill-only: there is no companion binary, package, or service.
The skill is experimental.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/factory`.

## What this recipe does

- **`dotfiles.factory_skill`** symlinks `skills/factory/` into
  `~/.claude/skills/factory`.
- **`dotfiles.factory_codex_skill`** symlinks the same dir into
  `~/.agents/skills/factory` so Codex discovers it too.

No profile filter: the skill lands on every machine.

The recipe does not install the three worker agent definitions or the
`factory-worker` belt hint that delivers the worker brief on subagent-start.
Both are machine-private wiring in the consuming repo;
`skills/factory/worker-definitions.md` and `skills/factory/worker-brief.md`
hold the text to copy.

## See also

- Skill content: `skills/factory/SKILL.md`
- Message contract: `skills/factory/contract.md`
- Worktree coordination it builds on: `skills/loom/` and
  `docs/adr/0014-loom-skill-only-worktree-coordination.md`
- Eval suites: `skills/factory/evals/README.md`
