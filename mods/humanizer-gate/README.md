# humanizer-gate

A Claude Code mod that runs `humanizer detect` on prose as it leaves the session, so text other people will read meets the humanizer before it ships. Detection only (vale, sub-second); the LLM judge never runs.

## What it checks

| Moment | What is scanned | What happens |
|--------|-----------------|--------------|
| `Write` or `Edit` on a `.md` or `.mdx` file | the file on disk, after the write | a status line under the prompt, `humanizer-gate: 2 warnings in README.md` (the engine adds the name); cleared when the scan is clean |
| `mcp__gh_com__create_pull_request`, `mcp__gh_com__update_pull_request` | the `body` field | status line; with an error-level finding and `holdOnError` on, a Proceed / Cancel dialog first; otherwise a finding at warning or above sends the findings back to the model once (below) |
| `Bash` running `git commit` | the message of the last `git commit` segment in the command: every quoted `-m` (`-am`, `--message` too), joined, or a heredoc after `-F -` or inside `-m "$(cat <<'EOF' ...)"` | status line; with an error-level finding and `holdOnCommitError` on, the same dialog; otherwise the same feedback as a PR body |

Markdown under `node_modules` or `.git` is skipped; dot directories such as `.claude/worktrees` and `~/.worktrees` are scanned. Only the commit's own segment of a shell command is read, so a script in a heredoc elsewhere in the command never gets scanned. A commit whose message the mod can't read (`-F file`, an editor, an unquoted word) passes straight through.

Tool-call hooks run before the permission check, so the hold dialog can appear before a permission dialog for the same call.

## Findings handed back to the model

With `feedbackWarnings` on (the default), a PR body or commit message with a finding at warning or above is denied on its first send, unless the dialog took it. The model reads the deny reason. It lists up to 20 findings, one per line: rule id, `line:column` in the scanned text, matched text, message. Then it tells the model to fix them, or to send the same text unchanged if they are false positives. The same text sent again passes. The mod keeps a 53-bit hash of each denied text in `$.state` under `humanizer-gate.denied`, the newest 32 for the current session. A changed text is scanned and judged again. An error-level finding whose hold is off (the default for commit messages) is handed back the same way.

## What it never blocks

- A markdown write: the write lands first, then the scan reports.
- Anything when `~/code/bin/humanizer` is missing: one debug line at session start, then every hook is a pass-through.
- Anything on a slow or failed scan: 5 s budget per run, then the event goes on as if the mod were absent.
- A PR or commit for good. The feedback deny comes once per text; after it only the person's Cancel in the dialog stops a call, answered with `humanizer-gate: cancelled by user`. Proceed, a dismissed dialog, or a `-p` run with nobody to ask all let it through. belt stays the guard.

## Config

Set under `pluginConfigs["humanizer-gate"].options` in settings, or from `/config`:

| Key | Type | Default | Meaning |
|-----|------|---------|---------|
| `holdOnError` | boolean | `true` | ask before a PR call whose body has an error-level finding |
| `holdOnCommitError` | boolean | `false` | ask before a `git commit` whose message has one |
| `feedbackWarnings` | boolean | `true` | deny a PR body or commit message once, with its findings as the reason; off reports in the status line only |
| `minSeverity` | `suggestion`, `warning`, `error` | `warning` | the lowest severity `humanizer detect` reports |

The last scan's counts sit in `$.state` under `humanizer-gate.lastReport` for another mod to read.

## pi face

`pi/index.ts` runs the same three checks in the pi coding agent on its split events: `tool_call` scans a `git commit` message or a github.com pull request body (direct `mcp__gh_com__<op>` or the `mcp` proxy with `server: gh_com`) and may hold it for the same Proceed / Cancel; `tool_result` scans a `.md` or `.mdx` file after `write` or `edit` landed. Status goes to `ctx.ui.setStatus`. pi has no per-plugin config, so the face runs on the defaults above. It never hands findings back to the model, since ADR-0023 lets a pi face deny only on the person's Cancel. The same pass-through rules apply: no binary, a failed scan, or no UI to ask all let the call through.

## Dev loop

```bash
claude --plugin-dir mods/humanizer-gate        # from the repo root; reloads on save
claude --debug --plugin-dir mods/humanizer-gate  # the debug log names every scan and skip
claude plugin validate mods/humanizer-gate
node --test mods/humanizer-gate/test/*.test.mjs  # lib and the pi face
claude plugin test mods/humanizer-gate           # the PR and commit gates against the engine's test kit
```

Needs `~/code/bin/humanizer` (thismoon `tools/humanizer`) and `vale` on PATH, which detect shells out to. The engine writes `.claude-plugin/types/` beside the mod on load; those files are the authority on event shapes for the build you run.
