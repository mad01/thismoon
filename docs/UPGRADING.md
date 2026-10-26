# Upgrading

Actions a change needs on a consuming machine, newest first. `ralph up` pulls main and rebuilds, but it cannot run an import, add a hook entry to a private settings template, or order two merges. Each entry names the component, the PR, what to run, and how to verify it. The entry lands in the same PR as the change (root `CLAUDE.md`, "Shipping a change to machines"), and a doctor check backs it when the missing action is detectable.

Entries name this repo and the consuming repo's Claude settings template only; the machine-private wiring itself stays in the consuming repo (docs/adr/0006). After any `ralph up`, check the service's `/version`: ralph can report ok while an old binary keeps running.

## 2026-10-26 moondb: subagent cost needs a SubagentStop hook entry

Change: thismoon #298. moondb records each subagent when it stops, from its own transcript. The turn and the session then add the subagent's cost to their own.

Run: in the consuming repo's Claude settings template, add a `SubagentStop` hook group with no matcher, running `~/code/bin/moondb record` asynchronously. Without a matcher, Claude Code's internal agents count too. Merge this service first and `ralph up`, then the hook entry. Then run `moondb import --since 30d --force` so the last month's sessions get their subagent cost. Without `--force` the import skips every session the store already holds. Run it while no other session is working: it closes an open turn until that turn's Stop arrives.

Verify: `moondb doctor` reports `hooks-wired` ok. A new session that spawns one subagent shows it in the Agents section on `http://moondb.this/`, with a cost.

## 2026-10-25 moondb: Bash capture needs a setting and a hook entry

Change: thismoon #281 and #288. The Bash hook records the files a Bash command changed, from the `bashEditDiff` that Claude Code attaches to the tool response.

Run: in the consuming repo's Claude settings template, set `bashEditDiffEnabled` to `true` and add a PostToolUse group with matcher `Bash` running `~/code/bin/moondb record` asynchronously; then `ralph up` in a login shell. Claude Code 2.1.269 or later.

Verify: `jq .bashEditDiffEnabled ~/.claude/settings.json` prints `true`. In a new session, change a file with `sed` in Bash; the turn lists the file on `http://moondb.this/`.

## 2026-10-25 moondb: import the transcript history once per machine

Change: thismoon #279. `moondb import` reads the Claude Code transcripts on the machine and records their sessions, turns and edits. Live capture starts once the hooks run, so a machine's earlier history stays empty until the import runs.

Run: `moondb doctor` green, then `moondb import --since 30d --dry-run`, read the counts, then `moondb import --since 30d`. Wait for `~/.local/state/moondb/spool` to drain. Re-runnable: a replayed record changes nothing.

Verify: the session count on `http://moondb.this/` matches the dry run. Older history: `--since 60d` or `90d`.

## 2026-10-25 moondb: first install, service before hooks

Change: thismoon #272 and #273. New service on port 7433 with a web UI, CLI and MCP server, fed by Claude Code hooks.

Run: the consuming repo registers the companion recipe, the MCP server, the `moondb.this` route and the hook entries (UserPromptSubmit and Stop asynchronous, PostToolUse `Edit|Write|MultiEdit|NotebookEdit` synchronous). Merge the service first and `ralph up`, then the hook entries: a hook that runs before the binary exists logs a missing command in every session.

Verify: `moondb doctor` all ok; `curl http://127.0.0.1:7433/version` reports the merged commit.
