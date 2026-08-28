# events, CLI + MCP + web service for a local event/audit log

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all over
one append-only JSONL store. Producers (this repo's tools, and Claude) emit
events; you browse the timeline at `http://events.this/` or query it from
Claude. **It is archive-only**: events are recorded, never fired. There is no
ticker and no notification.

## Module layout

```
events/
  cmd/events/          - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, emit/list/purge (manage.go), mcp, version
    event/             - Event model: validation, source sanitization, time-sortable ID (no I/O)
    store/             - per-source in-memory ring buffers + JSONL persistence (single writer)
    server/            - HTTP API + webkit web page (embedded index.html)
    client/            - HTTP client for serve (used by CLI + MCP)
    mcpserver/         - MCP tools (thin client over the API)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

To avoid two processes racing on the JSONL files, **`events serve` is the only
writer**:

- `events serve` owns the store (per-source in-memory ring buffers guarded by a
  mutex, each persisted to `~/.local/share/events/sources/<source>.jsonl`). It
  runs the HTTP server (web timeline + JSON API + `/version` + `/webkit/`).
- `events mcp` and the `events` CLI hold no state: they are thin HTTP clients
  to the serve API on `localhost:<port>` (default `7430`). If serve is down,
  calls return "events serve not reachable … (t-man status events)".

The MCP and the CLI never touch the store directly, and neither does the web
page or any other producer. Every write funnels through serve, so there are no
file locks.

Producers reach serve three ways: the `events` CLI (`events emit`), the
`events_emit` MCP tool, or a direct `POST /api/events`, the path other tools
in this repo use, since they can't import this module's `internal/` packages
across component boundaries.

## Data model & storage

### Store layout

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
  is compacted: rewritten atomically (temp + rename) from the capped in-memory
  slice. This is the "individually rotated" per-source log.
- **Global cap** (default 1500): the max events one query returns across sources.

### Event model

```go
Event{ ID, Time, Source, Component, Level, Title, Message, Tags, Data }
```

- `ID` is time-sortable (`"%020d-%04x"` of unix-nanos + 2 random bytes), so
  lexical order == time order. It is the cursor for `since` polling.
- `Time` is stamped server-side at ingest (UTC); producers don't set it.
- `Source` and `Title` are required. `Level` is `info` (default), `warn`, or
  `error`. `Component`, `Message`, `Tags`, `Data` (opaque JSON) are optional.

## Build / install / test

```bash
make build    # ./events binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/events + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir + injected clock, never touches real $HOME)
```

## HTTP API

- `GET  /`              : webkit-chromed, client-rendered timeline (newest-first; live-tailing; infinite scroll)
- `GET  /api/events[?source=&level=&q=&since=&before=&limit=]` : JSON array, newest-first (`since`/`before` are exclusive ID cursors: newer-than for tailing, older-than for paging)
- `POST /api/events`    : emit one event (JSON body); returns `{"id":"…"}` (201)
- `DELETE /api/events?source=<s>[&before=<id>]` : purge a source (all, or only events with id <= cursor); returns `{"purged":n}`
- `GET  /api/sources`   : `[{"source":"…","count":n}]`
- `GET  /version`       : the four-key build metadata object (`version`, `commit`, `tag`, `build_time`)
- `GET  /healthz`       : `200 ok`
- `GET  /webkit/`       : shared chrome

## Commands

CLI surface beyond `serve`/`mcp`: thin HTTP clients to `events serve`, one call
per HTTP endpoint above:

```bash
events emit --source deps --title "3 advisories" --level warn \
  --tag repo=catalog --message "OSV flagged 3 packages"

events list --source deps --level warn --limit 20
events purge --source deps                  # drop a whole source
events purge --source deps --before <id>    # or only events at/before a cursor
events version [-o json]                    # bare git sha; -o json prints the full build metadata object
```

The log is append-only in normal operation, but junk happens (e.g. test events
leaked before the `testing.Testing()` guards existed). `events purge --source X
[--before <id>]` deletes events: without `--before` the whole source goes
(memory + JSONL file); with a cursor only events at/before it. Deliberately CLI
+ API only; no MCP tool, deletion stays human-triggered.

## MCP tools

- `events_query(source?, level?, q?, since?, limit?)` → `{events: Event[], url}`:
  newest-first; the primary tool for debugging what happened. `q` is a
  case-insensitive substring matched against title, message, component, or
  tags; `since` is an event id (exclusive), for polling new activity. Call
  `events_sources` first to see which sources exist.
- `events_sources()` → `{sources: [{source, count}], url}`: every source with
  its current event count, sorted by name.
- `events_emit(source, title, level?, component?, message?, tags?)` →
  `{id, url}`: records an event; `source` and `title` are required.
- No `events_purge` tool. Purge is deliberately CLI + API only, so deletion
  stays human-triggered.

Every response carries `url`: the configured `--base-url` (typically
`http://events.this/`), falling back to `http://localhost:<port>`, a link the
human can open to see the timeline.

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-search>`, `<wk-seg>`, `<wk-page-header>`) come
from the in-module package **`github.com/mad01/thismoon/webkit`**, mounted at
`GET /webkit/` via `webkit.Mount(mux)`. events is a webkit consumer like the
other services in this repo. There is no pin or bump step: the binary compiles
against the webkit committed alongside it, so a webkit change ships at the next
build.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // mux.Handle("GET /webkit/", webkit.Handler()) - serves webkit.css/.js + /webkit/boot.js + /webkit/version
```

`internal/server/server.go`'s `Handler()` calls `webkit.Mount(mux)` alongside
the events routes.

### Header markup (events)

```html
<wk-header brand="events" title="Events"></wk-header>
```

`webkit.js` injects the full control set (font · fixation · size ± · reload ·
theme). Don't add those controls manually.

### Per-repo changes

- The timeline is client-rendered and paged: `index.html` fetches
  `/api/events` in 200-event pages, appends older pages with
  `?before=<oldestLoadedId>` as a sentinel at the bottom scrolls into view,
  and live-tails with `?since=<newestId>` every 7s. Active filters go to the
  API (one request per selected source chip, merged client-side) so matches
  older than the loaded window still surface; a filter change resets the
  window. All event text is set via `textContent`/`createElement`, never
  `innerHTML`; events come from untrusted producers.
- A run of 3+ consecutive same-source events within a 60s window collapses into
  one expandable burst card. Filters (`q`, `level`, active source chips) live
  in the URL, so a filtered view can be reloaded or bookmarked.
- events was already client-side before this became the repo-wide convention
  (see `docs/adr/0005-webkit-client-side-rendering.md`). Don't re-add
  palette/topbar/theme CSS locally; it lives in webkit only.

### Version check

`GET /webkit/version` → `{"module":"github.com/mad01/thismoon/webkit","version":"<hash>"}`:
confirms which embedded webkit assets the running events server serves.

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe (wave 1) registers the MCP.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `events docs`): serve-must-be-running, store layout, failure
  modes, version-skew checks. Keep those facts there, not here.
- **MCP is unsandboxed**: it's first-party code that only makes HTTP calls to
  localhost, so it runs without a seatbelt wrapper, same as `reminder` and
  `worklog`.
- **Codesign for the binary.** `make install` strips xattrs and re-signs (macOS
  kills adhoc-signed binaries with drifted provenance).
- **Version probe.** `GET /version` and `events version -o json` both return
  the shared four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`, every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, the cross-tool convention `ralph` and
  `status` use to probe the build a sibling tool is running. Plain
  `events version` stays a bare token.

## See also

- Recipe: `recipes/events/recipe.toml` (+ `recipes/events/CLAUDE.md`)
- Route: the consuming repo's `recipes/d-man/routes.toml` overlay (`events` → 7430; docs/adr/0006)
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json` (entry `events`, command `events mcp`)
- ADR: `docs/adr/0005-webkit-client-side-rendering.md` (events was already client-side)
