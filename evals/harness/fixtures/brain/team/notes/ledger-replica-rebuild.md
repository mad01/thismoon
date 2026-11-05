---
title: Rebuild a Postgres replica
tags:
    - runbook
    - postgres
type: note
status: validated
created: "2025-09-02"
updated: "2026-02-20"
---
# Rebuild a Postgres replica

When a `pg-orders` replica falls too far behind to catch up from WAL:

1. Stop Patroni on the lagging member and move its data directory aside.
2. Start Patroni again; it re-bootstraps the replica with `pg_basebackup`.
3. Watch `pg_stat_replication` on the primary until the replica is
   streaming.

Before a rebuild during an incident, check whether a failover is the better
move: [[pg-failover#Verify]] lists what to check afterwards. Hot standby
behaviour is summarised in [[postgres-hot-standby-docs]].
