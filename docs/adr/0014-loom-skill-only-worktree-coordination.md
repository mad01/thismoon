# ADR-0014: worktree coordination is skill-only; git is the state store

**Date:** 2026-09-01
**Status:** Accepted

**Scope:** `skills/loom` (replaces `skills/commit-pipeline`). Decided in
MAD-343.

## Context

commit-pipeline serialized commits because every session shared one working
tree: requesters queued request files under `~/.commits/` and one committer
session landed them with exact staging. Git worktrees dissolve the shared
tree — each session commits freely in its own worktree — and the remaining
coordination problem is merge order. The open question was whether that
coordinator should be a skill, a CLI tool, or a service with a store.

## Decision

Skill-only. The coordinator is a role (the weaver) plus conventions, and
every piece of coordinator state is derived from git at the moment it is
needed: `git worktree list` for what exists, `git status` for what is in
flight, `git log <default>..branch` for what is ready, `git branch
--merged` for what is done. No store, no queue, no daemon, no port.

Two things drove it. The platform has no lock, lease, or queue primitive,
deliberately — wire's non-goals refuse queue semantics and work routing,
worklog's remote assumes a single writer — and a coordinator service would
have introduced the first one for a workflow that a role covers. And the
decision is cheap to revisit: because the model defines state as
derived-from-git, a later CLI or service would automate the same derivation
rather than migrate a store, so nothing about going skill-first is load-
bearing if real complexity (cross-machine looms, enforced ordering) ever
shows up.

## Consequences

- Nothing enforces the weaver's exclusivity. Two sessions both weaving is a
  convention violation the skill warns about, not a blocked action —
  acceptable for the same reason the committer role was never enforced.
- The `~/.commits/` queue and its request-file format are gone entirely.
- Tooling that assumes one checkout per repo (belt's path-matched write
  guard, kof's absolute-path pins, `suspenders history clean`) is
  documented as known friction in the skill rather than fixed as part of
  this change.
