# keep architecture

## Overview

keep is the assertion store service: an agent session deposits a one-sentence
assertion about how a system behaves, pinned to evidence, and `keep check`
flips it stale when the pinned code changes. At runtime
`keep serve --port 7431` (loopback only, fronted by d-man as
`http://keep.this/`, run as a t-man agent) is the single writer: it owns the
store, resolves and hashes every evidence pin, and serves the web page, the
JSON API, and `/webkit/`. `keep mcp` and the mutating CLI commands hold no
state; both are thin HTTP clients to the serve API. The web page is read-only.

## Structure

```
cmd/keep/            entrypoint, delegates to internal/cli
internal/
  cli/               Cobra commands: serve, mcp, assert, list, get, check,
                     retract, version
  client/            HTTP client shared by the CLI and mcpserver
  store/             Assertion model and pure helpers (assertion.go), the
                     mutex-guarded JSONL store (store.go), ID minting (id.go)
  pin/               evidence pin model and working-tree hashing:
                     resolve, re-hash, compare
  server/            HTTP API + webkit web page (embedded shell.html + app.js)
  mcpserver/         MCP stdio setup (server.go) and the five tools (tools.go)
```

`internal/server` mounts the shared chrome with `webkit.Mount(mux)`; the page
is client-rendered from the JSON API per docs/adr/0005 and only lists
assertions with subject, kind, and status filters. Pin resolution lives in
serve because it is the process with a coherent view of the working trees the
pins point at.

## Data flow

Assert: `keep_assert` or `keep assert` sends `POST /api/assertions` through
`internal/client`. The server resolves each evidence pin with `internal/pin`:
it reads the named line range from the repo working tree, hashes the raw bytes
into `content_sha256`, records the HEAD commit, and derives the repo's
canonical `host/org/name` identity from its origin remote (empty when there is
none). The server also stamps an author into the provenance (`KEEP_AUTHOR`,
else the OS username), never trusting the request for it. An assertion with zero
pins, or a pin naming a missing file or an out-of-range span, is a 400; keep
never stores an assertion it could not ground. On success the store appends
the record with status fresh.

Check: `POST /api/check` (one id, or the whole store) re-reads each pin's line
range, re-hashes, and compares against the stored hash. A mismatch flips the
assertion stale with `stale_reason` naming the first failing pin; when every
pin matches again a stale assertion flips back fresh. Retracted assertions are
skipped. Query and get are `GET /api/assertions` (subject prefix, kind, and
status filters, newest first) and `GET /api/assertions/{id}`. Retract is
`POST /api/assertions/{id}/retract` with a counter-evidence note: terminal,
idempotent, and never re-checked. There is no un-retract and no removal path.

## Storage

The store is an append-only JSONL log under `~/.local/share/keep` (overridable
with `KEEP_WORKDIR`). Every mutation appends one complete record as one line;
load resolves the newest record per id (greater `updated_at` wins, a tie goes
to the later line), so history is never rewritten in place. Two files:
`assertions.jsonl` holds everything except machine-scoped records, and
`local.jsonl` holds records whose subject starts with `machine:` and never
leaves the machine. Routing happens at write time and subjects are immutable,
so a record never moves between files. The serve process polls the files every
two seconds and reloads the store when another process changed them on disk (a
git pull of a synced workdir, a manual append); the reload never repairs the
files and keeps the old data on any parse error, so serve remains the single
keep writer. A pre-JSONL `assertions.json` array is
migrated on startup and kept as `assertions.json.migrated`. Each record
carries the assertion (kind, subject, statement, confidence, provenance,
status, links) and its pins (repo path, file, line range, content hash, HEAD
commit, resolved-at).

## Interfaces

HTTP: `GET /` and `/app.js` (read-only web page), `GET /api/assertions`,
`POST /api/assertions`, `GET /api/assertions/{id}`,
`POST /api/assertions/{id}/retract`, `POST /api/check`, `GET /healthz`,
`GET /version`, `GET /webkit/`. CLI: `keep serve`, `mcp`, `assert` (with
repeatable `--pin repo_path:file:start-end`), `list`, `get`, `check [id]`,
`retract --note`, `version`. MCP tools: `keep_assert`, `keep_query`,
`keep_get`, `keep_check`, and `keep_retract`; responses include a `url`
pointing at the web page. Config surfaces are `--port` (`KEEP_PORT`),
`--workdir` (`KEEP_WORKDIR`), `KEEP_AUTHOR` for the provenance author stamp,
and `KEEP_BASE_URL` for the human-facing link.
