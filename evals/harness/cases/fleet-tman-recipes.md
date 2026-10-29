---
id: fleet-tman-recipes
title: Per-repo counts of files mentioning a term across a fleet
notes: "Ground truth 2026-10-29: recipe.toml files mentioning t-man are 12 to 14 in thismoon (rg 12, csl 14 after a reindex) and 7 in dotfiles; one private repo has 2. Budgets are estimates; no run of this case has been measured yet."
workspace: repo:thismoon
timeout: 300
max_turns: 10
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '\bthismoon\b[^\n\d]{0,12}\b1[2-4]\b'
    weight: 2
  - type: regex
    pattern: '\bdotfiles\b[^\n\d]{0,12}\b7\b'
    weight: 2
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 3.0
---
Across the repos checked out under ~/code/src/github.com/mad01, how many recipe.toml files in each repo mention t-man? Count files, not lines. Answer one repo per line as "repo: count", highest first, and leave out repos with zero.
