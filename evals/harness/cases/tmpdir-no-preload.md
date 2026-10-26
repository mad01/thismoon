---
id: tmpdir-no-preload
title: Plain task in a tmpdir loads no plan or GitHub tools
notes: Cost budget is twice the $1.12 one run cost on 2026-10-26 (Fable, default effort), a run that still preloaded the plan and GitHub tools. The turn and tool call budgets are estimates.
workspace: tmpdir
timeout: 240
max_turns: 6
permission_mode: acceptEdits
disallowed_tools: ["Bash(git push:*)"]
graders:
  - type: file
    path: todo.txt
    pattern: '(?s)buy milk.*call the bank.*fix the bike'
  - type: tool_not_used
    tool: '^ToolSearch$'
    input: '(?i)gh_com|PlanMode'
    weight: 2
  - type: tool_not_used
    tool: '^mcp__gh_com__'
  - type: max_tool_calls
    max: 3
  - type: max_turns
    max: 4
  - type: max_cost_usd
    max: 2.2
---
Create a file named todo.txt in the current directory with exactly these three lines: buy milk, call the bank, fix the bike. Then reply with "done".
