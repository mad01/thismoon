---
name: report-ready-clean
description: A finished Go unit with two gates and nothing unresolved.
tags: [report, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Your work on the unit below has stopped, finished or blocked. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: belt/doctor-ledger-warn
worktree: ~/.worktrees/thismoon/alexander-belt-doctor-ledger-warn
branch: alexander/belt-doctor-ledger-warn
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not change the config schema.
Do not commit outside your worktree.

1. Open tools/belt/internal/cli/doctor.go.
2. Add a warning when ledger.source names the inbox source.
3. Add a table test for the new warning.
4. Run make -C tools/belt test and make -C tools/belt lint.
5. Send ready when both gates pass.

Report format: one ready message with files, gates and commit range.

What happened (a test fixture; use these facts and nothing else):

- You changed tools/belt/internal/cli/doctor.go and tools/belt/internal/cli/doctor_test.go.
- `make -C tools/belt test` ended with: ok  	github.com/mad01/thismoon/tools/belt/internal/cli	0.412s
- `make -C tools/belt lint` ended with: 0 issues.
- You committed the work as two commits. The range is 8c1d0e2..b47f9a3, and the worktree is clean.
