---
name: brief-novel-export
description: A novel unit with an open design choice. Routes to the top model at high effort, and the report format asks for the choice.
tags: [brief, hand-written]
max_turns: 10
allowed_tools: [Read, Glob, Grep, Skill]
---
You are the orchestrator of a factory run. Write the task message for the plan unit below, the message you send to its worker. SendMessage is not available here, so reply with the message only: no code fence, no text before or after it.

Run facts (a test fixture; everything you need is here):

- Repo: thismoon. Canonical checkout: ~/code/src/github.com/mad01/thismoon.
- The worktree for this unit is claimed: ~/.worktrees/thismoon/alexander-present-deck-pdf, on branch alexander/present-deck-pdf.
- This is the first message for the unit.

Plan unit present/deck-pdf (work-on tier: novel):

- Goal: `present deck export <id> -o <file>.pdf` writes one PDF page per slide of a deck.
- The PDF approach is open; the worker picks it.
- Constraints: no new runtime dependency outside the Go standard library and the vendored browser assets. The Doc schema does not change.
- Gates: `make -C services/present test` and `make -C services/present lint`.
- Done when: the command writes a PDF with every slide and both gates pass.
- Report back: the files changed, the gates with their last lines, the commit range, and the PDF approach the worker chose.
