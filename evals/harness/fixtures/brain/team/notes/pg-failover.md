---
title: Postgres failover runbook (pg-orders)
tags:
    - runbook
    - postgres
    - oncall
type: note
status: validated
created: "2025-03-04"
updated: "2026-07-12"
---
# Postgres failover runbook (pg-orders)

The `pg-orders` cluster runs Postgres 16 under Patroni with three members in
two zones. It holds the `orders` and `ledger` databases. Target recovery
time is 5 minutes.

## When to use

Use a planned switchover for maintenance. Use a failover only when the
primary is down and Patroni has not promoted a replica by itself within
60 seconds.

## Steps

1. Page the database owner on call (see [[oncall-escalation]]).
2. Check replication lag on the replicas: `patronictl -c /etc/patroni.yml list`.
   Do not promote a replica that is more than 16 MB behind.
3. Planned: `patronictl switchover pg-orders --candidate <replica>`.
   Unplanned: `patronictl failover pg-orders --candidate <replica>`.
4. Confirm the new primary accepts writes and PgBouncer points at it.

## Verify

- `ledger-svc` and `dispatch-api` error rates are back to baseline.
- Restart [[ledger-svc]] pods if their connection pools still hold
  connections to the old primary.

Background on the REST API Patroni exposes: [[patroni-failover-docs]].
The quarterly practice run is [[pg-failover-drill]].
