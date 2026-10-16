---
type: llm
weight: 2
---
The reply is one task message from a factory orchestrator to a worker. You see only the reply, so the fixture it was written from is repeated here.

Fixture: plan unit mad-397/wiring in the public thismoon repo, work-on tier known-domain. Worktree ~/.worktrees/thismoon/alexander-mad-397-factory-skill-evals on branch alexander/mad-397-factory-skill-evals. The changes:

- Add recipes/factory/recipe.toml as a skill-only recipe on the pattern of recipes/loom/recipe.toml.
- Update the skills table and skill count in the root CLAUDE.md.
- Add a factory row to the root README.md skills table, marked experimental.

The rest of the fixture: Out of scope: skills/factory/ and all Go source. Public-repo rules: no employer names, internal hosts, internal ticket prefixes, or em dashes in prose. Gate: humanizer_detect on each changed markdown file, warnings fixed. Done when the three changes are in place and the gate shows no warnings. Report back: files changed, gate output, commit range.

PASS if all of these hold:

1. What: the steps cover all three changes: the recipe, the CLAUDE.md table and count, and the README.md row marked experimental.
2. Where: the steps name the recipe path, the root CLAUDE.md and the root README.md, and the header names the worktree and branch above.
3. Done criterion: a step says to send ready when the changes are in place and humanizer_detect shows no warnings.
4. Report format: the message ends with a line that starts with "Report format:" and asks for files, gate output and commit range.
5. Warnings: "Do not" lines before step 1 cover the out-of-scope paths and the public-repo rules.
6. Nothing outside the unit: no step edits skills/factory/ or Go source, and none asks for a push, a pull request or a merge.

FAIL if any of these hold:

- the reply is not a task message
- one of the three changes is missing
- the public-repo rules are absent
- there is no done criterion

Do not fail for any of these:

- a report-format line that spans two lines
- the public-repo rules split across several "Do not" lines
- a step that names recipes/handoff/recipe.toml as a second pattern
- the gate written as a CLI command instead of the MCP tool name
