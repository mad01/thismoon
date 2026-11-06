---
title: Kafka consumer lag runbook
tags:
  - runbook
  - kafka
type: note
status: validated
created: "2025-06-30"
updated: "2026-09-02"
id: 01JZ01TQY03D8APMK2GFRAEYN5
classification: internal
review_by: 2027-09-02
---
# Kafka consumer lag runbook

When `KafkaConsumerLagHigh` fires for a consumer group on `events-bus`:

1. Open the group's lag per partition on the events-bus dashboard.
2. Lag on every partition means a slow consumer: check its pods first. For
   [[parcel-tracker]], restart one pod at a time; it resumes from the last
   committed offset.
3. Never reset the offsets of a production group without the Messaging team.
