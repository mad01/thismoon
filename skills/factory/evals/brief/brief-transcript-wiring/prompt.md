---
name: brief-transcript-wiring
description: The repo-wiring part of the factory skill's own build, taken from the session that built it. Public-repo rules must turn into warnings.
tags: [brief, from-transcript]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are the orchestrator of a factory run. Write the task message for the plan unit below, the message you send to its worker. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

Run facts (a test fixture; everything you need is here):

- Repo: thismoon, a public repo. Canonical checkout: ~/code/src/github.com/mad01/thismoon.
- The worktree for this unit is claimed: ~/.worktrees/thismoon/alexander-mad-397-factory-skill-evals, on branch alexander/mad-397-factory-skill-evals.
- This is the first message for the unit.

Plan unit mad-397/wiring (work-on tier: known-domain):

- Add recipes/factory/recipe.toml as a skill-only recipe, on the pattern of recipes/loom/recipe.toml.
- Update the skills table and the skill count in the root CLAUDE.md.
- Add a row for the factory skill to the skills table in the root README.md, and mark the skill experimental there.
- Out of scope: everything under skills/factory/, and all Go source.
- Public-repo rules: no employer names, no internal hosts, no internal ticket prefixes, and no em dashes in prose.
- Gate: humanizer_detect on every markdown file the worker changed, with each warning fixed.
- Done when: the three changes are in place and the gate shows no warnings.
- Report back: the files changed, the gate output, and the commit range.
