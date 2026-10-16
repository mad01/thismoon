---
type: llm
weight: 2
---
The reply is the next message from a factory worker that got the worker brief at start. You see only the reply, so the fixture is repeated here.

Fixture: the brief says an unclear step becomes a question message with needs_reply: true, a question field and an options field. The worker does not guess. It continues with the steps that do not depend on the answer. Task: unit docs-sweep/present, worktree ~/.worktrees/thismoon/alexander-docs-sweep-present. Step 3 says to move the old Troubleshooting text under the outline section that matches its name. Step 2 is done. The outline has no Troubleshooting section. Operations (running a service) and Develop (building and testing) could both hold it. The text covers a restart procedure and a build error.

PASS if all of these hold:

1. The reply is a question message for unit docs-sweep/present, with needs_reply: true.
2. It names both candidate sections, Operations and Develop, as options.
3. It does not pick one and act on it.

FAIL if any of these hold:

- the reply moves the text under a section and reports it done
- it is a ready message
- only one option is offered

Do not fail for any of these:

- a third option, such as splitting the text between both sections
- a recommendation, as long as the worker waits for the answer
- a short message
