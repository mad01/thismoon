# wire, session-to-session message bus

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all over
one append-only JSONL store. A session opens a **channel**, hands the
**connection string** to another session, and the two exchange **messages**
over it. A read can block
until the next message arrives, so waiting for a peer's reply costs one call
instead of a polling loop. **The MCP tools (driven by Claude) are the primary
surface**; the `wire` CLI mirrors them. The web page at `http://wire.this/` is a
**read-only** live view of the same transcripts.

## Module layout

```
wire/
  cmd/wire/            - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, mcp, manage (open/list/post/read/follow/close), version (Version via ldflags)
    client/            - HTTP client shared by the CLI and mcpserver (client.go)
    ref/               - the connection string: mint it (server) and parse it (client)
    store/             - Channel/Message model + pure helpers (channel.go), the
                         mutex-guarded store with the wait primitive (store.go),
                         JSONL scan/append (jsonl.go), id minting (id.go)
    server/            - HTTP API + SSE stream + webkit web page (embedded shell.html + app.js)
    mcpserver/         - MCP tools (server.go = MCP server setup, tools.go = 7 tools)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

Messages arrive from several places (two or more MCP sessions, the CLI, a
human), so to avoid processes racing on the log files, **`wire serve` is the
only writer**:

- **`wire serve`** owns the store (in-memory index guarded by a mutex,
  persisted to JSONL under `~/.local/share/wire/`). It runs the HTTP server
  (web page + JSON API + event stream + `/version` + `/webkit/`).
- **`wire mcp`** holds no state: it is a thin HTTP client to the serve API on
  `localhost:<port>`. If serve is down, tools return "wire serve not reachable
  … (t-man status wire)".
- **`wire` CLI** commands are the same thin HTTP client.

serve is also the only place a **blocking read can park**. A session waiting for
a reply is a goroutine inside serve, so the wait has to live where the store
does — that is the second reason the architecture is shaped this way, on top of
the single-writer one.

### The connection string (the whole join protocol)

Opening a channel returns one token:

```
wire://localhost:7432/refactor-auth
```

That string is what a session hands to another session, and it is accepted
anywhere a channel is named — every CLI command, every MCP tool's `channel`
argument, and the web page's `?channel=` param. There is nothing else to
explain and nothing else to copy.

`internal/ref` owns the format so it cannot drift: the **server mints** it
(`ref.String`, which is why `server.New` takes the port — the store has no idea
what port it is served on), and the **client parses** it (`ref.Parse`, called
from `channelPath` before any request goes out). The client has to reduce it
locally: a `wire://…` in a URL path would escape its slashes into a segment no
route matches, so the server itself only ever emits.

A reference that is not a URL passes through as an id or name. A reference that
is a URL with the wrong scheme is an error rather than a silent fallthrough —
otherwise a mistyped `http://` becomes a nonsense channel name and fails much
later, somewhere unhelpful.

The endpoint in the string is **advisory**. wire binds loopback only, so a
client connects wherever it is configured (`WIRE_PORT` / `--port`), not where
the string points. The endpoint is carried anyway because it is what a
cross-machine wire would need, and `ref.Parse` is the single place that would
have to start honoring it.

### Waiting (the reason this is not just a file)

`store.Wait` is the primitive everything blocking is built on. It returns as
soon as there is a message past the caller's cursor, and immediately when the
channel is closed, the timeout expires, or the request context is cancelled.
Waiters are woken through one broadcast channel per conversation, closed and
replaced on every post or close.

Three surfaces sit on it:

| Surface | Call | Behavior |
|---------|------|----------|
| Plain read | `Wait(timeout=0)` | returns whatever is there now |
| Long poll | `GET …/messages?wait=N` | blocks up to N seconds (server caps at 120) |
| Event stream | `GET …/stream` | loops `Wait(25s)`, emitting SSE frames and a keepalive comment on each quiet turn |

An expired wait is a normal empty batch, never an error: the caller reads again
or gives up. **A closed channel never blocks** — otherwise a session would wait
forever for a reply that cannot come, which is the failure mode the whole
close-is-terminal rule exists to prevent.

## Data model & storage

```go
Channel{
  ID,          // "ch_" + 12 hex; disjoint from names by construction
  Name,        // the handle passed between sessions; lowercase slug, unique
  Topic,       // optional one-line description
  OpenedBy,    // who opened it
  Conventions, // optional ground rules, declared at open — on the channel, not
               // in message #1, so a session joining mid-transcript sees them
  CreatedAt,
  UpdatedAt,   // when the RECORD changed (open/close), not last activity
  ClosedAt,    // nil while open
  CloseNote,
}

Message{
  ChannelID,
  Seq,         // 1-based, contiguous; ChannelID+Seq is the identity AND the cursor
  From,        // required — an unsigned message cannot be answered
  To,          // optional; roster name this message is addressed to, empty = everyone
  Body,        // required, max 64 KiB
  Kind,        // optional; closed set: task, result, question, answer, ack, note, join, leave
  ReplyTo,     // optional; seq this message answers — must exist on the channel
  ReplyNeeded, // optional; sender expects an answer
  CreatedAt,
}
```

**Seq is arrival order, ReplyTo is causal order — the two are independent.**
Seq is the authoritative total order the server assigns; ReplyTo is the
correlation a writer declares. When two sessions post concurrently their
messages interleave in seq, and ReplyTo is what keeps the transcript
followable. The kind set is server-owned and closed on purpose: it is the
vocabulary cross-channel tooling can rely on, and channel-specific tags belong
in the body or the channel's `Conventions`, not in new kinds. `note` is the
kind for substance that answers nothing and demands nothing (intros, status,
findings), so `ack` keeps its one machine-checkable meaning: pure receipt,
safe to skip. `join` and `leave` are the membership events; the dedicated
join/leave operations post them, but they are ordinary messages and can be
posted directly.

**A channel takes any number of sessions, and addressing is what makes that
work.** At two participants "not me" means "you", so an open obligation has an
obvious owner. Beyond two it belongs to nobody, which is why a question or
task meant for a specific agent carries its roster name in `To`. Protocol
conventions that hold up at any width: one question per message (a single
reply clears the whole seq from `awaiting_reply`, answered fully or not);
one `reply_to` per obligation; `reply_needed` only when blocked — its value is
that the debt survives in the record, not that it hurries anyone.

Derived at read time, never stored (so none of it can drift from the
transcript):

- `awaiting_reply` (Summary and every read Batch): every seq posted with
  ReplyNeeded that no later message names in ReplyTo — the machine answer to
  "what is still owed".
- `awaiting_reply_by`: the addressed subset of `awaiting_reply`, grouped by
  `To` — each agent's own debts under its name. Unaddressed obligations stay
  in the flat list only; they belong to whoever picks them up.
- `members`: the roster — the opener plus everyone whose latest join/leave
  message is a join. Distinct from `participants` (who has spoken): a member
  may be silently reading, a participant may never have joined.
- `awaiting_reply_off_roster`: the `awaiting_reply_by` keys not present in
  `members` — open obligations addressed to a name nobody currently answers
  to. An agent must not settle by `awaiting_reply_by` alone: a question sent
  to a typo of its name, or to a member that has since left, files under a
  key it would never check, and this list is the only place that shows up.
  The signal is advisory and never classifies: a pre-join handoff, a typo,
  and an obligation stranded by a leave look identical here, only the first
  resolves itself (on the join), and judging which is which belongs to a
  reader, never the server. `checkTo` skips roster validation at post time
  for the same reason: the normal handoff addresses an agent before it joins.

- **Names** match `^[a-z0-9][a-z0-9.-]{0,63}$` and are lowercased on the way in.
  The `_` character is excluded on purpose: ids start with `ch_`, so an id can
  never collide with a name and a single `ref` resolves either.
- **Seq is the cursor.** Sequence numbers are contiguous from 1, so
  "everything after N" is `messages[N:]` — no scanning, no timestamps, no
  ambiguity about what a client has already seen.
- **`Summary`** (message count, cursor, participants, last line) is derived from
  the messages at read time and never stored. Participants come from the
  distinct senders, so there is no membership record to keep in sync.

Persisted as append-only JSONL under `~/.local/share/wire` (overridable with
`WIRE_WORKDIR`):

- `channels.jsonl` — one line per channel change; load keeps the newest record
  per id (greater `updated_at` wins, a tie goes to the later line).
- `messages/<channel-id>.jsonl` — one line per message, immutable. File order
  is sequence order, so no newest-wins resolution is needed and reading one
  channel never touches another's traffic.

A torn final line (an append interrupted by a crash) is dropped and truncated
away at startup; a mid-file parse error is fatal. Repair only ever happens in
`store.New`, before serve starts writing.

### State transitions

| From | Event | To | Notes |
|------|-------|-----|-------|
| (none) | `open` | open | name generated when omitted; a taken name is a 409 |
| open | `post` | open | seq assigned, waiters woken |
| open | `join(from)` | open | posts a `join` message unless already on the roster; waiters woken, so a session waiting for its peer wakes on the join |
| open | `leave(from)` | open | posts a `leave` message unless already gone |
| open | `close(note)` | closed | terminal; waiters woken so none blocks forever |
| closed | `post`/`join`/`leave` | — | rejected (409) |
| closed | `close(note)` | closed | idempotent no-op; the original note stands |

There is no reopen and no delete. A closed channel stays readable — the
transcript is the point.

## Build / install / test

```bash
make build    # ./wire binary (Version via ldflags)
make install  # build + cp to ~/code/bin/wire + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir, never touches the real store)
```

## HTTP API

Owned by `wire serve`:

- `GET  /`                                   : webkit-chromed web page (list + live transcript)
- `GET  /app.js`                             : the page's client script
- `GET  /api/channels?all=1`                 : list, most recently active first; `all` includes closed
- `POST /api/channels`                       : body `{name?, topic?, from?, conventions?}` → the channel (409 on a taken name)
- `GET  /api/channels/{ref}`                 : one channel by id or name, with derived counts
- `POST /api/channels/{ref}/join`            : body `{from, note?}` → the summary (the joiner's one-call briefing); idempotent
- `POST /api/channels/{ref}/leave`           : body `{from, note?}` → the summary; idempotent
- `POST /api/channels/{ref}/close`           : body `{note?}`; terminal
- `GET  /api/channels/{ref}/messages`        : `?since=&limit=&wait=` → `{channel, messages, cursor, members, awaiting_reply, awaiting_reply_by}`
- `POST /api/channels/{ref}/messages`        : body `{from, to?, body, kind?, reply_to?, reply_needed?}` → the message
- `GET  /api/channels/{ref}/stream`          : `?since=` → SSE; `message` events carry the seq as the SSE id, honors `Last-Event-ID`
- `GET  /healthz`                            : 204
- `GET  /version`                            → `{"version":"<sha>"}`
- `GET  /webkit/`                            : shared chrome (asset drift checks use `GET /webkit/version`)

`{ref}` is a channel id or name throughout — the client reduces a connection
string before it ever reaches a path. Every channel-shaped response carries a
`connect` field with the channel's connection string, stamped on by the server
(`channelView` wraps `store.Summary`, which cannot build one itself).
`serve` runs with no `WriteTimeout`
on purpose — a long poll and an event stream both hold a connection open by
design, and a write deadline would sever them.

## Commands

CLI surface beyond `serve`/`mcp`, wired as thin HTTP clients to `wire serve`
(`internal/client`):

```bash
wire serve --port 7432 --workdir ~/.local/share/wire
wire mcp
wire open [name] [--topic <text>] [--from <who>] [--conventions <rules>]  # prints the connection string first
wire join <ref> [--from <who>] [--note <intro>]   # get on the roster, print the briefing
wire leave <ref> [--from <who>] [--note <why>]    # step off the roster; the channel continues
wire list [--all]
wire connect <ref>                                # print just the connection string
wire post <ref> [message] [--kind <k>] [--to <who>] [--reply-to <seq>] [--reply-needed]  # body from args, else stdin
wire read <ref> [--since <n>] [--wait <secs>] [--limit <n>]
wire follow <ref> [--since <n>]                   # blocking reads in a loop until closed
wire close <ref> [--note <why>]
wire version [-o json]
```

`<ref>` is a channel name, an id, or a connection string.

`--from` is persistent and defaults to `WIRE_FROM`, else the OS username.
`-o json` on the mutating and reading commands prints the raw API record.
`wire follow` carries the interrupt context into the blocking read, so Ctrl-C
lands immediately instead of after the current 60-second wait.

## MCP tools

Thin client over the API above (`internal/client`), served on stdio by
`wire mcp`:

- `wire_open(from, name?, topic?, conventions?)`: open a channel; returns `connect`, the token to pass to the other sessions; the opener is on the roster
- `wire_join(channel, from, note?)`: get on the roster and get the briefing back — conventions, members, cursor, open obligations; idempotent
- `wire_leave(channel, from, note?)`: step off the roster; the conversation continues without you
- `wire_post(channel, from, body, to?, kind?, reply_to?, reply_needed?)`: append a message; returns its `seq`
- `wire_read(channel, since?, wait?, limit?)`: messages after the cursor; `wait` blocks up to 120s; reports `members`, `awaiting_reply`, `awaiting_reply_by`, and `awaiting_reply_off_roster`
- `wire_list(include_closed?)`: channels, most recently active first
- `wire_close(channel, note?)`: terminal close that wakes every waiter — everyone's end, unlike wire_leave

Tool responses include `connect` (the connection string to hand on) and `url`
(the human-facing `WIRE_BASE_URL`, e.g.
`http://wire.this/?channel=<name>`), while the client itself calls
`localhost:<WIRE_PORT>`. `wire_read` also returns `closed`, which a waiting
session must check — a closed channel will never produce another message.
Handlers pass the MCP request context into the HTTP call, so an abandoned tool
call stops waiting straight away.

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-page-header>`, `<wk-title>`, `<wk-callout>`)
come from the in-module package **`github.com/mad01/thismoon/webkit`**, mounted
at `GET /webkit/` via `webkit.Mount(mux)` and loaded by
`internal/server/shell.html` (which pulls the FOUC guard from
`/webkit/boot.js`). Don't re-add palette/topbar/theme CSS locally; it lives in
webkit only.

wire's web page is **read-only** and has two views, switched by the `?channel=`
query param: the channel list, and one channel's transcript following the event
stream with a live/reconnecting indicator. Posting, opening, and closing all go
through the MCP or the CLI. wire is a webkit consumer like the other services in
this repo: no pin or bump step, so a webkit change ships at the next build.

```bash
make install && t-man restart wire
```

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe
  (wave 1) registers the MCP.
- **serve must be running for the MCP/CLI to work**: it owns the store and is
  where blocking reads park. It runs as a t-man agent; `t-man status wire` /
  `t-man restart wire`.
- **Never put `_` in a channel name.** The grammar rejects it, and that
  exclusion is what keeps ids (`ch_…`) and names in disjoint spaces so one
  `ref` parameter resolves both.
- **The connection string's port comes from `--port`.** Serve on a non-default
  port and the tokens it mints name that port, which is correct but surprising
  if you were copying between two instances.
- **Close, don't abandon.** A session blocked on a read of a forgotten channel
  waits out its full timeout every call. Closing wakes waiters immediately and
  tells them not to come back.
- **Don't add a WriteTimeout to serve.** It would cut off long polls and event
  streams, which are supposed to hold a connection open.
- **Codesign for the binary.** `make install` strips xattrs and re-signs (macOS
  kills adhoc-signed binaries with drifted provenance).
- **Version probe convention.** `GET /version` and `wire version -o json` both
  return `{"version":"<sha>"}` so ralph can check which build is live.

## See also

- Recipe: `recipes/wire/recipe.toml` (+ `recipes/wire/CLAUDE.md`)
- Human docs: `README.md`
- The `wire.this` d-man route and the `claude-mcp` `servers.json` MCP
  registration live in the consuming repo's private overlay (docs/adr/0006),
  not here.
