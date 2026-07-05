# events

A Go CLI, web service, and MCP server keeping a local event/audit log at
`http://events.this/` (port 7430).

Tools emit tagged events (scan summaries, blocked commits, sandbox denials);
events records them in an append-only per-source log and serves a filterable
timeline. Archive-only by design: it records and displays, it never fires
notifications.

## Install

```bash
make install   # builds and installs ~/code/bin/events (adhoc codesigned on macOS)
```

Or via ralph — it ships from the thismoon monorepo:

```bash
ralph up   # builds events and registers the t-man agent
```

## Run

`events serve` is the single writer: it owns the store and runs the HTTP API.
The CLI and MCP server are thin HTTP clients to it, so serve must be running
for anything else to work. It runs as a launchd user agent via t-man:

```bash
events serve --port 7430
```

```bash
t-man status events      # check the agent
t-man restart events     # restart after a binary rebuild
t-man logs events        # stdout logs
```

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `7430` | Listen port |
| `--workdir` | `~/.local/share/events` | Store location |
| `--base-url` | `$EVENTS_BASE_URL` | Serve URL the CLI/MCP clients talk to |
| `--per-source-cap` | `500` | Newest events kept per source (serve only) |
| `--global-cap` | `1500` | Max events one query returns (serve only) |

## Emit and query

```bash
events emit --source deps --title "3 advisories" --level warn \
  --tag repo=catalog --message "OSV flagged 3 packages"

events list --source deps --level warn --limit 20
events purge --source deps            # drop a whole source
events purge --source deps --before <id>   # or only events at/before a cursor
```

Levels are `info` (default), `warn`, and `error`. Other processes emit by
POSTing JSON to `http://events.this/api/events`. Purge is deliberately CLI +
API only — deletion stays human-triggered.

## Endpoints

| Path | Description |
|------|-------------|
| `GET /` | Client-rendered timeline (filters, day separators, burst coalescing, ~7s live tail) |
| `GET /api/events` | JSON array, newest first (`?source=&level=&q=&since=&limit=`) |
| `POST /api/events` | Emit one event; returns `{"id":"…"}` (201) |
| `DELETE /api/events?source=<s>[&before=<id>]` | Purge a source; returns `{"purged":n}` |
| `GET /api/sources` | Sources with counts |
| `GET /healthz` | 200 |
| `GET /version` | `{"version":"<sha>"}` build sha |
| `GET /webkit/` | Shared chrome from the in-module `webkit` package |

## MCP tools

`events mcp` exposes `events_query`, `events_sources`, and `events_emit` —
thin clients over the same API, aimed at agent-driven debugging.

## Store

One append-only JSONL file per source under
`~/.local/share/events/sources/<source>.jsonl`, oldest first. Each source keeps
its newest 500 events in memory; when a file grows past 1.5× that cap it is
compacted in place (temp file + rename). Event IDs are time-sortable, so
lexical order is time order — the ID doubles as the `since` cursor for polling.

## Develop

```bash
make test           # go test ./...  (hermetic: t.TempDir + injected clock)
make build          # ./events
```
