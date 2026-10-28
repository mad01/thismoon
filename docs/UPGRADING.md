# Upgrading

Actions a change needs on a consuming machine, newest first. `ralph up` pulls main and rebuilds, but it cannot run an import, add a hook entry to a private settings template, or order two merges. Each entry names the component, the PR, what to run, and how to verify it. The entry lands in the same PR as the change (root `CLAUDE.md`, "Shipping a change to machines"), and a doctor check backs it when the missing action is detectable.

Entries name this repo and the consuming repo's Claude settings template only; the machine-private wiring itself stays in the consuming repo (docs/adr/0006). After any `ralph up`, check the service's `/version`: ralph can report ok while an old binary keeps running.

## 2026-10-28 moondb: sessions keep their cwd repo, entrypoint and tags

Change: thismoon PR TBD. A session now records the repository its cwd sits in, the harness entrypoint (`cli`, or `sdk-cli` for `claude -p`) and the tags a launcher sets in `MOONDB_SESSION_TAGS`. The sessions list shows a session that edited no repo under its cwd's repo, and marks headless and tagged sessions. The eval runner tags its sessions `eval`.

Run: `ralph up`, then `moondb import --since 30d --force` while no other session is working, so sessions recorded before this build get their location and entrypoint. A held session with no edit and no subagent is now imported too, for this. A session whose transcript file is gone keeps neither. Tags cannot be backfilled: they exist only in the hook's environment.

Verify: on `http://moondb.this/`, an older eval session such as "Where is the Go function `CanonicalRepo` defined" shows a hollow `mad01/thismoon` badge in the REPO column instead of "no repo". A `headless` badge sits beside its title, and its session page lists `sdk-cli` as the entrypoint.

## 2026-10-26 belt: kof-consult runs on the prompt, not at session start

Change: thismoon #309 (MAD-492). belt's `kof-consult` hint now runs on UserPromptSubmit and shows the repo's assertions that the prompt is about, in place of the whole repo's list at session start. The kof hints' events now carry `assertions` and `pins` tags.

Run: nothing in the settings template. Its UserPromptSubmit entry already runs `belt hint prompt`, so `ralph up` is enough. Global instruction files that call `belt[kof-consult]` a session-start injection are now out of date. Reword them when convenient, for example to "each prompt surfaces the cwd repo's assertions that it is about".

Verify: `belt doctor` lists `kof-consult` on the `prompt` event. In a new session inside a repo with assertions, a prompt that names one of them gets a `belt[kof-consult]:` block, and the session start gets none.

## 2026-10-26 belt and moondb: in-flight files at session start need moondb first

Change: thismoon #311 (MAD-493). A new belt session-start hint, `moondb-inflight`, asks moondb's new `GET /api/inflight` route which files of the checkout other sessions edited and no commit has taken yet.

Run: nothing in the settings template, since `belt hint session-start` is already wired. Install moondb before belt: `ralph up`, then check that `curl http://127.0.0.1:7433/version` reports the merged commit. A belt on an older moondb gets a 404 from the route, and the hint stays silent until moondb catches up.

Verify: `belt doctor` reports moondb serve as `reachable`, not as older than this belt. `moondb inflight` in a checkout prints a table or "no files".

## 2026-10-26 belt: read-size hint needs a Read hook entry

Change: thismoon, belt `read-size` hint (#308, MAD-494). The hint fires when a Read with no offset or limit pulls in 300 lines or more from a csl-indexed repo, for a file type `csl_outline` has symbols for. It points at `csl_outline` or a ranged read, once per file per agent per session.

Run: merge this change and `ralph up` first, so the installed belt knows the `read` event. Then, in the consuming repo's Claude settings template, add a PostToolUse group with matcher `Read` running `~/code/bin/belt hint read`. Order matters: an older belt fails `belt hint read` at argument parsing, and Claude Code reports that hook error on every Read. The hint is on by default; `hints.read-size.enabled: false` in the belt config switches it off.

Verify: `belt doctor` lists `read-size` on the `read` event. In a new session, Read a file of 300 lines or more from an indexed repo with no range; the result carries a `belt[read-size]` note naming `csl_outline`.

## 2026-10-26 moondb: subagent cost needs a SubagentStop hook entry

Change: thismoon #298. moondb records each subagent when it stops, from its own transcript. The turn and the session then add the subagent's cost to their own.

Run: in the consuming repo's Claude settings template, add a `SubagentStop` hook group with no matcher, running `~/code/bin/moondb record` asynchronously. Without a matcher, Claude Code's internal agents count too. Merge this service first and `ralph up`, then the hook entry. Then run `moondb import --since 30d --force` so the last month's sessions get their subagent cost. Without `--force` the import skips every session the store already holds. A session where no file was edited is imported too when it ran a subagent, so review and research sessions get their agents; one with neither stays skipped. Run it while no other session is working: it closes an open turn until that turn's Stop arrives.

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
