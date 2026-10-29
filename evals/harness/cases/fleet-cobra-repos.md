---
id: fleet-cobra-repos
title: Which repos in a fleet of checkouts import one Go library
notes: "Ground truth 2026-10-29 by rg over ~/code/src/github.com/mad01: thismoon, kitty-session, ralph and one private repo import github.com/spf13/cobra. The graders name the three public ones. Budgets are estimates; no run of this case has been measured yet."
workspace: repo:thismoon
timeout: 300
max_turns: 10
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: '\bthismoon\b'
  - type: regex
    pattern: '\bkitty-session\b'
  - type: regex
    pattern: '\bralph\b'
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 3.0
---
My repos are checked out under ~/code/src/github.com/mad01, one directory per repo. Which of them have Go code that imports github.com/spf13/cobra? Answer with the repo directory names only, one per line.
