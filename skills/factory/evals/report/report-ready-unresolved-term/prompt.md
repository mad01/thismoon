---
name: report-ready-unresolved-term
description: The finished spec sample unit with one unresolved term. The skill's sample ready message must pass every regex grader here.
tags: [report, from-spec]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Your work on the unit below has stopped, finished or blocked. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: docs-sweep/events
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events
branch: alexander/docs-sweep-events
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit Go source files or recipe TOML files.
Do not commit outside your worktree.

1. Open README.md and CLAUDE.md of the events service.
2. Reorder the sections to match the shared outline.
3. Replace each banned synonym with the glossary term.
4. Run the humanizer rule pack on both files.
5. Send ready when both files pass.

Report format: one ready message. List the files you changed.
List each term you could not resolve.

What happened (a test fixture; use these facts and nothing else):

- You changed services/events/README.md and services/events/CLAUDE.md.
- `humanizer detect services/events/README.md` printed: no findings
- `humanizer detect services/events/CLAUDE.md` printed: no findings
- You committed the work as one commit. The range is 3f1c2ab..9e04d17, and the worktree is clean.
- One term stays unresolved: "sink". The glossary has no entry for it, so you kept the word.
