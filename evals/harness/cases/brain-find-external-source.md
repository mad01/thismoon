---
id: brain-find-external-source
title: Answer from an external summary and give its source URL
notes: The URL is only in the source frontmatter key of notes/paper-tail-at-scale.md.
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: regex
    pattern: '(?i)hedged'
  - type: regex
    pattern: 'https://dl\.acm\.org/doi/10\.1145/2408776\.2408794'
  - type: regex
    pattern: 'paper-tail-at-scale'
  - type: vault_unchanged_except
---
What does the Tail at Scale paper suggest for cutting tail latency? Give me
a link to the paper as well.
