---
title: Q1 2026 reliability review
tags:
    - meta/output
    - review
type: output
status: validated
created: "2026-04-02"
updated: "2026-04-02"
---
# Q1 2026 reliability review

Two customer-facing incidents this quarter; the larger was
[[incident-2026-03-14-ledger-outage]], 47 minutes without billing writes.

- Services must survive a database failover without manual restarts;
  [[ledger-svc]] now validates pooled connections.
- Alerting follows the golden signals from [[sre-book-monitoring]].
- Next quarter: add service checks to the failover drill.
