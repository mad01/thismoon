---
id: brain-find-fact
title: Find an internal system fact and cite the note
notes: The port and the timeout are only in notes/ledger-svc.md.
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: regex
    pattern: '\b9090\b'
  - type: regex
    pattern: '(?i)\b800\s*(ms|milliseconds)\b'
  - type: regex
    pattern: '\[\[ledger-svc(\]\]|\||#)|notes/ledger-svc\.md'
  - type: brain_cmd
    pattern: '^(search|read|list)$'
  - type: vault_unchanged_except
  - type: max_turns
    max: 10
  - type: max_cost_usd
    max: 0.5
---
What port does ledger-svc's gRPC API listen on, and what is its statement
timeout?
