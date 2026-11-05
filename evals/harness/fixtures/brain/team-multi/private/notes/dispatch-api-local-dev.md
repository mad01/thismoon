---
title: dispatch-api local development
tags:
  - dispatch
  - local-dev
type: note
status: validated
created: "2026-03-09"
updated: "2026-10-12"
id: 01KK90CRW07ZX3465TZHGKE05H
classification: internal
review_by: 2027-10-12
---
# dispatch-api local development

How I run [[dispatch-api]] on my laptop.

- `make run-local` serves the API on port 18443, against the docker-compose
  Postgres and Redis.
- Point the driver-app simulator at `https://localhost:18443`.
- Route lookups go to the staging route-planner unless `RP_URL` says
  otherwise.
