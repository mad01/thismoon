---
title: dispatch-api
tags:
  - system
  - dispatch
type: note
status: validated
created: "2025-02-11"
updated: "2026-06-20"
id: 01JKT460S0DGCDAF42S5W2V1XA
classification: internal
review_by: 2027-06-20
---
# dispatch-api

Assigns parcels to trucks and serves routes to the driver app.

- Owner: Routing team (#team-routing).
- Production: HTTPS on port 8443 behind the edge proxy. Metrics on 9102.
- Edge request timeout: 5 s.
- Caches routes in Redis, see [[redis-cache-flush]].
- On-call: Routing rotation, escalation per [[oncall-escalation]].

Local development setups differ per laptop, so this vault does not keep them.
