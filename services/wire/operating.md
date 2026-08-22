# operating wire

wire is a message bus between agent sessions. One session opens a channel and
gets back a connection string (wire://localhost:7432/<name>); it hands that
token to another session, and the token works anywhere a channel is named:
every CLI command, every MCP tool's `channel` argument, the web page's
`?channel=` param. A read can block server-side until the next message lands,
so waiting for a peer's reply costs one call instead of a polling loop.

## how it runs

A single `wire serve` process is the only writer. It owns the store, assigns
every message its sequence number, and serves the JSON API plus a read-only
web page on localhost (default {{.BaseURL}}). It is also where a blocking
read parks: a waiting session is a goroutine inside serve. Everything else is
a thin HTTP client over that API: the CLI commands and the MCP server, a
stdio shim spawned as `wire mcp`. The shim being up says nothing about the
service: every tool call is a live HTTP request to serve, and it fails when
serve is down. Machines usually also route http://wire.this to serve via the
local domain front door; if the localhost port answers but the .this host
does not, the router is the problem, not this service.

## where state lives

Append-only JSONL under {{.StorePath}} (override with WIRE_WORKDIR).
`channels.jsonl` holds one line per channel change; on load the newest record
per id wins. `messages/<channel-id>.jsonl` holds one immutable line per
message, so file order is sequence order. Channels and transcripts survive a
serve restart. Parked reads do not: each waiter was a goroutine in the old
process.

## failure modes

Start with `wire doctor`: one command runs the reachability, store, and
version-skew checks below, one line per check, FAIL lines naming the cause.
The paragraphs here cover what each failure means and what to do next.

Connection refused, or "wire serve not reachable": serve is not running. t-man
typically supervises it; run `t-man list` to see whether the wire agent
exists, then `t-man restart wire`. A session blocked in wire_read does not
hang through this: its connection dies with serve and the call returns a
transport error. Once serve is back, re-issue the read with the same cursor;
nothing on disk was lost.

Read returns no messages: usually not an error; three cases to tell apart.
The channel may be closed: check `closed` on the tool result. A closed
channel never blocks and never produces another message, so stop waiting on
it; the transcript stays readable. There may be nothing new yet: an expired
`wait` is a normal empty batch, so read again with `since` set to the
returned `cursor`. Or the cursor is wrong: `since` means "I have seen
everything up to seq N", so a too-large value skips messages. The server
clamps an overshot cursor; trust the `cursor` it returns.

Two sessions not seeing each other: they are on different channels. Names
are lowercased on the way in, so paste the connection string exactly instead
of retyping it, and compare against `wire list --all`. The endpoint inside
the string is advisory: a client connects to its own configured port
(WIRE_PORT or --port), so two serves on different ports have disjoint stores.

Waiters piling up on a finished conversation: close, do not abandon. A
forgotten open channel makes every blocked reader wait out its full timeout;
`wire close <ref>` wakes them all immediately and marks the end.

## version skew

`wire version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: `t-man restart wire` and compare again. `wire doctor` runs this
comparison as its version-skew check. A restart drops parked reads (they
return errors and resume from their cursor) but no channel or message, so
restarting under live channels is safe.

## first moves

1. `wire doctor`: serve reachable, store readable, and version skew in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart wire`
3. On version skew: `t-man restart wire`, then `wire doctor` again
4. `wire list --all`, to confirm the store loads and see which channels are open
5. `wire read <ref> --since 0`, to dump a suspect channel's full transcript
