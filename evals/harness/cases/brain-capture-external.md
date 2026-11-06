---
id: brain-capture-external
title: Capture a pasted external finding to the inbox
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: new_notes
    dir: inbox
    min: 1
    max: 1
    content_regex: '(?i)sync_replication_slots|synchroni[sz]'
  - type: frontmatter
    path: inbox/*.md
    new_only: true
    key: type
    pattern: '^source$'
  - type: frontmatter
    path: inbox/*.md
    new_only: true
    key: status
    pattern: '^raw$'
  - type: frontmatter
    path: inbox/*.md
    new_only: true
    key: source
    pattern: '^https://www\.postgresql\.org/docs/17/release-17\.html$'
  - type: brain_cmd
    pattern: '^(write|capture)$'
  - type: vault_unchanged_except
    paths: [inbox/*.md]
---
Save this to the knowledge base for later review:

From the PostgreSQL 17 release notes
(https://www.postgresql.org/docs/17/release-17.html): logical replication
slots can now be synchronized to standby servers, so logical replication
consumers keep working after a failover. It needs `sync_replication_slots`
enabled on the standby and the slot created with `failover = true`.
