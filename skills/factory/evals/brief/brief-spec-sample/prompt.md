---
name: brief-spec-sample
description: The plan unit behind the spec's sample task. The sample itself must pass every grader here.
tags: [brief, from-spec]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are the orchestrator of a factory run. Write the task message for the plan unit below, the message you send to its worker. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

Run facts (a test fixture; everything you need is here):

- Repo: thismoon. Canonical checkout: ~/code/src/github.com/mad01/thismoon.
- The worktree for this unit is claimed: ~/.worktrees/thismoon/alexander-docs-sweep-events, on branch alexander/docs-sweep-events.
- This is the first message for the unit.

Plan unit docs-sweep/events (work-on tier: known-domain):

- Goal: bring the events service docs in line with the shared outline and the glossary.
- Scope: README.md and CLAUDE.md of the events service.
- Changes: reorder the sections of both files to match the shared outline. Replace each banned synonym with its glossary term.
- Out of scope: Go source files and recipe TOML files.
- Commits stay inside the worktree.
- Gate: the humanizer rule pack, run on both files.
- Done when: both files pass the rule pack.
- Report back: the files changed, and each term the worker could not resolve.
