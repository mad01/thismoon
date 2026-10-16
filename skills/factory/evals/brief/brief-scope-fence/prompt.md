---
name: brief-scope-fence
description: The plan mentions a neighbouring unit's file. The task must keep it out of the steps.
tags: [brief, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are the orchestrator of a factory run. Write the task message for the plan unit below, the message you send to its worker. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

Run facts (a test fixture; everything you need is here):

- Repo: thismoon. Canonical checkout: ~/code/src/github.com/mad01/thismoon.
- The worktree for this unit is claimed: ~/.worktrees/thismoon/alexander-docs-sweep-deps, on branch alexander/docs-sweep-deps.
- This is the first message for the unit.

Plan unit docs-sweep/deps (work-on tier: known-domain):

- Scope: README.md and CLAUDE.md of the deps service.
- Changes: reorder the sections of both files to match the shared outline. Replace each banned synonym with its glossary term.
- Note from the plan author: services/csl/README.md has the same section-order problem. Unit docs-sweep/csl owns that file and runs after this one.
- Out of scope: Go source files and recipe TOML files.
- Gate: the humanizer rule pack, run on both files.
- Done when: both files pass the rule pack.
- Report back: the files changed, and each term the worker could not resolve.
