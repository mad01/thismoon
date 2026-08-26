# reminder configuration

## Where config lives

reminder has no config file. It resolves its handful of settings from
persistent CLI flags on the `reminder` command, each with an environment
variable fallback and a compiled-in default. Precedence is flag over
environment variable over default. The flags are persistent, so they apply to
every subcommand (`serve`, `mcp`, `add`, `list`, ...), though only `serve`
and `mcp` read them for anything beyond talking to a running `reminder
serve`.

## Flags

- `--workdir` (string, default `~/.local/share/reminder`, env
  `REMINDER_WORKDIR`): directory holding `reminders.json`, the single JSON
  file that is the whole store. Only `reminder serve` reads this — it is the
  sole writer. A leading `~` expands to the user's home directory at
  startup, so `REMINDER_WORKDIR` does not need shell expansion.
- `--port` (int, default `7428`, env `REMINDER_PORT`): the port `reminder
  serve` listens on, and the port every other command (the MCP server, the
  CLI's `add`/`list`/`get`/`edit`/`cancel`/`test`/`fire`) connects to as an
  HTTP client. `serve` binds to `127.0.0.1` only; reminders are personal and
  the API has no authentication, so it must never be reachable off
  localhost.
- `--base-url` (string, default `http://localhost:<port>`, env
  `REMINDER_BASE_URL`): the human-facing URL the MCP server returns in tool
  responses (for example `http://reminder.this`, when a local domain front
  door routes to the service). This is display-only: the MCP client itself
  always calls `localhost:<port>` regardless of this value.

## Environment variables

- `EVENTS_BASE_URL` (default `http://127.0.0.1:7430`): base URL of the local
  events service that `reminder serve` best-effort archives fired
  notifications to. Read only by the notify package, not exposed as a flag.
  If the events service is down or this points nowhere, archiving is
  silently skipped: reminder still fires its own macOS notification either
  way, since this is an archive, not the delivery path.
