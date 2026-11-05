---
title: ledger-svc
tags:
    - system
    - billing
type: note
status: validated
created: "2025-02-11"
updated: "2026-03-20"
---
# ledger-svc

Records charges and carrier invoices.

- Owner: Billing team (#team-billing).
- gRPC API on port 9090. Metrics on 9103.
- Database: `ledger` on the `pg-orders` cluster, through PgBouncer.
- Statement timeout: 800 ms.
- Connection pool: max 40 connections per pod, 6 pods.
- Failover handling: follow the [[pg-failover|failover runbook]], then
  restart the pods.

History: [[incident-2026-03-14-ledger-outage]]. Internal API style:
[[adr-0009-grpc-internal-apis]].
