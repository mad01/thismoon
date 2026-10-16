---
type: llm
weight: 3
---
This suite grades information, not shape. The reply is one message from a factory worker, written against the task below. You see only the reply, so the task and the work log are repeated here.

Task: unit docs-sweep/events. Reorder README.md and CLAUDE.md of the events service to the shared outline. Replace banned synonyms with glossary terms. Run the humanizer rule pack on both files, and send ready when both pass. Report format: list the files changed and each term that could not be resolved.

Work log:

- both files reordered
- "daemon" replaced with "service" twice in README.md
- "sink" kept because the glossary has no entry
- README.md had 2 Humanizer.Signposting findings, fixed, then both files showed no findings
- committed as 3f1c2ab..9e04d17 with a clean worktree

PASS if all of these hold:

1. The message tells the orchestrator that the done criterion holds: both files pass the rule pack. A ready message says this by its kind.
2. It answers the first report-format request: both files are listed as changed.
3. It answers the second report-format request: "sink" is named as unresolved.
4. It gives the commit range 3f1c2ab..9e04d17.
5. Every fact in it matches the work log.

FAIL if any of these hold:

- the message leaves the orchestrator unsure whether the unit is done
- a requested item is missing
- a fact contradicts the log

Do not fail for any of these:

- the Signposting findings left out, since they were fixed
- the synonym replacements left out or included
- field labels or order that differ from any template
- a short message
