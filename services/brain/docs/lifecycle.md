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
`promoting` is a private note with an open pull request into a shared vault,
named in `shared_pr`; `brain sync` settles it (`brain docs sharing`).

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
and `status_at` and purges the note from both indexes at once. `brain edit`
never revokes: it sets only `raw`, `validated`, `promoting` or `disputed`,
and a revoked status through `--status` or `--set`, or any edit of a
tombstone, exits 2.

## Flagging a note

`brain flag <note> --reason <text>` marks a note whose claims you doubt:
status `disputed`, the reason in `status_reason`, the day in `status_at`. read
and peek show a DISPUTED banner until someone settles the note with
`brain edit --status validated` or a retraction. A second flag
replaces the reason. A revoked note refuses (exit 2). A shared note is not
flagged in place: flag opens an issue on its repo (`gh issue create`) naming
the note, its id and the reason, and `--dry-run` prints the issue instead. A
shared clone with no remote refuses with exit 6.

## Capture

`brain capture` writes a raw note into `inbox/` for an agent or a quick
human note: `status: raw`, `captured_by` from `--by` (`user` by default, or
`agent:<session>`), and `expires_at` 30 days out. `--source <url>` sets
`source` and makes the note type `source`. The body comes from `--body`,
`--body-file` or stdin; without `--title` the body's first line (or the
source) titles it. It runs write's checks: exists (4), secret (8) and
duplicate (9). An agent's later edits never move `expires_at`.

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

## Duplicates at write

Before `brain write` or `brain capture` creates a note, brain looks for a
note it likely duplicates in the target vault and in every shared vault:

- a title or alias equal to the new title once lowercased and with
  punctuation folded to spaces (`Kubernetes: Upgrades` matches
  `kubernetes-upgrades`; `+` and `#` count, so `C++` and `C#` differ);
- when the semantic layer is on and the vault already has an index, a chunk
  with a cosine similarity of 0.92 or more to the new note's first chunk.
  The index is used as it is, never refreshed for the check. With Ollama
  down the check is skipped with a `warning:` line.

Superseded, retracted and withdrawn notes are never candidates. A hit
refuses the write with exit 9 (`possible_duplicate`), names the candidates
and suggests `brain edit` on the first private one. `--distinct-from <note>`
(repeatable) stores the note in `distinct_from`, as `id:<ulid>` when it has
one, and removes it from the candidates; the write goes through once every
candidate is named. `brain edit` never runs the check. The exists check
(exit 4) and the secret scan (exit 8) run first.

## Drift

`brain drift [notes...]` checks the `sources[]` a note pins. A code ref,
`ref: repo:path@commit#symbol`, is looked up in a local checkout: the repo
(`host/owner/name`, or `owner/name` on github.com) resolves to
`<root>/<host>/<owner>/<name>` under each `code_roots:` entry of the config.

```yaml
sources:
  - ref: github.com/acme/app:pkg/serve.go@4f2a9c1#Serve
  - url: https://example.com/spec
    content_hash: sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
    archive: https://web.archive.org/web/2026/https://example.com/spec
```

`content_hash` is a full hex digest, sha256 unless prefixed `sha512:` or
`sha1:`. The findings:

- `moved`: the path is gone at HEAD (`moved_to` when git sees a rename).
- `changed`: the path's content differs from the pinned commit.
- `symbol-missing`: the symbol no longer appears as a whole word.
- `gone`: a url answers 404 or 410 (with the `archive` url).
- `hash-changed`: a url no longer matches `content_hash`.
- `bad-hash`: `content_hash` is not a full digest. drift finds it offline too.
- `too-large`: the file is over 8 MiB at HEAD, so drift skipped the symbol.
- `unresolved`: a repo not checked out, a commit not fetched, a failed fetch.

drift exits 1 when a note drifted; `unresolved` and `too-large` alone are
not drift. With no arguments it checks every note that is not revoked.

Urls are fetched only with `--online`, and only over http and https,
redirects included. A fetch to a loopback, link-local or private address is
refused unless `--allow-local`. Each fetch gets 20 seconds and all of them
together `--timeout` (2m by default); a url the budget does not reach, or a
body over 32 MiB, is `unresolved`. `--allow-local` and `--timeout` need
`--online` (exit 2 without it).

A `sources[]` `ref` may also name a note: `id:<ulid>`, `vault:path.md` or
`path.md`. drift skips it, `brain stale` reads it, and `brain check` reports
one that names no single note.

## Stale queue

`brain stale` lists the notes that need a human look. It is computed on each
run and never stored. A note is queued when its `review_by` has passed (on
any status in circulation) or, for a raw note, its `expires_at`. A disputed
note is queued. So is a dependent: a note with a `sources[]` ref resting on a
superseded, retracted or withdrawn note. With `--drift`, so is a note whose
pinned sources drift found changed. A wikilink is a mention, not a
dependency, and a dependent's own dependents are not queued. Revoked notes
never are.

The queue is grouped by domain: the owners the vault's `CODEOWNERS` (or
`.github/CODEOWNERS`, last matching rule wins) gives the note's path, else
its `steward`, else `unowned`. Larger groups come first, `unowned` last.
Each group shows its first `--limit` notes (10), oldest reason first. Add
`--online` to `--drift` to fetch urls too. stale exits 0 whatever it lists.

## brain check

`brain check` exits 1 on an error: a citation of a retracted or withdrawn
note, bad status fields, a shared id, a secret. A note path that is or goes
through a symbolic link is an error too (`symlink`). So is a NUL byte
(`nul-byte`), which would hide what follows it from the secret scan. A
`secret` finding names the scan rule and the match length, never its
characters. check skips a path that names no note.

The vault's `.brain/policy.yaml` names the classifications it accepts and
sets rule levels: `classifications: [public, internal]` and
`rules: {provenance: error, missing-field: off}`. A rule it does not name
keeps its default. At error: `schema`, `status-fields`, `link-revoked`,
`id-duplicate`, `classification`, `secret`, `symlink`, `nul-byte`. At
warning: `status-unknown`, `missing-field`, `provenance`, `link-dangling`,
`link-unvalidated`, `stem-duplicate`. `policy` (a policy file brain cannot
use) is always an error. `brain promote --to` runs the same check against
the shared vault's policy before it pushes anything.

## JSON shapes

| Command | Top-level keys |
|---|---|
| `capture` | `vault`, `path`, `warnings[]` (as write) |
| `flag` | `vault`, `path`, `sha`; on a shared note the issue (`brain docs sharing`) |
| `drift` | `ok`, `online`, `checked`, `drifted`, `offline_skipped`, `notes[] {vault, path, findings[] {kind, source, message, moved_to, archive}}` |
| `stale` | `total`, `drift`, `groups[] {domain, from, total, notes[] {vault, path, title, status, reasons[] {kind, detail, since}}}` |
| `inbox list` | `total`, `notes[] {vault, path, title, captured_by, expires_at, expired}` |
| `inbox keep` | `vault`, `path`, `sha`, `expires_at` |
| `inbox archive` | `dry_run`, `archived[] {vault, path, archived_to}`, `skipped[] {vault, reason}`, and `error {code, message}` when it failed partway after the moves listed |
