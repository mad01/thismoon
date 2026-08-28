# commit pipeline, one committer for a shared working tree

Two or more agent sessions editing the same checkout cannot each run
`git commit`. Staging interleaves, a `git add -A` sweeps another session's
half-finished work into an unrelated commit, and the message ends up
describing files its author never touched. The fix is ownership: exactly one
session, the **committer**, runs git for the repo. Every other session is a
**requester** that describes the commit it wants as a file under `~/.commits/`
and leaves the git work to the committer.

The request file is the whole protocol. Nothing has to be running: the
directory is the queue, the `status` field in the frontmatter is the state
machine, and a wire channel is an optional doorbell on top.

## Request file

One file per commit, named `~/.commits/<repo>-<slug>.md`:

```markdown
---
name: thismoon-opener-multi-url
repo: /Users/you/code/src/github.com/mad01/thismoon
branch: alexander/opener-multi-url
status: pending
requested_by: opener session
requested_at: 2026-08-28T14:05:00+02:00
after: ""
push: false
commit: ""
---

## Message

feat(opener): accept multiple URLs per open_url call

Opens each URL in order and reports per-URL failures instead of
stopping at the first.

## Files

- tools/opener/internal/cli/cli.go
- tools/opener/internal/mcpserver/tools.go
- tools/opener/internal/sysopen/sysopen.go
- tools/opener/internal/sysopen/sysopen_test.go

## Gates

- make -C tools/opener test

## Notes

The tree also carries unrelated belt changes from another session; they
must not be staged.
```

Field rules:

- `repo` is the absolute path to the checkout. `branch` is where the commit
  must land; the committer never switches branches to satisfy a request.
- `status` is the lifecycle: `pending` (requester wrote it) → `claimed`
  (committer picked it up) → `done` or `failed`. Only the committer moves a
  request past `pending`.
- `after` names another request file (by `name`) that must commit first.
  Several commits from one session are several request files chained with
  `after`.
- `push: true` asks the committer to push the branch after committing.
  Requests targeting the default branch never get pushed.
- `commit` starts empty; the committer fills in the sha on `done`.
- Files are listed one per line, relative to the repo root. Globs, `.`, and
  `-A` are not accepted. Prefix a deleted path with `D `.
- The Message section is the full commit message as it should land: subject
  line, then body, in conventional-commit format. Attribution trailers stay
  out.
- Gates are commands the committer runs from the repo root before staging;
  omit the section when there are none.

## Committer rules

Process the queue serially per repo, oldest `requested_at` first, honoring
`after`:

1. Set `status: claimed`.
2. Verify the repo path exists, the checkout is on `branch`, and every listed
   file has working-tree changes. Any mismatch means `failed` with the reason
   in Notes; never checkout, stash, or discard to make a request fit.
3. Run the gates in order. A failing gate means `failed` with the output tail
   in Notes.
4. Stage exactly the listed paths, commit with the message as written, record
   the sha in `commit`, set `status: done`, and move the file to
   `~/.commits/done/`.
5. Leave `failed` requests in `~/.commits/` so the requester can fix the
   problem and reset them to `pending`.

Files not listed in a request do not exist as far as the committer is
concerned: never stage them, never clean them, never fold them in because
they "belong together". They may be another session's work in progress.
Pushing follows the same restraint: only when the request asks, never to the
default branch.

## Doorbell, optional

Scanning `~/.commits/` on demand works. When the sessions are live at the
same time, the committer can open a wire channel and share the connection
string; a requester posts its request filename after writing the file, and
the committer's blocking `wire_read` wakes immediately instead of waiting
for the next scan.
