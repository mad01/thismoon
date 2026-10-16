---
type: llm
weight: 2
---
The reply is one task message from a factory orchestrator to a worker. You see only the reply, so the fixture it was written from is repeated here.

Fixture: plan unit adr-links/opener, work-on tier trivial. Worktree ~/.worktrees/thismoon/alexander-adr-links-opener on branch alexander/adr-links-opener. tools/opener/README.md links to docs/adr/0014-loom.md, which does not exist; the link must point at docs/adr/0014-loom-skill-only-worktree-coordination.md. Every other file is out of scope. Gate: `humanizer detect tools/opener/README.md` reports no findings. Done when the link points at the existing file and the gate is clean. Report back: the files changed.

PASS if all of these hold:

1. What: a step replaces the broken link with docs/adr/0014-loom-skill-only-worktree-coordination.md.
2. Where: the steps name tools/opener/README.md, and the header names the worktree and branch above.
3. Done criterion: a step says to send ready when the link is fixed and the gate is clean.
4. Report format: the message ends with a line that starts with "Report format:" and asks for the files changed.
5. Nothing outside the unit: no step touches another file, and none asks for a push, a pull request or a merge.
6. A "Do not" line before step 1 keeps the worker off every other file.

FAIL if any of these hold:

- the reply is not a task message
- the target path is wrong or missing
- there is no done criterion
- a step edits a second file

Do not fail for any of these:

- a report-format line that spans two lines
- a message shorter than five steps
- the gate command written in backticks or without them
- an extra warning about commits outside the worktree
- the report format asking for the commit range as well
