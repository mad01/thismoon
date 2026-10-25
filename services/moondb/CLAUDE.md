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
  facts.go             - port, workdir, capture caps and budgets, default excludes, trace limits, totals backfill pace, listing status rule, model prices, agentdoc.Facts
  embed.go             - embeds operating.md for `moondb docs`
  internal/
    cli/               - cobra: root, serve, record, import, mcp, why, history, sessions, trace, doctor, docs, version
    record/            - the capture hook: payload decode, envelopes, observed read, HEAD blob, transcript tail, Bash diffs, turn-edge snapshots, per-session state
    importer/          - the transcript import: transcript parse, turns and edits as envelopes, file-history originals
    spool/             - envelope format, atomic writer, drain reader
    ingest/            - serve's spool drain: envelopes to records, turn totals at close, the totals backfill, session titles
    derive/            - before and after content per Edit, snapshot records, unattributed deltas
    patch/             - forward and reverse patch apply, hunk context checks, line endings
    gitlink/           - git blob ids and commit links
    store/             - Store and Chunks interfaces and record types (store.go, types.go)
      local/           - the local disk backend: JSONL log, gzip chunks, in-memory index
      storetest/       - conformance suite every backend must pass
    query/             - why, history and annotate answers over the store and git; traces with their cache
    trace/             - turn and session traces from transcript lines: spans, gaps, totals, prices (pure)
    server/            - HTTP API + webkit web page
    client/            - HTTP client shared by the CLI and mcpserver
    mcpserver/         - MCP tools
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Capture: four hook lines feed `moondb record`

The consuming repo wires four Claude Code hook lines to `moondb record`. Each
one pipes its JSON payload on stdin:

| Hook | Mode | What `record` spools |
|------|------|----------------------|
| UserPromptSubmit | async | turn-open: `prompt_id`, the prompt text as a chunk, `session_id`, cwd, the transcript byte offset. Then a `pre` snapshot of each worktree in play. |
| PostToolUse on `Edit\|Write\|MultiEdit\|NotebookEdit` | sync, 3 s timeout | one edit envelope per file. It holds `tool_use_id`, the tool input and patch, the location (repo, worktree, branch, HEAD), the observed bytes with their blob id, and the path's HEAD blob id and content. |
| PostToolUse on `Bash` | async | one edit envelope per file in the payload's `bashEditDiff.files`: its hunks as the patch, the observed bytes and blob id, and the HEAD blob id and content, with no tool input. One more per other `changedFiles` path, with no hunks. Nothing when the payload has no `bashEditDiff`. |
| Stop | async | turn-close: the final assistant message, the transcript tail from the turn's offset, and a head-moved report for each touched worktree whose HEAD changed. Then a `post` snapshot of each worktree in play. |

The Edit hook is synchronous on purpose. Edits in one assistant message land
milliseconds apart, so an async hook would read the disk after the next edit
had already changed it. NotebookEdit is recorded by path only. A file outside
any git repo lands in a `norepo` bucket.

Claude Code reports `bashEditDiff` only when the user setting
`bashEditDiffEnabled` is on (v2.1.269 or later). `files` holds up to five
diffs and `changedFiles` up to 200 paths. A Bash create seen only in
`changedFiles` has no diff to say so and gets kind `unknown`.

The Bash hook is async, so its disk read can land after a later Edit of the
same file. Every envelope is stamped when its hook starts, before any read,
so that Edit sorts after the Bash edit. The Bash record may then carry the
Edit's bytes in its observed read and after content. When the two after
blobs coincide, the newest-edit tie-break keeps the commit link and `why`
with the Edit.

`record` follows belt's record contract (docs/adr/0024): nothing on stdout,
exit 0 always, panics recovered. It works within `RecordBudget` (200 ms, in
`facts.go`) and spools whatever it has gathered when the budget runs out.
Files matching a `capture.exclude` glob are recorded by path only. Files over
`MaxChunkBytes` (1 MiB) are recorded by path and patch. HEAD content is read
on every edit of a tracked file, and the spool writer leaves it out while the
store holds it.

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
until the live Stop arrives. A session without a file edit is skipped. `--dry-run` reads and counts everything and writes
nothing; with serve down it runs without the store lookups and says so.
`--projects` and `--file-history` point at other transcript and
file-history directories; the defaults live in `facts.go`.

Ids match the hooks'. A turn is the transcript's `promptId`, the same id the
hook calls `prompt_id`; a transcript without one keys the turn by the prompt
entry's uuid. An edit is `<tool_use_id>:<index>`. Every import envelope is
marked `backfill`, so importing a session again, or one partly captured live,
only fills: a stored edit stays, and a session or turn keeps its values and
gets its empty fields filled, an open turn closed. An earlier imported
prompt still moves a start time back. A subagent's edits come from `<session>/subagents/agent-<id>.jsonl`, carry its
id and type, and belong to the main turn its entries name.

What a transcript gives back:

- recovered: the prompt, the final message, the turn's transcript lines, the
  tool input, the structured patch, `originalFile` when Claude Code kept it,
  a Write's content, and the subagent
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
- A subagent span runs from the subagent's first recorded edit to its last,
  clamped to the turn's span: the Bash hook is async, so an edit can land
  after Stop. Its own transcript is a separate file the turn does not keep,
  so its model calls are not priced.
- The gaps cover the turn from start to end, each stretch classed by the
  event that ends it. An assistant line ends model time and a tool result
  ends tool time. A new input after the model finished (its stop reason was
  `end_turn`, or a Stop line came) ends user time. Anything else is harness
  time: hooks, request setup, and the Stop hooks up to the Stop line (a
  `system` line with subtype `stop_hook_summary`). The tail after a Stop
  line is user time.
- The totals by kind add up to the wall time to the millisecond. Each kind
  is cut from the running sum of the kinds, so the sub-millisecond rest of
  one carries into the next.
- User time inside a turn is real. The harness keeps one prompt id across a
  Stop when a teammate message or a task notification restarts the loop, so
  that wait lands in the same turn. The session trace adds the stretches
  between turns.

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

serve computes traces on read. `query.Service` caches them in memory by
transcript chunk, edit count and turn times, at most `TraceCacheTurns`, the
least recently read dropped first.

### Turn totals: what the session listing reads

A listing that computed traces would read every transcript chunk. So ingest
stores each closed or failed turn's trace totals on its Turn record when the
turn closes, live and imported alike (`Turn.Totals`, `internal/ingest/totals.go`).
A `TurnTotals` holds:

- the milliseconds per class: wall, model, tool, harness and user
- the model calls, tool calls and failed tool calls
- the tokens by kind per rate bucket: model, speed, US-only geo and the
  long-prompt tier

It never holds a cost. The listing prices the stored tokens on read, so a
price change or a `[prices]` table reprices every row. The buckets keep
the rate modifiers, so the read-time price equals the trace's.

A turn stored before totals existed, or by an older trace build
(`TurnTotals.Version` below `trace.TotalsVersion`), gets them from the
totals backfill. serve runs it on the ingest goroutine every
`TotalsBackfillEvery`, and only while the spool is empty. A batch tries at
most `TotalsBackfillTurns` turns, computed or not, so a waiting envelope
waits a moment at most. It takes the newest session first and a session's
turns in Seq order. A computed turn no longer qualifies, so the totals
backfill is idempotent. When a batch finds nothing left to try after
earlier work, serve logs how many turns it computed.

A turn whose chunk cannot be read while the store answers a ping is logged
and skipped until serve restarts. So is a turn that panics: each turn runs
under a recover. A failed store write ends the batch and skips nothing, and
so does a failed chunk read while the ping fails too. The next batch tries
that turn again. Bump `trace.TotalsVersion` when a trace change alters the
totals, and the totals backfill recomputes the stored ones. A turn-open
that moves a closed turn's start back recomputes its totals; a transcript
it cannot read then clears them for the totals backfill.

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
      Totals *TurnTotals }
TurnTotals{ Version, WallMS, ModelMS, ToolMS, HarnessMS, UserMS,
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

Sessions and turns upsert (the newest record per id wins). Edits, deltas,
snapshots and commit links are append-only facts, idempotent on their id. A
snapshot's `MapChunk` is a JSON array of its files sorted by path.

## Sandbox access

`spec.access` in `service-info.yaml` declares what `moondb serve` may reach
under srt. `make sandbox-settings` renders it to
`recipes/moondb/moondb.srt.json`, which CI diffs, so change the manifest and
never the JSON. Authoring guide: `docs/SANDBOXING.md`, "Declaring access".

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
- `GET  /api/sessions`          : one page of session rows, newest first (`sessions`), the totals of every session the filters keep (`window`), and the repositories the time window's sessions edited with their session counts (`repos`); `since`, `repo` and `limit` filter
- `GET  /api/sessions/{id}`     : one session with its turns and counts, plus `summary`, its listing row
- `GET  /api/sessions/{id}/trace`: the session's trace: every turn's spans, gaps and totals, the user gaps between turns, session totals
- `GET  /api/turns/{id}`        : one turn with its edits, unattributed deltas and snapshots, plus `trace_totals` when its transcript lines hold a timeline
- `GET  /api/edits`             : edits, newest first, narrowed by repo, worktree, path, blob id, since and limit
- `GET  /api/why`               : the answer for one line of one file
- `GET  /api/history`           : the recorded edits of one path, newest first
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
- `status`: `running` while the last turn start, turn end or edit is within
  `SessionIdleAfter`, or the last turn is open and that activity is within
  `OpenTurnStaleAfter`. Otherwise `ended`.
- `started_at`, `last_activity_at`: the session's start, and its latest
  turn start, turn end or edit.
- `ended_at`: once the session is `ended`, its last turn's end (an open
  turn ends where it started). Absent while it runs.
- `transcript_path`: the transcript file the session record keeps.
- `duration_ms`: the first turn's start to the last turn's end. An open
  turn ends where it started.
- `turn_count`, `edit_count`, `file_count` (distinct worktree and path) and
  `agent_count` (distinct agents that edited, the main conversation among
  them).
- `tokens`: the sum of the stored turn totals, main conversation only.
- `cost`, `unpriced`: the stored tokens priced on read. `cost` is absent
  when no model is priced, never a guessed zero.
- `repos`, `repo_names` (each repo's last two path segments) and
  `worktrees`.

`since` keeps the sessions active in the window: last active at or after
it, whenever they started. Rows stay ordered and grouped by the day they
started. `window` adds up every session the filters keep, past the limit,
whole: sessions, turns, edits, distinct files, tokens and cost. `repos`
ignores the `repo` filter, so a page can offer every repo of the time
window.

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
  A row has a repo badge, the title as the link, a running or ended dot
  with how long ago the session was last active and how long it ran, then
  its turn, edit, file and agent counts and its cost. A window switch and
  repo chips (`repo`) sit above a strip that adds the window up. `Show
  more` grows `limit`. The filters stay in the URL.
- `/?session=<id>`: the title, repo badges, status, run time and start
  time, then a Trace section, the turns in order and the worktrees. Each
  turn has the prompt's first line, status, edit count and the agents
  seen. The session id, cwd, harness, model and transcript path sit last,
  in a closed "Session details" disclosure. The Trace section has a totals
  strip (wall, model, tool, tokens, cost) and one timeline lane per turn
  with a row each for model, tool and subagent bars in the webkit series
  colours, a failed tool call in red. Each lane is scaled to its own turn.
  A bar's hover title gives its duration, tokens and cost, and clicking it
  opens the turn.
- `/?turn=<id>`: the full prompt and final message with fixation reading,
  the agents (main conversation or subagent type), and the edits grouped by
  worktree, then path. Each edit shows its tool, kind, agent, time, before
  source and observed match. Its unified diff is drawn in the browser from
  the patch chunk, and a create without hunks shows its after content. Then the
  unattributed deltas with their diffs, the snapshots with their file and
  skipped counts, the commit links, and the raw transcript lines behind a
  closed disclosure. Clicking an added line asks
  `/api/why` about that line's text and shows the answer inline.
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
moondb history <path>          # the recorded edits of a path, newest first
moondb sessions                # recorded sessions, newest first, with their title and duration
moondb trace <session-id> [--json]   # time and cost totals, the longest tool calls, the most expensive model calls
moondb doctor                  # serve reachable, store readable, version skew
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
- `moondb_turn(id)`: one turn: prompt, final message, edits, unattributed deltas, snapshots, commit links
- `moondb_sessions()`: recorded sessions, newest first, each with its title, status, times, counts, tokens and cost
- `moondb_trace(id)`: one session's time split (model, tool, harness, user), tokens by kind and cost, with the longest tool calls and the most expensive model calls
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
