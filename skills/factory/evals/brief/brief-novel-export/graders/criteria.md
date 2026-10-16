---
type: llm
weight: 2
---
The reply is one task message from a factory orchestrator to a worker. You see only the reply, so the fixture it was written from is repeated here.

Fixture: plan unit present/deck-pdf, work-on tier novel. Worktree ~/.worktrees/thismoon/alexander-present-deck-pdf on branch alexander/present-deck-pdf. Goal: `present deck export <id> -o <file>.pdf` writes one PDF page per slide. The worker picks the PDF approach. Constraints: no new runtime dependency outside the Go standard library and the vendored browser assets; the Doc schema does not change. Gates: `make -C services/present test` and `make -C services/present lint`. Done when the command writes a PDF with every slide and both gates pass. Report back: files changed, gates with last lines, commit range, and the chosen PDF approach.

PASS if all of these hold:

1. What: the steps ask for the export command and one PDF page per slide.
2. Where: the steps keep the work in services/present, and the header names the worktree and branch above.
3. Done criterion: a step says to send ready when the PDF holds every slide and both gates pass.
4. Report format: the message ends with a line that starts with "Report format:" and asks for the chosen PDF approach, plus the files, gates and commit range.
5. Both constraints appear as "Do not" lines before step 1: no new runtime dependency, no Doc schema change.
6. Nothing outside the unit: no step asks for changes to other services, a push, a pull request or a merge.

FAIL if any of these hold:

- the reply is not a task message
- the message picks the PDF approach for the worker
- a constraint is missing
- there is no done criterion

Do not fail for any of these:

- a step that tells the worker to send a question if no approach fits the constraints
- a step that names a test file to add
- the report format split over two lines
