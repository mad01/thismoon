# Recipe for the factory skill

Ships the factory agent skill from the repo-root `skills/factory/` dir: the
procedure (`SKILL.md`), the message contract, the routing table, the worker
brief and the worker definitions, plus the eval suites under `evals/`. One
orchestrator session runs a plan through worker subagents, one loom worktree
per unit. There is no companion binary, package, or service. The skill is
experimental.

This recipe is consumed remotely: a machine's ralph config declares a
`[[recipe_sources]]` stanza pointing at this repo (source name `thismoon`),
and ralph merges the recipe with the identity `thismoon/factory`.

## What this recipe does

- **`dotfiles.factory_skill`** symlinks `skills/factory/` into
  `~/.claude/skills/factory`.
- **`dotfiles.factory_codex_skill`** symlinks the same dir into
  `~/.agents/skills/factory` so Codex discovers it too.
- **`dotfiles.factory_worker_{low,medium,high}`** link the three worker
  definitions, `skills/factory/agents/factory-worker-<tier>.md`, into
  `~/.claude/agents/`, one link per file. `directories.claude_agents_dir`
  declares that directory.
- **`dotfiles.factory_worker_agents_json`** links `agents.json` to
  `~/.config/factory/agents.json`. It holds the same three definitions in the
  `--agents` form an unattended worker needs, since `claude -p` with
  `--setting-sources ""` skips `~/.claude/agents`.
- **`dotfiles.factory_worker_hint`** links
  `skills/factory/factory-worker-hint.sh` to
  `~/.config/factory/factory-worker-hint.sh`. It is the script behind belt's
  `factory-worker` custom hint, which prints the worker brief from the
  installed skill to factory workers and nothing to any other subagent.

No profile filter: everything lands on every machine.

## What stays in the consuming repo

Machine-private wiring, per `docs/adr/0006`:

- the belt config entry that registers the `factory-worker` hint on
  `subagent-start` with the path above (`skills/factory/worker-brief.md`
  shows the shape), and the ledger tags for the three agent types
- the settings file and MCP file an unattended worker is launched with
- any routing override in `~/.config/factory/routing.yaml`
  (`skills/factory/routing.md`)

## Gotchas

- Edit the definitions under `skills/factory/agents/`, never `agents.json`.
  Then run `python3 recipes/factory/render_agents_json.py`.
  `agents-json-test.sh` runs the same script with `--check` under
  `make test-recipes`, so CI fails while the two differ.
- Keep each definition's `name` equal to its file name. An unnamed spawn
  reports the frontmatter `name` as its agent type, and the hint matches on
  it.
- The item keys are the ones the consuming repo used before these files
  moved here, so ralph state carried over. They are global across sources:
  a consuming recipe that declares one of them again fails validation.

## See also

- Skill content: `skills/factory/SKILL.md`
- Worker definitions: `skills/factory/worker-definitions.md`
- Worker brief and hint delivery: `skills/factory/worker-brief.md`
- Message contract: `skills/factory/contract.md`
- Worktree coordination it builds on: `skills/loom/` and
  `docs/adr/0014-loom-skill-only-worktree-coordination.md`
- Eval suites: `skills/factory/evals/README.md`
