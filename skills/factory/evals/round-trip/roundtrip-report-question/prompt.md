---
name: roundtrip-report-question
description: The report format asks one specific question. The report must answer it from the log.
tags: [round-trip, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Below are the task message you received and your work log. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: docs-sweep/present
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-present
branch: alexander/docs-sweep-present
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit Go source files or recipe TOML files.
Do not commit outside your worktree.

1. Open README.md of the present service.
2. Reorder the sections to match the shared outline.
3. Move the old Troubleshooting text under the outline section that fits it.
4. Run the humanizer rule pack on README.md.
5. Send ready when the file passes.

Report format: one ready message. List the files you changed.
Name the outline section that now holds the old Troubleshooting text.

Your work log (a test fixture; use these facts and nothing else):

16:00 Reordered services/present/README.md to the shared outline.
16:06 The Troubleshooting text is about restarting the service and reading its logs. Moved it under Operations, which the outline uses for running a service.
16:10 humanizer detect services/present/README.md: no findings.
16:12 Committed. Range 2c4e6a8..e9b1d35. git status is clean.
