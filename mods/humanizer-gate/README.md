# humanizer-gate

A Claude Code mod that runs `humanizer detect` on prose as it leaves the session, so text other people will read meets the humanizer before it ships. Detection only (vale, sub-second); the LLM judge never runs.

## What it checks

| Moment | What is scanned | What happens |
|--------|-----------------|--------------|
| `Write` or `Edit` on a `.md` or `.mdx` file | the file on disk, after the write | a status line under the prompt: `humanizer: 2 warnings in README.md`; cleared when the scan is clean |
| `mcp__gh_com__create_pull_request`, `mcp__gh_com__update_pull_request` | the `body` field | status line; with an error-level finding and `holdOnError` on, a Proceed / Cancel dialog first |
| `Bash` running `git commit` | the message from `-m "..."` (every `-m`, joined) or a heredoc after `-F -` or inside `-m "$(cat <<'EOF' ...)"` | status line; with an error-level finding and `holdOnCommitError` on, the same dialog |

Markdown under `node_modules`, `.git`, or any dot directory except `.github` is skipped. A commit whose message the mod can't read (`-F file`, an editor, an unquoted word) passes straight through.

## What it never blocks

- A markdown write: the write lands first, then the scan reports.
- Anything when `~/code/bin/humanizer` is missing: one debug line at session start, then every hook is a pass-through.
- Anything on a slow or failed scan: 5 s budget per run, then the event goes on as if the mod were absent.
- A PR or commit unless the person picks Cancel in the dialog. Cancel answers the tool call with `humanizer-gate: cancelled by user`; Proceed, a dismissed dialog, or a `-p` run with nobody to ask all let it through. belt stays the guard.

## Config

Set under `pluginConfigs["humanizer-gate"].options` in settings, or from `/config`:

| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `holdOnError` | boolean | `true` | ask before a PR call whose body has an error-level finding |
| `holdOnCommitError` | boolean | `false` | ask before a `git commit` whose message has one |
| `minSeverity` | `suggestion`, `warning`, `error` | `warning` | the lowest severity `humanizer detect` reports |

The last scan's counts sit in `$.state` under `humanizer-gate.lastReport` for another mod to read.

## Dev loop

```bash
claude --plugin-dir mods/humanizer-gate        # from the repo root; saves hot-reload
claude --debug --plugin-dir mods/humanizer-gate  # the debug log names every scan and skip
claude plugin validate mods/humanizer-gate
node --test mods/humanizer-gate/test/*.test.mjs
```

Needs `~/code/bin/humanizer` (thismoon `services/humanizer`) and `vale` on PATH, which detect shells out to. The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run.
