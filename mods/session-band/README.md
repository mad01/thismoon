# session-band

A Claude Code mod that draws one or two dim lines above the prompt:

```
* working | worklog MAD-379 | Phase 4.unit.2/3 | present 1e52d87017 | belt denies 0
```

## What it shows, and where each signal comes from

| Segment | Source |
|---------|--------|
| agent state (`working`, `waiting for input`, `idle`) | `turn.start`, `turn.complete`, every `tool.call`, and `classic.Notification` with `permission_prompt` or `elicitation_dialog` |
| active worklog key | `~/code/bin/worklog list --status active --repo <repo>` at session start (repo from `git rev-parse --git-common-dir`), then every `worklog_checkpoint` call |
| work-on phase | the last line of an answer that starts `[Phase ...]`, the marker the work-on skill prints after each interaction |
| present page | the id and url in a `present_create` or `present_update` result |
| belt denies | `GET http://localhost:7430/api/events?source=belt&since=<cursor>` every 15 s, counting titles that start `blocked ` |

Toasts: `done (N s)` when a turn ran longer than 60 s, `needs input` on a permission prompt. Both last 4 s.

After a compaction the mod adds a short Claude-only context block: the worklog key, the last phase marker, the present page id, and a reminder to checkpoint. It goes in through `classic.SessionStart` and again through `prompt.context` for the first message after it. The dotfiles compact hook keeps its own job; this block sits beside it.

## Dev loop

```bash
claude --plugin-dir mods/session-band        # from the repo root; saves hot-reload
claude plugin validate mods/session-band     # what the engine would refuse
node --test mods/session-band/test/*.test.mjs
```

The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run.

## What it never does

- never blocks: every hook carries a `.catch` that logs to the debug log and passes the event on. The events probe has a 400 ms budget and stays silent when the service is down
- never edits the transcript or the model's answer; the band is a drawing, the context block is extra, not a rewrite
- never required by any skill, hook, or service; belt stays the guard
