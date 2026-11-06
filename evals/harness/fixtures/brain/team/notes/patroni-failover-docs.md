---
title: 'Patroni docs: switchover and failover endpoints'
tags:
    - external
    - postgres
    - vendor-docs
type: source
status: validated
source: https://patroni.readthedocs.io/en/latest/rest_api.html
evidence: A
created: "2025-03-01"
updated: "2025-03-01"
---
# Patroni docs: switchover and failover endpoints

Summary of the Patroni REST API docs.

- `POST /switchover` moves the leader to a healthy replica; it can be
  scheduled for a later time.
- `POST /failover` promotes a named candidate even when the cluster has no
  healthy leader; it needs a candidate.
- Patroni refuses a switchover to a replica whose lag is above
  `maximum_lag_on_failover` (1 MB by default).
- `patronictl switchover` and `patronictl failover` call these endpoints.

We use these in the pg-orders failover runbook.
