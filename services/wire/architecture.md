# wire architecture

## Overview

wire is the session-to-session message bus: one session opens a named channel,
hands the name to another, and both post to it and read from it, with reads
that can block until the next message arrives. At runtime
`wire serve --port 7432` (loopback only, fronted by d-man as
`http://wire.this/`, run as a t-man agent) is the single writer: it owns the
store, holds every blocking reader, and serves the web page, the JSON API, the
event stream, and `/webkit/`. `wire mcp` and the CLI hold no state; both are
thin HTTP clients to the serve API. The web page is read-only.

## Structure

```
cmd/wire/            entrypoint, delegates to internal/cli
internal/
  cli/               Cobra commands: serve, mcp, open, list, post, read,
                     follow, close, version
  client/            HTTP client shared by the CLI and mcpserver
  ref/               connection string: minted by the server, parsed by the
                     client, so the format has one definition
  store/             Channel/Message model and pure helpers (channel.go), the
                     mutex-guarded store and the Wait primitive (store.go),
                     JSONL scan/append with torn-line repair (jsonl.go),
                     id minting (id.go)
  server/            HTTP API, SSE stream, webkit web page (embedded
                     shell.html + app.js)
  mcpserver/         MCP stdio setup (server.go) and the five tools (tools.go)
```

`internal/server` mounts the shared chrome with `webkit.Mount(mux)`; the page is
client-rendered from the JSON API per docs/adr/0005, with two views switched by
the `?channel=` query param.

## Data flow

Open: `wire_open` or `wire open` sends `POST /api/channels`. The store
normalizes the name to a lowercase slug (or generates one), rejects a name
already in use, mints a `ch_`-prefixed id, and appends the channel record. Ids
and names occupy disjoint spaces, because the name grammar excludes the `_`
that an id always contains, so every later call takes a single `ref` that
resolves either. The server stamps a `connect` field onto every channel
response — `wire://localhost:<port>/<name>`, built by `internal/ref` from the
port serve was started on — and that token is what one session hands to
another. The client reduces it back to a channel name before any request goes
out, because its slashes would otherwise escape into a path segment no route
matches; so the server only ever mints and the client only ever parses.

Post: `POST /api/channels/{ref}/messages` validates that the message is signed
and non-empty, assigns the next sequence number (1-based, contiguous), appends
one line to that channel's message log, and closes the channel's broadcast
channel to wake every waiter.

Read: `GET /api/channels/{ref}/messages?since=&limit=&wait=` slices the
messages after the cursor. Because sequence numbers are contiguous, the cursor
is the index, so there is no scan. With `wait` set the request parks in
`store.Wait`, which returns as soon as a message lands past the cursor, or
immediately when the channel is closed, the wait expires (an empty batch, not
an error), or the client disconnects. The server caps a wait at 120 seconds.

Stream: `GET /api/channels/{ref}/stream` is the same primitive as SSE — a loop
of `Wait(25s)` emitting one `message` event per message with its sequence as the
SSE id, a `: keepalive` comment on each quiet turn, and a final `closed` event
when the conversation ends. Because the id is the sequence, a browser reconnect
resumes exactly where it stopped via `Last-Event-ID`.

Close: `POST /api/channels/{ref}/close` marks the channel closed with an
optional note, appends the updated record, and wakes every waiter so none of
them blocks for a reply that cannot come. It is terminal and idempotent; a
repeat close keeps the original note. Posting afterwards is a 409, reading
still works.

## Storage

Append-only JSONL under `~/.local/share/wire` (overridable with
`WIRE_WORKDIR`). `channels.jsonl` holds one line per channel change and load
keeps the newest record per id (greater `updated_at` wins, a tie goes to the
later line). `messages/<channel-id>.jsonl` holds one line per message; messages
are immutable, so file order is sequence order and no resolution is needed —
and one conversation's reads never touch another's traffic. A torn final line,
the artifact of an append interrupted by a crash, is dropped and truncated away
at startup; a mid-file parse error is fatal. Repair happens only in
`store.New`, before serve begins writing, so the running server never rewrites
a file underneath itself. Everything is held in memory as well: an index of
channels by id and by name, and the message slices the waiters read from.

## Interfaces

HTTP: `GET /` and `/app.js` (read-only web page), `GET /api/channels`,
`POST /api/channels`, `GET /api/channels/{ref}`,
`POST /api/channels/{ref}/close`, `GET|POST /api/channels/{ref}/messages`,
`GET /api/channels/{ref}/stream`, `GET /healthz`, `GET /version`,
`GET /webkit/`. The server runs without a `WriteTimeout` because long polls and
event streams hold connections open deliberately. CLI: `wire serve`, `mcp`,
`open`, `list`, `post`, `read`, `follow`, `close`, `version`. MCP tools:
`wire_open`, `wire_post`, `wire_read`, `wire_list`, and `wire_close`; responses
include a `url` pointing at the channel's page, and `wire_read` reports whether
the channel is closed so a waiting session knows to stop. Config surfaces are
`--port` (`WIRE_PORT`), `--workdir` (`WIRE_WORKDIR`), `--from` (`WIRE_FROM`) for
the CLI's sender name, and `WIRE_BASE_URL` for the human-facing link. `--port`
also fixes the endpoint in every connection string the instance mints.
