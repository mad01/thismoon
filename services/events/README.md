# events

A Go CLI, web service, and MCP server keeping a local event/audit log at
`http://events.this/` (port 7430). Tools emit tagged events (scan summaries,
blocked commits, sandbox denials); events records them in an append-only
per-source log and serves a filterable timeline. Archive-only by design: it
records and displays, it never fires notifications.

## How it works

One background process, `events serve`, owns the per-source JSONL store and
runs the web page and JSON API. The `events` CLI and the MCP tools are thin
HTTP clients to it; nothing else touches the files, so serve must be running
for anything else to work, and there's no lock contention.

Producers reach it three ways: `events emit` on the CLI, the `events_emit` MCP
tool, or a direct `POST /api/events` (what other tools in this repo use, since
they can't import this module's internal packages). The timeline itself
renders client-side: the page fetches `/api/events` and `/api/sources` and
live-tails every 7s for new activity.

## Install

```bash
make install   # builds and installs ~/code/bin/events (adhoc codesigned on macOS)
```

Or via ralph; it ships from the thismoon monorepo:

```bash
ralph up   # builds events and registers the t-man agent
```

## Usage

`events serve` runs as a launchd user agent via t-man:

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

Emit and query events:

```bash
events emit --source deps --title "3 advisories" --level warn \
  --tag repo=catalog --message "OSV flagged 3 packages"

events list --source deps --level warn --limit 20
events purge --source deps            # drop a whole source
events purge --source deps --before <id>   # or only events at/before a cursor
```

Levels are `info` (default), `warn`, and `error`. Other processes emit by
POSTing JSON to `http://events.this/api/events`. Purge is deliberately CLI +
API only; deletion stays human-triggered.

## Endpoints

| Path | Description |
|------|-------------|
| `GET /` | Client-rendered timeline (filters, day separators, burst coalescing, ~7s live tail) |
| `GET /api/events` | JSON array, newest first (`?source=&level=&q=&since=&limit=`) |
| `POST /api/events` | Emit one event; returns `{"id":"…"}` (201) |
| `DELETE /api/events?source=<s>[&before=<id>]` | Purge a source; returns `{"purged":n}` |
| `GET /api/sources` | Sources with counts |
| `GET /healthz` | 200 |
| `GET /version` | Build metadata: `version`, `commit`, `tag`, `build_time` |
| `GET /webkit/` | Shared chrome from the in-module `webkit` package |

## MCP

Ask in plain language: "what's happened with deps today", "any errors in the
last hour", "log that the release finished". Claude calls the `events_*` MCP
tools, registered in the consuming repo's `recipes/claude-mcp/servers.json`:

- `events_query`: query the log, newest-first; filter by source, level, text, or since; the primary tool for debugging what happened
- `events_sources`: list sources with their event counts
- `events_emit`: record a single event (`source` and `title` required)

Purge has no MCP tool; deletion stays CLI + API only, human-triggered.

## Where things live

- Events: one JSONL file per source under `~/.local/share/events/sources/<source>.jsonl`, oldest first
- Binary: `~/code/bin/events`
- Web + API: `http://events.this/` (or `http://localhost:7430/`)

Each source keeps its newest 500 events in memory (`--per-source-cap`); once a
source file grows past 1.5× that cap in lines, it's compacted in place (temp
file + rename). Event IDs are time-sortable, so lexical order is time order;
the ID doubles as the `since` cursor for polling. Removing the service doesn't
delete the JSONL files.

## Develop

```bash
make test           # go test ./...  (hermetic: t.TempDir + injected clock)
make build          # ./events
```

See [`CLAUDE.md`](CLAUDE.md) for the architecture: the store, the HTTP API, and
the MCP tools.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
