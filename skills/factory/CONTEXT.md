# factory

Running a plan with worker subagents on top of the loom. These terms apply to
the files under `skills/factory/`; the repo-wide terms are in the root
CONTEXT.md.

The factory builds on the loom vocabulary, such as the weaver and the claimed
worktree, but those terms stay in `skills/loom/CONTEXT.md` by design. A
path-scoped check of a file under `skills/factory/` applies this glossary and
the root one only. A factory worker message is checked without a path, so it
gets every glossary, the loom one included.

## Language

**Factory**:
The pattern the factory skill runs on top of the loom. One orchestrator splits a plan into units and hands each to a worker in its own claimed worktree. The weaver lands the finished branches. Experimental.
_Avoid_: swarm, agent pipeline

**Unit**:
One piece of a factory plan: one change with one done criterion, one branch, one claimed worktree and one worker. Every message header names it as `<plan>/<slug>`.
_Avoid_: work item, subtask

**Orchestrator**:
The session that runs a factory plan. It splits the plan into units, routes them, claims their worktrees, writes the task messages, spawns the workers, parks questions and keeps the board. It may act as the weaver too.
_Avoid_: coordinator, dispatcher

**Worker**:
A subagent that owns one unit and its claimed worktree. It follows its task message, commits in the worktree and answers ready or blocked. It never integrates.
_Avoid_: helper agent, child agent

**Board**:
The present page an orchestrator keeps for one factory run: one row per unit and a Questions section for parked questions.
