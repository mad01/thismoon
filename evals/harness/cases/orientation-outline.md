---
id: orientation-outline
title: Orient in a package with an outline before reading files
notes: Budgets are estimates; no run of this case has been measured yet.
workspace: repo:thismoon
timeout: 300
max_turns: 15
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: tool_used
    tool: '^mcp__csl__csl_outline$'
  - type: tool_before
    tool: '^mcp__csl__csl_outline$'
    before: '^(Read|mcp__csl__csl_read)$'
    weight: 2
  - type: llm
    rubric: >-
      The answer names at least three of the packages under tools/belt/internal
      (cache, cli, config, guard, hint, hook, ledger, mcptool, watch) and says
      what each named one does. It is a short orientation, not a file-by-file dump.
  - type: max_turns
    max: 8
  - type: max_cost_usd
    max: 2.5
---
I'm new to tools/belt in this repo. Give me a short orientation: the main packages under tools/belt/internal, the central types, and how they fit together. Keep it under 15 lines.
