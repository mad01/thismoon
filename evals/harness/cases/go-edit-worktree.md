---
id: go-edit-worktree
title: Small Go edit with a table test in a throwaway worktree
notes: Budgets are estimates; no run of this case has been measured yet.
workspace: worktree:thismoon
timeout: 600
max_turns: 25
permission_mode: acceptEdits
disallowed_tools: ["Bash(git push:*)"]
graders:
  - type: file
    path: kit/envdefault/envdefault.go
    pattern: 'func Float\(name string, fallback float64\) float64'
    weight: 2
  - type: file
    path: kit/envdefault/envdefault_test.go
    pattern: 'Float'
  - type: tool_used
    tool: '^Bash$'
    input: 'go test.*envdefault'
  - type: tool_not_used
    tool: '^Bash$'
    input: 'git (commit|push)'
  - type: max_turns
    max: 20
  - type: max_cost_usd
    max: 4.0
---
In kit/envdefault, add `func Float(name string, fallback float64) float64` that behaves like `Int`: an unset or empty variable falls back silently, and an unparseable value warns once on stderr and falls back. Follow the package's existing pattern with an injected writer for the warning, and add a table test beside the existing ones. Run `go test ./kit/envdefault/` and report the result. Do not commit.
