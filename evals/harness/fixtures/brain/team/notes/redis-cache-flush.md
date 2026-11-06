---
title: Flush the dispatch route cache
tags:
    - runbook
    - redis
type: note
status: validated
created: "2025-11-12"
updated: "2025-11-12"
---
# Flush the dispatch route cache

[[dispatch-api]] caches computed routes in Redis (`redis-dispatch`, db 2)
for 15 minutes.

1. Confirm the stale routes with `redis-cli -n 2 --scan --pattern 'route:*' | head`.
2. Flush only the route keys: `redis-cli -n 2 --scan --pattern 'route:*' | xargs redis-cli -n 2 del`.
3. Expect a short rise in route-planner traffic while the cache refills.

Unlike pg-failover, this does not need a change ticket.
