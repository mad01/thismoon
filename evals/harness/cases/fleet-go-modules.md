---
id: fleet-go-modules
title: Find the Go modules in a fleet of checkouts and their module paths
notes: "Ground truth 2026-10-29: three repos under ~/code/src/github.com/mad01 have a root go.mod: thismoon, ralph, kitty-session, each with the module path github.com/mad01/<repo>. Budgets are estimates; no run of this case has been measured yet."
workspace: repo:thismoon
timeout: 300
max_turns: 10
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: 'github\.com/mad01/thismoon\b'
  - type: regex
    pattern: 'github\.com/mad01/ralph\b'
  - type: regex
    pattern: 'github\.com/mad01/kitty-session\b'
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 3.0
---
Which repos checked out under ~/code/src/github.com/mad01 are Go modules, meaning they have a go.mod at the repo root? Answer one per line as "repo: module path".
