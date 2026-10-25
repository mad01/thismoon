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
  facts.go             - port, workdir, capture caps and budgets, default excludes, agentdoc.Facts
  embed.go             - embeds operating.md for `moondb docs`
  internal/
    cli/               - cobra: root, serve, record, mcp, why, history, sessions, doctor, docs, version
    record/            - the capture hook: payload decode, envelopes, observed read, HEAD blob, transcript tail, per-session state
    spool/             - envelope format, atomic writer, drain reader
    derive/            - before and after content per Edit, unattributed deltas
    patch/             - forward and reverse patch apply, hunk context checks, line endings
    gitlink/           - git blob ids and commit links
    store/             - Store and Chunks interfaces and record types (store.go, types.go)
      local/           - the local disk backend: JSONL log, gzip chunks, in-memory index
      storetest/       - conformance suite every backend must pass
    query/             - why, history and annotate answers over the store and git
    server/            - HTTP API + webkit web page
    client/            - HTTP client shared by the CLI and mcpserver
    mcpserver/         - MCP tools
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Capture: three hooks feed `moondb record`

The consuming repo wires three Claude Code hooks to `moondb record`. Each one
pipes its JSON payload on stdin:

| Hook | Mode | What `record` spools |
|------|------|----------------------|
| UserPromptSubmit | async | turn-open: `prompt_id`, the prompt text as a chunk, `session_id`, cwd, the transcript byte offset |
| PostToolUse on `Edit\|Write\|MultiEdit\|NotebookEdit` | sync, 3 s timeout | one edit envelope per file. It holds `tool_use_id`, the tool input and patch, the location (repo, worktree, branch, HEAD), and the observed bytes with their blob id. A path's first edit also ships its HEAD content. |
| Stop | async | turn-close: the final assistant message, the transcript tail from the turn's offset, and a head-moved report for each touched worktree whose HEAD changed |

The Edit hook is synchronous on purpose. Edits in one assistant message land
milliseconds apart, so an async hook would read the disk after the next edit
had already changed it. NotebookEdit is recorded by path only. A file outside
any git repo lands in a `norepo` bucket.

`record` follows belt's record contract (docs/adr/0024): nothing on stdout,
exit 0 always, panics recovered. It works within `RecordBudget` (200 ms, in
`facts.go`) and spools whatever it has gathered when the budget runs out.
Files matching a `capture.exclude` glob are recorded by path only. Files over
`MaxChunkBytes` (1 MiB) are recorded by path and patch.

`record` keeps a little state per session under `<workdir>/sessions/`, read
back only by later hooks of the same session:

- `<sid>.turn`: the open turn's prompt id and the transcript offset it opened at
- `<sid>.paths`: the files the session edited; a path's first edit ships its HEAD content
- `<sid>.touched`: one worktree per line, where the session edited
- `<sid>.heads`: the HEAD last seen in each touched worktree

### Single-writer architecture (load-bearing)

`moondb serve` is the only writer of the store. `record` writes only to the
spool and its session state. It reads `objects/` only to skip shipping a
chunk the store already has.

- `moondb serve` drains the spool (a file watch plus a 1 s poll), derives
  each Edit, appends the records, and serves the web page, the JSON API,
  `/version` and `/webkit/`.
- `moondb mcp` holds no state: a thin HTTP client to the serve API on
  `localhost:<port>`. If serve is down, tools return an unreachable error
  that names `moondb doctor`.
- `moondb why`, `history` and `sessions` are the same thin HTTP client.

A spool envelope lives until serve has ingested it, so nothing is lost while
serve is down or restarting. Ingest is idempotent on the envelope id, so a
crash between append and delete replays harmlessly.

### Derivation: after content is never trusted from disk

serve derives the after content of every Edit from the tool payload: Write
takes `tool_input.content`, Edit forward-applies the patch to the before
content. serve keeps the bytes `record` read from disk as the observed
content. When observed and derived differ (a formatter hook ran, a human
typed), serve records an unattributed delta for the difference.

Before content comes from the first source that verifies against the patch's
context lines:

1. `originalFile` from the tool response, when Claude Code sent it (small files)
2. the last recorded after content for the same worktree and path
3. the HEAD content `record` shipped on the path's first edit
4. the observed bytes with the patch reverse-applied
5. none: the Edit keeps its hunks and records `before_source: unknown`

A Write that creates a file has an absent before, not an empty one. A file
with carriage-return line endings (CRLF) is matched on a newline-only view of
its text and stored as its exact bytes with an `eol` flag.

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

### moondb never writes into the user's `.git`

No shadow commits, refs, notes or trailers. moondb reads git with
`--no-optional-locks` so a read never takes the index lock, and it keeps
everything it records in its own store.

## Data model & storage

```go
Session{ ID, Harness, Cwd, Title, Model, TranscriptPath, StartedAt, EndedAt }
Turn{ ID, SessionID, Seq, PromptChunk, FinalChunk, TranscriptChunk,
      TranscriptOffset, Status /* open | closed | failed */, StartedAt, EndedAt }
Edit{ ID, ToolUseID, SessionID, TurnID, AgentID, AgentType, Tool, At,
      Location{ Repo, Worktree, Canonical, Branch, Head }, Path,
      Kind /* create | update | delete | unknown */,
      PatchChunk, BeforeChunk, AfterChunk, BeforeSource, AfterBlob,
      ObservedChunk, ObservedMatches, EOL, Excluded }
UnattributedDelta{ ID, SessionID, TurnID, At, Path, FromChunk, ToChunk,
                   PatchChunk, Cause, Ambiguous }
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

Sessions and turns upsert (the newest record per id wins). Edits, deltas and
commit links are append-only facts, idempotent on their id.

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
- `GET  /api/sessions`          : sessions, newest first
- `GET  /api/sessions/{id}`     : one session with its turns
- `GET  /api/turns/{id}`        : one turn with its edits and unattributed deltas
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

## Web page

The page is one static shell (`internal/server/shell.html`) and one script
(`internal/server/app.js`) on the `Webkit` global, rendered in the browser
from the JSON API (docs/adr/0005). There is no build step and no framework.
Colours come only from webkit roles, so light and dark both follow the
theme. The query string picks one of four views. The `?session=` form is the URL the MCP tools
return, so keep it stable.

- `/`: sessions, newest first, with the turn and edit counts and the repos
  each one touched. A repo filter and a since filter map to the
  `/api/sessions` query parameters and stay in the URL.
- `/?session=<id>`: the session's worktrees and its turns in order, each
  with the prompt's first line, status, edit count and the agents seen.
- `/?turn=<id>`: the full prompt and final message with fixation reading,
  the agents (main conversation or subagent type), and the edits grouped by
  worktree, then path. Each edit shows its tool, kind, agent, time, before
  source and observed match. Its unified diff is drawn in the browser from
  the patch chunk, and a create without hunks shows its after content. Then the
  unattributed deltas with their diffs, the commit links, and the raw
  transcript lines behind a closed disclosure. Clicking an added line asks
  `/api/why` about that line's text and shows the answer inline.
- `/?worktree=<abs>&path=<rel>`: the file today, from `/api/annotate`, with
  a gutter band per turn (solid for blame, dashed for a content match);
  hovering a line names its turn's prompt and clicking opens the turn.
  Below it, the path's recorded history as a timeline with diffs on demand.
  A file outside any repository (`?path=<abs>`) shows the history only.

Large records stay cheap. A diff draws its first 400 rows and the rest on
request. The transcript shows its first 200 KB beside a download link to
the chunk.

## Commands

```bash
moondb serve [--port 7433] [--workdir ~/.local/state/moondb]
moondb record                  # one hook payload on stdin; wired by the hooks, not run by hand
moondb mcp                     # stdio MCP shim over serve
moondb why <path>:<line>       # the Edit, turn, prompt, session and commit behind one line
moondb history <path>          # the recorded edits of a path, newest first
moondb sessions                # recorded sessions, newest first
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
- `moondb_turn(id)`: one turn: prompt, final message, edits, unattributed deltas
- `moondb_sessions()`: recorded sessions, newest first
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
