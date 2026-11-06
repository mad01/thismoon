---
id: brain-cite-in-output
title: Write an output artifact that cites existing notes
workspace: brain:team
timeout: 420
max_turns: 30
graders:
  - type: new_notes
    dir: outputs
    min: 1
    max: 1
  - type: wikilinks
    path: outputs/*.md
    new_only: true
    min: 3
  - type: frontmatter
    path: outputs/*.md
    new_only: true
    key: type
    pattern: '^output$'
  - type: llm
    rubric: >-
      The vault diff adds one output note: a brief on Postgres failover
      readiness for pg-orders that draws on the failover runbook, the drill
      results and the March 2026 ledger-svc incident, and cites them with
      wikilinks.
  - type: brain_cmd
    pattern: '^write$'
  - type: vault_unchanged_except
    paths: [outputs/*.md]
---
Write a one-page brief for new on-call engineers on how ready we are for a
Postgres failover on pg-orders: what the runbook says, what the drills have
shown, and what went wrong before. Save it to the knowledge base as an output
and cite the notes it draws on.
