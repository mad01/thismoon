# ADR-0008: belt gains advisory hints alongside deny-only guards

**Date:** 2026-07-18
**Status:** Accepted

**Scope:** `tools/belt`. Decided after a 14-day audit of local session
transcripts measured how often the documented tool conventions are actually
followed.

## Context

belt has one shape: a guard inspects a tool call before it runs and either
allows it or denies it with a reason. Every guard so far prevents something
hard to undo — a push to main, an internal name landing in a public repo.

Two new needs do not fit that shape.

The first is search-tool routing. The session audit found 518 bash commands
doing multi-file or recursive sweeps in a 14-day window, 502 of them (97%)
inside repos that csl already indexes. The interesting part is who was doing
it: subagents made 49% of all csl calls, so both subagents and main sessions
use csl heavily and still reach for `grep -r`. This is not a knowledge gap
that more documentation closes. The convention has been in CLAUDE.md, with a
grep-to-zoekt translation table, for months.

The second is keeper-of-facts (then named keep). Assertions only pay off when
a later session reads them, and in the same window 3 of 95 sessions touched
the store at all, with zero
organic queries. The store cannot earn its keep if nothing surfaces it at the
moment the question arises.

Denying was considered for both and rejected. A recursive grep in an indexed
repo returns the right answer; it costs some context and latency. Blocking it
would put a tooling preference in the same enforcement tier as a leaked
hostname, and a false positive would stall a session over a style rule. The
audit reinforced this: the behaviour is habit, not ignorance, and a deny that
fires on a very common command has a much worse failure mode than the problem
it corrects.

The Claude Code hooks reference confirms PostToolUse accepts
`hookSpecificOutput.additionalContext`, delivered as a system reminder placed
next to the tool result. That is an advisory channel with no error framing,
which is what both needs want and what belt currently has no way to use.

## Decision

belt carries two concepts:

- **guard** — PreToolUse. Returns a denial or nil. Guards run in registration
  order and the first denial wins. Reserved for damage that is hard to undo.
- **hint** — PostToolUse. Returns advisory text or nil. Every enabled hint
  runs and their output is concatenated into one `additionalContext` block.
  A hint has no denial path in its signature, so it cannot block a tool call
  even by mistake.

Two hints ship with this decision: `kof-assertions` surfaces stored
assertions matching the subject of a csl search, and `prefer-csl` hands back
the translated zoekt query after a multi-file sweep in an indexed repo.

Both hints emit to events.this on every fire, so the effect on sweep volume
is measurable without a separate counting phase.

`updatedToolOutput` is available on PostToolUse and is deliberately not used.
Rewriting a tool's result to inject commentary makes the tool untrustworthy
and risks breaking anything that parses the output.

## Consequences

- Adding a hint means implementing the `Hint` interface, registering it in
  `hint.ForEvent`, and adding a `[hints.<id>]` toggle to the consuming repo's
  belt config overlay. Same shape as adding a guard.
- Hints default to enabled when config is missing, matching guards. The
  failure modes are not symmetric: a missing config silently disables nothing
  in either case, but an over-eager hint costs context on every matching tool
  call, so relevance gating lives in the hint rather than in the config.
- Hook registration stays machine-private in the consuming repo (ADR-0006).
  The `hooks.PostToolUse` entries are not added here.
- A hint that fires too often becomes wallpaper and stops being read, which
  is a silent failure — nothing errors, the advice is simply ignored. The
  `kof-assertions` hint caps at three assertions, dedupes per session, and
  requires the search to have hit at least one path segment below the repo
  root. If those gates prove wrong, the fix is tighter gating, not a louder
  channel.
- Subject matching runs wider than the search and narrows afterwards.
  Assertions are labelled at whatever depth the session that wrote them chose,
  usually the component (`.../services/csl`), while a search returns hits
  deeper inside it (`.../services/csl/internal/semantic`). kof matches
  subjects by prefix, so querying the hit's own subject finds nothing. The
  hint queries the repo and ranks by shared path segments, dropping anything
  that overlaps only on a generic top segment like `services`.
- belt's short description is now inaccurate wherever it says the binary is
  only a PreToolUse guard. README, CLAUDE.md, and the cobra command help all
  need to describe both concepts.
