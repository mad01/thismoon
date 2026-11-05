---
title: 'Incident 2026-03-14: ledger-svc outage after failover'
tags:
    - incident
    - billing
    - postgres
type: note
status: validated
created: "2026-03-16"
updated: "2026-03-20"
---
# Incident 2026-03-14: ledger-svc outage after failover

Duration: 47 minutes. Impact: no invoices or charges recorded; 1,900 delivery
confirmations queued and replayed later.

## What happened

An unplanned Patroni failover on `pg-orders` finished in 40 seconds, but
[[ledger-svc]] pods kept their pooled connections to the old primary and
failed every write until they were restarted by hand.

## Learnings

- The failover drill only checked that the database came back, not the
  services on it. [[pg-failover-drill]] now includes a ledger-svc write
  check.
- ledger-svc now validates pooled connections before use.
- Pool max per pod lowered from 60 to 40 to stay under PgBouncer limits.
