# panel-policy

A Claude Code mod that turns one standing rule into engine behaviour: review and verifier subagents run on Opus. The skills that spawn them (subagent-review-panel, sre-panel, work-on's review gates) keep saying `sonnet` or nothing; the mod re-points the spawn on its way to the engine. Without the mod, every spawn runs exactly as the skill wrote it.

## The rule

On `agent.spawn` the mod reads the description and the first 400 characters of the prompt. A spawn is a reviewer when either carries one of: review, reviewer, verify, verifier, critic, panel, audit, challenger, second opinion (whole words, any case, plurals included). A description that instead announces research, explore, implement, build, promote, promoter, write, draft, fix, scaffold or refactor settles the spawn as other before the prompt is read. That keeps work-on's impact agent ("name a verify method per consumer") and the brain-research promoter ("verified inbox note") on their own model. A reviewer whose model is not already `reviewModel` goes out as `next({ ...e, model: reviewModel })`. The word list and the tests against the skills' real phrasing live in `lib/policy.ts` and `test/policy.test.mjs`.

Left alone: teammates (`isTeammate`), forks (the engine ignores `model` on a fork, so there is nothing to set), and every spawn while `enforce` is off. A description that names a panel as the thing to build counts as review; phrase around it or turn `enforce` off for that session.

## Turning it off

`/config` lists the mod's options, stored under `pluginConfigs["panel-policy"].options`:

| Option | Default | Effect |
|--------|---------|--------|
| `enforce` | `true` | `false` leaves every model as the skill set it |
| `reviewModel` | `opus` | the alias or id reviewers run on |
| `fanoutToast` | `4` | toast once when a turn's spawn count reaches this; `0` is off |
| `fanoutHold` | `0` | ask Proceed or Cancel on the Nth spawn of a turn; `0` never asks |

## The band and the pane

While a batch is in flight, and for a minute after its last member returned, one dim line sits above the prompt:

```
panel: 2/5 returned · reviewers on opus
```

The batch is the set of spawns from the newest turn that spawned anything; the model suffix goes with `enforce: false`. Nothing is drawn while idle.

`/panel` opens a pane listing the last 100 members this session: role, the first 60 characters of its description, running or done, and its duration. It redraws on every spawn and return; Esc closes it.

The mod keys each member by the `agentId` the engine returns from the spawn, the same id the subagent's `turn.complete` carries. Should a spawn resolve without one, the mod keys the member by the Agent tool call's id, and the next unmatched `turn.complete` marks the oldest such running member done, by order.

## Dev loop

```bash
claude --plugin-dir mods/panel-policy        # from the repo root; saves hot-reload
claude plugin validate mods/panel-policy     # what the engine would refuse
node --test mods/panel-policy/test/*.test.mjs
```

The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run.

## What it never does

- never denies a spawn by default: `fanoutHold` is off, and the hold's Cancel is the only path to `{ deny }`
- never blocks: every hook carries a `.catch` that logs to the debug log and passes the event on; when the hold cannot ask (a `-p` run), the spawn proceeds
- never required by any skill; the skills' own `model` lines still stand without it
