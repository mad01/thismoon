---
title: Route planner upstream timeouts
tags:
    - route-planner
    - timeouts
    - dispatch
type: note
status: raw
created: "2026-10-20"
updated: "2026-10-27"
---
# Route planner upstream timeouts

Captured after the [[incident-2026-06-02-dispatch-latency]] review. Checked
against the dispatch-api config in the deploy repo on 2026-10-27 by Ines
Varga; ready to promote.

- dispatch-api calls route-planner with a 1200 ms per-attempt timeout.
- Up to 2 retries, jittered backoff between 100 and 400 ms.
- Total budget for a route lookup is 3 s, then dispatch-api serves the
  cached route from Redis (see [[redis-cache-flush]] for the cache).
- Retries only on 503 and connection resets, never on 504, so a slow
  route-planner does not get three times the load.
