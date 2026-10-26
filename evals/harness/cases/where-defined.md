---
id: where-defined
title: Find where a Go function is defined
notes: Cost budget is twice the $1.32 one run cost on 2026-10-26 (Fable, default effort). The turn budget is an estimate.
workspace: repo:thismoon
timeout: 300
max_turns: 12
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: 'kit/gitloc/gitloc\.go'
    weight: 2
  - type: tool_used
    tool: '^mcp__csl__csl_search$'
    input: 'sym:'
  - type: tool_not_used
    tool: '^(Grep|Glob)$'
  - type: max_turns
    max: 5
  - type: max_cost_usd
    max: 2.6
---
Where is the Go function `CanonicalRepo` defined in this repo? Answer with the file path and line number only.
