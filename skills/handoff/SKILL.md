---
name: handoff
description: Write a handoff document and persist key learnings to memory so the next agent (or future session) can continue this work without re-discovering context.
argument-hint: "What will the next session focus on?"
---

Create a handoff so work survives a context reset. The user will open a brand new Claude Code session and feed it the handoff document; that document is the only bridge. Anything not in the handoff or in durable memory is lost.

Save the handoff to a durable, agent-agnostic directory, not the workspace: `~/.handoff/handoff-<project>-<YYYYMMDD-HHMM>.md` (create the directory with `mkdir -p` if needed). `~/.handoff/` is shared across every coding agent, so a handoff written by one agent can be picked up by another (Claude Code, Codex, opencode, pi). Don't use `$TMPDIR` or `/tmp`: macOS prunes them, so a handoff left there is gone by the time the next session needs it. Never write it into the repo, and never write it under a single agent's private state directory (e.g. `~/.claude/`).

## Steps

### 1. Gather context

- Read the full conversation: what was the goal, what happened, what's left.
- If a prior handoff document was fed into this session, carry its still-relevant context forward. Don't lose the chain.

### 2. Persist learnings to memory

Before writing the handoff document, save what future sessions should know, even ones that never read the handoff file. Each fact goes to the one store that owns it. When the agent's global config defines memory routing, follow it; otherwise route like this:

- **A preference, a correction, or a fact every agent needs** (how the user wants work done, a tool or machine fact): the shared agent-memory store at `~/.config/agent-memory/`. Write one fact per file and add a line to its `MEMORY.md` index.
- **A fact about one repo** (a build gotcha, an ordering rule, a declined design): that repo's docs or `CLAUDE.md`, where the next agent in the repo reads it anyway.
- **Progress on the task** (where it stands, what comes next): the worklog item for the ticket or topic, through the `worklog` skill.
- **A project or harness gotcha the repo does not record** (how the code behaves at run time, a quirk of the coding agent itself): the agent's own auto-memory, and only that.

Never save a recap of shipped or resolved work, in any store: the tracker, the worklog and git history already hold it. Skip the step when nothing non-obvious was learned. Before adding an entry, read the target store's index and update an existing entry instead of writing a near-copy. State only the next session needs goes in the handoff document.

### 3. Write the handoff document

Write the handoff file with these sections:

```
# Handoff

## Goal
What we're trying to accomplish: the top-level objective.

## Current Progress
What's been done so far. Reference artifacts (PRs, commits, files, issues) by path or URL; don't duplicate their content. Link issues by their tracker key, whichever tracker the project keys on.

## What Worked
Approaches and strategies that succeeded, so the next agent repeats them.

## What Didn't Work
Approaches that failed and why, so the next agent doesn't retry them.

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
- **Stores are for durable facts, the handoff is for session state.** A user preference goes in shared memory and a decision's rationale in the repo's docs. A list of "files left to edit" goes in the handoff doc.
- **Tell the user what was saved.** After writing, report the handoff file path (so they can pass it to the next session) and one line per entry written or updated, naming its store.
