# ADR-0030: brain notes carry a lifecycle status, and revoking one never deletes its file

**Date:** 2026-11-05
**Status:** Accepted

**Scope:** `services/brain`: `internal/lifecycle`, `internal/check`, the
`supersede`, `retract`, `withdraw` and `check` commands, the status filter
behind every search, and the secret scan on write, edit and promote.

## Context

A vault note used to have a free-text `status` that nothing read. A note
proved wrong stayed in search results with the same weight as a sound one,
and the only way to stop agents citing it was to delete the file. Deleting
breaks every wikilink to it and loses the record of what was wrong. A
shared vault also needs a gate its CI can run on a pull request, since its
notes never pass through brain's write path.

## Decision

`status` is one of seven values: `raw`, `validated`, `promoting`,
`disputed`, `superseded`, `retracted` and `withdrawn`. `expired` is
computed (a raw note past `expires_at`, a validated note past `review_by`)
and never stored. A write fills `expires_at` on a raw note (30 days) and
`review_by` on a validated one (90 days for `volatility: high`, 365
otherwise), and every status change records `status_at`.

Three commands revoke a note, and none removes the file. `supersede`
points `superseded_by` at the replacement's id. `retract` keeps the body
under a banner with a reason. `withdraw` leaves a tombstone of only `id`,
`status` and `status_at`, and purges the note's chunks from both indexes
at once. Links to any of them still resolve. Default search hides all
three; `--include-revoked` shows them.

The lifecycle fields live in the frontmatter's unmodeled keys, read
leniently, so a vault written before this change parses and searches as it
did. `brain check` applies the strict rules, at levels a vault sets in
`.brain/policy.yaml`. Without that file a missing field is a warning, so
an old vault checks clean of errors.

## Consequences

- Citing a revoked note is a `brain check` error, so the problem shows up
  in the pull request that adds the citation.
- A withdrawn note is gone from search the moment the command returns, but
  its old text stays in git history. Withdraw is no substitute for rotating
  a leaked credential; the secret scan refuses such a write in the first
  place.
- `brain count` still counts every status, so a count can be higher than
  the hits a default search returns.
