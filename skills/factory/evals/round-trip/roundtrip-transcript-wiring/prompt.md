---
name: roundtrip-transcript-wiring
description: The repo-wiring unit of the factory skill's own build, task and log taken from the session that built it. Done, with five paths to report.
tags: [round-trip, from-transcript]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are a factory worker. Below are the task message you received and your work log. Write the message you send to the orchestrator now. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

The task message you received:

kind: task
unit: mad-397/wiring
worktree: ~/.worktrees/thismoon/alexander-mad-397-factory-skill-evals
branch: alexander/mad-397-factory-skill-evals
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit files under skills/factory/.
Do not edit Go source files.
Do not write employer names, internal hosts or internal ticket prefixes.
Do not use em dashes in prose.

1. Add recipes/factory/recipe.toml on the pattern of recipes/loom/recipe.toml.
2. Add recipes/factory/CLAUDE.md and its AGENTS.md symlink.
3. Add the factory skill to the skills table in the root CLAUDE.md, and update the skill count.
4. Add a factory row to the skills table in the root README.md, marked experimental.
5. Run humanizer_detect on each markdown file you changed, and fix each warning.
6. Send ready when the gate shows no warnings.

Report format: one ready message. List the files, the gate output and the commit range.

Your work log (a test fixture; use these facts and nothing else):

15:10 Wrote recipes/factory/recipe.toml from the loom recipe, with the factory skill paths.
15:12 Wrote recipes/factory/CLAUDE.md. Added recipes/factory/AGENTS.md as a symlink to CLAUDE.md.
15:15 Root CLAUDE.md: the count now says eight skills, and the table has a factory row.
15:17 Root README.md: the skills table has a factory row, marked experimental.
15:20 humanizer_detect on recipes/factory/CLAUDE.md, CLAUDE.md and README.md: 0 warnings, 2 suggestions (judgment calls, left as they are).
15:24 Committed. Range d3a90b1..4f6c2e8. git status is clean.
