# session-band

A Claude Code mod that counts belt denials in this session and pins the count in the status row under the prompt, from the first deny on:

```
session-band: belt denies 2
```

The engine prefixes a plugin's status line with its name; the text the mod composes starts at `belt`. Before the first deny the line stays clear, so a quiet session shows nothing. Nothing is drawn above the prompt: the mod registers no `ui.render` hook, so the engine's band area stays hidden.

## Where the count comes from

A `tool.call` observer. belt runs as a settings `PreToolUse` hook beneath every mod's `tool.call` hook, so a belt deny comes back from `next(e)` as an errored result whose text is belt's reason. A result counts when the first line of that text carries `belt[<guard>]:` (the engine may put its own hook-error prefix in front). A command that fails and quotes belt in a later line does not count, and neither does a successful call. Every caller counts: the main loop and each subagent.

The mod does not hook `classic.PreToolUse`: when the built-in guard mod is seated, it forwards every `classic.*` event past the user tier, and the hook is skipped. The count is per session. It lives in `$.state`, so it survives a hot reload and a compaction; `/clear` puts it back to zero and the line goes with it on the next tool call.

## pi face

`pi/index.ts` keeps the same count in the pi coding agent. pi has no hook chain to read a decision from, so the dotfiles permission gate announces each belt denial on the extension bus as `belt:deny`; the face counts those, keeps the count in the session (`pi.appendEntry`, restored from the active branch), and pins `belt denies N` with `ctx.ui.setStatus`. Silent without a UI.

## Dev loop

```bash
claude --plugin-dir mods/session-band        # from the repo root; a save hot-reloads
claude --debug --plugin-dir mods/session-band  # every pin lands in the debug log as `$.ui.status (session-band): <text>`
claude plugin validate mods/session-band     # what the engine would refuse
node --test mods/session-band/test/*.test.mjs  # lib and the pi face
claude plugin test mods/session-band         # the count against the engine's test kit
npx -p typescript tsc -p mods/session-band   # after one load wrote .claude-plugin/types/
```

The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run. Each counted deny writes `session-band: belt deny <n> counted on <tool>` to the debug log; a belt deny without that line was not counted.

## What it never does

- never blocks: every hook carries a `.catch` that logs to the debug log and passes the event on, and the `tool.call` observer returns the result untouched
- never edits the transcript or the model's answer
- never probes a service: the one signal comes from an event the session already raises
- never required by any skill, hook, or service; belt stays the guard
