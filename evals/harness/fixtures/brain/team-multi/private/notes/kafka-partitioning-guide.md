---
title: Kafka partitioning guide
tags:
  - kafka
  - messaging
  - guide
type: note
status: validated
created: "2026-07-22"
updated: "2026-10-28"
id: 01KY4MEM10TE24PCXYR8ZXBB2Z
classification: internal
review_by: 2027-10-28
---
# Kafka partitioning guide

What I worked out about partitions on `events-bus` while chasing lag on
[[parcel-tracker]].

- Key every shipment topic by `shipment_id`, as
  [[adr-0011-kafka-for-shipment-events]] decided, so one shipment's events
  stay in order.
- `parcel-events` has 24 partitions. Keep partition counts a multiple of 6,
  since the consumer groups run 6 or 12 pods.
- Adding partitions remaps keys. Drain the consumers first, or a shipment's
  events can arrive out of order for a few minutes.
- Lag on a single partition is a hot key, not a slow consumer; the
  [[kafka-consumer-lag]] runbook covers the rest.
