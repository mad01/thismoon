# Upgrading

Actions a change needs on a consuming machine, newest first. `ralph up` pulls main and rebuilds, but it cannot run an import, add a hook entry to a private settings template, or order two merges. Each entry names the component, the PR, what to run, and how to verify it. The entry lands in the same PR as the change (root `CLAUDE.md`, "Shipping a change to machines"), and a doctor check backs it when the missing action is detectable.

Entries name this repo and the consuming repo's Claude settings template only; the machine-private wiring itself stays in the consuming repo (docs/adr/0006). After any `ralph up`, check the service's `/version`: ralph can report ok while an old binary keeps running.

## 2026-10-29 csl and belt: one csl rules file, the CLI for fleet search

Change: thismoon, MAD-514 and MAD-518. The agent-facing csl rules now live in one file, `services/csl/claude-md.md`. It sends fleet-wide searches and anything that would page to the `csl` CLI, and keeps the MCP tools for answers that fit one call. The csl recipe links the file to `~/.claude/CLAUDE.csl.md`. `examples/dotfiles/CLAUDE.csl.md` is gone, and the eval harness's default `-steer-file` is the new file. belt's `prefer-csl` hint now leads with the `csl search` command. It names the repo the command searched, after a `cd`, a `git -C` or an absolute path, instead of the session's. It stays quiet on a grep of one or two named files.

Run: `ralph up`. It links `~/.claude/CLAUDE.csl.md` and rebuilds belt. If a regular file already sits at that path, ralph moves it to `~/.claude/CLAUDE.csl.md.bak.<timestamp>` and links in its place. The global CLAUDE.md line that imports the file, `@~/.claude/CLAUDE.csl.md`, lands separately in the consuming repo (MAD-519). Until it does, the link exists but no session reads it.

Verify: `ls -l ~/.claude/CLAUDE.csl.md` shows a symlink ending in `services/csl/claude-md.md` inside the thismoon sources cache. In a session whose cwd is one indexed repo, `cd <another indexed repo> && grep -rn foo .` gets a `belt[prefer-csl]` hint that starts with `csl search` and names the other repo.

## 2026-10-28 catalog and keeper-of-facts: removed

Change: thismoon #332. The catalog and keeper-of-facts (kof) services leave the repo with their recipes. deps keeps reading `~/.config/catalog/registry.yaml`, which the consuming repo still installs. moondb's scorecard drops its `kof` source and the `hints.kof` block and now reports `version` 2. belt loses its kof-consult, kof-assertions and kof-deposit hints and its kof doctor probe.

Run, in order:

1. In the consuming repo, drop the `kof` and `catalog` routes from the d-man routes file. In its belt config, delete every `hints.kof-*` block (kof-consult, kof-assertions, kof-deposit). The new belt still runs its guards and hooks with an unknown hint id in the config, but `belt doctor` reports it and exits 1.
2. `ralph up --enable-cleanup`. The first apply after a recipe removal needs the flag. It runs the recipes' `pre_uninstall` hooks, which stop both agents. It also removes the binaries and applies the route and config edits.
3. If `t-man list` still shows them, run `t-man remove catalog-web` and `t-man remove keeper-of-facts`.
4. Delete the srt links `~/.config/srt/catalog-web.json` and `~/.config/srt/keeper-of-facts.json`, and any copies of them under `~/.config/srt/merged/`.
5. Run `claude mcp remove --scope user kof`. agent-sync's `mcp.prune` is off by default, and with it off, dropping kof from `servers.json` leaves the server registered. On Codex, delete the `[mcp_servers.kof]` table from `~/.codex/config.toml`.

The assertion store stays at `~/.local/share/kof`, and the t-man logs stay under `~/Library/Logs/keeper-of-facts` and `~/Library/Logs/catalog-web`. Nothing reads them now. Whether to delete them is your call.

Verify: `t-man list` shows neither `catalog-web` nor `keeper-of-facts`. d-man's managed block no longer names the routes, so `grep -E 'kof\.this|catalog\.this' /etc/hosts` prints nothing. `claude mcp list` has no `kof` entry. `belt doctor` reports no kof probe and no unknown-hint warning. `moondb scorecard <session-id> --json` reports `"version": 2`.

## 2026-10-28 moondb: sessions keep their cwd repo, entrypoint and tags

Change: thismoon #320. A session now records the repository its cwd sits in, the harness entrypoint (`cli`, or `sdk-cli` for `claude -p`) and the tags a launcher sets in `MOONDB_SESSION_TAGS`. A sessions row lists the repos the session worked in without editing them (`cwd_repos`): its own cwd's and each subagent's cwd repo. The repo filter and chips count the session under its edited repos and its `cwd_repos`. The REPO column shows the first of them as a badge, hollow for a worked-in repo, and folds the rest into "+N"; the session page shows a badge for each. The list also marks headless and tagged sessions. The eval runner tags its sessions `eval`.

Run: `ralph up`. The subagents' repos in `cwd_repos` need no import, since recorded agents already carry their location. A session's own cwd location and its entrypoint still need `moondb import --since 30d --force` on sessions recorded before this build; run it while no other session is working. A held session with no edit and no subagent is now imported too, for this. A session whose transcript file is gone keeps neither. A forced import fills a location only when the session's cwd still exists or can be placed, so eval sessions that ran in a throwaway `worktree:` workspace under `~/.cache/harness-eval` get none. Tags cannot be backfilled: they exist only in the hook's environment.

Verify: on `http://moondb.this/`, an older eval session such as "Where is the Go function `CanonicalRepo` defined" shows a hollow `mad01/thismoon` badge in the REPO column instead of "no repo". A `headless` badge sits beside its title, and its session page lists `sdk-cli` as the entrypoint. A session started from a scratch directory whose subagents only read in a repo shows that repo as a hollow badge and appears under `?repo=` for it.

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
