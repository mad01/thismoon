---
name: report-blocked-missing-worktree
description: The task names a build worktree that was removed after its branch merged. Taken from the session that built this skill. The worker must ask, not switch sources on its own.
tags: [report, from-transcript]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Your work on the unit below has stopped, finished or blocked. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: mad-397/ste-gate
worktree: ~/.worktrees/thismoon/alexander-mad-397-factory-skill-evals
branch: alexander/mad-397-factory-skill-evals
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit files in the humanizer worktree.
Do not commit outside your worktree.

1. Build humanizer from ~/.worktrees/thismoon/alexander-mad-396-humanizer-ste-lite into /tmp/humanizer-ste.
2. Run /tmp/humanizer-ste ste --profile procedure on the three sample messages in skills/factory/contract.md.
3. Fix each finding in the samples.
4. Commit the fixes.
5. Send ready when the samples pass.

Report format: one ready or blocked message.

What happened (a test fixture; use these facts and nothing else):

- `cd ~/.worktrees/thismoon/alexander-mad-396-humanizer-ste-lite` failed with: no such file or directory
- `git log --oneline -1 origin/main` in the canonical checkout printed: 35c7e3a feat(humanizer): STE profiles and repo glossary for agent-to-agent text (#218)
- The humanizer on PATH reports version 35c7e3a and has the ste command.
- You built nothing, ran no check and changed no file.
