# loom, one worktree per session, one weaver for the merges

Two or more agent sessions editing the same checkout cannot each run git:
staging interleaves and a stray `git add -A` sweeps another session's
half-finished work into an unrelated commit. The old answer was a commit
queue — every session described its commit as a request file and one
committer session landed them serially. Worktrees remove the shared tree
instead: each session owns an isolated checkout of its own branch, commits
locally whenever it likes, and the only thing left to coordinate is which
branch lands on the default branch, in what order. That sequencing role is
the **weaver**.

Git is the whole state store. There is no queue directory, no status
frontmatter, nothing to garbage-collect: `git worktree list` says what
exists, `git status` says what is in flight, `git log main..branch` says
what is ready, `git branch --merged` says what is done. Every question the
weaver asks is answered live from git.

## Layout

Worktrees live under `~/.worktrees/<repo>/<slug>`:

- `<repo>` is the repo directory's basename (`thismoon`, not the org).
- `<slug>` is the branch name with `/` replaced by `-`, so
  `alexander/opener-multi-url` claims `~/.worktrees/thismoon/alexander-opener-multi-url`.

The directory sits outside `~/code` on purpose: code indexers and repo
walkers that sweep the checkout roots never see it, so worktrees do not show
up as duplicate repos. The flip side: searches over the canonical checkout
do not see a worktree's uncommitted work either.

Claiming is always explicit and always from the canonical checkout:

```
git -C <canonical> worktree add ~/.worktrees/<repo>/<slug> -b <branch>
```

Drop `-b` to claim an existing branch. Nothing auto-creates worktrees; a
session claims one when it starts a task and the claim ends when the branch
lands.

## States

The weaver classifies each worktree from git alone:

- **In flight** — the working tree is dirty (`git -C <wt> status
  --porcelain` has output). Someone is mid-task; leave it alone.
- **Ready** — clean, and ahead of the default branch
  (`git -C <wt> log --oneline <default>..HEAD` is non-empty), and its owner
  has reported it done. Cleanliness alone is not readiness: only the owner
  knows whether the last commit was the last commit.
- **Integrated** — the branch is reachable from the default branch
  (`git branch --merged <default>` lists it). The worktree is a leftover;
  remove it.
- **Abandoned** — clean or dirty, untouched for a long time, owner gone.
  Report it to the user with its branch and last-commit age; never remove
  or resolve it without their say-so.

## Integration

Default order is oldest ready branch first. When one branch builds on
another, the dependent branch is rebased onto its parent by its owner and
becomes ready only after the parent lands — dependencies are stated on the
wire channel or by the user, not stored anywhere.

Per ready branch, in its own worktree:

1. Update the default branch in the canonical checkout
   (`git -C <canonical> pull --ff-only`).
2. Rebase the branch onto the fresh default tip. Rebase, not merge
   commits: linear history keeps each path-scoped conventional commit
   meaningful to release tooling that reads the default branch
   (release-please here).
3. Re-run the repo's gates in the worktree. The rebase may have composed
   this branch with work it never saw.
4. Land it by repo policy:
   - **PR-only repos** (this one): push the branch, open a PR, merge it
     when the user directs. After the merge, pull the canonical checkout.
   - **Direct-main repos**: `git -C <canonical> merge --ff-only <branch>`,
     then push if the repo has a remote.

A rebase conflict goes back to the branch's owner, who resolves it in
their worktree and reports ready again; the weaver moves on to the next
branch meanwhile. Only when the owner session is gone may the weaver
resolve it, and only with the user's go-ahead.

## Teardown

After a branch is visible from the default branch:

```
git -C <canonical> worktree remove ~/.worktrees/<repo>/<slug>
git -C <canonical> branch -d <branch>
```

`branch -d` only — if git refuses, the branch is not actually merged and
force-deleting it would eat work. `git worktree prune` clears any stale
administrative entries left by a worktree removed by hand.

Before removing a worktree, harvest anything durable that points into it:
evidence-pinned assertions (kof) record absolute paths, so a pin made in a
worktree dies with it — re-pin against the canonical checkout.

## Doorbell, optional

Surveying on demand works. When sessions are live at the same time, the
weaver can open a wire channel and share the connection string; an owner
posts its branch name when it goes ready, and the weaver's blocking
`wire_read` wakes immediately instead of waiting for the next survey.

## Known frictions

Worktrees are still unusual enough locally that some tooling assumes one
checkout per repo:

- Path-matched write guards (belt's `write-internal-names`) match the
  canonical checkout path, so a write that is exempt there can be denied in
  a worktree. That is machine-config territory; a repo file cannot fix a
  guard (ADR-0012).
- `suspenders history clean` refuses to run while linked worktrees exist.
  Tear the loom down first.
- suspenders' pre-commit hook is safe: hooks resolve through
  `git rev-parse --git-path hooks`, which is shared across worktrees, so
  one `suspenders hook install` covers them all.
