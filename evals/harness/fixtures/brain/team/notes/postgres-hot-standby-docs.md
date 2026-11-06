---
title: 'Postgres docs: hot standby'
tags:
    - external
    - postgres
    - vendor-docs
type: source
status: validated
source: https://www.postgresql.org/docs/current/hot-standby.html
evidence: A
created: "2025-09-02"
updated: "2025-09-02"
---
# Postgres docs: hot standby

Summary of the PostgreSQL manual chapter on hot standby.

- A hot standby serves read-only queries while it replays WAL.
- Replay can conflict with queries on the standby; `max_standby_streaming_delay`
  bounds how long replay waits before it cancels them (30 s by default).
- `hot_standby_feedback` stops the primary from vacuuming rows a standby
  query still needs, at the cost of bloat on the primary.
