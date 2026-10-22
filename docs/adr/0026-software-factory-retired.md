# ADR-0026: the software factory is retired; the mechanisms it drove stay

**Date:** 2026-10-22
**Status:** Accepted

**Scope:** removes `skills/factory`, `tools/factory-eval`, `mods/factory-band`
and their recipes, plus the factory-worker role in loom-pane's members line.
Amends ADR-0024: the ledger's consumers are loom-pane's members line and the
weaver's survey, not a factory orchestrator.
Decided in MAD-394 after the pilot in MAD-412.

## Context

The software factory was an attempt to run a plan through worker subagents.
One orchestrator session owned the plan. Each unit got its own loom worktree
and its own worker. Every message followed a fixed contract that humanizer
checked against Simplified Technical English (STE). belt recorded each
worker's start, stop and denials to a ledger, and a status-line band read
that ledger. A Go program ran the same task through the factory and through
a single agent, then judged both. All of it shipped between 2026-10-16 and
2026-10-19 across nineteen pull requests in this repo and the consuming repo,
each after one to three review rounds.

The keep-or-drop question was put to a measurement rather than a debate. The
pilot task was the component docs sweep: bring four component README and
CLAUDE files to the shared outline, one branch per component. Both arms ran
once at the mid tier (Sonnet, medium effort) on 2026-10-22 under the same
unattended permission profile.

What the run showed:

- The single agent delivered all four units with a judge score of 6.75 in
  14 minutes for 3 dollars.
- The factory delivered two of four with a judge score of 3.5 in 11 minutes
  for 3.69 dollars. On the two units a worker finished, its score equalled
  the single agent's score on the same unit.
- Both lost units died the same way. The worker ran one compound shell
  command: a loop with a process substitution in one case, a cd followed by a
  read in the other. The permission rules denied it. The worker sent blocked
  and stopped, as the brief told it to. Nobody can answer a permission
  question in dontAsk mode, and Claude Code's own denial text invites another
  route. The one worker that took another route (Write, Edit, the humanizer
  MCP tool) delivered.
- Parallel workers saved a fifth of the wall clock, not the half the pass
  criterion asked for. Each worker spent most of its 8 to 10 minutes on fixed
  overhead: reading the brief, claiming, running gates, composing the report.
  The units were too small for parallelism to pay.
- Nothing the factory added raised quality. What it added was governance:
  context isolation, one worktree per change, a checkable message, routing by
  tier, a ledger. A plain subagent fan-out into loom worktrees gets most of
  the isolation without the contract or the brief.

Fixing the brief and rerunning would have cost another round of paid runs.
The best case on offer was parity on quality and a modest saving on wall
clock. The owner stopped there.

## Decision

Remove the factory-specific code. That is the factory skill and its eval
suites, the factory-eval program, the factory band mod and their recipes. The
factory-worker role in loom-pane's members line and the factory-worker
examples in belt's docs and tests go with them. The factory-eval release line
ends at v0.4.0; its tags stay. The consuming repo removes its wiring in the same window. That
is the worker settings, agent definitions, routing, the hint script, the mod
enable, the agent-contract profile, and the per-unit model and effort fields
in work-on.

Keep what stands on its own:

- belt: agent identity in hook payloads, the payload deny, the ledger and
  `belt record`, custom hints gated on agent types with trailing-star keys,
  model and effort tags, and `belt watch`. Nothing writes the inbox source
  today; if no writer appears, a later ADR removes watch.
- humanizer: the `ste` command, its two profiles and the per-directory
  glossary, still labelled experimental. They check any brief or report an
  agent sends another agent.
- loom: ready as a message plus a ledger line, the weaver sweep of leaked
  worktree-agent branches, and squash-merge detection.
- present: the locked edit, section patch, progress reports and page history.

The pilot results file stays with the owner outside the repo as the record.
The code is in history at the parent of the removal commit.

## Consequences

ADR-0024 stands with smaller consumers: loom-pane's members line and the
weaver's survey read the ledger.

This is the second multi-agent scheme to lose a pilot to one agent. The
consuming repo's ADR-0015 (June 2026) records the first, a declarative
workflow interpreter that lost an equal-budget eval on research tasks. Both
times the single agent won on small, read-heavy or docs-heavy units. The next
proposal of this kind should name a task where the units are large enough for
parallel workers to beat one agent, and run that pilot first.

The permission-rule findings from the factory work stay in factory-eval's
docs at the parent commit and in the Linear tickets under MAD-394. In short:
Bash rules are whole-command globs, and a cd outside the session cwd fails
the whole compound in dontAsk mode. Sandbox exclusion lists merge across
settings scopes, and a deny list is never a boundary.

A retry starts by fixing the worker brief's rule that a denied call means
stop, and reruns the pilot before building anything new. Larger units are the
other precondition: parallelism needs work that takes longer than the fixed
cost of a worker.

The experiment paid for itself in what it left behind. The ledger, the
agent-type gate on hints, the STE check and the present store primitives all
came out of it. And the question was answered with a measurement.
