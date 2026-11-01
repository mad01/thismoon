---
id: bulk-ask-cross-files
title: Question across one directory's files through csl_ask
notes: >-
  Budgets are estimates; no run of this case has been measured yet.
  bulk-ask-cross-files-without-ask is the same case with csl_ask denied;
  -compare the two runs to see what the tool buys.
workspace: repo:thismoon
timeout: 300
max_turns: 15
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '(?is)humanizer-check-nudge.*lint-policy:[^\n]*repo.*read-size:[^\n]*agent'
    weight: 2
  - type: tool_used
    tool: '^mcp__csl__csl_ask$'
  - type: tool_not_used
    tool: '^Grep$'
  - type: tool_not_used
    tool: '^Bash$'
    input: '\b(grep|rg|find)\b'
  - type: max_turns
    max: 12
  - type: max_cost_usd
    max: 2.5
---
I need to know which belt hints in tools/belt/internal/hint record a marker in the per-session seen file so they don't repeat, and what marker id each one writes. Answer with one line per hint, sorted by hint id.
