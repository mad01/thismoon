---
id: brain-promote-with-links
title: Promote an inbox note and keep every link to it working
notes: >-
  inbox/rp-timeouts.md is titled "Route planner upstream timeouts", so
  promote gives it a new stem. Three notes link [[rp-timeouts]].
workspace: brain:team
timeout: 300
max_turns: 20
graders:
  - type: file
    path: inbox/rp-timeouts.md
    absent: true
  - type: new_notes
    dir: notes
    min: 1
    max: 1
    content_regex: '1200 ms'
  - type: frontmatter
    path: notes/*.md
    new_only: true
    key: status
    pattern: '^validated$'
  - type: file
    path: '**/*.md'
    pattern: '\[\[rp-timeouts(\]\]|\||#)'
    absent: true
  - type: wikilinks
    path: '**/*.md'
  - type: brain_cmd
    pattern: '^promote$'
  - type: vault_unchanged_except
    paths: [inbox/rp-timeouts.md, notes/*.md, MOCs/*.md]
---
The route planner timeouts capture in the inbox has been checked against the
deploy config. Promote it to notes.
