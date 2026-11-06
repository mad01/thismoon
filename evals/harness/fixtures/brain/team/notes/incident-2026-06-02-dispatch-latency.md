---
title: 'Incident 2026-06-02: dispatch latency spike'
tags:
    - incident
    - dispatch
type: note
status: validated
created: "2026-06-04"
updated: "2026-06-20"
---
# Incident 2026-06-02: dispatch latency spike

Duration: 2 hours. Impact: drivers waited up to 20 seconds for routes in
the morning peak.

## What happened

route-planner slowed down after a map data update. [[dispatch-api]]
retried every slow call three times, which tripled route-planner load and
made it slower still.

## Learnings

- Retry only on fast failures, not timeouts. New settings: [[rp-timeouts]].
- Tail latency grows with fan-out; see [[paper-tail-at-scale]].
