---
name: commit-pipeline
description: Coordinate commits when multiple agent sessions work in the same repo. As a requester, write a commit request file to ~/.commits/ instead of running git commit; as the committer, process the queue serially with exact staging. Use when the user says "commit pipeline", "queue this commit", "act as committer", or when another session owns git for the shared checkout.
argument-hint: "request <slug> | run (act as the committer)"
---

When several sessions share one working tree, git needs a single owner. This
skill has two roles; the full file format and lifecycle live in `pipeline.md`
beside this file; read it before acting in either role.

Pick the role from the arguments or context: `request` (or a description of a
commit to queue) makes this session a requester; `run` (or the user saying
this session is the committer) makes it the committer.

## Requester

You do not run `git commit` while the pipeline is active for this repo.
Instead:

1. Decide the commit: which of the working tree's changed files are yours,
   what the conventional-commit message is, which gates should pass first.
   `git status` and `git diff` on your own paths only — other sessions' dirty
   files are not yours to read or reason about.
2. Write `~/.commits/<repo>-<slug>.md` in the format from `pipeline.md`
   (`mkdir -p ~/.commits` if needed), `status: pending`, files listed
   explicitly. Several commits become several files chained with `after`.
3. Tell the user the request file path so they can hand it to the committer
   session. If a wire channel for the pipeline was shared with you, post the
   filename there too.
4. Stop. Do not watch the file for status changes unless asked; the
   committer owns it from here.

## Committer

You are the only session running git for the repos in the queue.

1. Scan `~/.commits/*.md` for `status: pending`. Order per repo by
   `requested_at`, honoring `after`. No requests: say so and stop.
2. For each request, follow the committer rules in `pipeline.md`: claim it,
   verify repo/branch/files, run gates, stage exactly the listed paths,
   commit, record the sha, mark `done`, move it to `~/.commits/done/`.
   On any mismatch or gate failure, mark `failed` with the reason in Notes
   and continue with the next request.
3. Report the results: one line per request with name, outcome, and sha or
   failure reason.
4. If the user wants the queue watched continuously, open a wire channel as
   the doorbell (ask first, then surface the connection string) and block on
   `wire_read` between scans.

## Rules

- Exact staging always: only paths listed in the request, never `-A`, never
  `.`. Unlisted dirty files belong to someone else.
- Commit messages follow conventional-commit format and carry no attribution
  trailers.
- Never push to the default branch; push other branches only when the
  request sets `push: true`.
- Never checkout, stash, or discard to make a request applicable — that is a
  `failed`, not a fix.
