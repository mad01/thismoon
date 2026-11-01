---
id: bulk-ask-large-file
title: Answer from a large file through csl_ask
notes: >-
  Run 20261101-182117 answered the first version of this prompt (header
  directions and the node and edge caps) without csl_ask. A csl_search and
  a 28-line csl_read got every fact, because each one was a lexical hit.
  This prompt asks for the error paths of the whole pipeline, which no
  single search line answers. Each fact grader weighs 0.4, so the five
  facts together weigh 2 like the one regex they replace.
  bulk-ask-large-file-without-ask is the same case with csl_ask denied;
  compare the two runs to see what the tool buys.
workspace: repo:thismoon
timeout: 300
max_turns: 12
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
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
In services/present/internal/mermaid/mermaid.go, which distinct errors can Convert return to its caller, and at which stage of its pipeline does each one arise? One line per error, in pipeline order. I only need the answer.
