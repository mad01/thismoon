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
its own worktree) makes this session a worktree owner; `weave` (or the user
saying this session integrates) makes it the weaver. `status` surveys the
loom without taking a role.

## Owner (a session with a claimed worktree)

1. Claim: from the canonical checkout, create the worktree and branch —
   `git worktree add ~/.worktrees/<repo>/<slug> -b <branch>` (slug is the
   branch name with `/` replaced by `-`). One worktree per task.
2. Work entirely inside the worktree. Commit there directly, conventional
   format, staging only the files you changed. No queue, no request files.
3. When the branch is done: run the repo's gates in the worktree, then
   report the branch as ready — to the weaver on the loom's wire channel if
   one was shared, otherwise to the user. Do not integrate it yourself.
4. Stop. The weaver owns the branch from here; your worktree disappears
   after the branch lands.

## Weaver (the one session that integrates)

1. Survey: `git worktree list --porcelain` from the canonical checkout,
   then classify each worktree per `loom.md` — in flight, ready, abandoned,
   or integrated. No worktrees: say so and stop.
2. For each ready branch, in order: rebase it onto the default branch tip
   in its own worktree, re-run the gates, then land it by repo policy —
   push + PR for PR-only repos, fast-forward merge for direct-main repos.
   A rebase conflict hands the branch back to its owner; it is not yours
   to resolve.
3. After a branch lands: remove its worktree, delete the branch, update
   the canonical checkout.
4. Report one line per worktree: branch, state, and what happened or what
   blocks it.

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
- Branch deletion is `-d` after the merge is visible from the default
  branch, never `-D`.
