---
id: brain-edit-preserve-frontmatter
title: Add a tag without losing extra frontmatter keys
notes: >-
  Expected to fail today: brain edit re-renders frontmatter from a typed
  struct and drops the owner and classification keys. Tags passed to edit
  also replace the whole list.
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: frontmatter
    path: notes/data-retention-policy.md
    key: tags
    pattern: '(?m)^gdpr$'
  - type: frontmatter
    path: notes/data-retention-policy.md
    key: tags
    pattern: '(?m)^policy$'
  - type: frontmatter
    path: notes/data-retention-policy.md
    key: owner
    pattern: '^data-governance$'
  - type: frontmatter
    path: notes/data-retention-policy.md
    key: classification
    pattern: '^internal-restricted$'
  - type: brain_cmd
    pattern: '^edit$'
  - type: vault_unchanged_except
    paths: [notes/data-retention-policy.md]
---
Tag the data retention policy note with gdpr.
