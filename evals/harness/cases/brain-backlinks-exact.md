---
id: brain-backlinks-exact
title: List exactly the notes that link to a stem
notes: >-
  brain has no backlinks command. Five notes link [[pg-failover]], one as
  [[pg-failover|...]] and one as [[pg-failover#Verify]]. Decoys:
  oncall-escalation, the March incident and pg-failover itself link only
  [[pg-failover-drill]]; redis-cache-flush names pg-failover in prose.
workspace: brain:team
timeout: 300
max_turns: 20
graders:
  - type: regex
    pattern: 'pg-failover-drill'
  - type: regex
    pattern: 'ledger-svc'
  - type: regex
    pattern: 'ledger-replica-rebuild'
  - type: regex
    pattern: '(?i)mocs/databases|\[\[databases\]\]|\bdatabases\.md'
  - type: regex
    pattern: '2026-07-10-failover-drill-report'
  - type: llm
    weight: 3
    rubric: >-
      The answer lists exactly these five notes as linking to pg-failover:
      notes/pg-failover-drill.md, notes/ledger-svc.md,
      notes/ledger-replica-rebuild.md, MOCs/databases.md and
      outputs/2026-07-10-failover-drill-report.md. It must not list
      notes/oncall-escalation.md, notes/incident-2026-03-14-ledger-outage.md,
      notes/redis-cache-flush.md or notes/pg-failover.md as linking notes;
      naming them as excluded is fine.
  - type: brain_cmd
    pattern: '^search$'
  - type: vault_unchanged_except
---
I'm about to rename the pg-failover runbook. Which notes link to it? Give me
the exact list of vault paths.
