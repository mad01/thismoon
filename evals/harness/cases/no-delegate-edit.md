---
id: no-delegate-edit
title: Edit inside a large Go file from a read, not a csl_ask answer
notes: >-
  Budgets are estimates; no run of this case has been measured yet.
  An edit needs the real lines, so the read must come before the edit
  whether or not csl_ask is offered. no-delegate-edit-without-ask is the
  same case with csl_ask denied; -compare the two runs.
workspace: worktree:thismoon
timeout: 600
max_turns: 20
permission_mode: acceptEdits
disallowed_tools: ["Bash(git push:*)"]
graders:
  - type: file
    path: services/speak/internal/playback/engine.go
    pattern: 'result = "Nothing has played yet\."'
    weight: 2
  - type: tool_used
    tool: '^(Read|mcp__csl__csl_read)$'
    input: '"(file_path|file)":"[^"]*services/speak/internal/playback/engine\.go"'
  - type: tool_before
    tool: '^(Read|mcp__csl__csl_read)$'
    input: '"(file_path|file)":"[^"]*services/speak/internal/playback/engine\.go"'
    before: '^(Edit|MultiEdit|Write)$'
  - type: tool_used
    tool: '^Bash$'
    input: 'go test.*playback'
  - type: tool_not_used
    tool: '^Bash$'
    input: 'git (commit|push)'
  - type: max_turns
    max: 10
  - type: max_cost_usd
    max: 2.5
---
I want the speak status reply "No recent playback." in services/speak/internal/playback/engine.go to read "Nothing has played yet." instead, and nothing else changed. Run `go test ./services/speak/internal/playback/`, report the result, and don't commit.
