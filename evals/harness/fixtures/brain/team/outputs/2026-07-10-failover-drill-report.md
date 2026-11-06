---
title: Failover drill report, July 2026
tags:
    - meta/output
    - drill
    - postgres
type: output
status: validated
created: "2026-07-10"
updated: "2026-07-10"
---
# Failover drill report, July 2026

Drill run per [[pg-failover-drill]] against staging `pg-orders`.

- Writes recovered after 38 seconds (target under 60).
- ledger-svc write check passed without restarts, the fix from the March
  incident holds.
- The [[pg-failover]] runbook's lag check used the 16 MB limit; Patroni's
  own limit is lower, see [[patroni-failover-docs]].
