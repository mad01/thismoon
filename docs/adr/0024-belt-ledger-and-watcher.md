# ADR-0024: belt records lifecycle events, and one command wakes sessions

**Date:** 2026-10-16
**Status:** Accepted

**Scope:** `tools/belt`. Amends ADR-0008. Decided in MAD-395, the hook
layer for running one orchestrator session with worker subagents and
background sessions in git worktrees.

**Amended by ADR-0026 (2026-10-22):** the factory orchestrator this was
decided for is retired. The ledger's consumers are loom-pane's members line
and the weaver's survey; watch stays with no writer for its inbox today.

## Context

A session that hands work to subagents and parallel sessions loses sight of
them. Nothing records which worker started where, what it reported when it
stopped, which tool call auto mode denied it, or when it went idle. And an
orchestrator that finishes its turn stays asleep until a human types, even
when a worker has just reported back.

Claude Code exposes all of this as hooks: SubagentStart, SubagentStop,
TaskCompleted, TeammateIdle, PermissionDenied, Notification, PreCompact,
PostCompact and SessionEnd. It also offers asyncRewake, a background hook
whose exit 2 wakes an idle session with the hook's stderr as a system
reminder. belt is already the binary those hooks would exec (why.md): it
reads a payload on stdin and answers through a fixed contract.

The trap is that many of these hooks steer the session through output that
looks harmless. On SubagentStop, additionalContext keeps the subagent
running. On TaskCompleted, TeammateIdle and PreCompact, exit 2 blocks the
task, the idle or the compaction. PermissionDenied reads its output for a
retry flag. WorktreeCreate and WorktreeRemove replace Claude Code's own
`git worktree` handling outright: a hook there that prints nothing makes
worktree creation fail, and one that exits 0 counts as the removal. So a
hook meant only to observe can break a session as surely as a broken guard
can.

## Decision

belt carries a third concept beside guards and hints. ADR-0008's "belt
carries two concepts" now reads three:

- A ledger entry records one lifecycle event to the local events service,
  under its own `belt-ledger` source. Each entry is tagged with the session
  and agent ids and with the repo, linked worktree, main checkout and branch
  the event happened in. It never reaches the model and never steers the
  session.

`belt record` takes no output writer. The entrypoint gets stdin and nothing
to write to, so no future change can make it emit context or a retry flag
by accident. This is the same structural argument ADR-0008 made
for hints, whose interface has no denial path. It also recovers its own
panics, since an escaped panic exits 2 with a stack trace on stderr, which
would block those events and hand the trace to the model. The config may
switch the ledger off or rename its source and nothing else. A config file
belt cannot read records with the defaults: the ledger is not a guard, and
an extra entry is harmless where a missing deny is not.

belt never registers on WorktreeCreate or WorktreeRemove. Observing them
would mean taking over worktree creation. Those hooks also never fire for a
`git worktree add` run through Bash, which is how loom worktrees are made
(ADR-0014). The ledger reads the worktree from git on every entry instead.

`belt watch` is the one belt command that exits 2. It runs as an
asyncRewake hook on Stop and SessionStart. It polls the events service for
entries addressed to the session, meaning a `to` tag equal to the session
id, and exits 2 with them on stderr. On asyncRewake exit 2 blocks nothing:
the turn it would have blocked has already ended, and the exit is the wake
signal. Every other outcome exits 0 silently. The watcher is opt-in
(`watch.enabled`), because it holds a process per idle session for up to
its max wait. Delivery is at most once, because the cursor moves before the
wake. If the cursor cannot be written, nothing is delivered: an item
delivered without one would wake the session again on every Stop.

The wake text crosses a trust boundary. The title, message and `from` tag
are whatever the producer posted, and belt verifies none of them. Anything
that can reach the events service can address a session and name any
sender. The woken session reads the text as data a hook delivered, the way
it reads a tool result, and not as an instruction from the user.

There is no store and no port. The events service is the inbox and the
cursor is a cache file beside the hints' seen files. That keeps all of this
inside belt rather than in a new service.

The `ledger` and `watch` config values are resolved, never rejected. A
config error makes every guarded tool call deny, which is far too much for
a typo in a feature that may be off. Out-of-range numbers are clamped,
source names are rewritten the way the events service stores them, and
`belt doctor` warns about each adjustment.

## Consequences

- "belt always exits 0" gains one documented exception. Every doc that
  states the rule names `belt watch` as it.
- The ledger and the watcher depend on the events service. When it is down
  entries are dropped and no session wakes; `belt doctor` probes it and
  warns when an enabled watcher cannot work.
- The watcher is a delivery mechanism, not a queue. It has no
  acknowledgement, no retry and no routing beyond the `to` tag, matching
  ADR-0014's refusal of queue semantics. Needing any of those would mean a
  store, and with it a separate tool and a new decision.
- The events service caps each source's in-memory history, so an inbox
  item left unread long enough can age out. The separate sources keep a busy
  ledger from evicting guard denials, and inbox items from either.
- Adding a lifecycle event means adding it to the ledger catalog and to the
  hooks block. It never means writing to stdout.
