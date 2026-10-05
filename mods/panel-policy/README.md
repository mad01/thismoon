# panel-policy

A Claude Code mod that turns one standing rule into engine behaviour: review and verifier subagents run on Opus. The skills that spawn them through the Agent tool (subagent-review-panel in subagent mode, sre-panel, work-on's review gates) say `sonnet` or nothing. The mod re-points the spawn on its way to the engine. Without the mod, every spawn runs exactly as the skill wrote it.

## The rule

A mod may change the model of a subagent spawn only behind a config key, only when the caller set no model or a model the key lists, and never for a model the caller chose explicitly. Here the key is `enforce`, the list is `overrideFrom`, and the target is `reviewModel`.

On `agent.spawn` the mod classifies the spawn from its description and the first line of its prompt; the rest of the prompt is the artifact and is never read. A description that opens with a work verb (implement, build, fix, update, find, write, scaffold, refactor, research, explore, promote) is work, whatever it names: "Implement review comment threading" stays put. Otherwise a review word in the description makes a reviewer: review, verify, critic, panel, audit, challenger, second opinion, analyst, assessor, adherence, validation, with their plurals. Failing that, the prompt's first line decides by its opener: `You are a/the ... reviewer|critic|auditor|verifier|analyst|assessor`, `Review`, `Audit`, `Verify`, or `Role:`. The word lists and the tests against the skills' literal labels live in `lib/policy.ts` and `test/policy.test.mjs`.

A reviewer moves to `reviewModel` when the caller set a model `overrideFrom` lists, or set none (or `inherit`) on a general-purpose spawn. Explore, Plan and plugin agents keep their own model unless the caller set a listed one. A model the caller chose that the list leaves out, say `haiku`, always stands. Teammates and forks are never touched: a fork inherits whatever a hook sets.

## Turning it off

`/config` lists the mod's options, stored under `pluginConfigs["panel-policy"].options`:

| Option | Default | Effect |
|--------|---------|--------|
| `enforce` | `true` | `false` leaves every model as the skill set it |
| `reviewModel` | `opus` | the alias or id reviewers run on |
| `overrideFrom` | `sonnet` | comma-separated models a reviewer is moved off when the caller set one of them |
| `fanoutToast` | `4` | toast once when a turn's spawn count reaches this; `0` is off |
| `fanoutHold` | `0` | ask Proceed or Cancel on the Nth spawn of a turn; `0` never asks |
| `staleMinutes` | `30` | a member in flight longer than this is stale: it leaves the in-flight count and no longer holds the band open; `0` never marks one stale |

The fan-out count takes the main loop's own spawns, not a subagent's or a teammate's. With `fanoutHold` set, spawns the model issues in parallel after the Nth proceed while the dialog is open; Cancel refuses that one spawn only.

## The band and the pane

While a batch is in flight, and for a minute after its last member returned, one dim line sits above the prompt:

```
panel: 2/5 returned · reviewers on opus
```

The batch is the spawns of the newest turn that spawned anything. The suffix names the review model only while `enforce` is on and the batch holds a reviewer. A member in flight past `staleMinutes` is stale: the line reads `panel: 1/3 returned, 1 stale`, and a batch whose members are all returned or stale clears after the linger. Nothing is drawn while idle. A one-second ticker keeps the durations and the linger moving and stops itself once the band is clear.

`/panel` opens a pane with one row per member, as many as fit the pane, newest last: role, the first 60 characters of its description, running, done or stale, and its duration. The state keeps the newest 100. Esc closes the pane.

Each member is keyed by the `agentId` the engine returns from the spawn. Two signals mark it returned, whichever arrives first. The subagent's `turn.complete` carries the id as `agentId`; the classic `SubagentStop` settings event carries it as `agent_id`, and a background subagent's end reaches the main session that way. The second signal is a no-op. A spawn that resolves without an id started no agent and is not tracked. The debug log names which signal matched, or which running ids a signal failed to match.

## Dev loop

```bash
claude --plugin-dir mods/panel-policy        # from the repo root; reloads on save
claude plugin validate mods/panel-policy     # what the engine would refuse
node --test mods/panel-policy/test/*.test.mjs
```

The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run.

## What it never does

- never denies a spawn by default: `fanoutHold` is off, and the hold's Cancel is the only path to `{ deny }`
- never blocks: every hook carries a `.catch` that logs to the debug log and passes the event on; when the hold cannot ask (a `-p` run), the spawn proceeds
- never required by any skill; the skills' own `model` lines still stand without it
