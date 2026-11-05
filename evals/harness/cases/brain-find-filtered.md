---
id: brain-find-filtered
title: List the runbooks in notes/ with a given tag
notes: >-
  Three notes carry both runbook and postgres. Decoys: patroni-failover-docs
  and postgres-hot-standby-docs (postgres, not runbooks), the March incident
  (postgres, not a runbook), kafka-consumer-lag and redis-cache-flush
  (runbooks, not postgres).
workspace: brain:team
timeout: 240
max_turns: 15
graders:
  - type: regex
    pattern: 'pg-failover([^-]|$)'
  - type: regex
    pattern: 'pg-failover-drill'
  - type: regex
    pattern: 'ledger-replica-rebuild'
  - type: regex
    pattern: 'kafka-consumer-lag|redis-cache-flush'
    absent: true
  - type: llm
    weight: 2
    rubric: >-
      The answer lists exactly three runbooks: pg-failover (the Postgres
      failover runbook), pg-failover-drill and ledger-replica-rebuild. It does
      not present any other note (for example patroni-failover-docs,
      postgres-hot-standby-docs or the 2026-03-14 ledger incident) as a
      postgres runbook. Naming other notes as excluded is fine.
  - type: brain_cmd
    pattern: '^(list|search)$'
  - type: vault_unchanged_except
---
List every runbook in notes/ that is tagged postgres.
