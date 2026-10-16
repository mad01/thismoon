---
name: report-blocked-denial
description: A denied install step. The skill's sample blocked message must pass every regex grader here.
tags: [report, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Your work on the unit below has stopped, finished or blocked. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: csl/prune-shards
worktree: ~/.worktrees/thismoon/alexander-csl-prune-shards
branch: alexander/csl-prune-shards
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit files outside services/csl.
Do not commit outside your worktree.

1. Add a test for a shard whose repo left discovery.
2. Delete such shards in the index refresh.
3. Run make -C services/csl test.
4. Run make -C services/csl install.
5. Check that the /version commit of csl matches your head commit.
6. Send ready when the check matches.

Report format: one ready or blocked message.

What happened (a test fixture; use these facts and nothing else):

- Steps 1 to 3 are done. `make -C services/csl test` ended with: ok
- You committed steps 1 and 2 as 41aa9c0..7d2e115.
- Step 4 was denied. Claude Code printed exactly: Permission to use Bash with command make -C services/csl install has been denied.
- You did not retry the call and did not run steps 5 and 6.
