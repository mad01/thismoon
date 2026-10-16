# factory-band

Experimental. A Claude Code mod for the session that runs a factory plan (the factory skill). It draws one band above the prompt with a row per worker, two run counts and one button. The skill is unchanged; without the mod, a factory run works as before. Like the skill, the band will change as pilot runs show what breaks.

```
factory · 2 workers · denies 1 · inbox 0   [ weave now ]
events     ready      claude-sonnet-5-5  -
csl-prune  in flight  claude-opus-5-5    -
```

The band stays hidden until the first factory worker spawns, and it yields to a survey. It shows what no other surface puts in one place: which worktree is ready to land.

## Where each field comes from

- Worker: a spawn of `factory-worker-low`, `-medium` or `-high`, with or without a plugin prefix. The name is the spawn's `name`, else the task's `unit`.
- Worktree: the spawn's cwd, else the `worktree:` line of the task header in the spawn prompt.
- State: one of the states below, from the agent list, git in the worktree, and the kind of the worker's last message. That kind is the last `kind:` line (`status`, `question`, `ready` or `blocked`) of its SendMessage, or of its final answer for an unnamed worker.
- Model: what the spawn resolved, else the ledger's `model` tag.
- Effort: the ledger's `effort` tag. belt does not tag effort or model yet (MAD-406), so effort shows `-` today.
- denies: belt's guard denials of the workers' own tool calls, plus the auto-mode denials belt's ledger recorded for those workers (`belt-ledger`, `event: permission-denied`). The orchestrator's denials are left to session-band.
- inbox: items on `belt-inbox` whose `to` tag is this session, posted since the first worker spawned. These are not parked questions: those live in kof and on the board.

## States

- `in flight`: the worker's loop runs or waits idle, or its tree is dirty after a ready.
- `ready`: clean, ahead of the default branch, the worker sent ready, and a merge would still change the default branch.
- `integrated`: the worker sent ready and the default branch already holds the changes, by a fast-forward or a squash merge.
- `blocked`: the worker's last message was `blocked` or `question`.
- `stopped`: the loop ended without a ready. Whether that worktree is abandoned is the weaver's call after a survey.
- `-`: git or the agent list did not answer.

Git answers from the worktree: `rev-parse` for `origin/HEAD` and its tree, `status --porcelain`, `rev-list --count`. For a ready worker's clean tree, one `merge-tree --write-tree` more: a merged tree equal to the default branch's tree means integrated. An integrated worker whose loop ended drops off the band; the last one closes the run.

The band is read again after a worker spawns, when a worker reports or ends a turn, and when a main-loop turn ends. One refresh runs at a time and none starts within 5 s of the last. One trailing refresh at the 5 s mark catches up what was skipped. A worker's turn end always gets one, since the agent list can still say running at that moment. Each worker's git and each events query (`localhost:7430`) gets 400 ms, side by side. A source that fails or runs out of time reads as unknown.

## Weave now

The button (hotkey `w` once the band has focus) runs `/loom weave` through the engine's command API, as if typed, once the session is idle. The loom skill does the weaving; loom-pane, when enabled, sees the command and names the session the weaver. A command that cannot start says why in a toast. The button turns primary while any worker is ready.

## Contract guard and limits

Off by default. With `contractGuard` on, a factory worker spawn whose prompt does not open with the eight-line task header is denied. A resumed worker and every other agent type pass. The guard fails open and duplicates no belt guard. Otherwise the mod never denies, never adds context, and stays silent when the events service is down; no skill, hook or service requires it. Known gaps: git before 2.38 has no `merge-tree --write-tree`, so a squash-merged branch stays `ready` there. The ledger and inbox are unwired on machines until the dotfiles hook wiring lands. The live pilot is MAD-402.

## Config

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `events` | boolean | `true` | query the events service for the ledger and the inbox |
| `contractGuard` | boolean | `false` | deny a worker spawn without a task header |

## Dev loop

```bash
claude --plugin-dir mods/factory-band          # from a checkout; a save hot-reloads
claude plugin validate mods/factory-band       # what the engine would refuse
node --test mods/factory-band/test/*.test.mjs  # header, kinds, ledger, inbox, states, squash merge and band text
claude plugin test mods/factory-band           # the band, the button and the guard against the engine's test kit
npx -p typescript tsc -p mods/factory-band     # after one load wrote .claude-plugin/types/
```
