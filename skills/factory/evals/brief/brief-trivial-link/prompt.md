---
name: brief-trivial-link
description: A trivial unit, one broken link. Routes to the mid model at low effort.
tags: [brief, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are the orchestrator of a factory run. Write the task message for the plan unit below, the message you send to its worker. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

Run facts (a test fixture; everything you need is here):

- Repo: thismoon. Canonical checkout: ~/code/src/github.com/mad01/thismoon.
- The worktree for this unit is claimed: ~/.worktrees/thismoon/alexander-adr-links-opener, on branch alexander/adr-links-opener.
- This is the first message for the unit.

Plan unit adr-links/opener (work-on tier: trivial):

- Problem: tools/opener/README.md links to docs/adr/0014-loom.md, which does not exist.
- Change: point the link at docs/adr/0014-loom-skill-only-worktree-coordination.md.
- Out of scope: every other file.
- Gate: `humanizer detect tools/opener/README.md` reports no findings.
- Done when: the link points at the existing ADR file and the gate is clean.
- Report back: the files changed.
