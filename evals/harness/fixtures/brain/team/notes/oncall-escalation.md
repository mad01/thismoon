---
title: On-call escalation policy
tags:
    - process
    - oncall
type: note
status: validated
created: "2025-01-20"
updated: "2026-04-08"
---
# On-call escalation policy

- Each service team runs its own weekly rotation: a primary and a secondary.
- The primary acknowledges a page within 5 minutes.
- If the primary has not acknowledged after 10 minutes, the page goes to
  the secondary; after 30 minutes, to the team's engineering manager.
- Database incidents also page the Platform team.
- Every rotation joins the quarterly drills, for example
  [[pg-failover-drill]].
