# events — CLI + MCP + web service for a local event/audit log

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all over
one append-only JSONL store. Producers (this repo's tools, Claude) **emit**
events; you browse the timeline at `http://events.this/` or query it from Claude.
**It is archive-only** — events are recorded, never fired. There is no ticker
and no notification.

## Single-writer architecture (load-bearing)

To avoid two processes racing on the JSONL files, **`events serve` is the only
writer**:

- **`events serve`** owns the store (per-source in-memory ring buffers guarded
  by a mutex, each persisted to `~/.local/share/events/sources/<source>.jsonl`).
  It runs the HTTP server (web timeline + JSON API + `/version` + `/webkit/`).
- **`events mcp`** and the **`events` CLI** hold no state — they are thin HTTP
  clients to the serve API on `localhost:<port>` (default **7430**). If serve is
  down, calls return "events serve not reachable … (t-man status events)".

So the MCP, the CLI, the web page, and any producer all funnel writes through
serve, and there are no file locks.

## Store layout

```
~/.local/share/events/
  sources/
    deps.jsonl        # one JSON event per line, oldest first
    reminder.jsonl
    sandbox-watch.jsonl
```

- Per-source **ring cap** (default 500): the in-memory slice keeps the newest
  500 events; appends past that drop the oldest.
- Each append is an O(1) line append. When a file grows past `cap * 1.5` lines it
  is **compacted** — rewritten atomically (temp + rename) from the capped
  in-memory slice. This is the "individually rotated" per-source log.
- **Global cap** (default 1500): the max events one query returns across sources.

## Event model

```go
Event{ ID, Time, Source, Component, Level, Title, Message, Tags, Data }
```

- **ID** is time-sortable (`"%020d-%04x"` of unix-nanos + 2 random bytes), so
  lexical order == time order. It is the cursor for `since` polling.
- **Time** is stamped server-side at ingest (UTC); producers don't set it.
- **Source** + **Title** are required. **Level** is `info` (default) | `warn` |
  `error`. `Component`, `Message`, `Tags`, `Data` (opaque JSON) are optional.

## Emitting

```bash
events emit --source deps --title "3 advisories" --level warn \
  --tag repo=catalog --message "OSV flagged 3 packages"
```

Or from Claude via the `events_emit` MCP tool. Other tools in this repo emit by
POSTing JSON to `http://events.this/api/events` (they can't import this module's
`internal/` packages).

## Purging

The log is append-only in normal operation, but junk happens (e.g. test events
leaked before the `testing.Testing()` guards existed). `events purge --source X
[--before <id>]` deletes events: without `--before` the whole source goes
(memory + JSONL file); with a cursor only events at/before it. Deliberately CLI
+ API only — no MCP tool, deletion stays human-triggered.

## Build / install / test

```bash
make build    # ./events binary (Version via ldflags)
make install  # build + cp to ~/code/bin/events + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir + injected clock, never touches real $HOME)
```

## HTTP API (owned by serve)

- `GET  /`              — webkit-chromed, client-rendered timeline (newest first, live-tails)
- `GET  /api/events[?source=&level=&q=&since=&limit=]` — JSON array, newest first
- `POST /api/events`    — emit one event (JSON body); returns `{"id":"…"}` (201)
- `DELETE /api/events?source=<s>[&before=<id>]` — purge a source (all, or only events with id <= cursor); returns `{"purged":n}`
- `GET  /api/sources`   — `[{"source":"…","count":n}]`
- `GET  /version`       — `{"version":"<sha>"}`
- `GET  /healthz`       — `200 ok`
- `GET  /webkit/`       — shared chrome

## MCP tools (thin client over the API)

- `events_query(source?, level?, q?, since?, limit?)` — newest-first; the primary debugging tool
- `events_sources()` — sources with counts
- `events_emit(source, title, level?, component?, message?, tags?)` — returns the new id

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-search>`, `<wk-seg>`, `<wk-page-header>`) come
from the in-module package **`github.com/mad01/thismoon/webkit`**, mounted at
`GET /webkit/` via `webkit.Mount(mux)`. The timeline is **client-rendered**: `index.html` fetches
`/api/events` + `/api/sources` and builds the DOM in JS (escaping all event text
via `textContent`/`createElement` — events come from untrusted producers), then
live-tails with `?since=<newestId>` every 7s. Don't re-add palette/topbar/theme
CSS locally — it lives in webkit only.

events is a webkit consumer like the other services in this repo. There is no
pin or bump step: the binary compiles against the webkit committed alongside
it, so a webkit change ships at the next build. Confirm with `GET /webkit/version`.

## Gotchas

- **Wave 0 builder.** Builds before `recipes/claude-mcp` (wave 1) registers the MCP.
- **serve must be running for the MCP/CLI/producers to work** — it owns the
  store. It runs as a t-man agent; `t-man status events` / `t-man restart events`.
- **MCP is unsandboxed** (first-party, only HTTP-calls localhost) — like
  `reminder`/`worklog`, no seatbelt wrapper.
- **Codesign for the binary.** `make install` strips xattrs and re-signs (macOS
  kills adhoc-signed binaries with drifted provenance).
