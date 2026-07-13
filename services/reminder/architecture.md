# reminder architecture

## Overview

reminder runs as one long-lived process plus thin clients. `reminder serve`, a
t-man launchd agent on port 7428 behind `http://reminder.this/`, is the single
writer: it owns the store (an in-memory map guarded by a mutex, persisted to
one JSON file), runs the firing ticker, and serves the HTTP API plus the
webkit web page. `reminder mcp` and the `reminder` CLI hold no state; both are
HTTP clients to the serve API on localhost. That funnel is what keeps the
ticker, MCP, CLI, and web page on the same data without file locks. Everything
listens on localhost only.

## Structure

```
cmd/reminder/        entrypoint, delegates to internal/cli
internal/cli/        cobra: serve, mcp, manage (add/list/get/edit/cancel/
                     test/fire), version
internal/client/     HTTP client shared by the CLI and the MCP server
internal/store/      Reminder model + pure helpers; mutex-guarded JSON store
internal/notify/     Notifier interface, osascript macOS notification,
                     best-effort event emit to events.this
internal/ticker/     firing loop: scan due, notify, advance state
internal/server/     HTTP API + web page (embedded shell.html + app.js)
internal/mcpserver/  MCP server setup and the 7 reminder_* tools
```

`internal/server` mounts the in-module webkit Go package via
`webkit.Mount(mux)`. The embedded `shell.html` carries the `<wk-header>` and
reminder-local styling; `app.js` renders the list client-side from the JSON
API, building `<wk-page-header>`, `<wk-card>`, and `<wk-badge>` elements with
`Webkit.el`, per docs/adr/0005.

## Data flow

Writes converge on the serve API. An MCP tool call or CLI command goes through
`internal/client` to the HTTP API (for example `POST /api/reminders`), which
mutates the store under its mutex and rewrites the JSON file atomically. The
web page reads `GET /api/reminders` and can cancel or delete, but not create.

The ticker in `internal/ticker` wakes about every 30 seconds, scans for
pending reminders whose due time has passed, delivers each through the
injected `notify.Notifier` (osascript at the edge, with titles and bodies
escaped into an AppleScript string literal), and advances state: a one-shot
becomes `fired`; a recurring reminder loops its `Due` forward past any missed
intervals and stays `pending`. Firing emits a best-effort event to
events.this. The test and fire endpoints reuse this exact delivery path
because the server is constructed with the same `Notifier` (`server.New(st,
version, n)`): test is a dry run with no state change, fire is the real thing.

## Storage

One file, `~/.local/share/reminder/reminders.json`, rewritten atomically
(temp file + rename) on every write. Each record is
`Reminder{ID, Title, Body, Due, Repeat, Status, CreatedAt, UpdatedAt,
FiredAt}` with `Due` in RFC3339 UTC. `Status` is stored, not derived, so a
one-shot fires exactly once; `overdue` is derived at read time. Removing the
service leaves the file behind.

## Interfaces

HTTP API (owned by serve): `GET /` (web page), `GET/POST /api/reminders`,
`GET/PUT /api/reminders/{id}`, `POST .../cancel`, `POST .../test` (dry run),
`POST .../fire` (real firing), `POST /api/test` (generic test notification),
`DELETE /api/reminders/{id}` (web-only hard delete), `GET /version`,
`GET /webkit/`.

CLI: `serve`, `mcp`, and the manage commands `add`, `list`, `get`, `edit`,
`cancel`, `test`, `fire`, plus `version [-o json]`.

MCP tools: `reminder_create`, `reminder_list`, `reminder_get`,
`reminder_edit`, `reminder_cancel`, `reminder_test`, `reminder_fire` — all
thin calls through `internal/client`; responses link to the human-facing
`REMINDER_BASE_URL` while the client itself calls `localhost:<port>`.

Config: `--workdir`/`REMINDER_WORKDIR` (directory holding `reminders.json`),
`--port`/`REMINDER_PORT` (default 7428), `--base-url`/`REMINDER_BASE_URL`.
