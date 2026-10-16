---
type: llm
weight: 3
---
This suite grades information, not shape. The reply is one message from a factory worker, written against the task below. You see only the reply, so the task and the work log are repeated here.

Task: unit docs-sweep/present. Reorder services/present/README.md to the shared outline, move the old Troubleshooting text under the fitting outline section, run the humanizer rule pack, send ready when it passes. Report format: list the files changed, and name the outline section that now holds the old Troubleshooting text.

Work log:

- README.md reordered
- the Troubleshooting text, about restarting the service and reading its logs, now sits under Operations
- humanizer detect shows no findings
- committed as 2c4e6a8..e9b1d35, clean worktree

PASS if all of these hold:

1. The message tells the orchestrator the done criterion holds. A ready message says this by its kind.
2. It lists services/present/README.md as changed.
3. It answers the report-format question: the Troubleshooting text is under Operations.
4. It gives the range 2c4e6a8..e9b1d35.
5. Every fact in it matches the work log.

FAIL if any of these hold:

- the section is missing or wrong
- a fact contradicts the log

Do not fail for any of these:

- a one-sentence reason for the choice
- the answer in a field with any reasonable label
- a short message
