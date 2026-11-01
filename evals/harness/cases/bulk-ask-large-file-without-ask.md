---
id: bulk-ask-large-file-without-ask
title: Answer from a large file with csl_ask denied
notes: >-
  The without-ask arm of bulk-ask-large-file: same prompt and graders,
  csl_ask denied and its tool_used grader dropped.
workspace: repo:thismoon
timeout: 300
max_turns: 12
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)", mcp__csl__csl_ask]
graders:
  - type: regex
    pattern: '(?i)ErrNotFlowchart|not a flowchart'
    weight: 0.4
  - type: regex
    pattern: '(?i)cannot parse|unexpected|line \d'
    weight: 0.4
  - type: regex
    pattern: '(?i)TooManyNodes|2,?000'
    weight: 0.4
  - type: regex
    pattern: '(?i)TooManyEdges|5,?000'
    weight: 0.4
  - type: regex
    pattern: '(?i)errNoNodes|no nodes'
    weight: 0.4
  - type: tool_not_used
    tool: '^(Read|mcp__csl__csl_read)$'
    input: '"(file_path|file)":"[^"]*services/present/internal/mermaid/mermaid\.go"'
    input_without: '"(offset|limit|start_line|end_line)"'
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 2.0
---
In services/present/internal/mermaid/mermaid.go, which distinct errors can Convert return to its caller, and at which stage of its pipeline does each one arise? One line per error, in pipeline order. I only need the answer.
