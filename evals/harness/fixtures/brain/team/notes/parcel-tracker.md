---
title: parcel-tracker
tags:
    - system
    - tracking
type: note
status: validated
created: "2025-04-02"
updated: "2026-01-31"
---
# parcel-tracker

Turns carrier scan events into the tracking timeline customers see.

- Owner: Tracking team (#team-tracking).
- HTTP API on port 7070.
- Consumes the `parcel-events` topic on `events-bus` as consumer group
  `parcel-tracker-v2`.
- Lag alerts: [[kafka-consumer-lag]].

Moved from RabbitMQ to Kafka with [[adr-0011-kafka-for-shipment-events]].
