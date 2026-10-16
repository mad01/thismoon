---
name: roundtrip-sample-done
description: The spec sample task and a work log in which the done criterion holds. The report must answer the brief, term included.
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

09:02 Opened services/events/README.md and services/events/CLAUDE.md in the worktree.
09:05 README.md: moved Configuration above Usage, per the shared outline. CLAUDE.md: moved Operations below Develop.
09:11 Replaced "daemon" with "service" twice in README.md. The root glossary bans daemon.
09:12 Found "sink" in README.md. The glossary has no entry for it. Kept the word.
09:15 humanizer detect services/events/README.md: 2 findings, both Humanizer.Signposting. Rewrote the two sentences.
09:16 humanizer detect services/events/README.md: no findings.
09:16 humanizer detect services/events/CLAUDE.md: no findings.
09:18 Committed "docs(events): follow the shared outline and glossary". Range 3f1c2ab..9e04d17. git status is clean.
