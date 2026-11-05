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
`status_at`.

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

## brain check

`brain check` exits 1 on an error: a citation of a retracted or withdrawn
note, bad status fields, a shared id, a secret. It skips a path that names no
note. `.brain/policy.yaml` in the vault sets `classifications:
[public, internal]` and `rules: {provenance: error, missing-field: off}`.
