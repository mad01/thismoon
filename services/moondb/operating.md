# operating moondb

moondb records every file edit a Claude Code session makes between commits.
Each edit keeps its before and after content and the turn and prompt that
produced it. It also keeps the session, the agent, and the commit it later
landed in. `moondb why <path>:<line>` answers which turn wrote a line.

## how it runs

A single `moondb serve` process is the only writer. It owns the store, drains
the spool, derives each edit, links edits to commits, and serves the JSON API
plus a read-only web page on localhost (default {{.BaseURL}}). Machines
usually also route http://moondb.this to serve via the local domain front
door. If the localhost port answers but the .this host does not, the router
is the problem, not this service.

Capture runs in Claude Code hooks, not in serve. Each hook pipes its payload
into `moondb record`, which writes an envelope to the spool and exits 0,
printing nothing on stdout. It takes about 200 ms, or up to 2 s for the
async Bash and SubagentStop hooks. SubagentStop records the subagent that
stopped, with its own transcript cut to short heads. At a turn's start and stop
it also snapshots the worktrees in play, within 2 s. So capture keeps working
while serve is down: envelopes wait in the spool and serve ingests them when
it comes back. `moondb import` brings in past sessions from their
transcripts through the same spool, so it too needs serve running before
the store shows anything.

Everything else is a thin HTTP client over the API: the query commands
(`why`, `history`, `sessions`, `trace`) and the MCP server, a stdio shim spawned as
`moondb mcp`. The shim being up says nothing about the service: every tool
call is a live HTTP request to serve, and it fails when serve is down.

## where state lives

The workdir, default {{.StorePath}} (override with MOONDB_WORKDIR), holds:

- `log/NNNNNN.jsonl`: the append-only record log, one record per line
- `objects/<ab>/<sha>.gz`: file contents, prompts and patches, gzipped and
  named by the SHA-256 of their bytes
- `spool/`: envelopes the hooks wrote that serve has not ingested yet
- `sessions/`: small per-session state files the hooks read back

Files are 0600. Nothing is rewritten in place, so the log is safe to read
directly. moondb never writes into a repository's `.git`. Config lives at
`~/.config/moondb/config.toml`; `capture.enabled = false` there turns
capture off on this machine.

## failure modes

Start with `moondb doctor`: one command runs the config, hook wiring,
reachability, store, and version-skew checks below and prints one line per
check, FAIL lines naming the cause. The paragraphs here cover what each
failure means.

`hooks-wired` fails: the Claude Code settings file (`~/.claude/settings.json`,
or the config's `claude_settings`) lacks a `moondb record` hook entry for
the events it names. Capture misses those events until the entry is added.
With `capture.enabled = false` the check passes with a note.

Connection refused, or "moondb serve not reachable": serve is not running.
t-man typically supervises it. Run `t-man list` to see whether the moondb
agent exists, then `t-man restart moondb`. For a quick test without t-man,
`moondb serve` in a spare terminal also works. Capture is not lost meanwhile.

`spool/` keeps growing: serve is down or cannot ingest. Check `t-man logs
moondb --stderr` for the envelope it refused.

`why` returns method `none`, or `history` is empty: usually not an error.
Either the hooks are not wired on this machine, `capture.enabled` is false,
the file matched a `capture.exclude` glob, or the edit predates capture. Ask
`moondb sessions` whether anything was recorded recently. An edit from before
capture can be backfilled: `moondb import --dry-run` counts what the
transcripts still hold, `moondb import` spools it. A `content` answer
is a text match, not proof: check its candidates before trusting it.

`moondb trace`, or the `moondb_trace` tool, shows cost `unknown` or names a
model as unpriced: the built-in price table has no row for that model id.
Add a `[prices."<model id>"]` table to the config and restart serve. A turn
with no spans has no transcript lines: it is still open, or its lines were
never recorded. User time is waiting for input, between turns and also
inside a turn: from the model stopping to a new input such as a teammate
message. Harness time far above a few minutes usually means a turn longer
than the 4 MiB transcript tail, whose cut head counts as harness; its
trace's lines then say `cut_head`.

A cost with no subagent cost recorded, or a session that ran subagents
with none listed: the SubagentStop hook is not wired, or the subagents ran
before it was. `moondb import --force` brings their transcripts in, also
for a session where no file was edited.
Subagents "not priced yet" have no priced model, or their totals wait for
the totals backfill, which runs while the spool is empty. An agent with `lines_cut` had a transcript over 8 MiB
after the cut to heads; the calls in its cut head add no cost. One marked
`partial` ran out of the hook's 2 s budget before the file's end.

An edit with `before_source: unknown`: serve could not reconstruct the file
as it was before the edit. The patch hunks are still recorded.

An unattributed delta: the file on disk differed from what the tool call
should have produced (cause `observed`), or a turn-edge snapshot found content
no recorded edit explains (cause `snapshot`). A formatter hook, a human edit,
or a Bash command changed it. This is expected, not corruption.

A Bash command's changes are missing from a turn: the Bash hook line is not
wired, or the user setting `bashEditDiffEnabled` is off, so Claude Code sends
no diff. The diff only covers the repository of the session's working
directory, so a session started in the main checkout never captures Bash
edits made in a linked worktree. A file `git status` reports clean after the
command is never recorded: a pull, checkout or reset put it back to its
committed state. The same goes for a file the 2 s Bash budget did not
reach, and for one whose status git could not give, a gone file outside
any worktree included. The turn counts all three under `skipped` (`clean`,
`timeout`, `failed`; the page's "Skipped" row), and `record` logs the counts
on stderr. A file a command edits and commits in one go is clean too; `git
blame` names that commit. The next snapshot still catches a skipped file
that stays dirty as an unattributed delta. A snapshot
marked truncated ran out of its 2 s budget or its 1000-file cap; its skipped
counts say which.

## version skew

`moondb version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart moondb`) and compare again. `moondb
doctor` runs this comparison as its version-skew check.

## first moves

1. `moondb doctor`: hooks wired, serve reachable, store readable, and version skew in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart moondb`
3. On version skew: `t-man restart moondb`, then `moondb doctor` again
4. `moondb sessions`, to confirm capture is recording on this machine
5. List the workdir's `spool/`: files piling up there mean serve is not draining
