---
type: llm
weight: 2
---
The reply is one task message from a factory orchestrator to a worker. You see only the reply, so the fixture it was written from is repeated here.

Fixture: plan unit docs-sweep/deps, work-on tier known-domain. Worktree ~/.worktrees/thismoon/alexander-docs-sweep-deps on branch alexander/docs-sweep-deps. Scope: README.md and CLAUDE.md of the deps service. Changes: reorder sections to the shared outline; replace banned synonyms with glossary terms. A note says services/csl/README.md has the same problem but belongs to unit docs-sweep/csl, which runs later. Out of scope: Go source and recipe TOML. Gate: the humanizer rule pack on both files. Done when both pass. Report back: files changed and unresolved terms.

PASS if all of these hold:

1. What: the steps cover the reorder and the synonym replacement.
2. Where: the steps name README.md and CLAUDE.md of the deps service, and the header names the worktree and branch above.
3. Done criterion: a step says to send ready when both files pass the rule pack.
4. Report format: the message ends with a line that starts with "Report format:" and asks for the files changed and the unresolved terms.
5. Nothing outside the unit: no numbered step names a path under services/csl or tools/csl.
6. The out-of-scope rules appear as "Do not" lines before step 1.

FAIL if any of these hold:

- the reply is not a task message
- a numbered step names a path under services/csl or tools/csl
- a change is missing
- there is no done criterion

Do not fail for any of these:

- a report-format line that spans two lines
- a "Do not" warning that names the csl README as off limits
- no mention of csl at all
- a step that uses csl_search or another csl tool to find files
- an extra warning about commits outside the worktree
