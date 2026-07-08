# reminder, CLI + MCP + web service for time-based reminders

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all over
one JSON store. Reminders fire a native macOS notification at their due time and
can recur. **The MCP tools (driven by Claude) are the primary surface**; the
`reminder` CLI mirrors them. The web page at `http://reminder.this/` is a
**read-only** view of the same store: it can cancel/delete but not create
(creating is MCP/CLI only).

## Module layout

```
reminder/
  cmd/reminder/        - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, mcp, manage (add/list/get/edit/cancel/test/fire), version (Version via ldflags)
    client/            - HTTP client shared by the CLI and mcpserver (client.go)
    store/             - Reminder model + pure helpers (reminder.go) and the
                         mutex-guarded JSON store (store.go, id.go)
    notify/            - Notifier interface + osascript macOS notification (notify.go),
                         best-effort event archiving to events.this (events.go)
    ticker/            - firing loop: scan due → notify → Trigger (reschedule/mark)
    server/            - HTTP API + webkit web page (embedded shell.html + app.js)
    mcpserver/         - MCP tools (server.go = MCP server setup, tools.go = 7 tools)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

Unlike `present` (where the MCP writes files and serve only reads), here both
the firing ticker and CRUD mutate state. To avoid two processes racing on the
JSON file, **`reminder serve` is the only writer**:

- **`reminder serve`** owns the store (in-memory map guarded by a mutex,
  persisted to `~/.local/share/reminder/reminders.json`). It runs the HTTP
  server (web + JSON API + `/version` + `/webkit/`) and the background ticker.
- **`reminder mcp`** holds no state: it is a thin HTTP client to the serve API
  on `localhost:<port>`. If serve is down, tools return "reminder serve not
  reachable … (t-man status reminder)".

So the MCP, the CLI, the web page, and the ticker all see the same data, and
there are no file locks.

## Data model & storage

```go
Reminder{ ID, Title, Body, Due (RFC3339 UTC), Repeat, Status, CreatedAt, UpdatedAt, FiredAt }
```

- `Status` is stored (`pending` → `fired`/`cancelled`/`done`), not purely
  derived, so the ticker fires a one-shot exactly once. The web and API also
  expose a derived `overdue` flag (`pending && due ≤ now`).
- `Repeat`: `""` (one-shot) | `daily` | `weekly` | a Go duration (`"24h"`). On
  fire, a recurring reminder advances `Due` to the next occurrence after now
  (looping past missed intervals so a long-down service doesn't spam) and stays
  `pending`; a one-shot becomes `fired`.
- Cancel is soft (status `cancelled`, record kept); hard delete is web-only
  (`DELETE /api/reminders/{id}`), mirroring `present`.

Persisted to a single file, `~/.local/share/reminder/reminders.json`, rewritten
atomically (temp file + rename) on every write.

## Build / install / test

```bash
make build    # ./reminder binary (Version via ldflags)
make install  # build + cp to ~/code/bin/reminder + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir + injected clock, never touches real $HOME)
```

## HTTP API

Owned by `reminder serve`:

- `GET  /`                              : webkit-chromed web page, read-only list (cancel/delete buttons; no create)
- `GET  /api/reminders[?status=]`       : list, soonest due first, each with `overdue`
- `POST /api/reminders`                 : create; body takes `due` (RFC3339) OR `in` (Go duration), plus `title`, `body?`, `repeat?`
- `GET  /api/reminders/{id}`            : one reminder
- `PUT  /api/reminders/{id}`            : edit (optional `title`/`body`/`due`/`repeat`); future `due` re-arms a fired one
- `POST /api/reminders/{id}/cancel`     : soft cancel
- `POST /api/reminders/{id}/test`       : deliver this reminder's notification now, **no state change** (dry run; no event emitted)
- `POST /api/reminders/{id}/fire`       : fire for real now (recurring → reschedule, one-shot → fired; emits an event), same path as the ticker
- `POST /api/test`                      : generic test notification, no reminder needed → `{"ok":true}`
- `DELETE /api/reminders/{id}`          : hard delete (web only)
- `GET  /version` → `{"version":"<sha>"}`; `GET /webkit/` : shared chrome (asset drift checks use `GET /webkit/version`)

The server is constructed with a `notify.Notifier` (`server.New(st, version, n)`)
so the test/fire endpoints reuse the exact delivery path the ticker uses. **test
vs fire:** test is a dry run that never mutates state (doesn't consume a one-shot
or advance a recurring schedule): the primary "do notifications work here?"
check; fire is a real firing that advances state and is recorded in the event log.

## Commands

CLI surface beyond `serve`/`mcp`, wired in `internal/cli/manage.go` as thin
HTTP clients to `reminder serve` (`internal/client`):

```bash
reminder add <title> [--in <dur> | --due <time>] [--body <note>] [--repeat <spec>]
reminder list [--status pending|fired|done|cancelled]
reminder get <id>
reminder edit <id> [--title] [--body] [--due] [--repeat]
reminder cancel <id>
reminder test [id]
reminder fire <id>
reminder version [-o json]
```

`--due` accepts RFC3339 or `2006-01-02T15:04` (parsed in the local timezone,
then normalized to RFC3339 before the API call); `--in` accepts a Go duration
(e.g. `2h30m`). `reminder version` prints the git commit that built the
binary; `-o json` prints `{"version":"<sha>"}`, the convention sibling tools
follow so ralph can probe any of them for the build they are running.

## MCP tools

Thin client over the API above (`internal/client`):

- `reminder_create(title, due?|in?, body?, repeat?)`: `due` absolute RFC3339 OR `in` Go duration; `repeat` daily/weekly/duration
- `reminder_list(status?)`: soonest first, with `overdue`
- `reminder_get(id)`
- `reminder_edit(id, title?, body?, due?, repeat?)`: future `due` re-arms a fired reminder
- `reminder_cancel(id)`: soft cancel
- `reminder_test(id?)`: fire a notification now to verify notifications work; with `id` sends that reminder's notification (no state change), without `id` sends a generic test
- `reminder_fire(id)`: fire a reminder for real now (advances recurring/one-shot state)

Tool responses include `url` (the human-facing `REMINDER_BASE_URL`, e.g.
`http://reminder.this`), while the client itself calls `localhost:<REMINDER_PORT>`.

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-page-header>`, `<wk-title>`) come from the
in-module package **`github.com/mad01/thismoon/webkit`**, mounted at `GET /webkit/`
via `webkit.Mount(mux)` and loaded by `internal/server/shell.html` (which pulls
the FOUC guard from `/webkit/boot.js`). Don't re-add palette/topbar/theme CSS
locally; it lives in webkit only.

reminder is a webkit consumer like the other services in this repo. There is no
pin or bump step: the binary compiles against the webkit committed alongside it,
so a webkit change ships at the next build:

```bash
make install && t-man restart reminder
```

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // mux.Handle("GET /webkit/", webkit.Handler()) - serves webkit.css/.js + /webkit/boot.js + /webkit/version
```

`internal/server/server.go`'s `Handler()` calls `webkit.Mount(mux)` alongside
the reminder routes.

### Header markup (reminder)

```html
<wk-header brand="reminder" title="Reminders"></wk-header>
```

`webkit.js` injects the full control set (font · bionic · size ± · reload ·
theme). Don't add those controls manually.

### Per-repo changes

- Header markup lives in `internal/server/shell.html`; reminder-specific
  layout (list wrapper, hint text, card layout, delete modal) stays local in
  its inline `<style>` block: palette, header chrome, page-header, card,
  badge, modal, and toast visuals all come from `/webkit/webkit.css`.
- The reminder list itself is rendered client-side by `app.js`, which builds
  `<wk-page-header>`, `<wk-title>`, `<wk-card class="rem">`, and `<wk-badge>`
  (status → badge variant/label) via `Webkit.el`, plus the delete-confirmation
  `<wk-modal>` and `<wk-toast-host>` already in `shell.html`.

### Version check

Confirm with `GET /webkit/version`: every consumer built from the same commit
reports the same asset hash.

## Gotchas

- **Wave 0 builder.** Builds before `recipes/claude-mcp` (wave 1) registers the MCP.
- **serve must be running for the MCP/CLI to work**: it owns the store. It runs
  as a t-man agent; `t-man status reminder` / `t-man restart reminder`.
- **MCP is unsandboxed.** It's first-party code that only makes HTTP calls to
  localhost, so it runs without a seatbelt wrapper, the same as `worklog`.
- **Notifications fire within ~30s** of due (the ticker interval), not to the second.
- **osascript at the edge.** Only the ticker fires `osascript`; titles/bodies are
  escaped into an AppleScript string literal (`notify.appleScriptString`) so
  quotes can't break out.
- **Codesign for the binary.** `make install` strips xattrs and re-signs (macOS
  kills adhoc-signed binaries with drifted provenance).

## See also

- Recipe: `recipes/reminder/recipe.toml` (+ `recipes/reminder/CLAUDE.md`)
- Route: `recipes/d-man/routes.toml` (`reminder` → 7428)
- Skill: `recipes/claude/skills/reminder/SKILL.md`
- MCP registration: `recipes/claude-mcp/servers.json`
- Human docs: `README.md`
