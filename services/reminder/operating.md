# operating reminder

reminder stores time-based reminders and fires a native macOS notification
when one comes due. A reminder is one-shot by default; a repeat of daily,
weekly, or a Go duration makes it recur. Reminders are set via the MCP tools
or the CLI and fire whether or not any agent session is open; the web page
is a read-only view of the same list.

## how it runs

A single `reminder serve` process is the only writer. It owns the store,
runs the firing ticker, and serves the JSON API plus the web page on
localhost (default {{.BaseURL}}). Everything else is a thin HTTP client over
that API: the CLI commands (`add`, `list`, `get`, `edit`, `cancel`, `test`,
`fire`) and the MCP server, a stdio shim spawned as `reminder mcp`. The shim
being up says nothing about the service: every tool call it handles is a
live HTTP request to serve, and it fails when serve is down. Machines usually
also route http://reminder.this to serve via the local domain front door; if
the localhost port answers but the .this host does not, the router is the
problem, not this service.

Firing happens inside serve. The ticker scans about every 30 seconds for
pending reminders past their due time, delivers each via osascript, then
advances state: a one-shot becomes `fired`, a recurring reminder moves its
due time past any missed intervals and stays `pending`. A reminder fires
within that 30 second window of its due time, not to the second.

## where state lives

The store is one JSON file, `reminders.json`, in the workdir, default
{{.StorePath}} (override with REMINDER_WORKDIR). Every write rewrites the
file atomically (temp file + rename), so it is safe to read directly.
Removing the service leaves the file behind.

## failure modes

Connection refused, or "reminder serve not reachable": serve is not
running. t-man typically supervises it. Run `t-man list` to see whether the
reminder agent exists, then `t-man restart reminder`. For a quick test
without t-man, `reminder serve` in a spare terminal also works.

A reminder did not fire: only serve fires, so first check it was running at
the due time (`t-man list`, then the serve log). If serve was down, the
first scan after restart fires whatever came due meanwhile: a one-shot
fires once, a recurring one advances past all missed intervals and fires
once. If serve was up, check the reminder itself with `reminder get <id>`.
A `fired` status with `fired_at` set means it did fire and the notification
was missed or dismissed; a `pending` one past due means delivery failed and
the ticker retries it every cycle. `reminder test <id>` delivers that
reminder's notification now with no state change, the direct check that
delivery works on this machine.

Empty list: usually a filter, not an error. `reminder list` without flags
returns every status; `--status pending` hides fired, done, and cancelled
records, so a one-shot that already fired disappears from it. Retry without
the filter before concluding the store is empty, then check that
`reminders.json` exists in the workdir and is non-empty.

## version skew

`reminder version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came
from. When the `commit` values differ, an old process is still serving
after an upgrade: restart it (`t-man restart reminder`) and compare again.

## first moves

1. `curl -s -o /dev/null -w '%{http_code}' {{.BaseURL}}/healthz` (204 means serve is up)
2. If unreachable: `t-man list`, then `t-man restart reminder`
3. Compare `reminder version -o json` with the `/version` endpoint for skew
4. `reminder list` with no filters, to confirm the store loads and has records
5. `reminder test`, to confirm notifications deliver on this machine
