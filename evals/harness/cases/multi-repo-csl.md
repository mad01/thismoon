---
id: multi-repo-csl
title: Cross-repo question answered with csl, not grep
notes: Budgets are estimates; no run of this case has been measured yet.
workspace: repo:thismoon
timeout: 300
max_turns: 15
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '(?s)kofconsult\.go.*recipes/claude/settings\.json|recipes/claude/settings\.json.*kofconsult\.go'
    weight: 2
  - type: tool_used
    tool: '^mcp__csl__csl_(search|hybrid_search)$'
  - type: tool_not_used
    tool: '^Grep$'
  - type: tool_not_used
    tool: '^Bash$'
    input: '\b(grep|rg|find)\b'
  - type: max_turns
    max: 8
  - type: max_cost_usd
    max: 2.5
---
A question across two of my repos. Which Go file in thismoon implements belt's kof-consult session-start hint, and which file in the dotfiles repo registers `belt hint session-start` as a Claude Code SessionStart hook? Answer with the two repo-relative paths.
