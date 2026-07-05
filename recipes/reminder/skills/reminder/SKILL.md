---
name: reminder
description: Set, list, edit, cancel, and test-fire time-based reminders that fire a macOS notification at their due time. Use when the user says "remind me", "set a reminder", "what are my reminders", "cancel that reminder", "send a test notification", or asks to be nudged about something later. Backed by the reminder MCP (reminder_create/list/get/edit/cancel/test/fire); view them at http://reminder.this/.
argument-hint: "[what + when]  — e.g. 'me to push the branch in 2h', or omit to list"
---

# reminder

Schedule reminders that pop a native macOS notification when they come due, and
manage them. State lives in one JSON store owned by the `reminder serve` agent;
the same reminders show up on the web page at **http://reminder.this/**, via these
MCP tools, and via the `reminder` CLI.

## Tools

Prefer the **MCP tools** (no shell prompt): `reminder_create`, `reminder_list`,
`reminder_get`, `reminder_edit`, `reminder_cancel`, `reminder_test`,
`reminder_fire`. The `reminder` CLI is the
fallback if the MCP server isn't connected — but note the MCP and CLI both just
talk to the running `reminder serve` agent, so if one is down so is the other
(`t-man status reminder`).

The tools are self-describing; you can call them directly without this skill.
This skill mainly covers **converting the user's wording into the right time**.

## Time conversion — the important part

The user speaks in natural language; the store needs a concrete time. You know
today's date, so do the conversion yourself:

- **Relative** ("in 2 hours", "in 45 minutes", "in 90s") → pass `in` as a Go
  duration: `"2h"`, `"45m"`, `"90s"`. Combine units like `"2h30m"`.
- **Absolute** ("tomorrow 9am", "next Monday at noon", "June 30 at 17:00") →
  compute the wall-clock time in the user's local timezone and pass `due` as an
  RFC3339 timestamp **with offset**, e.g. `2026-06-26T09:00:00+02:00`. Including
  the offset avoids UTC/local confusion.
- Pass `due` OR `in`, never both.

For recurring reminders ("every morning", "daily standup", "weekly"), add
`repeat`: `"daily"`, `"weekly"`, or a Go duration like `"24h"`. A recurring
reminder reschedules itself to the next occurrence after each fire.

## Modes

### Create
Extract the subject (→ `title`), any detail (→ `body`), the time, and any
recurrence. Call `reminder_create`. Confirm back to the user in their terms
("Set — I'll nudge you at 9am tomorrow") and mention http://reminder.this/ if
they want to see all of them.

### List / get
"What are my reminders?" → `reminder_list` (soonest first; note the `overdue`
flag). Filter with `status` (pending|fired|done|cancelled) when asked ("what
have I already been reminded about" → `fired`). For one item's detail use
`reminder_get` with its id.

### Edit
"Move the standup reminder to 10am" → find it via `reminder_list`, then
`reminder_edit` with the id and the changed field(s). Moving `due` to a future
time re-arms a reminder that already fired.

### Cancel
"Cancel that" → `reminder_cancel` with the id (soft — keeps the record, stops it
firing). Permanent deletion is web-only (the Delete button at
http://reminder.this/), so if the user truly wants it gone, point them there.

### Test / fire
"Do notifications work?", "send a test notification" → `reminder_test` with **no
id** (generic test). "Test the standup reminder" → `reminder_test` with its id —
sends that reminder's exact notification **without changing its state** (no
one-shot consumed, no schedule advanced). Use this to verify the notification
path on a machine, e.g. right after setup.

"Fire the standup now" / "trigger it early" → `reminder_fire` with the id — a
**real** firing ahead of due time: it advances state (recurring reschedules, a
one-shot becomes fired) and is recorded in the event log. Reach for `test` when
verifying, `fire` only when the user actually wants the reminder to go off now.

## Notes

- Reminders fire within ~30s of their due time (the serve agent scans on a
  ticker). Don't promise to-the-second precision.
- If a tool returns "reminder serve not reachable", the serve agent is down —
  tell the user to check `t-man status reminder`.
