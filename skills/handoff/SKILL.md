---
name: handoff
description: Write a handoff document and persist key learnings to memory so the next agent (or future session) can continue this work without re-discovering context.
argument-hint: "What will the next session focus on?"
---

Create a handoff so work survives a context reset. The user will open a brand new Claude Code session and feed it the handoff document — that document is the only bridge. Anything not in the handoff or in durable memory is lost.

Save the handoff to a durable, agent-agnostic directory, not the workspace: `~/.handoff/handoff-<project>-<YYYYMMDD-HHMM>.md` (create the directory with `mkdir -p` if needed). `~/.handoff/` is shared across every coding agent, so a handoff written by one agent can be picked up by another (Claude Code, Codex, opencode, pi). **Do NOT use `$TMPDIR` or `/tmp`** — macOS prunes those, so a handoff left there is gone by the time the next session needs it. Never write it into the repo, and never write it under a single agent's private state directory (e.g. `~/.claude/`).

## Steps

### 1. Gather context

- Read the full conversation: what was the goal, what happened, what's left.
- If a prior handoff document was fed into this session, carry its still-relevant context forward — don't lose the chain.
- Remember: the next session starts cold — no conversation history, no tool results, no implicit context. The handoff must stand alone.

### 2. Persist learnings to memory

Before writing the handoff document, save anything from this session that future conversations should know — even ones that never read the handoff file. Use the auto-memory system (`~/.claude/projects/.../memory/`):

- **feedback** — corrections, validated approaches, user preferences discovered this session
- **project** — decisions made, constraints learned, who owns what, deadlines surfaced
- **user** — new context about the user's role, expertise, or working style
- **reference** — external resources, dashboards, docs, or links that proved useful

Skip memory writes when nothing non-obvious was learned. Don't save ephemeral task state (that goes in the handoff doc). Don't duplicate what's already in memory — check `MEMORY.md` first.

### 3. Write the handoff document

Write the handoff file with these sections:

```
# Handoff

## Goal
What we're trying to accomplish — the top-level objective.

## Current Progress
What's been done so far. Reference artifacts (PRs, commits, files, issues) by path or URL — don't duplicate their content. Link issues by their tracker key, whichever tracker the project keys on.

## What Worked
Approaches and strategies that succeeded — so the next agent repeats them.

## What Didn't Work
Approaches that failed and why — so the next agent doesn't retry them.

## Key Decisions
Decisions made during this session and their rationale.

## Suggested Skills
Skills the next agent should invoke to continue effectively (e.g. /golang-pro, /work-on, /ios-dev).

## Next Steps
Concrete action items for the next session, ordered by priority.
```

### 4. Tailor to arguments

If the user passed arguments, treat them as a description of what the next session will focus on. Emphasize the sections most relevant to that focus and add specific guidance for it.

## Rules

- **Redact sensitive data.** Strip API keys, passwords, tokens, and PII before writing.
- **Reference, don't repeat.** Link to PRDs, plans, ADRs, issues, commits, and diffs by path or URL. The handoff should be a map, not a copy.
- **Memory is for durable insights, handoff is for session state.** A decision rationale or user preference goes in memory. A list of "files left to edit" goes in the handoff doc.
- **Tell the user what was saved.** After writing, report: the handoff file path (so they can pass it to the next session), and a one-line summary of each memory written or updated.
