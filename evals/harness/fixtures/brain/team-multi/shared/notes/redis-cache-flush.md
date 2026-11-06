---
title: Redis route cache flush
tags:
  - runbook
  - redis
  - dispatch
type: note
status: validated
created: "2025-05-20"
updated: "2026-06-12"
id: 01JVPGQQD0XCRCCGCWJM2BV378
classification: internal
review_by: 2027-06-12
---
# Redis route cache flush

dispatch-api keeps computed routes in the `route-cache` Redis cluster for 10
minutes. After a bad route-planner deploy, flush it so drivers stop getting
the broken routes:

1. Announce the flush in #team-routing; route lookups slow down for a minute.
2. Run the `flush-route-cache` job from the deploy pipeline. Never run
   `FLUSHALL` by hand.
3. Watch the cache hit rate on the dispatch-api dashboard climb back over 80%.
