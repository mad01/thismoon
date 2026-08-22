# operating events

events is the local event/audit log: producers record what happened on the
machine, and one filterable timeline serves the human and the agent. It is
archive-only: events are recorded, never fired, so there is no ticker and no
notification.

## how it runs

A single `events serve` process is the only writer. It owns the store and
serves the JSON API plus the web timeline on localhost (default
{{.BaseURL}}). Everything else is a thin HTTP client over that API: the CLI
commands (`emit`, `list`, `purge`) and the MCP server, a stdio shim spawned
as `events mcp`. The shim being up says nothing about the service: every tool
call it handles is a live HTTP request to serve, and it fails when serve is
down. Machines usually also route http://events.this to serve via the local
domain front door; if the localhost port answers but the .this host does not,
the router is the problem, not this service.

Producers are other processes. Sibling tools cannot import this component's
internal packages, so they emit by POSTing JSON to `/api/events`; the CLI and
the `events_emit` MCP tool go through the same handler. An emit must land
before the producing process exits: a fire-and-forget goroutine in a
short-lived CLI dies with the process before the POST completes. Producers
emit synchronously with a short timeout and treat a failed emit as non-fatal.

## where state lives

The store is one append-only JSONL file per source under
{{.StorePath}}/sources/ (override with EVENTS_WORKDIR), one JSON event per
line, oldest first. Each source keeps its newest 500 events in memory; a file
that outgrows 1.5x that cap is compacted, rewritten atomically from the
capped slice. The store drops events past the cap by design, not by
accident.

## failure modes

Start with `events doctor`: one command runs the reachability, store, and
version-skew checks below and prints one line per check, FAIL lines naming the
cause. The paragraphs here cover what each failure means and what to do next.

Connection refused, or "events serve not reachable": serve is not running.
t-man typically supervises it. Run `t-man list` to see whether the events
agent exists, then `t-man restart events`. For a quick test without t-man,
`events serve` in a spare terminal also works.

A producer's events missing from `events_query`: usually a producer-side emit
problem, not a store problem. Either the producing process exited before its
POST landed (the fire-and-forget failure above) or a testing guard suppressed
the emit. Check `events_sources` first: when the source is absent entirely,
no emit has ever landed, so fix the producer, not the store.

Empty query result: check the filters before concluding loss. `source` must
match exactly, `since` is an exclusive id cursor that returns nothing when it
already points at the newest event, and the per-source cap drops the oldest
events once a source passes 500.

## version skew

`events version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart events`) and compare again. `events
doctor` runs this comparison as its version-skew check.

## first moves

1. `events doctor`: serve reachable, store readable, and version skew in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart events`
3. On version skew: `t-man restart events`, then `events doctor` again
4. `events list` with no filters, to confirm the store loads and has records
5. List `{{.StorePath}}/sources/`, to confirm the per-source JSONL files exist
