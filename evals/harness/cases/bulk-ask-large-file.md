---
id: bulk-ask-large-file
title: Answer from a large file through csl_ask
notes: >-
  Budgets are estimates; no run of this case has been measured yet.
  bulk-ask-large-file-without-ask is the same case with csl_ask denied;
  -compare the two runs to see what the tool buys.
workspace: repo:thismoon
timeout: 300
max_turns: 12
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '(?s)\bRL\b[^\n]*\bLR\b.*\b2,?000\b[^\n]*\b5,?000\b'
    weight: 2
  - type: tool_used
    tool: '^mcp__csl__csl_ask$'
    input: 'internal/mermaid|mermaid(\\\\)?\.go'
  - type: tool_not_used
    tool: '^(Read|mcp__csl__csl_read)$'
    input: '"(file_path|file)":"[^"]*services/present/internal/mermaid/mermaid\.go"'
    input_without: '"(offset|limit|start_line|end_line)"'
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 2.0
---
In services/present/internal/mermaid/mermaid.go, how does the Mermaid converter map each header direction, and how many nodes and edges does it accept? I only need the answer, in two lines with directions first.
