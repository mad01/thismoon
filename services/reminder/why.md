# why reminder

## The problem

Work in an agent session throws off time-based follow-ups: push the branch
in two hours, check the release tomorrow morning. Writing those into a note
means never seeing them again; keeping them in your head means losing them.
The missing piece was a way to say "remind me" in plain language,
mid-session, and get a native macOS notification when it is due, with the
agent, the CLI, and a web page all looking at the same list.

## Why its own service

A reminder must fire whether or not any agent session is open, so an MCP
surface alone is not enough: something long-running has to watch the clock.
That firing loop owns state (a one-shot must fire exactly once, a recurring
one must reschedule), which makes it a service with a store rather than a
feature bolted onto another component. The store is a single local JSON file
with no accounts and no sync, and that is the point: reminders set by an
agent are machine-local working state, not calendar data.

## Why this shape

The load-bearing decision is single-writer: `reminder serve` is the only
process that mutates the store. It holds the reminders in memory behind a
mutex, persists atomically to one JSON file, runs the firing ticker, and
serves the HTTP API; the MCP and the CLI are thin HTTP clients to it. This
is deliberately unlike present, where the MCP writes files directly: here
both CRUD and the ticker mutate state, and funneling every write through one
process removes the race on the file without locks. Status is stored
rather than derived (`pending`, `fired`, `cancelled`), so the ticker fires a
one-shot exactly once, and a recurring reminder advances past missed
intervals instead of spamming after downtime. The web page at
`reminder.this` is a read-only view of the same store, a webkit shell
rendered client-side (docs/adr/0005); creating reminders stays with the MCP
and the CLI.

## Non-goals

reminder is not a calendar and syncs nowhere; removing the service leaves
the JSON file behind. Notifications are native macOS only, delivered via
osascript, and fire within the ticker's ~30-second window, not to the
second. The MCP offers soft cancel but no hard delete; permanent removal is
web-only. Natural-language time parsing lives in the agent, not here: the
API takes an RFC3339 time or a Go duration.
