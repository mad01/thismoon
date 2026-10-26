---
id: kof-assertion
title: Answer from a stored kof assertion and cite it
notes: Budgets are estimates; no run of this case has been measured yet.
workspace: repo:thismoon
timeout: 300
max_turns: 12
disallowed_tools: [Edit, Write, NotebookEdit, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: 'testing\.Testing'
  - type: regex
    pattern: '(?i)\bkof\b|keeper-of-facts|01792525857952842000-da40'
    weight: 2
  - type: llm
    rubric: >-
      The answer says a test cannot observe an event emitted through kit/notify
      EmitEvent because EmitEvent returns immediately under testing.Testing(), and
      it names a kof (keeper-of-facts) assertion as a source.
  - type: max_turns
    max: 6
  - type: max_cost_usd
    max: 2.0
---
Can a Go test in this repo observe an event emitted through kit/notify's `EmitEvent`? Answer in two sentences and say where that knowledge comes from.
