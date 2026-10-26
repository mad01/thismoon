---
id: commit-worktree
title: Commit a docs change with a conventional subject and no push
notes: Budgets are estimates; no run of this case has been measured yet.
workspace: worktree:thismoon
timeout: 600
max_turns: 20
permission_mode: acceptEdits
disallowed_tools: ["Bash(git push:*)"]
graders:
  - type: commit
    pattern: '^(feat|fix|docs|chore|refactor|test|build|ci|perf|style)(\([a-z0-9/._-]+\))?!?: \S.{0,70}$'
    weight: 2
  - type: file
    path: kit/repofind/README.md
    pattern: 'go test \./kit/repofind/'
  - type: tool_not_used
    tool: '^Bash$'
    input: 'git push'
  - type: max_turns
    max: 15
  - type: max_cost_usd
    max: 3.0
---
Add a short "Testing" section to kit/repofind/README.md that says the package's tests run with `go test ./kit/repofind/`. Then commit the change on the current branch. Do not push.
