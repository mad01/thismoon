---
type: llm
weight: 3
---
This suite grades information, not shape. The reply is one message from a factory worker, written against the task below. You see only the reply, so the task and the work log are repeated here.

Task: unit mad-397/wiring. Its steps:

- Add recipes/factory/recipe.toml, recipes/factory/CLAUDE.md and its AGENTS.md symlink.
- Add the factory skill to the root CLAUDE.md table and count.
- Add a factory row marked experimental to the root README.md.
- Run humanizer_detect on each changed markdown file and fix each warning.
- Send ready when the gate shows no warnings.

Report format: files, gate output, commit range.

Work log:

- all five paths written or changed (recipes/factory/recipe.toml, recipes/factory/CLAUDE.md, recipes/factory/AGENTS.md, CLAUDE.md, README.md)
- humanizer_detect on the three markdown files gave 0 warnings and 2 suggestions, left as judgment calls
- committed as d3a90b1..4f6c2e8, clean worktree

PASS if all of these hold:

1. The message tells the orchestrator the done criterion holds: no warnings. A ready message says this by its kind.
2. It lists all five paths.
3. It reports the gate output: 0 warnings.
4. It gives the range d3a90b1..4f6c2e8.
5. Every fact in it matches the work log.

FAIL if any of these hold:

- a path is missing
- the message says warnings remain
- a fact contradicts the log

Do not fail for any of these:

- the suggestions left out, since they do not block
- the AGENTS.md symlink described as a symlink
- a short message
