# brain note lifecycle and freshness

`brain docs lifecycle` prints this page. `brain docs` prints the CLI guide:
commands, flags, exit codes and the JSON shapes of the other commands.

## Statuses

`status` is `raw`, `validated`, `promoting`, `disputed`, `superseded`,
`retracted` or `withdrawn`. An empty field means the tier default: `raw` in
`inbox/`, `validated` everywhere else. `expired` is computed and never
stored: a raw note past `expires_at`, or a validated note past `review_by`.

write sets `expires_at` on a raw note (30 days) and `review_by` on a validated
one (90 days with `volatility: high`, else 365). Every status change sets
`status_at`. An edit leaves `expires_at` alone, whoever makes it; only
`brain inbox keep` moves it (or an explicit `--set expires_at=...`).

Search hides a superseded, retracted or withdrawn note without
`--include-revoked`; read and peek print its banner first. Hits carry
`status`, `review_by` and `stale` (past `review_by`). write and edit warn on
stderr, and in `warnings[]`, for each body link to a note that is not
validated or is expired.

## Revocation

`supersede`, `retract` and `withdraw` take a note out of circulation and
never delete the file, so links to it still resolve. supersede stores the
replacement as `superseded_by: id:<ulid>`. retract keeps the body under a
banner with `retracted_reason`. withdraw leaves a tombstone of `id`, `status`
and `status_at` and purges the note from both indexes at once.

## Flagging a note

`brain flag <note> --reason <text>` marks a note whose claims you doubt:
status `disputed`, the reason in `status_reason`, the day in `status_at`. read
and peek show a DISPUTED banner until someone settles the note with
`brain edit --status validated` or a retraction. A second flag
replaces the reason. A revoked note refuses (exit 2). A shared vault refuses
with exit 6: open an issue on its repo naming the note and the reason; the
message names the repo's origin remote when it has one.

## Inbox expiry

A raw capture expires 30 days after it was written unless someone keeps it.

- `brain inbox list [--expired]` lists the raw notes of every inbox, soonest
  `expires_at` first, with `captured_by` and an `expired` flag.
- `brain inbox keep <note> [--days N]` sets `expires_at` to N days from today
  (30 by default), never earlier than it was. A note that is not raw refuses
  (exit 2), a shared vault too (exit 6).
- `brain inbox archive --expired [--dry-run]` moves every expired raw note to
  the same path under `.brain/archive/` in its vault and drops it from both
  indexes. Every walk skips `.brain`, so search, check and the link graph no
  longer see it; nothing is deleted. A name an earlier archive took gets a
  `-2` suffix. Restore a note by moving it back under `inbox/`, then keep it.
  Shared vaults are skipped unless `--vault` names one (exit 6).

## brain check

`brain check` exits 1 on an error: a citation of a retracted or withdrawn
note, bad status fields, a shared id, a secret. It skips a path that names no
note. `.brain/policy.yaml` in the vault sets `classifications:
[public, internal]` and `rules: {provenance: error, missing-field: off}`.

## JSON shapes

| Command | Top-level keys |
|---|---|
| `flag` | `vault`, `path`, `sha` |
| `inbox list` | `total`, `notes[] {vault, path, title, captured_by, expires_at, expired}` |
| `inbox keep` | `vault`, `path`, `sha`, `expires_at` |
| `inbox archive` | `dry_run`, `archived[] {vault, path, archived_to}`, `skipped[] {vault, reason}` |
