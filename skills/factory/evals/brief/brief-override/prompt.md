---
name: brief-override
description: A known-domain unit the plan overrides to the top model at high effort. The header must carry the override, not the default.
tags: [brief, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are the orchestrator of a factory run. Write the task message for the plan unit below, the message you send to its worker. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

Run facts (a test fixture; everything you need is here):

- Repo: thismoon. Canonical checkout: ~/code/src/github.com/mad01/thismoon.
- The worktree for this unit is claimed: ~/.worktrees/thismoon/alexander-release-mods-paths, on branch alexander/release-mods-paths.
- This is the first message for the unit.

Plan unit release/mods-paths (work-on tier: known-domain):

- Routing override: top model at high effort. The change edits release-please-config.json, which drives every component's releases.
- Change: exclude mods/ from the root package paths in release-please-config.json, so a mod edit bumps no component.
- Constraints: .release-please-manifest.json is never edited by hand. No component CHANGELOG.md changes.
- Gates: `jq empty release-please-config.json` exits 0, and `make test` passes.
- Done when: mods/ is excluded and both gates pass.
- Report back: the files changed, the gates with their last lines, and the commit range.
