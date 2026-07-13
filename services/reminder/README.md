# reminder

Set a reminder, get a macOS notification when it's due. Mostly you'll set them by
asking Claude; there's also a `reminder` CLI and a read-only web view at
`http://reminder.this/`. They're all the same reminders.

## How it works

One background agent (`reminder serve`) owns a JSON file of reminders, runs the
web page, and fires the notifications. Everything else (the CLI, the Claude Code
tools, the web page) talks to that agent, so you see the same list everywhere
and nothing gets out of sync.

A reminder has a due time, an optional note, and an optional repeat (daily,
weekly, or any interval). When it comes due, you get a native notification. A
one-shot is then marked fired; a repeating one reschedules itself for next time.

## Install

It ships from the thismoon monorepo. `ralph up` builds the binary to `~/code/bin/reminder`,
registers the `reminder serve` agent with t-man, adds the `reminder.this` route,
and wires the Claude Code tools.

To check the service is up:

```
t-man status reminder
```

## Usage

```
$ reminder add "push the branch" --in 2h
002a0afc1f  push the branch  due Thu Jun 25 22:13  repeat -  pending

$ reminder list
002a0afc1f  push the branch  Thu Jun 25 22:13  -      pending
c42fa3a0dc  standup           Fri Jun 26 09:00  daily  pending
```

```bash
reminder add "<title>" --in 90m            # relative time
reminder add "<title>" --due 2026-06-30T17:00   # absolute time
reminder add "standup" --in 12h --repeat daily  # recurring
reminder list                              # everything, soonest first
reminder list --status pending             # filter by status
reminder edit <id> --due 2026-07-01T09:00  # reschedule
reminder cancel <id>                       # stop it firing
reminder test                              # generic test notification (does it work?)
reminder test <id>                         # send that reminder's notification, no state change
reminder fire <id>                         # fire it for real now
```

Open `http://reminder.this/` for the read-only web view: every reminder with
its status. You can't create reminders here (that's Claude or the CLI), but
each card has a **Test** button (fires the notification now to check it works,
without changing the reminder), a **Cancel** button (stops it firing, keeps
the record), and a **Delete** button (removes it for good).

## MCP

Ask in plain language — "remind me to call the dentist tomorrow at 9", "what are
my reminders", "cancel the standup one". Claude works out the time and calls
the tools below.

| Tool | Purpose |
|------|---------|
| `reminder_create(title, due?\|in?, body?, repeat?)` | Create a reminder; `due` absolute RFC3339 or `in` a Go duration; `repeat` daily/weekly/duration |
| `reminder_list(status?)` | List reminders, soonest first, with `overdue` |
| `reminder_get(id)` | Get one reminder's full detail |
| `reminder_edit(id, title?, body?, due?, repeat?)` | Edit a reminder; a future `due` re-arms a fired one |
| `reminder_cancel(id)` | Soft-cancel (keeps the record) |
| `reminder_test(id?)` | Fire a notification now to verify it works; with `id` sends that reminder's notification (no state change), without sends a generic test |
| `reminder_fire(id)` | Fire a reminder for real right now (advances recurring/one-shot state) |

To check notifications actually work on a machine, ask Claude to "send a test
notification" (`reminder_test` with no id) or "test the standup reminder"
(`reminder_test` with its id — sends the notification without changing the
reminder). `reminder_fire` triggers a reminder for real ahead of its due time.

## Where things live

- Reminders: `~/.local/share/reminder/reminders.json`
- Binary: `~/code/bin/reminder`
- Web + API: `http://reminder.this/` (or `http://localhost:7428/`)

Removing the service doesn't delete your reminders file.

## Develop

```bash
make test    # go test ./...  (hermetic: temp dirs + a fixed clock, never touches your real reminders)
make build   # ./reminder
```

Architecture, the data model, the HTTP API, and the build commands are in
[`CLAUDE.md`](CLAUDE.md).
