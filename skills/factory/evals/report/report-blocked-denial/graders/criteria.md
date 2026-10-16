---
type: llm
weight: 2
---
The reply is one message from a factory worker to its orchestrator. You see only the reply, so the fixture it was written from is repeated here.

Fixture: unit csl/prune-shards, worktree ~/.worktrees/thismoon/alexander-csl-prune-shards, branch alexander/csl-prune-shards, model sonnet, effort medium. Steps 1 to 3 are done and the test gate passed; steps 1 and 2 are committed as 41aa9c0..7d2e115. Step 4, `make -C services/csl install`, was denied with exactly this text: "Permission to use Bash with command make -C services/csl install has been denied." Steps 5 and 6 did not run. The worker did not retry.

PASS if all of these hold:

1. The message is a blocked message for unit csl/prune-shards.
2. The blocker field carries the denial text word for word, in a fenced code block under the blocker line.
3. It asks the human one question about the install step.
4. It lists steps 4 and 5 (or 4 to 6) as not done.
5. It states nothing the fixture does not support, and it does not claim a retry or a workaround.

FAIL if any of these hold:

- the denial text is paraphrased instead of quoted
- there is no question
- the message claims the install ran, or that the worker found another way to install
- the message is a ready message

Do not fail for any of these:

- any wording for the blocker line above the fenced denial
- a mention of the commits for steps 1 and 2
- step 6 listed as not done as well
- a question with two options in one sentence
- a short message
