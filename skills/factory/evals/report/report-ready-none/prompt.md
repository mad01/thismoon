---
name: report-ready-none
description: The report format asks for unresolved terms and there are none. The field must still be there, with none.
tags: [report, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Your work on the unit below has stopped, finished or blocked. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: docs-sweep/speak
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-speak
branch: alexander/docs-sweep-speak
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit Go source files or recipe TOML files.
Do not commit outside your worktree.

1. Open README.md and CLAUDE.md of the speak service.
2. Reorder the sections to match the shared outline.
3. Replace each banned synonym with the glossary term.
4. Run the humanizer rule pack on both files.
5. Send ready when both files pass.

Report format: one ready message. List the files you changed.
List each term you could not resolve.

What happened (a test fixture; use these facts and nothing else):

- CLAUDE.md already followed the outline and used no banned synonym, so you left it unchanged.
- You changed services/speak/README.md only.
- `humanizer detect services/speak/README.md` printed: no findings
- `humanizer detect services/speak/CLAUDE.md` printed: no findings
- You resolved every term.
- You committed the work. The range is 5be0c91..0a7f3d4, and the worktree is clean.
