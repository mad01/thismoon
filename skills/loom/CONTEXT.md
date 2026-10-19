# loom

Worktree coordination for parallel agent sessions. These terms apply to the
files under `skills/loom/`; the repo-wide terms are in the root CONTEXT.md.

## Language

**Loom**:
The worktree coordination pattern the loom skill runs: each agent session claims its own git worktree under `~/.worktrees/` and commits there, and one weaver lands the finished branches in order. All state is derived from git; nothing is stored.
_Avoid_: commit pipeline (the retired queue model), merge queue

**Claimed worktree**:
A linked git worktree at `~/.worktrees/<repo>/<slug>`, created explicitly for one task and owned by exactly one session until its branch lands and the worktree is removed.
_Avoid_: sandbox, workspace, checkout (that means the canonical one)

**Weaver**:
The single session that integrates a loom's ready branches: rebase onto the default branch tip, gates, then PR or fast-forward merge by repo policy. A role, not a process: nothing enforces its exclusivity.
_Avoid_: committer (the retired queue model's role), merger
