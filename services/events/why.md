# why events

## The problem

The platform's components do things in the background (dependency scans,
blocked commits, sandbox denials) and the traces scatter across per-process
logs or vanish entirely. A macOS notification is gone the moment it is
dismissed. When something looks wrong, "what happened on this machine in the
last day" has no single answer: you grep several log files and reconstruct a
timeline by hand. events gives every producer one place to record a tagged
event and gives the human and the agent one filterable timeline to read.

## Why its own service

An event log is cross-cutting by nature: every component produces events, so
it cannot live inside any one of them. The monorepo makes this concrete:
components cannot import each other's internal packages, so the only way for
sibling tools to write to a shared log is over HTTP, which means a running
process must own the store and expose `POST /api/events`. That same process
serves the timeline and the MCP tools, so the CLI, the web page, and the agent
all read the identical log. A file-based log without an owning process would
put every producer in a write race.

## Why this shape

Single-writer first: `events serve` is the only process that touches the
files; the CLI and MCP tools are thin HTTP clients, and other components emit
by POSTing JSON. Every write funnels through one process, so there are no file
locks. The store is an append-only per-source JSONL log: one file per
source, one JSON event per line, an O(1) append per event, with a per-source
ring cap in memory and atomic compaction once a file outgrows it. The result
is bounded disk and memory, and plain files that outlive the service if it is
removed.
Event IDs are time-sortable, so lexical order is time order and the ID doubles
as the polling cursor. Above all, events is archive-only by design: it records
and displays, and it never fires a notification — alerting stays with the
producer (deps fires its own banner), while events keeps the durable record.
The timeline renders client-side from the JSON API, the repo-wide convention
recorded in `docs/adr/0005` — events was already built that way before the
convention landed.

## Non-goals

events does not notify, schedule, or trigger anything; there is no ticker.
It is not a message queue: nothing consumes or acknowledges events, they are
only read. Retention is deliberately bounded by the per-source cap, so it is
not an unbounded archive. And deletion stays human-triggered: purge exists
only on the CLI and the HTTP API, never as an MCP tool, so an agent can record
history but cannot erase it.
