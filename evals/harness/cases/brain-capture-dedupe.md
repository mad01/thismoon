---
id: brain-capture-dedupe
title: Update the existing note instead of adding a duplicate
notes: notes/ledger-svc.md already holds the pool size (40).
workspace: brain:team
timeout: 300
max_turns: 20
graders:
  - type: new_notes
    max: 0
  - type: file
    path: notes/ledger-svc.md
    pattern: '\b50\b'
  - type: frontmatter
    path: notes/ledger-svc.md
    key: tags
    pattern: '(?m)^billing$'
  - type: brain_cmd
    pattern: '^edit$'
  - type: vault_unchanged_except
    paths: [notes/ledger-svc.md]
---
For the knowledge base: on 2026-10-30 the Billing team raised ledger-svc's
connection pool max from 40 to 50 connections per pod, after the PgBouncer
upgrade.
