---
name: roundtrip-sample-not-done
description: The spec sample task and a work log in which one file still fails. The report must say the done criterion does not hold.
tags: [round-trip, from-spec]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Below are the task message you received and your work log. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

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

Your work log (a test fixture; use these facts and nothing else):

10:01 Opened services/events/README.md and services/events/CLAUDE.md in the worktree.
10:04 Reordered both files to the shared outline.
10:09 Replaced "daemon" with "service" once in CLAUDE.md. No other banned synonyms.
10:13 humanizer detect services/events/CLAUDE.md: no findings.
10:13 humanizer detect services/events/README.md: 1 finding, Humanizer.PromotionalLanguage on "seamless".
10:15 "seamless" sits inside an error message that README.md quotes word for word from services/events/internal/server/server.go. Changing the README quote makes it differ from what the service prints. Changing the Go string is out of scope.
10:20 Committed the reorder and the synonym fix. Range 3f1c2ab..c81e5d0. git status is clean.
