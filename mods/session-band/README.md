# session-band

A Claude Code mod that counts belt denials in this session and pins the count in the status row under the prompt, from the first deny on:

```
session-band: belt denies 2
```

The engine prefixes a plugin's status line with its name; the text the mod composes starts at `belt`. Before the first deny the line stays clear, so a quiet session shows nothing. Nothing is drawn above the prompt: the mod registers no `ui.render` hook, so the engine's band area stays hidden.

## Where the count comes from

`classic.PreToolUse`: the decision `next(e)` hands back is belt's own, and a `deny` whose reason carries `belt[<guard>]:` anywhere counts as one (the engine may wrap the reason in its own hook-error prefix). The count is per session, since the mod only sees this session's calls. It lives in `$.state`, so it survives a hot reload and a compaction; `/clear` puts it back to zero and the line goes with it on the next tool call.

## pi face

`pi/index.ts` keeps the same count in the pi coding agent. pi has no hook chain to read a decision from, so the dotfiles permission gate announces each belt denial on the extension bus as `belt:deny`; the face counts those, keeps the count in the session (`pi.appendEntry`, restored from the active branch), and pins `belt denies N` with `ctx.ui.setStatus`. Silent without a UI.

## Dev loop

```bash
claude --plugin-dir mods/session-band        # from the repo root; a save hot-reloads
claude --debug --plugin-dir mods/session-band  # every pin lands in the debug log as `$.ui.status (session-band): <text>`
claude plugin validate mods/session-band     # what the engine would refuse
node --test mods/session-band/test/*.test.mjs  # lib and the pi face
npx -p typescript tsc -p mods/session-band   # after one load wrote .claude-plugin/types/
```

The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run. Every `classic.PreToolUse` decision is described in the debug log (its keys and the head of its JSON), so a deny that failed to count can be read off the log.

## What it never does

- never blocks: every hook carries a `.catch` that logs to the debug log and passes the event on, and the `PreToolUse` observer returns belt's decision untouched
- never edits the transcript or the model's answer
- never probes a service: the one signal comes from an event the session already raises
- never required by any skill, hook, or service; belt stays the guard
