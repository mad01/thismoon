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
stopped, with its own transcript cut to short heads, its spawn brief (the
prompt it was spawned with) and the messages its parent sent it later,
each with the time it arrived. At a turn's start and stop it also snapshots
the worktrees in play, within 2 s. So capture keeps working
while serve is down: envelopes wait in the spool and serve ingests them when
it comes back. `moondb import` brings in past sessions from their
transcripts through the same spool, so it too needs serve running before
the store shows anything.

Everything else is a thin HTTP client over the API: the query commands
(`why`, `history`, `inflight`, `sessions`, `trace`) and the MCP server, a stdio shim spawned as
`moondb mcp`. The shim being up says nothing about the service: every tool
call is a live HTTP request to serve, and it fails when serve is down.
`moondb sessions hide` and `show` ask serve first too, so with serve down
they fail before writing anything. With serve up they spool a visibility
envelope and wait up to 5 s for serve to apply it. A request serve has not
applied by then stays in the spool until serve drains it.

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
reachability, store, version-skew and agent-briefs checks below and prints
one line per check, FAIL lines naming the cause. The paragraphs here cover
what each failure means.

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

`history` misses edits a session made in a worktree that is gone: they
carry no repository identity, and the list across checkouts matches on it.
The hooks record the identity, the origin remote's host/owner/repo, for any
file in a checkout with an origin. An edit `moondb import` brought in can
lack it when its worktree was deleted before the import. That happens when
the worktree's name matched no repository the import knew, or it was a
temporary clone such as an eval run's. For any worktree deleted before the
import, an ordinary one included, the stored root can also sit below the
checkout's own root, with the path relative to it.

To list those edits, pass the stored root: `moondb history --worktree <root>
<root>/<path>` works from any directory. The `moondb_history` tool given only
`path` lists every edit of that path, in any repository or none, each with
its stored root. Importing again does not add the identity: a stored edit
stays as it is.

`moondb trace`, or the `moondb_trace` tool, shows cost `unknown` or names a
model as unpriced: the built-in price table has no row for that model id.
Add a `[prices."<model id>"]` table to the config and restart serve. A turn
with no spans has no transcript lines: it is still open, or its lines were
never recorded. User time is waiting for input, between turns and also
inside a turn: from the model stopping to a new input such as a teammate
message. The trace walks turns in the order they started, so a turn an
import added sits where it ran, under a higher number than the live turns
after it. Harness time far above a few minutes usually means a turn longer
than the 4 MiB transcript tail, whose cut head counts as harness; its
trace's lines then say `cut_head`.

A session shows two times in `moondb sessions`, on the web page and in
the `moondb_sessions` tool. Active time (`active_ms`) is the time
inside its turns, less the time a turn waited for input or sat idle. Wall
time (`duration_ms`) runs from the first prompt to the last turn's end,
the nights between turns included. Idle time is a stretch inside a turn
with no transcript line for over 30 minutes, past its first 30 minutes,
which stay model, tool or harness time. A laptop asleep, a session resumed
days later or a request that hung leaves one. `moondb trace` lists it as
`idle`, and the session page draws it as a hatched break in the turn's
lane. Turns stored before idle time existed get it from the totals
backfill once serve runs the new build.

A cost with no subagent cost recorded, or a session that ran subagents
with none listed: the SubagentStop hook is not wired, or the subagents ran
before it was. `moondb import --force` brings their transcripts in, also
for a session where no file was edited. Subagents "not priced yet" have no
priced model, or their totals wait for the totals backfill, which runs
while the spool is empty. An agent with `lines_cut` had a transcript over
8 MiB after the cut to heads; the calls in its cut head add no cost. One
marked `partial` ran out of the hook's 2 s budget before the file's end.

`why` or a turn shows a subagent's spawn brief as unknown, or `moondb
doctor` notes `agent-briefs` with a count of agents without a spawn brief
or later messages: those agents were recorded before moondb kept them. Run
`moondb import --since 30d --force` while no session is active. It fills
the brief and the later messages of every agent whose transcript is still
under `~/.claude/projects`. The doctor count covers the agents that ended
in those 30 days. An agent whose transcript file is gone stays unknown,
and so does an agent of type `internal`, which has no transcript, and one
moondb knows only from its edits.

The spawn brief is the first message a subagent got and nothing after it.
Its parent can send it more while it works, to a teammate or to a running
subagent, which `later_messages` counts. moondb keeps each of those
messages with the time it arrived. `why` on a line the subagent wrote
shows the last one before the edit as `last message`, which often holds
the instruction the edit followed. One that came in while the edit's own
round of tool calls ran counts as after it. A count that ends in `none
before this edit` means none had arrived yet. One that ends in `none
shown` means the agent was recorded before moondb kept them, which the
forced import above fills.

A repo's cost looks wrong: the page, `moondb sessions --repo` and the
`moondb_sessions` tool split a session's cost by agent, since model calls
carry no repo. A subagent's cost goes to the repo most of its edits
touched, else to the repo of its cwd, else to `no_repo` ("no repo (cwd)"
on the page). The main conversation's goes to the repo most of its own
edits touched, else to "orchestration". So a session that edited a repo
can give it nothing (`repo_cost` 0). A repo chip and the repo filter take
a session that edited the repo, worked in it, or sends it a part of its
cost. Working in it means the session's cwd or a subagent's cwd sits
there, so a subagent that only read there is enough. The chip's count and
cost then match the filtered list. An agent waiting for the totals
backfill adds no cost until it has totals, but the repo its cwd sits in
counts the session already.

`moondb scorecard` says hints `unavailable`, or its sources line says
events `unavailable`: the events service did not answer within 2 s. Check
it is running and that `EVENTS_BASE_URL` points at it. Events `partial`
means the service has no entry older than the session's first turn: it
keeps 500 entries per source, so an older session's early hints may be
gone. A subagent's reads show
as `unsized` and its searches as `unread` because its transcript keeps only
the head of each result.

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

1. `moondb doctor`: hooks wired, serve reachable, store readable, version skew and agents without a brief or later messages in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart moondb`
3. On version skew: `t-man restart moondb`, then `moondb doctor` again
4. `moondb sessions`, to confirm capture is recording on this machine
5. List the workdir's `spool/`: files piling up there mean serve is not draining
