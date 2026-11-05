---
title: Postgres failover drill
tags:
    - runbook
    - postgres
    - drill
type: note
status: validated
created: "2025-06-18"
updated: "2026-07-10"
---
# Postgres failover drill

Every quarter we run the [[pg-failover]] runbook against the staging copy of
`pg-orders`, then once a year against production in a maintenance window.

1. Announce the drill in #platform-ops a day ahead.
2. Start a synthetic write load (`orders-loadgen --rate 200`).
3. Kill the primary's Patroni process and time how long until writes
   succeed again. The target is under 60 seconds.
4. Record the result in an output note under outputs/.

Last results: [[2026-07-10-failover-drill-report]].
