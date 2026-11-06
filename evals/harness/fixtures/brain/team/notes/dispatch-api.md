---
title: dispatch-api
tags:
    - system
    - dispatch
type: note
status: validated
created: "2025-02-11"
updated: "2026-06-20"
---
# dispatch-api

Assigns parcels to trucks and serves routes to the driver app.

- Owner: Routing team (#team-routing).
- Port: 8443 (HTTPS). Metrics on 9102.
- Edge request timeout: 5 s.
- Calls route-planner for routes; timeouts and retries are in
  [[rp-timeouts]].
- Caches routes in Redis, see [[redis-cache-flush]].
- On-call: Routing rotation, escalation per [[oncall-escalation]].
