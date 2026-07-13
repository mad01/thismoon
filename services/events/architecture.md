# events architecture

## Overview

events is the local event log service: producers record what happened on the
machine and humans and agents read one filterable timeline. At runtime
`events serve --port 7430` (loopback only, fronted by d-man as
`http://events.this/`, run as a t-man agent) is the single writer: it owns the
per-source store and serves the timeline, the JSON API, and `/webkit/`. The
`events` CLI and `events mcp` hold no state; both are thin HTTP clients to the
serve API. events is archive-only: it records and displays, never fires a
notification, and has no ticker.

## Structure

```
cmd/events/          entrypoint, delegates to internal/cli
internal/
  cli/               Cobra commands: serve, emit, list, purge, mcp, version
  event/             Event model: validation, source sanitization,
                     time-sortable ID minting (no I/O)
  store/             per-source in-memory ring buffers + JSONL persistence
  server/            HTTP API + webkit web page (embedded index.html)
  client/            HTTP client used by the CLI and mcpserver
  mcpserver/         MCP stdio tools, thin client over the API
```

`internal/server` mounts the shared chrome with `webkit.Mount(mux)`. The
timeline is client-rendered: `index.html` fetches `/api/events` and
`/api/sources`, builds the DOM in the browser, and live-tails with
`?since=<newestId>` every 7 seconds (docs/adr/0005; events was client-side
before that convention landed).

## Data flow

The emit path has three entrances that converge on one handler: `events emit`
(CLI), the `events_emit` MCP tool, and a direct `POST /api/events`, which is
how sibling components in this repo emit since they cannot import each other's
internal Go packages. The server validates the payload through
`internal/event` (source and title required, source sanitized, level one of
info/warn/error), stamps `Time` server-side in UTC, and mints a time-sortable
ID (zero-padded unix nanos plus two random bytes) so lexical order is time
order. The store then appends the event to that source's mutex-guarded ring
buffer and appends one line to the source's JSONL file.

Reads run the same path in reverse: `GET /api/events` filters by source,
level, substring, and `since` cursor and returns newest first; `events_query`
and `events list` call it over HTTP. The `since` cursor is an event ID, which
makes polling for new activity a lexical comparison. Purge is the one
subtractive path: `DELETE /api/events?source=` drops a whole source or only
events at or before a cursor, and it is deliberately reachable from the CLI
and HTTP API only, never as an MCP tool.

## Storage

The store lives under `~/.local/share/events/` (overridable with
`EVENTS_WORKDIR`) as one JSONL file per source:
`sources/<source>.jsonl`, one JSON event per line, oldest first. Each source
keeps the newest 500 events in its in-memory ring (appends past the cap drop
the oldest); an append is an O(1) line write, and when a file outgrows 1.5x
the cap it is compacted, rewritten atomically (temp file, then rename) from
the capped in-memory slice. A global cap (default 1500) bounds how many
events one query returns across sources. The files are plain JSONL and
outlive the service if it is removed.

## Interfaces

HTTP: `GET /` (timeline), `GET /api/events` with `source`/`level`/`q`/
`since`/`limit` filters, `POST /api/events`, `DELETE /api/events`,
`GET /api/sources`, `GET /healthz`, `GET /version`, `GET /webkit/`. CLI:
`events serve`, `emit`, `list`, `purge`, `mcp`, `version`. MCP tools:
`events_query`, `events_sources`, and `events_emit`; every response carries a
`url` the human can open. There is no purge tool, so an agent can record
history but not erase it. Config surfaces are flags and their environment
mirrors: `--port` (`EVENTS_PORT`), `--base-url` (`EVENTS_BASE_URL`), and
`--workdir` (`EVENTS_WORKDIR`).
