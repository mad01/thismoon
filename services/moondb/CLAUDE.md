# moondb, edit history between commits

Go CLI, capture hook, HTTP server with an embedded webkit web UI, and an MCP
server, all over one store. moondb records every file edit a Claude Code
session makes between commits. Each edit is grouped by session, turn and
agent, keyed by repo, worktree and branch, and stored with its before and
after content. A later reader can then ask why a line looks the way it does
and get back the turn, the prompt and the commit the change landed in.
The MCP tools, driven by Claude, are the main read surface; the `moondb` CLI
mirrors them. The web page at `http://moondb.this/` is a read-only view of
the same store.

## Module layout

```
moondb/
  cmd/moondb/          - entrypoint (delegates to internal/cli)
  facts.go             - port, workdir, capture caps and budgets, agent transcript head and cap, default excludes, trace limits, idle gap, totals backfill pace, listing status rule, scorecard peers and thresholds, model prices, agentdoc.Facts
  embed.go             - embeds operating.md for `moondb docs`
  internal/
    cli/               - cobra: root, serve, record, import, mcp, why, history, inflight, sessions, trace, scorecard, doctor, docs, version
    record/            - the capture hook: payload decode, envelopes, observed read, HEAD blob, transcript tail, Bash diffs, turn-edge snapshots, subagent stops, per-session state
    agentfile/         - one subagent's transcript file read and cut to its agent transcript, its meta file, the type and name rule
    importer/          - the transcript import: transcript parse, turns, edits and agents as envelopes, file-history originals
    spool/             - envelope format, atomic writer, drain reader
    ingest/            - serve's spool drain: envelopes to records, turn and agent totals, the totals backfill, session titles
    derive/            - before and after content per Edit, snapshot records, unattributed deltas
    patch/             - forward and reverse patch apply, hunk context checks, line endings
    gitlink/           - git blob ids and commit links
    store/             - Store and Chunks interfaces and record types (store.go, types.go), session summaries and the cost attribution rule (summary.go, attribution.go), the in-flight rule (inflight.go)
      local/           - the local disk backend: JSONL log, gzip chunks, in-memory index
      storetest/       - conformance suite every backend must pass
    query/             - why, history and annotate answers over the store and git; traces with their cache; session scorecards
    trace/             - turn, agent and session traces from transcript lines: spans, gaps, segments, totals, prices (pure)
    scorecard/         - a session's scorecard: tool calls parsed from transcript lines, search and read stats, hint and ledger events, kof pins, flags
    server/            - HTTP API + webkit web page
    client/            - HTTP client shared by the CLI and mcpserver
    mcpserver/         - MCP tools
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Capture: five hook lines feed `moondb record`

The consuming repo wires five Claude Code hook lines to `moondb record`. Each
one pipes its JSON payload on stdin:

| Hook | Mode | What `record` spools |
|------|------|----------------------|
| UserPromptSubmit | async | turn-open: `prompt_id`, the prompt text as a chunk, `session_id`, cwd and its location, the transcript byte offset. Then a `pre` snapshot of each worktree in play. |
| PostToolUse on `Edit\|Write\|MultiEdit\|NotebookEdit` | sync, 3 s timeout | one edit envelope per file. It holds `tool_use_id`, the tool input and patch, the location (repo, worktree, branch, HEAD), the observed bytes with their blob id, and the path's HEAD blob id and content. |
| PostToolUse on `Bash` | async | one edit envelope per file in the payload's `bashEditDiff.files`: its hunks as the patch, the observed bytes and blob id, and the HEAD blob id and content, with no tool input. One more per other `changedFiles` path, with no hunks. A file `git status` reports clean after the command gets none, and neither does one whose status git cannot give or one the budget does not reach; a `skipped` envelope counts them. Nothing when the payload has no `bashEditDiff`. |
| Stop | async | turn-close: the final assistant message, the transcript tail from the turn's offset, and a head-moved report for each touched worktree whose HEAD changed. Then a `post` snapshot of each worktree in play. |
| SubagentStop | async | agent-close: the agent's type, name, description and model, the turns it ran under, its start and stop, its cwd and location, and its agent transcript as a chunk. |

The Edit hook is synchronous on purpose. Edits in one assistant message land
milliseconds apart, so an async hook would read the disk after the next edit
had already changed it. NotebookEdit is recorded by path only. A file outside
any git repo lands in a `norepo` bucket.

Every envelope also says how the session runs, from the hook's environment.
The entrypoint is Claude Code's `CLAUDE_CODE_ENTRYPOINT`: `cli` for an
interactive terminal session, `sdk-cli` for `claude -p`. The tags are the
comma-separated `MOONDB_SESSION_TAGS` a launcher set, such as `eval` from
the eval runner. A tag is lowercase letters, digits and dashes, starts
with a letter or digit and is at most 32 bytes. Invalid ones are dropped,
and a session keeps at most 8 (`store.ParseTags`). The entrypoint follows
the same rule and is dropped when invalid (`store.ParseEntrypoint`), here
and in the import. The turn-open looks up its cwd's location within
`RecordBudget`. An answered lookup is cached for the run, and the pre
snapshot reuses it. A lookup the budget cuts is not cached: the snapshot
repeats the git calls, and that turn-open carries no location, which a
later turn fills. serve keeps the first location and entrypoint a session
gets, and the union of its tags.

Claude Code reports `bashEditDiff` only when the user setting
`bashEditDiffEnabled` is on (v2.1.269 or later). `files` holds up to five
diffs and `changedFiles` up to 200 paths. A Bash create seen only in
`changedFiles` has no diff to say so and gets kind `unknown`.

The diff covers only the repository of the session's cwd, so a session
started in the main checkout never captures Bash edits made in a linked
worktree. Within that repository it lists every file the command changed,
the ones a `git pull`, `ralph up`, `git checkout`, `git reset` or `git
stash` rewrote included. Those are back at their committed state and the
command authored no Edit of them. Recording them would make `why` name the
turn that ran the pull instead of the commit.

So before any per-file work `record` places every listed file once, from
its nearest existing directory, with symlinks resolved on that directory.
Only the first file of each worktree costs a git lookup. `record` places
every later file under a located worktree by its path alone. A directory
on the way that holds a `.git` of its own (a nested repository, submodule
or linked worktree) sends the file back to git. A directory outside any
repository is looked up once per run too.

Then `record` runs one `git --no-optional-locks status --porcelain=v2 -z`
per worktree, limited to those paths, untracked and ignored files
included. A file it
reports (modified, added, deleted, renamed, untracked or ignored) is
recorded. One it does not report is clean and gets no edit. A file outside
any repository is recorded unchecked, as before, unless it is gone: then
only an absolute path would be left to record, so it is skipped. The
trade-off: a file a command edits and commits in one go (`sed ... && git
commit`) is clean afterwards and gets no edit either. `git blame` names
that commit. The head-moved report at Stop ties it to the turn only when
the session had already recorded an edit in that worktree, since Stop
reports touched worktrees only.

The files the Bash hook skips it counts, in one `skipped` envelope per
call. `clean` counts the files status reported clean. `timeout` counts the
ones the budget, or a git call's deadline, ran out on before their
placement, status or disk read. `failed` counts the ones whose status git
could not give: git failed, its status passed `MaxStatusBytes`, or the file
is gone and no worktree holds it. serve adds the count to the turn's
`Skipped` list, and `record` logs it on stderr. A file the budget does not
reach is skipped, never spooled without its worktree or its disk read.

The Bash hook is async, so its disk read can land after a later Edit of the
same file. Every envelope is stamped when its hook starts, before any read,
so that Edit sorts after the Bash edit. The Bash record may then carry the
Edit's bytes in its observed read and after content. When the two after
blobs coincide, the newest-edit tie-break keeps the commit link and `why`
with the Edit.

`record` follows belt's record contract (docs/adr/0024): nothing on stdout,
exit 0 always, panics recovered. It works within `RecordBudget` (200 ms, in
`facts.go`) and spools whatever it has gathered when the budget runs out.
The async Bash hook, which nobody waits on, gets `BashBudget` (2 s) instead:
one command can change hundreds of files.
Files matching a `capture.exclude` glob are recorded by path only. Files over
`MaxChunkBytes` (1 MiB) are recorded by path and patch. HEAD content is read
on every edit of a tracked file, and the spool writer leaves it out while the
store holds it.

SubagentStop reads the agent's own transcript file
(`<session>/subagents/agent-<id>.jsonl`, named by the payload's
`agent_transcript_path`) and its `agent-<id>.meta.json`
(`internal/agentfile`). A payload without the field gets the path beside
its `transcript_path`, and a leading `~` is expanded. A path outside the
session's `subagents/` directory, symlinks resolved, records nothing and
is logged on stderr. So does a `session_id` that is not one path element,
and a `transcript_path` that is not absolute. It keeps the timeline lines
(user, assistant, API request, system) with the fields a trace reads and
cuts each text, thinking
block, tool input and tool result to `AgentHeadBytes` (240). The usage, ids
and times survive, so the cut transcript prices the same as the file: an
11 MB transcript keeps about 0.5 MB. Past `MaxAgentTranscriptBytes`
(8 MiB) whole lines go from the head and are counted on the record. Their
calls add no tokens or cost. The work fits in `AgentBudget` (2 s); a file
the budget does not reach the end of, or whose read fails partway, is
recorded up to there and marked partial.

The harness also fires SubagentStop for a main segment: a stretch of the
main conversation that it runs under an agent id of its own. A teammate's
message or a queued prompt that wakes the session while it waits on
background agents starts one. Its payload names no `agent_type`, and the
transcript file it names never exists, since the lines go to the main
transcript. A transcript that cannot be read records the agent without
lines, as the meta file and the payload describe it. The exception is an
`internal` agent with no line read, which is a main segment's record
(`store.Agent.IsMainSegment`) and which ingest drops. `record` spools
nothing for one and logs it on stderr. An untyped agent with no meta
file whose transcript holds no timeline line yet looks the same and is
skipped too. A later `moondb import` records it once its file has lines.

The agent's type is the meta file's `spawnedAgentType`, else its
`agentType` when that differs from the name, else the hook's `agent_type`
when that differs from the name. A named agent with no other type is a
`subagent`, and one with neither is `internal`. A name equal to the type
is dropped. The model is the first assistant model that is not
`<synthetic>`. The turns it ran under are the `promptId`s on its lines.

A teammate stops after every message it works through, so one agent sends
many agent-close envelopes. Each id is `<agent>:close:<lines read>`, so a
replay is a no-op and each later stop is a new envelope. serve keeps the
newest record whole: a live envelope replaces the stored agent when it read
as many lines or more, an import only when it read more.

`record` keeps a little state per session under `<workdir>/sessions/`, read
back only by later hooks of the same session:

- `<sid>.turn`: the open turn's prompt id and the transcript offset it opened at
- `<sid>.paths`: the files the session edited, which its snapshots cover
- `<sid>.touched`: one worktree per line, where the session edited
- `<sid>.heads`: the HEAD last seen in each touched worktree
- `<sid>.snap`: per worktree, the last snapshot's state hash, the size,
  mtime and chunk of each file it read, and the chunk of each HEAD blob it
  shipped

### Snapshots: the worktree at each turn edge

After its own envelope, UserPromptSubmit and Stop each take a snapshot of
every worktree in play: the cwd's worktree and each one the session touched.
The scope is the worktree's dirty files plus the paths the session touched
there. Dirty comes from `git --no-optional-locks status --porcelain=v2 -z
-uall`. Status does not show ignored files or files marked skip-worktree or
assume-unchanged, so a snapshot holds those only when the session touched
them.

Each file's content is its own chunk, so an unchanged file is stored once.
The file map (path, chunk, blob id, size, status) is one more chunk. A
deleted file is listed as deleted. A path the snapshot cannot hold the
content of is listed without it and with a reason: excluded, symlink,
not-regular, unreadable or too-large. A path a whole snapshot lacks was
therefore clean. A file whose size and mtime match the last snapshot reuses
that chunk without a read. For a path HEAD tracks, one `git ls-tree` call
adds the HEAD blob id, and the HEAD content ships unless the store holds it.

The work fits in `SnapshotBudget` (2 s). A snapshot cut short is written with
what it read, marked truncated, and counts the rest as timeout.
`MaxSnapshotFiles` (1000) caps the paths, touched ones first. A snapshot
carries when it started, and its envelope is stamped once its reads are
done. Its id holds a digest of its state and file map, so a second Stop
for one prompt records its own. A state hash over HEAD, the status, and
each path's size and mtime skips the snapshot when nothing changed since
the session's last one of the worktree. When nothing is in scope and this
session has no snapshot of the worktree in its `<sid>.snap`, none is
taken. If it has one, a clean worktree gets an empty snapshot: its files
match HEAD again.

### Import: past sessions from their transcripts

`moondb import` reads the Claude Code session transcripts under
`~/.claude/projects` written within `--since` (default 30d) and writes the
envelopes the hooks would have written, through the same spool writer. serve
ingests them with no import code of its own. A session the store already
holds was captured live and is skipped unless `--force`. Run `--force` only
while no session is active, because it closes a session's in-flight turn
until the live Stop arrives. A session with neither a file edit nor a
subagent transcript is skipped, unless `--force` is set and the store holds
it. Such a held session is imported for its turns, which fills what its
session and turns have empty, such as the session's location and
entrypoint. It counts under `sessions_turns_only`. One with a
subagent transcript and no edit is imported for its turns and agents, and
counted under `sessions_agents_only`. `--dry-run` reads and counts
everything and writes nothing; with serve down it runs without the store
lookups and says so.
`--projects` and `--file-history` point at other transcript and
file-history directories; the defaults live in `facts.go`.

Ids match the hooks'. A turn is the transcript's `promptId`, the same id the
hook calls `prompt_id`; a transcript without one keys the turn by the prompt
entry's uuid. An edit is `<tool_use_id>:<index>`. Every import envelope is
marked `backfill`, so importing a session again, or one partly captured live,
only fills: a stored edit stays, and a session or turn keeps its values and
gets its empty fields filled, an open turn closed. An earlier imported
prompt still moves a start time back. A subagent's edits come from
`<session>/subagents/agent-<id>.jsonl`, carry its id and type, and belong
to the main turn its entries name. Each subagent file also gives one
agent-close envelope, written after the session's turns and cut as the
SubagentStop hook cuts it. Its stop is the time of its last line. A file
that cannot be read, or holds no timeline line, gives none and counts as
skipped.

What a transcript gives back:

- recovered: the prompt, the final message, the turn's transcript lines, the
  tool input, the structured patch, `originalFile` when Claude Code kept it,
  a Write's content, the subagent behind an edit, each subagent's agent
  transcript, and the session's entrypoint (the first valid one a line names)
  and its cwd's location, placed as a file in it would be
- not recoverable: the session's tags, which only the hook's environment
  holds
- from the file history: the transcript has no `originalFile` for a file over
  about 10 KB. The import takes the copy under
  `~/.claude/file-history/<session>/` taken last before the edit, when no
  other edit of the file came between that copy and the edit
- not recovered: the disk read after the edit, so there is no observed
  content and no unattributed delta. HEAD at the time is not recorded
  either, so imported edits carry none and the import writes no head-moved
  envelope. Their commit links come from a later live turn that moves HEAD.
  Bash changes are left out. A transcript keeps a Bash diff only when
  `bashEditDiffEnabled` was on, and the import does not read it yet.
- placed by what is left: a file whose directory still exists is placed in
  git as it is now, as the hook does. Most imported files sat in worktrees
  deleted since. For those the import tries, in order:
  1. the work tree of the nearest directory still there, unless the file sat
     in a worktree nested inside it
  2. the worktree its path names by layout: `.worktrees/<repo>/<slug>`,
     `.worktrees/<repo>-<slug>` or `<checkout>/.claude/worktrees/<slug>`.
     The repo is the one a known location gives that name: the store's, or
     the session's own checkout.
  3. the session cwd, when it is gone too and holds the file
  4. no worktree and an absolute path, like a file outside any repo

  The branch is the transcript's when the session's cwd sits in that
  worktree. The dry run counts each outcome.

### Single-writer architecture (load-bearing)

`moondb serve` is the only writer of the store. `record` writes only to the
spool and its session state. It reads `objects/` only to skip shipping a
chunk the store already has.

- `moondb serve` drains the spool (a file watch plus a 1 s poll), derives
  each Edit, appends the records, runs the totals backfill while the spool
  is empty, and serves the web page, the JSON API, `/version` and `/webkit/`.
- `moondb mcp` holds no state: a thin HTTP client to the serve API on
  `localhost:<port>`. If serve is down, tools return an unreachable error
  that names `moondb doctor`.
- `moondb why`, `history` and `sessions` are the same thin HTTP client.

A spool envelope lives until serve has ingested it, so nothing is lost while
serve is down or restarting. Ingest is idempotent on the envelope id, so a
crash between append and delete replays harmlessly.

### Derivation: after content is never trusted from disk

serve derives after content from the tool input: Write's `content`, Edit's
`old_string` swapped for `new_string`, MultiEdit's edits in order. The disk
read is evidence; where it differs, serve records an unattributed delta.
Before content is the first source the call fits: absent for a create,
`originalFile`, the latest earlier after content of the path, the path in
the worktree's latest earlier snapshot, HEAD, the disk read with the call
undone, else `before_source: unknown`. The chain yields to a snapshot begun
after its edit that holds the path's content, or is whole and lacks the
path, since the snapshot saw the file later. An entry without content
leaves the chain in place. A source fits when
each `old_string` occurs once (at least once with `replace_all`) and the
hunks apply with tabs read as two spaces. A CRLF file matches on its LF view
and keeps CRLF. Hunks alone fit a user-modified edit or a missing `old_string`.

A Bash edit has no tool input. Its after content is the disk read, and its
before is the first of `originalFile`, the chain, the snapshot and HEAD that
exists, unvetted. A file gone from disk makes it a delete.

A snapshot reveals changes no tool call explains. serve compares each file
in it with the latest content known of the path before: the worktree's
previous snapshot or the latest edit's disk read, whichever is newer. A path
a whole previous snapshot lacked held HEAD's content, which counts when both
snapshots sat on one HEAD. A difference becomes an unattributed delta with
cause `snapshot`: a person in an editor, a formatter, or a Bash command
without a diff wrote the file between the two. Nothing earlier known means
no delta, and so do an edit in between that left no after content and an
edit stamped while the snapshot read. Two overlapping snapshots of one
worktree can record the same delta twice.

### Commit links

Every after content carries its git blob id (the `git hash-object` form). At
Stop, `record` compares each touched worktree's HEAD with the one it last saw.
When HEAD moved, it lists the (path, blob id) pairs of the new commits with
`git diff-tree`, and serve links each recorded after-blob it finds to that
commit. The link is by content, so it survives a squash merge and a deleted
worktree. moondb never writes a trailer, a note or a ref into the user's repo.

### In-flight files

`GET /api/inflight` answers which files of one checkout hold edits no
commit has taken yet, and which sessions made them. An edit is in flight
while neither it nor a later edit of the same path has a commit link. A
commit takes the file's whole content, and gitlink links only the newest
edit whose blob matches. So a linked edit covers every edit of the path
before it. Only a link from a commit made in the queried worktree counts:
a commit in another checkout can link an identical blob edited here.
`store.InFlight` is the pure rule.

The handler reads the checkout's edits in the window with `Edits`. It
reads the commit links with one `Turn` read per turn, since a link is filed
under its edit's turn, and each session's title with `Session`. A path is
listed when an in-flight edit came from a session other than
`exclude_session`. Its `linked_commit` is the commit that took its newest
committed edit. Only a commit a session saw land gets a link: Stop reports
a moved HEAD. A file a person committed or reverted after the session
ended stays listed until the window passes. So does a file whose last edit
left no after-content to match a commit by: a delete, an excluded path, a
NotebookEdit, a file over `MaxChunkBytes`, or a link under a turn with no
record. The belt `moondb-inflight`
hint asks at session start and keeps only the paths `git status` reports
dirty for that reason.

### `why <path>:<line>`

1. A committed line: `git blame` names the commit. The path's blob id at that
   commit finds the Edit whose after content has that id. The Edit leads to
   its turn, prompt, session, agent and commit link. Method `blame`.
2. An uncommitted line, or a blob that joins nothing: query matches the
   normalized line text against the lines Edits added to that path, newest
   first. It skips trivial lines (braces, blank lines, bare `return`,
   imports). Method `content`, with ranked candidates.
3. Nothing explains the line: method `none`.

A per-worktree line walk through later hunks is planned for a later release.

### Annotate: every line of a file at once

`query.Service.Annotate` answers `why` for a whole file on disk. It runs one
`git blame --porcelain` for the file, then one `git rev-parse <commit>:<path>`
per blamed commit, cached, and joins each blob to the Edit whose after
content it is. The blame and the rev-parse calls share the 2 s blame budget.
Lines blame leaves unexplained go through the same content match as `why`,
against one scan of the path's edits, so every line names the edit `why`
would name. Annotate only reads a worktree that holds at least one recorded
edit, so the route cannot read arbitrary files on the machine.

### Trace: what took time and what it cost

`trace.Build` reads a turn's transcript chunk and returns its Trace: the turn
span, a span per model call, tool call and subagent, the turn's gaps and its
totals. It is a pure function and never fails on its input. A line that
does not parse is counted as skipped, and a turn without lines gets an empty
trace with a reason.

- A model span covers one assistant `message.id`. It starts at the event
  before the message (usually the `api-request` line) and ends at its last
  line. Usage counts once per message, from its last line in chunk order.
- A tool span runs from the assistant line with the `tool_use` to the first
  user line carrying its `tool_result`, with `is_error` and the Edit ids
  whose `tool_use_id` matches. A call without a result runs to the turn's
  end, marked unfinished. The span includes any wait on a permission
  prompt.
- A subagent span from `Build` runs from the subagent's first recorded edit
  to its last, clamped to the turn's span: the Bash hook is async, so an
  edit can land after Stop. A subagent with an Agent record gets its span
  from its agent transcript instead (`trace.WithAgents`, below).
- The gaps cover the turn from start to end, each stretch classed by the
  event that ends it. An assistant line ends model time and a tool result
  ends tool time. A new input after the model finished (its stop reason was
  `end_turn`, or a Stop line came) ends user time. Anything else is harness
  time: hooks, request setup, and the Stop hooks up to the Stop line (a
  `system` line with subtype `stop_hook_summary`). The tail after a Stop
  line is user time.
- A model, tool or harness stretch longer than `IdleGapAfter` (30 minutes)
  keeps its first 30 minutes in its own class, and the rest is idle time
  (`GapIdle`, `idle_ms`). The transcript gets no line while a laptop
  sleeps, a session waits days to be resumed, or a request hangs. Without
  the rule, the line that ends such a stretch would claim all of it. The
  rule applies to each stretch between two lines, before neighbours of one
  class merge. User time is never idle. The stretch before the first kept
  line of a cut head stays harness time at any length, since the harness
  wrote lines there and the size cap dropped them.
- The totals by kind add up to the wall time to the millisecond. Each kind
  is cut from the running sum of the kinds, so the sub-millisecond rest of
  one carries into the next.
- User time inside a turn is real. The harness keeps one prompt id across a
  Stop when a teammate message or a task notification restarts the loop, so
  that wait lands in the same turn. The session trace adds the stretches
  between turns.
- The session trace (`trace.BuildSession`) walks the turns in time order:
  by start, then by Seq. Seq can disagree with time, since an import
  numbers the turns it adds after the live ones even when they ran first.
  A user gap runs from the latest turn end so far to the next turn's
  start, so a turn that starts before an earlier one ended adds none. The
  page's trace lanes and turn list follow the same order and keep the Seq
  as the turn's number.
- The session trace's wall time is the turns' wall times plus the user
  gaps. Its user time adds the gaps to the turns' own, and its idle time is
  the turns'. The time split then adds up to the wall whenever each turn's
  does. Without overlapping turns the wall runs from the first start to the
  last end. A stretch two turns overlap counts in each, so this wall can
  exceed the session row's `duration_ms` by that overlap.

Cost comes from `moondb.DefaultPrices` (USD per million tokens), matched by
exact model id, then by family (a dated or provider suffix). Fast mode
multiplies a row that has a fast price. US-only inference (usage
`inference_geo` "us") multiplies a Claude 4.6 or later row by
`USOnlyMultiplier`, on top of fast mode. A `[prices."<model id>"]` table in
the config file sets the rates of the row its id matches and keeps that
row's long-prompt tier and multipliers; an id no row matches is added with
its rates alone. An unknown model, or fast mode on a model without a fast
price, gives tokens and a nil cost, listed as unpriced, never a zero. A
config rate that is negative, NaN or infinite fails the config closed.

`trace.BuildAgent` traces one Agent from its agent transcript with the same
parser, which reads sidechain lines only when asked (a turn's trace skips
them). Its run span goes from its spawn to its stop, stretched to its last
line, with model and tool spans, gaps and totals as a turn's. Its
segments split the run by the turn each stretch ran under. A new segment
starts at each line whose `promptId` differs from the one before. The
first segment starts with the run and the last ends with it, and each span
counts in the segment its start falls in. A teammate messaged in three
turns has three segments.

`trace.WithAgents` adds the agents to a turn's trace. Each agent with a
segment under the turn gets one subagent span from its first such segment
to its last, with the calls, tokens and cost there. It replaces the span the
agent's edits made, keeps their edit count, and is not clamped: a
background agent runs past Stop. The turn's `tokens`, `cost` and `usage`
then add the agents', with `tokens_main`, `tokens_agents`, `cost_main`,
`cost_agents` and `unpriced_main` beside them and the agents' calls in
`agent_model_calls` and `agent_tool_calls`. Time and the main call counts stay the main
conversation's. A session's trace adds every agent's whole run to its
totals the same way, lists the runs under `agents`, and ranks the agents'
tool and model calls with the main conversation's, each marked with
`agent_id` and `agent`.

serve computes traces on read. `query.Service` caches turn traces in memory
by transcript chunk, edit count and turn times, and agent traces by
transcript chunk, edit count, times, label and cut lines, at most
`TraceCacheTurns` each, the least recently read dropped first.

### Scorecard: how a session spent its context

`query.Service.SessionScorecard` joins a session's trace with three more
inputs and `scorecard.Build` turns them into one JSON object, versioned by
`scorecard.Version`. The eval suite scores against it, so add fields and
never rename one.

- **Tool calls** come from the turns' and agents' transcript chunks
  (`scorecard.ParseCalls`): each `tool_use` paired with its `tool_result`,
  plus `toolUseResult.file` `numLines`, `totalLines` and `startLine` for a
  Read. A read took the whole file when the tool says it started at line 1
  and returned every line. Without that result, it did when it gave no
  offset, limit or page range. A turn's chunk keeps the raw lines, so its
  reads are sized and its csl searches classified from the result text.
  `search.go` follows each csl search tool's text and JSON forms, and reads
  only the trailer for a cap. An agent transcript keeps only a 240-byte head
  of each input and result and no `toolUseResult`, so most agent reads count
  as `unsized` and most agent searches as `unread`. A cut input is still
  read for the file it names.
- **Hints and ledger** come from the events service, kept to the session's
  `tags.session_id`. Source `belt` holds the hints shown to the session.
  `belt-ledger` holds subagent starts and stops, permission denials,
  compactions and stop failures. A kof hint's assertion ids come from its
  `assertions` tag (comma-separated) or else from the `kof_get id=` lines
  of its message. Its `pins` tag is read too (`<id>=<path>` entries, or
  bare paths that every assertion of the fire shares), but only as the
  fallback for an assertion keeper-of-facts does not hold. events keeps 500
  entries per source, so a listing with no entry older than the session's
  first turn is `partial`.
- **Pins** come from keeper-of-facts: one `GET /api/assertions` listing,
  filtered to every shown id, so a scorecard costs it one request and each
  assertion gets its own pins, all of them, not the fire's shared first
  pins. A file an assertion
  is pinned to is `used` when any agent of the session touched it later with
  Read, Edit, MultiEdit, Write, NotebookEdit or csl_read. A hint serves the
  whole session. A csl_read must name the pin's repository as its full
  owner/name.

Each input reports its status under `sources` (`ok`, `partial`,
`unavailable`, `skipped`). A peer that does not answer within `PeerTimeout`
leaves its part null, and the scorecard still returns 200. For the events
service that is `hints` and `agents.ledger`; for keeper-of-facts, each shown
assertion's `pins` and `used`. Without hints there is no assertion to look
up, so keeper-of-facts is `skipped` and its detail says why. `flags` name
the costliest patterns once a count reaches its threshold in `facts.go`.

### Turn totals: what the session listing reads

A listing that computed traces would read every transcript chunk. So ingest
stores each closed or failed turn's trace totals on its Turn record when the
turn closes, live and imported alike (`Turn.Totals`, `internal/ingest/totals.go`).
A `TurnTotals` holds:

- the milliseconds per class: wall, model, tool, harness, user and idle
- the model calls, tool calls and failed tool calls
- the tokens by kind per rate bucket: model, speed, US-only geo and the
  long-prompt tier

It never holds a cost. The listing prices the stored tokens on read, so a
price change or a `[prices]` table reprices every row. The buckets keep
the rate modifiers, so the read-time price equals the trace's. An Agent
keeps totals of the same shape for its whole run (`Agent.Totals`), set
when its agent-close is ingested.

A turn stored before totals existed, or by an older trace build
(`TurnTotals.Version` below `trace.TotalsVersion`), gets them from the
totals backfill. serve runs it on the ingest goroutine every
`TotalsBackfillEvery`, and only while the spool is empty. A batch tries at
most `TotalsBackfillTurns` turns, computed or not, so a waiting envelope
waits a moment at most. It takes the newest session first and a session's
turns in Seq order. A computed turn no longer qualifies, so the totals
backfill is idempotent. When a batch finds nothing left to try after
earlier work, serve logs how many turns it computed. The turns go first;
a batch with room left computes Agents that lack totals the same way, and
skips an agent whose transcript cannot be read until serve restarts.

A turn whose chunk cannot be read while the store answers a ping is logged
and skipped until serve restarts. So is a turn that panics: each turn runs
under a recover. A failed store write ends the batch and skips nothing, and
so does a failed chunk read while the ping fails too. The next batch tries
that turn again. Bump `trace.TotalsVersion` when a trace change alters the
totals. The totals backfill then recomputes the stored ones, the Agents'
too. Version 2 added idle time, so totals of version 1 are computed again.
A turn-open that moves a closed turn's start back recomputes its totals; a
transcript it cannot read then clears them for the totals backfill.

Sessions get their names on the same paths. `Session.PromptLine` is the
first line of a prompt that has text once tags are gone, cut to
`SessionPromptLineRunes` (`trace.PromptLine`, which a trace's turn prompts
use too). It comes from the session's first turn, in Seq order, that has a
prompt, and it is filled once and never replaced. A turn open and a turn
close fill it when it is empty, and the totals backfill fills it for every
session that had none when serve started.

`Session.Title` follows one rule on every path (`store.OfferTitle`). A
name the person gave (a transcript `agent-name` line) outranks a generated
title. A generated title is an `ai-title` line or the hook's session title,
whose origin the hook does not say, and it outranks the prompt line. Within
one rank the first title seen wins. `Session.TitleSource` (`named` or
`generated`) keeps the rank, and a title stored before sources existed
ranks as generated. Every envelope offers the hook's title. A turn close
offers the title its lines give, even when it changes nothing on the turn,
so a replayed close makes up a lost fill. The totals backfill offers the
title of older turns, and an import offers the transcript's own pick by the
same rule.

A turn's transcript chunk keeps the last `MaxTranscriptBytes` of its lines,
cut at a line boundary, so a longer turn loses its head. A turn opens on its
prompt, so a timeline whose first line is an assistant line or a tool result
lost its head, and the trace's lines say `cut_head`. The stretch from the
turn's start to the first kept line is then harness time, a model span never
starts before its first kept line, and the calls in the cut head add no
tokens or cost.

### moondb never writes into the user's `.git`

No shadow commits, refs, notes or trailers. moondb reads git with
`--no-optional-locks` so a read never takes the index lock, and it keeps
everything it records in its own store.

## Data model & storage

```go
Session{ ID, Harness, Cwd, Title, TitleSource /* named | generated */,
         PromptLine, Model, TranscriptPath, StartedAt, EndedAt }
Turn{ ID, SessionID, Seq, PromptChunk, FinalChunk, TranscriptChunk,
      TranscriptOffset, Status /* open | closed | failed */, StartedAt, EndedAt,
      Origin /* person | task | teammate | subagent | continuation | none */,
      Totals *TurnTotals, Skipped []SkippedFiles }
Agent{ ID, SessionID, Type, Name, Description, Model, TurnIDs, Cwd, Location,
       StartedAt, EndedAt, TranscriptChunk, Lines AgentLines, Totals *TurnTotals }
SkippedFiles{ Tool, ToolUseID, At, Clean, Timeout, Failed }
TurnTotals{ Version, WallMS, ModelMS, ToolMS, HarnessMS, UserMS, IdleMS,
            Usage []ModelUsage{ Model, Speed, Geo, Long, Tokens },
            ModelCalls, ToolCalls, ToolErrors }
Edit{ ID, ToolUseID, SessionID, TurnID, AgentID, AgentType, Tool, At,
      Location{ Repo, Worktree, Canonical, Branch, Head }, Path,
      Kind /* create | update | delete | unknown */,
      PatchChunk, BeforeChunk, AfterChunk, BeforeSource, AfterBlob,
      ObservedChunk, ObservedMatches, EOL, Excluded }
UnattributedDelta{ ID, SessionID, TurnID, At, Path, FromChunk, ToChunk,
                   PatchChunk, Cause /* observed | snapshot */, Ambiguous }
Snapshot{ ID, SessionID, TurnID, Phase /* pre | post */, At, Location,
          Scope, MapChunk, FileCount, Truncated,
          Skipped{ Timeout, TooMany, TooLarge, Excluded } }
CommitLink{ ID, Repo, Worktree, Branch, Commit, Parent, Path, Blob,
            EditID, TurnID, SessionID, At }
```

`internal/store/types.go` is the source of truth for the fields; the sketch
above is for orientation. Every `*Chunk` field holds a chunk key: the
lowercase hex SHA-256 of the uncompressed bytes.

The `local` backend keeps two things under the workdir (default
`~/.local/state/moondb`, overridable with `MOONDB_WORKDIR`):

- `log/NNNNNN.jsonl`: the append-only record log, one `{"t":kind,"r":record}`
  per line. A segment rolls to the next number at `SegmentBytes` (64 MiB).
  serve replays every segment on start into an in-memory index that serves
  all reads. A write appends one line and syncs before the index changes, so
  a reader never sees a record that is not on disk.
- `objects/<ab>/<sha>.gz`: chunks, gzipped, named by the SHA-256 of their
  uncompressed bytes, sharded by the first two hex characters.

The spool sits beside them at `spool/` and the hook state at `sessions/`.
Files are 0600 and directories 0700.

Sessions, turns and agents upsert (the newest record per id wins). Edits, deltas,
snapshots and commit links are append-only facts, idempotent on their id. A
snapshot's `MapChunk` is a JSON array of its files sorted by path.

## Sandbox access

`spec.access` in `service-info.yaml` declares what `moondb serve` may reach
under srt. `make sandbox-settings` renders it to
`recipes/moondb/moondb.srt.json`, which CI diffs, so change the manifest and
never the JSON. Authoring guide: `docs/SANDBOXING.md`, "Declaring access".
`local: [events, keeper-of-facts]` names the loopback peers the scorecard
reads (`EVENTS_BASE_URL`, `KOF_PORT`).

## Build / install / test

```bash
make build    # ./moondb binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/moondb + codesign
make test     # go test ./...  (hermetic: t.TempDir + throwaway git repos, never touches real $HOME)
make lint     # golangci-lint run ./...
```

`internal/store/storetest` is the conformance suite: a new backend passes it
before it ships.

## HTTP API

Owned by `moondb serve`, bound to `127.0.0.1` only:

- `GET  /`                      : webkit-chromed web page, read-only (see Web page below)
- `GET  /app.js`                : the page's client script
- `GET  /api/sessions`          : one page of session rows, newest first (`sessions`), the totals of every session the filters keep (`window`), and the repositories the time window's sessions edited or sent attributed cost, with their session counts and attributed cost (`repos`); `since`, `repo` and `limit` filter
- `GET  /api/sessions/{id}`     : one session with its turns and counts, plus `summary`, its listing row, and `agents`, its agent rows (below)
- `GET  /api/sessions/{id}/trace`: the session's trace: every turn's spans, gaps and totals, the user gaps between turns, the agents' runs, session totals
- `GET  /api/sessions/{id}/scorecard`: the session's scorecard (Scorecard above): totals, tools, csl searches, reads, hints, agents, flags and the status of each source
- `GET  /api/turns/{id}`        : one turn with its edits, unattributed deltas and snapshots, `agents` (the agents that worked in it), plus `trace_totals` when its transcript lines hold a timeline or an agent ran under it, and `skipped` when the Bash hook skipped files of a command
- `GET  /api/agents/{id}`       : one Agent record with `row`, its session row, and `trace`, its agent trace
- `GET  /api/edits`             : edits, newest first, narrowed by repo, worktree, path, blob id, since and limit
- `GET  /api/why`               : the answer for one line of one file
- `GET  /api/history`           : the recorded edits of one path, newest first
- `GET  /api/inflight`          : the files of one checkout (`worktree`, required) with edits no commit has taken yet, newest edit first, each with `path`, `last_edit_at`, `linked_commit` (or null) and `sessions` (`id`, `title`, `last_edit_at`, `edits`); `exclude_session` leaves one session's edits out, `since` (default `InflightWindow`, 72h) and `limit` (default `InflightLimit`, 20) bound it
- `GET  /api/annotate`          : every line of one file on disk (`worktree`, `path`, optional `repo`), each with the edit, turn, session, agent and commit behind it
- `GET  /api/chunks/{sha}`      : one chunk's uncompressed bytes
- `GET  /healthz`               : 204
- `GET  /version`               : the four-key build metadata object (`version`, `commit`, `tag`, `build_time`)
- `GET  /webkit/`               : shared chrome (asset drift checks use `GET /webkit/version`)

There is no write endpoint. The spool is the only way records get in. A
request the query rejects (a relative worktree, a file outside the worktree)
answers 400.

A session row comes from the index alone, never a chunk
(`internal/server/sessions.go`):

- `title`: `Session.Title`, else `Session.PromptLine`, else the last
  segment of the cwd, else the session id. It is never empty.
- `status`: `running` while the last turn start, turn end, edit or agent
  stop is within `SessionIdleAfter`, or the last turn is open and that
  activity is within `OpenTurnStaleAfter`. Otherwise `ended`. The last
  turn is the one that started last, the higher Seq on a tie.
- `started_at`, `last_activity_at`: the session's start, and its latest
  turn start, turn end, edit or agent stop.
- `ended_at`: once the session is `ended`, the latest turn end, where
  `duration_ms` ends. That turn need not be the last turn. Absent while
  it runs.
- `transcript_path`: the transcript file the session record keeps.
- `duration_ms`: the wall time, from the earliest turn start to the latest
  turn end, the pauses between turns included. An open turn ends at its
  last activity: the latest edit or agent stop at or after its start and
  before the next turn starts.
- `active_ms`: the active time (`store.Summarize`). Each turn adds its
  totals' wall time less their user and idle time, or without totals its
  recorded span, and never less than zero. An open turn ends at its last
  activity, as in `duration_ms`. The gaps between turns never count, so a
  session left open overnight adds nothing for the night. It never exceeds
  `duration_ms`: overlapping turns, or totals that outlast the recorded
  span, are capped at the wall time.
- `turn_count`, `edit_count`, `file_count` (distinct worktree and path) and
  `agent_count`: the agents with a record, the ones known only from an
  edit, and the main conversation when it edited. `agents` names the
  subagents, each label once.
- `tokens`: the sum of the stored turn and agent totals.
- `cost`, `unpriced`: the stored tokens priced on read, the main
  conversation and its subagents together; `cost_main` and `cost_agents`
  split it. `cost` is absent when no model is priced, never a guessed
  zero, and `cost_agents` when no agent has totals.
- `repo_costs`: the attributed cost, one entry per repository with a
  part, then `orchestration` and `no_repo` when they have one. Each
  entry has `repo`, `name`, `cost` and `unpriced`, its cost absent when
  none of its models is priced.
- `repo_cost`: only when the listing filters by `repo`, the part of
  `cost` attributed to that repository. It is absent when no price covers
  that part (its `repo_costs` entry names the models) or the session has
  no priced cost, and otherwise 0 when the rule sends nothing there.
- `repos`, `repo_names` (each repo's last two path segments) and
  `worktrees`.
- `cwd_repo`, `cwd_repo_name`: the repository the session's cwd sits in
  (`Session.Location`) and its short name, whether or not the session
  edited there. Absent for a cwd outside any repository, and for a session
  recorded before sessions kept a location until an import fills it.
- `entrypoint`: how the harness was started (`cli`, `sdk-cli`), and
  `tags`: the tags its launcher set, such as `eval`. Both absent when not
  recorded.

The attributed cost (`store/attribution.go`) is a view over the records,
never stored. A subagent's cost goes to the repository most of its edits
touched, a tie to the one it edited first. Without an edit in a
repository it goes to its cwd's repository (`Agent.Location`), else to
`no_repo`. The main conversation's cost goes to the repository most of
its own edits touched, else to `orchestration`. An edit outside any
repository counts toward none. An agent without totals, or with no tokens
in them, adds nothing. `store.Summarize` keeps the parts as unpriced
usage (`RepoUsage`), and the server prices each one on read.

`since` keeps the sessions active in the window: last active at or after
it, whenever they started. Rows stay ordered and grouped by the day they
started. `window` adds up every session the filters keep, past the limit,
whole: sessions, active time, turns, edits, distinct files, tokens and cost, with
`cost_main`, `cost_agents` and `repo_costs`. `repos` ignores the `repo`
filter, so a page can offer every repo of the time window. A repository
there counts each session that edited it or sends it a part of its cost,
such as through a subagent whose cwd sits in it. A session that edited no
repository counts toward its `cwd_repo` too, so the row's cwd badge finds
it. The `repo` filter keeps sessions by the same rule (`chipRepos`). So a
chip's `sessions` is the filtered `window`'s, and its `cost` equals the
filtered window's part for it and the sum of the filtered rows'
`repo_cost`. A session kept by its cwd alone adds its attributed part,
often 0, never its whole cost.

An agent row (`internal/server/agents.go`) is the main conversation
(`type` `main`, id empty) or one subagent. It carries `id`, `type`, `name`,
`model`, `turn_ids`, `started_at`, `ended_at`, `duration_ms`,
`model_calls`, `tool_calls`, `tool_errors`, `edit_count`, `repos`,
`cost_repo`, `tokens`, `cost` and `recorded`. `cost_repo` names where the
attribution rule sends a session row's cost; a row without tokens, and
every turn row, leaves it out. A session lists the main conversation first, its time and
calls from the turn totals, then each Agent record by start, then each
subagent known only from its edits (`recorded` false, no calls or cost). A
turn lists the same rows for its own work: the main row from the turn's
trace, then a row per subagent span.

A main segment's record (`store.Agent.IsMainSegment`: type `internal`, no
line read, no agent transcript) is not an Agent. Such records were stored
before `record` skipped them, and no listing shows one: it gets no agent
row, and the listing's `agent_count` and `agents`, the session trace and
the scorecard leave it out. `GET /api/agents/{id}` still serves one asked
for by id. Ingest drops such an envelope from an older spool, so no store
migration is needed.

## Web page

The page is one static shell (`internal/server/shell.html`) and one script
(`internal/server/app.js`) on the `Webkit` global, rendered in the browser
from the JSON API (docs/adr/0005). There is no build step and no framework.
Colours come only from webkit roles, so light and dark both follow the
theme. The query string picks one of four views. The `?session=` form is the URL the MCP tools
return, so keep it stable.

- `/`: the sessions active in the last 7 days unless `since` says
  otherwise (`since=any` lifts the bound), grouped by the day they started,
  newest first.
  A row has a repo badge, the title as the link, and a running or ended
  dot with how long ago the session was last active. A session that
  edited no repo shows its `cwd_repo` as a hollow badge instead, which
  filters to that repo, and "no repo" only when its cwd sits in none.
  Small badges follow the title. A set entrypoint other than `cli` gets
  one: `headless` when it starts with `sdk-` (`claude -p` and the SDKs),
  else the value itself, such as a desktop app's or an IDE extension's.
  Its hover title names the entrypoint. Each tag gets a badge too. The
  row's active time follows, or its wall time when it has none. When the wall time crosses a
  local midnight, "over N days" follows, on a second line when the column
  is too narrow, and the hover title gives the wall time. Then come
  its turn, edit, file and agent counts and its cost, the main
  conversation's and its subagents'. A window switch and repo chips
  (`repo`) sit above a strip that adds the window up, active time
  included, with the subagents'
  part of the cost beside the total. A cost's hover title splits it.
  Each chip counts the sessions its filter lists, the ones that edited
  its repo or sent it cost, and shows the cost they attribute to it.
  Under the strip's figures a bar stacks the window's cost by repo, then
  orchestration and "no repo (cwd)", the page's name for `no_repo`. A
  row's "no repo" badge stays apart: that session edited no repo and its
  cwd sits in none.
  A label under it gives each part's amount, and a hover title its share
  and the attribution rule. Filtered to a repo, the cost column turns into
  "Attributed": the part each session gives that repo, with the session
  total in the hover title. The day rows then add those parts.
  `Show more` grows `limit`. The filters stay in the URL.
- `/?session=<id>`: the title, repo badges (or the hollow cwd badge), the
  entrypoint and tag badges, status, the active time over
  the wall time ("active 1h 51m over 23h 05m (2 days)") and start time,
  and a "Cost by repo" line with the session's `repo_costs`. Then a Trace
  section, an Agents section, the turns in the order they started and the
  worktrees. The Agents section lists the main conversation first, then
  each subagent with its type, model, the turns it ran under, time, calls,
  edits, tokens and cost; clicking the Cost heading sorts the subagents
  costliest first. Under each agent's name sits its `cost_repo`, so the
  rows add up to the cost line. Each turn has its prompt label, a badge when a person
  did not start it (`task`, `teammate`, `subagent`, `continued`),
  status, edit count and every agent that ran under it. The page labels and
  classifies a turn recorded before origins existed from its prompt, as
  `trace.PromptLabel` and `store.PromptOrigin` do. The session id, cwd,
  harness, entrypoint, model and transcript path sit last, in a closed "Session
  details" disclosure. The Trace section has a totals strip (wall, model,
  tool, tokens, cost) and one timeline lane per turn, in the same order.
  The wall's note gives the time waiting for input and, when a turn had
  some, the idle time. A lane has rows for model, tool and subagent bars in
  the webkit series colours, a failed tool call in red. Parallel subagents
  get a row each, up to four. Each lane is scaled to its own turn, and a
  subagent bar runs from its spawn to its stop, cut at the lane's end. An
  idle stretch is a hatched break over the bars, at most 4% of the lane
  wide. Its title gives its length and "no transcript line for over
  30 min". The rest of the lane scales to the turn's other time, so a
  turn with days of idle time still shows its calls. A bar's hover title
  gives its duration, calls, tokens and cost, and clicking it opens the
  turn.
- `/?turn=<id>`: the full prompt and final message with fixation reading,
  the turn's cost split between the main conversation and its subagents, an
  "Idle" row when the turn has idle time, who started it, a "Skipped" row
  when the Bash hook skipped files of a command (clean after it, past the
  budget, or with a status git could not give), the agents that worked in
  it with their calls, time, edits and cost (those without edits too), and
  the edits grouped by worktree, then path. Each edit shows its tool, kind,
  agent, time, before source and observed match. Its unified diff is drawn
  in the browser from the patch chunk, and a create without hunks shows its
  after content. Then the unattributed deltas with their diffs, the
  snapshots with their file and skipped counts, the commit links, and the
  raw transcript lines behind a closed disclosure. Clicking an added line
  asks `/api/why` about that line's text and shows the answer inline.
- `/?worktree=<abs>&path=<rel>`: the file today, from `/api/annotate`, with
  a gutter band per turn (solid for blame, dashed for a content match);
  hovering a line names its turn's prompt and clicking opens the turn.
  Below it, the path's recorded history as a timeline with diffs on demand.
  A file outside any repository (`?path=<abs>`) shows the history only.

Large records stay cheap. A diff draws its first 400 rows and the rest on
request. A trace draws its first 25 turn lanes and the rest on request.
The transcript shows its first 200 KB beside a download link to
the chunk.

## Commands

```bash
moondb serve [--port 7433] [--workdir ~/.local/state/moondb]
moondb record                  # one hook payload on stdin; wired by the hooks, not run by hand
moondb import [--since 30d] [--dry-run] [--force] [--json]   # import past sessions from their transcripts
moondb mcp                     # stdio MCP shim over serve
moondb why <path>:<line>       # the Edit, turn, prompt, session and commit behind one line
moondb history <path>          # the recorded edits of a path across every checkout, newest first; --worktree narrows to one
moondb inflight [--worktree <root>] [--exclude-session <id>]   # files in the checkout other sessions edited that no commit has taken yet
moondb sessions [--repo <repo>]  # recorded sessions, newest first, with their title, active time and wall time; --repo adds COST and ATTRIBUTED
moondb trace <session-id> [--json]   # time and cost totals, the agents, the longest tool calls, the most expensive model calls
moondb scorecard <session-id> [--json]   # how a session spent its context: searches, reads, hints, agents, flags
moondb doctor                  # config, hooks wired, serve reachable, store readable, version skew
moondb docs                    # print the operating doc
moondb version [-o json]
```

`--port`, `--workdir` and `--base-url` are persistent flags, so every command
takes them. The query commands are thin HTTP clients of serve.
`moondb version` prints the bare git commit that built the binary; `-o json`
prints the full build metadata object.

## MCP tools

Thin client over the API above (`internal/client`), served on stdio by
`moondb mcp`. Every tool is read-only:

- `moondb_why(path, line)`: the Edit behind one line, with its turn, prompt, session, agent and commit link, or ranked candidates when only the content matched
- `moondb_history(path)`: the recorded edits of a path, newest first
- `moondb_inflight(worktree)`: the files of a checkout with edits no commit has taken yet, with the sessions behind them; `exclude_session` leaves the caller's own out
- `moondb_turn(id)`: one turn: prompt, final message, edits, unattributed deltas, snapshots, commit links, and the agents that worked in it; the summary names its subagents
- `moondb_sessions()`: recorded sessions, newest first, each with its title, status, times, active and wall time, counts, subagent names, tokens and cost split between the main conversation and the subagents, and its attributed cost per repository (`repo_costs`); with `repo`, also `repo_cost` beside the whole `cost`
- `moondb_trace(id)`: one session's time split (model, tool, harness, user, idle), tokens by kind and cost with its main and agent parts, the subagents costliest first, and the longest tool calls and most expensive model calls of every agent
- `moondb_scorecard(id)`: the session's scorecard, the same JSON as `GET /api/sessions/{id}/scorecard`, with a one-line summary
- `moondb_doctor()`: the same checks as `moondb doctor`, returned as JSON, for a client that can call a tool but has no shell

Tool responses include a `url` built from `MOONDB_BASE_URL` (for example
`http://moondb.this`). The client itself always calls `localhost:<port>`.

## Shared UI: webkit

The page chrome and components come from the in-module package
`github.com/mad01/thismoon/webkit`, mounted at `GET /webkit/` by
`webkit.Mount(mux)` and loaded by the embedded shell page. The page is
client-rendered from the JSON API (docs/adr/0005). Don't add palette, topbar
or theme CSS locally; it lives in webkit only. A webkit change ships at the
next build:

```bash
make install && t-man restart moondb
```

Confirm the shared assets with `GET /webkit/version`.

## Gotchas

- **The Edit hook is on the session's critical path.** It is synchronous, so
  `record` latency is felt on every edit. Keep new work in `record` inside
  `RecordBudget` and move anything slow to serve.
- **Snapshots belong to the async hooks.** Their 2 s budget is fine at a
  turn edge and far too slow for the Edit hook.
- **record never fails a hook.** It exits 0 and prints nothing on stdout even
  when the spool is unwritable. A capture gap shows up in `moondb doctor` and
  in a missing Edit, never as a broken session.
- **Disk is evidence, not truth.** Never derive after content from the
  observed read; a mismatch must stay visible as an unattributed delta.
- **No writes into `.git`.** Not even a note or a ref. Every git call uses
  `--no-optional-locks`.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `moondb docs`): serve-must-be-running, store layout, failure
  modes, version-skew checks. Keep those facts there, not here.
- **Version probe convention.** `GET /version` and `moondb version -o json`
  return the shared four-key build metadata object. Plain `moondb version`
  stays a bare token, which status parses.

## See also

- Recipe: `recipes/moondb/recipe.toml` (+ `recipes/moondb/CLAUDE.md`)
- Human docs: `README.md`, `why.md`, `architecture.md`, `config.md`
- The hook wiring in Claude Code settings, the `moondb.this` d-man route and
  the MCP registration live in the consuming repo's private overlay
  (docs/adr/0006), not here.
