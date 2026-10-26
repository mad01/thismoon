---
id: large-file-ranged-read
title: Answer from a large file with ranged reads
workspace: repo:thismoon
timeout: 300
max_turns: 12
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '(?is)hard.*soft|soft.*hard'
  - type: tool_used
    tool: '^(Read|mcp__csl__csl_read)$'
    input: '"(offset|limit|start_line|end_line)"'
  - type: tool_not_used
    tool: '^(Read|mcp__csl__csl_read)$'
    input: 'config/config\.go'
    input_without: '"(offset|limit|start_line|end_line)"'
    weight: 2
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 2.0
---
In tools/belt/internal/config/config.go, which mode values does `validMode` accept, and what does it do with anything else? Answer in two sentences.
