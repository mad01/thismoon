# moondb architecture

## Overview

moondb records every file edit a Claude Code session makes between commits.
Capture and storage are separate processes. Claude Code hooks run
`moondb record`, which writes envelopes to a spool on disk and exits.
`moondb serve --port 7433` is the single writer. It binds loopback only,
runs as a t-man agent, and d-man fronts it as `http://moondb.this/`. It
drains the spool, derives each edit's content, links edits to commits, and
serves the web page, the JSON API and `/webkit/`. `moondb mcp` and the query
commands hold no state; they are thin HTTP clients of serve.

```
Claude Code hooks ──stdin JSON──▶ moondb record ──files──▶ spool/ ──▶ moondb serve (single writer)
  UserPromptSubmit (async), then snapshots                               ├─ store: log/ + objects/
  PostToolUse Edit|Write|MultiEdit|NotebookEdit (sync)                   ├─ HTTP API /api/*
  PostToolUse Bash (async, needs bashEditDiffEnabled)                    └─ web page
  Stop (async), then snapshots
moondb import ──reads ~/.claude transcripts──▶ spool/ (the same envelopes, backfilled)
moondb mcp (stdio shim) ──HTTP──▶ serve
moondb why | history | sessions ──HTTP──▶ serve
```

## Structure

```
cmd/moondb/         entrypoint, delegates to internal/cli
facts.go            port, workdir, caps and budgets, default excludes, agentdoc.Facts
internal/
  cli/              Cobra commands: serve, record, import, mcp, why,
                    history, sessions, doctor, docs, version
  record/           the hook side: payload decode, envelope builders,
                    observed read and blob id, HEAD content, transcript
                    tail, Bash diffs, turn-edge snapshots, per-session
                    state, the 200 ms and 2 s budgets
  importer/         the transcript backfill: parse a session transcript
                    and its subagent transcripts, map turns and edits to
                    envelopes, take large files' originals from the file
                    history
  spool/            envelope format, atomic writer, drain reader
  derive/           before and after content per Edit, snapshot records,
                    unattributed deltas
  patch/            forward and reverse patch apply, hunk context checks,
                    line-ending handling
  gitlink/          git blob ids and commit links
  store/            Store and Chunks interfaces and record types
    local/          local disk backend: JSONL log, gzip chunks, index
    storetest/      conformance suite for every backend
  query/            why and history over the store and git
  server/           HTTP API + webkit web page
  client/           HTTP client shared by the CLI and mcpserver
  mcpserver/        MCP stdio setup and the read-only tools
```

## Data flow

Capture: a hook runs `moondb record` with its payload on stdin. On
UserPromptSubmit it spools a turn-open envelope with the prompt text and the
transcript offset. On PostToolUse for a file tool it spools one edit
envelope per file. The envelope holds the tool input and structured patch,
the file's repo, worktree, branch and HEAD, and the observed bytes with
their blob id, and the path's HEAD blob id and content. Every envelope is
stamped when its hook starts. On PostToolUse for Bash it reads the payload's
`bashEditDiff`: one edit envelope per diffed file, with the hunks as the
patch and no tool input, and one per other changed path without hunks. On
Stop it spools a turn-close envelope with the
final message and the transcript tail. For each touched worktree whose HEAD
moved since the hook last looked, it adds a head-moved envelope listing the
new commits' (path, blob id) pairs.

Snapshots: after its own envelope, UserPromptSubmit and Stop each spool a
snapshot envelope per worktree in play, the cwd's and every touched one.
It lists the worktree's dirty files and the session's touched paths, each
with its content chunk, blob id, size and HEAD blob. A path it cannot read
is listed with the reason, and skipped paths are counted. The envelope
carries when the capture started and is stamped when its reads are done.
A state hash over HEAD, the status and each file's size and mtime skips a
snapshot when nothing changed. All snapshots of one hook share a 2 s budget.

The writer puts each chunk file in place
before it renames the envelope, so serve never sees an envelope without its
chunks.

Import: `moondb import` reads the session transcripts under
`~/.claude/projects` and writes the envelopes the hooks would have written.
It uses the hooks' ids and marks every envelope `backfill`, so a re-run or
an overlap with live capture only fills: a stored edit stays, and a session
or turn keeps its values and gets its empty fields filled, an open turn
closed. An earlier imported prompt still moves a start time back. A
transcript has no disk read and no HEAD, so imported edits
carry neither and the import writes no head-moved envelope. Most imported
files sat in worktrees deleted since. The import places those by the
nearest surviving work tree, the worktree layout of their path, or the
session cwd. The repo comes from a known location of the same name.

Ingest: serve watches the spool and polls it every second. It reads
envelopes in name order, stores their chunks, derives each Edit and
Snapshot, appends the records, and deletes the envelope. Ingest is idempotent on the envelope id.

Derive: after content comes from the tool input: Edit and MultiEdit swaps,
or a Write's content. Before content is the first source the call fits:
nothing for a create, the original, the latest earlier after content of the
path, the worktree's latest earlier snapshot, HEAD, or the observed bytes
with the call undone. The chain yields to a snapshot begun after its edit
that holds the path's content or, whole, lacks the path. A source fits when
each `old_string` occurs once and the hunks apply, tabs read as two spaces.
A Bash edit's after is its observed bytes, its before the first source that
exists. Differing observed bytes become an unattributed delta.

A snapshot's file map becomes one chunk. Each file in it is compared with
the latest content known of the path before: the worktree's previous
snapshot or the latest edit's observed bytes, whichever is newer. A path a
whole previous snapshot lacked held HEAD's content, which counts when both
snapshots sat on one HEAD. A difference becomes an unattributed delta with
cause `snapshot`, unless an edit of the path landed while the snapshot read.

Link: a head-moved envelope's blob ids are matched against recorded
after-blob ids, and each match becomes a commit link.

Query: `why` runs `git blame` on the line, takes the path's blob id at the
blamed commit, and looks up the Edit with that after-blob. When that finds
nothing, it matches the normalized line text against lines Edits added to
the path. `history` lists a path's Edits newest first.

## Storage

The `local` backend keeps everything under the workdir
(`~/.local/state/moondb`, overridable with `MOONDB_WORKDIR`):

- `log/NNNNNN.jsonl`: append-only records, one per line. A segment rolls at
  64 MiB. serve replays every segment on start into an in-memory index of
  sessions, turns, edits by path and by blob id, snapshots by worktree,
  and commits.
- `objects/<ab>/<sha>.gz`: gzipped chunks named by the SHA-256 of their
  uncompressed bytes.
- `spool/`: envelopes and their chunk files, waiting for serve.
- `sessions/`: the hook's per-session state (`<sid>.turn`, `.paths`,
  `.touched`, `.heads`, `.snap`).

A write appends one line and syncs before the index changes. Files are 0600
and directories 0700. The `Store` interface takes a context on every method
so a later networked backend fits behind it, and `storetest` holds every
backend to the same behavior.

## Interfaces

HTTP: `GET /` and `/app.js` (read-only web page), `GET /api/sessions`,
`GET /api/sessions/{id}`, `GET /api/turns/{id}`, `GET /api/edits`,
`GET /api/why`, `GET /api/history`, `GET /api/annotate`,
`GET /api/chunks/{sha}`, `GET /healthz`, `GET /version`,
`GET /webkit/`. There is no write endpoint. CLI: `moondb serve`, `record`,
`import`, `mcp`, `why <path>:<line>`, `history <path>`, `sessions`, `doctor`, `docs`,
`version`. MCP tools: `moondb_why`, `moondb_history`, `moondb_turn`,
`moondb_sessions`, `moondb_doctor`, all read-only. Config surfaces are
`--port` (`MOONDB_PORT`), `--workdir` (`MOONDB_WORKDIR`), `--base-url`
(`MOONDB_BASE_URL`, display-only), and `~/.config/moondb/config.toml` for
the capture settings.
