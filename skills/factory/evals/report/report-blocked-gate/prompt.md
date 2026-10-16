---
name: report-blocked-gate
description: A gate fails on a test the change did not touch. Not ready, so the worker sends blocked with a question.
tags: [report, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Your work on the unit below has stopped, finished or blocked. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: deps/osv-timeout
worktree: ~/.worktrees/thismoon/alexander-deps-osv-timeout
branch: alexander/deps-osv-timeout
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit files outside services/deps.
Do not commit outside your worktree.

1. Add a 10 second timeout to each OSV query.
2. Add a test for a query that times out.
3. Run make -C services/deps test.
4. Commit when the gate passes.
5. Send ready.

Report format: one ready or blocked message.

What happened (a test fixture; use these facts and nothing else):

- Steps 1 and 2 are done in services/deps/internal/osv/client.go and services/deps/internal/osv/client_test.go.
- `make -C services/deps test` ended with: FAIL	github.com/mad01/thismoon/services/deps/internal/scan	0.913s
- The failing test is TestScanRepoCachesAdvisories in services/deps/internal/scan. It does not call the code you changed.
- Your new timeout test passes.
- You did not commit, because the gate failed.
