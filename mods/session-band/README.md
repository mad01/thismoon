# session-band

A Claude Code mod that pins one line in the status row under the prompt:

```
session-band: working | worklog MAD-379 | Phase 4.unit.2/3 | present 1e52d87017 | belt denies 0
```

The engine prefixes a plugin's status line with its name; the text the mod composes starts at `working`. The line is recomposed on every change of a signal and cleared when there is nothing to say. Nothing is drawn above the prompt: the mod registers no `ui.render` hook, so the engine's band area stays hidden.

## What it shows, and where each signal comes from

| Segment | Source |
|---------|--------|
| agent state (`working`, `waiting for input`, `idle`) | `working` is set by `turn.start`, which only the main loop raises, and cleared by the main loop's `turn.complete`; `waiting` is set by `classic.Notification` with `permission_prompt` or `elicitation_dialog` on the main thread and cleared by `classic.PostToolUse`, `classic.PermissionDenied`, or `turn.complete` |
| active worklog key | `worklog list --status active --repo <repo>` at session start (`~/code/bin/worklog`, or `worklog` on PATH; repo from `git rev-parse --path-format=absolute --git-common-dir`), then every `worklog_checkpoint` call |
| work-on phase | the last line of an answer that starts `[Phase ...]` or `[<key> Phase ...]`, the marker the work-on skill prints after each interaction; the line shows the bracket text, key included |
| present page | the `{id, url}` JSON a `present_create` or `present_update` call returns |
| belt denies | `classic.PreToolUse`: the decision `next(e)` hands back is belt's own, and a `deny` whose reason carries `belt[<guard>]:` anywhere (the engine may wrap it in its own hook-error prefix) counts one. Per session, since the mod only sees this session's calls |

Toasts: `done (N s)` when an answered turn ran longer than 60 s, `needs input` on a permission prompt. Both last 4 s.

After a compaction the mod adds a short Claude-only context block to the next message through `prompt.context`. It carries the worklog key, the last phase marker, the present page id, and a reminder to checkpoint. `classic.SessionStart` with `source: compact` arms it, one injection disarms it. The dotfiles compact hook keeps its own job; this block sits beside it.

## Dev loop

```bash
claude --plugin-dir mods/session-band        # from the repo root; saves hot-reload
claude --debug --plugin-dir mods/session-band  # every pin lands in the debug log as `$.ui.status (session-band): <text>`
claude plugin validate mods/session-band     # what the engine would refuse
node --test mods/session-band/test/*.test.mjs
npx -p typescript tsc -p mods/session-band   # after one load wrote .claude-plugin/types/
```

The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run. Every `classic.PreToolUse` decision is also described there (its keys and the head of its JSON), so a deny that failed to count can be read off the log.

## What it never does

- never blocks: every hook carries a `.catch` that logs to the debug log and passes the event on, and the `PreToolUse` observer returns belt's decision untouched
- never edits the transcript or the model's answer; the status line is a pin, the context block is extra, not a rewrite
- never probes a service: every signal comes from events the session already raises
- never required by any skill, hook, or service; belt stays the guard
