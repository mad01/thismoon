---
id: fleet-recipe-most
title: Count a file name per repo across a fleet and name the top repo
notes: "Ground truth 2026-10-29: find and csl both put dotfiles first with 49 recipe.toml files; the next repo has under 40. Budgets are estimates; no run of this case has been measured yet."
workspace: repo:thismoon
timeout: 300
max_turns: 10
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '\bdotfiles\b[^\n\d]{0,12}\b(4[7-9]|5[0-2])\b'
    weight: 2
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 3.0
---
Ralph recipes are files named recipe.toml. Across the repos checked out under ~/code/src/github.com/mad01, which single repo contains the most recipe.toml files, and how many does it hold? Answer as "repo: count" and nothing else.
