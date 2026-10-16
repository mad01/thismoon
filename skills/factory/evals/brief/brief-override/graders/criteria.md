---
type: llm
weight: 2
---
The reply is one task message from a factory orchestrator to a worker. You see only the reply, so the fixture it was written from is repeated here.

Fixture: plan unit release/mods-paths, work-on tier known-domain, with a routing override to the top model (alias opus) at high effort. Worktree ~/.worktrees/thismoon/alexander-release-mods-paths on branch alexander/release-mods-paths. Change: exclude mods/ from the root package paths in release-please-config.json. Constraints: never hand-edit .release-please-manifest.json; no component CHANGELOG.md changes. Gates: `jq empty release-please-config.json` exits 0 and `make test` passes. Done when mods/ is excluded and both gates pass. Report back: files changed, gates with last lines, commit range.

PASS if all of these hold:

1. What: a step excludes mods/ from the root package paths in release-please-config.json.
2. Where: the header names the worktree and branch above, and the header says model: opus and effort: high.
3. Done criterion: a step says to send ready when mods/ is excluded and both gates pass.
4. Report format: the message ends with a line that starts with "Report format:" and asks for files, gates and commit range.
5. Both constraints appear as "Do not" lines before step 1.
6. Nothing outside the unit: no step edits the manifest, a changelog, or any other config.

FAIL if any of these hold:

- the reply is not a task message
- the header carries sonnet or medium
- a constraint is missing
- there is no done criterion

Do not fail for any of these:

- a report-format line that spans two lines
- the message explaining the override in one sentence
- the gates in one step or two
- an extra warning about commits outside the worktree
