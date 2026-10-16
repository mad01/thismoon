---
type: llm
weight: 2
---
The reply is the next message from a factory worker that got the worker brief at start. You see only the reply, so the fixture is repeated here.

Fixture: the brief defines a ready message as eight header lines, a blank line, then the fields files changed, gates run and commit range. Task: unit adr-links/opener, worktree ~/.worktrees/thismoon/alexander-adr-links-opener, branch alexander/adr-links-opener, model sonnet, effort low. Facts: the link in tools/opener/README.md is fixed; `humanizer detect tools/opener/README.md` printed "no findings"; committed as 77b2e01..c05d9e4; clean worktree.

PASS if all of these hold:

1. The reply is a ready message for unit adr-links/opener, with needs_reply: false.
2. It lists tools/opener/README.md, the gate with "no findings", and the range 77b2e01..c05d9e4.
3. It states nothing the fixture does not support.

FAIL if any of these hold:

- a fact is wrong or missing
- the reply claims a push, a merge or a pull request

Do not fail for any of these:

- a short message
- no prose beyond the fields
- the gate command with or without backticks
