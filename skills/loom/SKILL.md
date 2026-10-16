---
name: loom
description: Coordinate parallel agent sessions on one repo with git worktrees. Each session claims its own worktree under ~/.worktrees and commits there directly; one weaver session rebases the finished branches and lands them on the default branch in order. Use when the user says "loom", "claim a worktree", "act as the weaver", or when several sessions would otherwise share one checkout.
argument-hint: "claim <branch> | status | weave (act as the integrator)"
---

When several sessions work one repo in parallel, each gets its own git
worktree and the coordination point is the merge, not the commit. This skill
has two roles; the worktree layout, state model, and integration rules live
in `loom.md` beside this file. Read it before acting in either role.

Pick the role from the arguments or context: `claim` (or a task that needs
its own worktree) makes this session a worktree owner. `weave` (or the user
saying this session integrates) makes it the weaver. `status` surveys the
loom without taking a role.

## Owner (a session with a claimed worktree)

1. Claim: from the canonical checkout, create the worktree and branch:
   `git worktree add ~/.worktrees/<repo>/<slug> -b <branch>` (slug is the
   branch name with `/` replaced by `-`). One worktree per task.
2. Work entirely inside the worktree. Commit there directly, conventional
   format, staging only the files you changed. No queue, no request files.
3. When the branch is done: run the repo's gates in the worktree, commit,
   then send the weaver a factory ready message over SendMessage, to the
   task's `reply_to`. The factory skill owns the message's header and body;
   the weaver reads three fields from it: files changed, gates run (command
   and last line), and commit range. That format is defined in
   `skills/factory`, not here; loom depends on it for those three fields
   only. When the owner is a subagent, belt's ledger also records its
   stop. With no weaver session to address, report ready to the user
   instead. Do not integrate the branch yourself.
4. Stop. The weaver owns the branch from here; your worktree disappears
   after the branch lands.

## Weaver (the one session that integrates)

1. Survey: read the ready messages that reached you and the repo's
   belt-ledger entries, then `git worktree list --porcelain` from the
   canonical checkout, and classify each worktree per `loom.md`: in
   flight, ready, abandoned, or integrated. A ready message is a claim.
   The proof is git: `git log origin/main..<branch>` lists the claimed
   commits and `git -C <worktree> status --porcelain` is empty. Check
   integrated before ready: the branch has at least one commit beyond
   main, and `git merge-tree --write-tree origin/main <branch>` prints the
   tree id `git rev-parse 'origin/main^{tree}'` prints. A squash-merged
   branch passes it while its commits still read as ahead. When main has
   changed the branch's lines since, that check conflicts; `loom.md` walks
   main's first-parent commits for the squash instead. A clean worktree
   with no commits beyond main is not integrated: it is a fresh claim or a
   fast-forward leftover, and git cannot tell them apart. Report it.
   No worktrees and no `worktree-agent-*` branches: say so and stop.
2. For each ready branch, in order: rebase it onto the default branch tip
   in its own worktree, re-run the gates, then land it by repo policy.
   Push + PR for PR-only repos, fast-forward merge for direct-main repos.
   A rebase conflict hands the branch back to its owner; it is not yours
   to resolve.
3. After a branch lands, and for every worktree the survey found
   integrated: remove its worktree and delete the branch per `loom.md`
   Teardown, with the proof printed above the delete. A clean worktree
   with no commits beyond main goes only on its owner's ready message or
   the user's go-ahead. Then update the canonical checkout.
4. Sweep: list the `worktree-agent-*` branches and `.claude/worktrees/`
   entries the Agent tool left behind, per `loom.md`, and offer those with
   nothing beyond main or integrated. Remove them only on the user's
   explicit go-ahead.
5. Report one line per worktree: branch, state, and what happened or what
   blocks it. Add the sweep listing.

## Rules

- The canonical checkout stays on the default branch while the loom is
  active; nobody edits files there. Sessions work in worktrees only.
- Never touch another session's worktree: no commits, no cleanup, no
  conflict resolution. Abandoned worktrees are reported to the user, never
  removed on your own call.
- Never push the default branch on PR-only repos; on direct-main repos only
  the weaver's fast-forward integration touches it.
- Commit messages follow conventional-commit format and carry no
  attribution trailers.
- Branch deletion is `-d` once the branch is integrated. `-D` only when
  `-d` refuses a squash-merged branch, and only after the merge-tree proof
  printed equal tree ids on the lines above it. The proof needs
  `git merge-tree --write-tree`, so git 2.38 or later; on an older git,
  keep the branch and report it. Never `-D` in any other case.
