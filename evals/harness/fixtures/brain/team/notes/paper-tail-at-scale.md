---
title: 'Paper: The Tail at Scale (Dean and Barroso, 2013)'
tags:
    - external
    - paper
    - latency
type: source
status: validated
source: https://dl.acm.org/doi/10.1145/2408776.2408794
evidence: A
created: "2026-06-05"
updated: "2026-06-05"
---
# Paper: The Tail at Scale (Dean and Barroso, 2013)

Communications of the ACM 56(2). Why rare slow responses dominate latency
in systems with large fan-out, and how to tolerate them.

- If one server is slow 1% of the time, a request that waits on 100 servers
  is slow 63% of the time.
- Hedged requests: send the same request to a second replica after a short
  delay (for example the 95th percentile latency) and use the first answer.
- Tied requests: send to two servers at once and let each cancel the other
  when it starts work.
- Retrying on timeout without a budget adds load exactly when the system is
  slow.
