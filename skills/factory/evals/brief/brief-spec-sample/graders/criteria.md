---
type: llm
weight: 2
---
The reply is one task message from a factory orchestrator to a worker. You see only the reply, so the fixture it was written from is repeated here.

Fixture: plan unit docs-sweep/events, work-on tier known-domain. Worktree ~/.worktrees/thismoon/alexander-docs-sweep-events on branch alexander/docs-sweep-events. Scope: README.md and CLAUDE.md of the events service. Changes: reorder the sections of both files to match the shared outline; replace each banned synonym with its glossary term. Out of scope: Go source files and recipe TOML files. Commits stay inside the worktree. Gate: the humanizer rule pack on both files. Done when both files pass. Report back: the files changed and each unresolved term.

This reference message passes:

kind: task
unit: docs-sweep/events
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events
branch: alexander/docs-sweep-events
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit Go source files or recipe TOML files.
Do not commit outside your worktree.

1. Open README.md and CLAUDE.md of the events service.
2. Reorder the sections to match the shared outline.
3. Replace each banned synonym with the glossary term.
4. Run the humanizer rule pack on both files.
5. Send ready when both files pass.

Report format: one ready message. List the files you changed.
List each term you could not resolve.

PASS if all of these hold:

1. What: the steps cover both changes, the reorder and the synonym replacement.
2. Where: the steps name README.md and CLAUDE.md of the events service, and the header names the worktree and branch above.
3. Done criterion: a step says to send ready when both files pass the rule pack.
4. Report format: the message ends with a line that starts with "Report format:" and asks for the files changed and the unresolved terms.
5. Nothing outside the unit: no step asks for work on other files, other services, Go code, recipes, a push, a pull request or a merge.
6. Both out-of-scope rules appear as "Do not" lines before step 1.

FAIL if any of these hold:

- the reply is not a task message (a plan, a summary, or a question to the user)
- a change from the fixture is missing
- there is no done criterion
- a step asks for work outside the unit

Do not fail for any of these:

- wording that differs from the reference
- steps split or merged differently
- an extra warning that restates the worktree boundary
- a report-format line that spans two lines
- a step that names the gate command more exactly than the fixture does
