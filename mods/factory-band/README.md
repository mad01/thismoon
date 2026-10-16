# factory-band

Experimental. A Claude Code mod for the session that runs a factory plan (the factory skill). It draws one band above the prompt with a row per worker, two run counts and one button. The skill is unchanged; without the mod, a factory run works as before. Like the skill, the band will change as pilot runs show what breaks.

```
factory · 2 workers · denies 1 · parked 0   [ weave now ]
events     ready      claude-sonnet-5-5  -
csl-prune  in flight  claude-opus-5-5    -
```

The band stays hidden until the first factory worker spawns, and it yields to a survey. It shows what no other surface puts in one place: which worktree is ready to land.

## Where each field comes from

- Worker: a spawn of `factory-worker-low`, `-medium` or `-high`, with or without a plugin prefix. The name is the spawn's `name`, else the task's `unit`.
- Worktree: the spawn's cwd, else the `worktree:` line of the task header in the spawn prompt.
- State, in the loom skill's words plus `blocked` from the factory contract. A worker whose loop runs is `in flight`. A clean tree whose worker sent `kind: ready` is `ready` while it is ahead of the default branch, and `integrated` once it is not. A worker whose last message was `blocked` or `question` is `blocked`. An idle worker is still `in flight`; one that ended or left the agent list leaves its worktree `abandoned`. The kind comes from the worker's SendMessage or, for an unnamed worker, its final answer. Git answers from the worktree: `status --porcelain` and `rev-list --count origin/main..HEAD`, with the default branch read from `origin/HEAD`.
- Model: what the spawn resolved, else the ledger's `model` tag.
- Effort: the ledger's `effort` tag. belt does not tag effort or model yet (MAD-406), so effort shows `-` today.
- denies: belt's guard denials since the first worker spawned, read off the PreToolUse decision the way session-band reads them. Added to that, the auto-mode denials belt's ledger recorded for this session in the same window (`belt-ledger`, `event: permission-denied`).
- parked: items on `belt-inbox` whose `to` tag is this session, posted since the first worker spawned.

The band is read again after a worker spawns, when a worker reports or ends a turn, and when a main-loop turn ends. One refresh runs at a time and none starts within 5 s of the last; one trailing refresh at the 5 s mark catches up whatever was skipped. Each git call and each events query (`localhost:7430`) gets 400 ms, and they run side by side. A source that fails, refuses or runs out of time reads as unknown: a field turns to `-`, parked turns to `-`, and denies falls back to the guard count.

## Weave now

The button (hotkey `w` once the band has focus) runs `/loom weave` through the engine's command API, as if typed, once the session is idle. The loom skill does the weaving; loom-pane, when enabled, sees the command and names the session the weaver. A command that cannot start says why in a toast. The button turns primary while any worker is ready.

## Contract guard

Off by default. With `contractGuard` on, a factory worker spawn whose prompt does not open with the eight-line task header is denied, so the band never tracks a worker it cannot place. A resumed worker and every other agent type pass. The guard fails open on a throw and duplicates no belt guard.

## What it never does

- never adds context or edits the transcript; every hook passes its event on unchanged
- never denies, except the contract guard above
- never errors or toasts when the events service is down
- never required by any skill, hook or service; belt stays the guard

Known gaps: a squash-merged branch never becomes reachable from the default branch, so it stays `ready` (loom's own `branch --merged` check has the same gap). The ledger and inbox are unwired on machines until the dotfiles hook wiring lands; until then effort, the ledger denies and parked read from nothing. The live pilot is MAD-402.

## Config

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `events` | boolean | `true` | query the events service for the ledger and the inbox |
| `contractGuard` | boolean | `false` | deny a worker spawn without a task header |

## Dev loop

```bash
claude --plugin-dir mods/factory-band          # from a checkout; a save hot-reloads
claude plugin validate mods/factory-band       # what the engine would refuse
node --test mods/factory-band/test/*.test.mjs  # header, ledger, inbox, state and band text in lib/
claude plugin test mods/factory-band           # the band, the button and the guard against the engine's test kit
```
