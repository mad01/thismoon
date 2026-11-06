---
title: 'ADR-0011: Kafka for shipment events'
tags:
  - adr
  - kafka
  - messaging
type: note
status: validated
created: "2025-09-18"
updated: "2026-09-30"
id: 01K5E21230YYZDJT60Y46TVHGX
classification: internal
review_by: 2027-09-30
---
# ADR-0011: Kafka for shipment events

Accepted on 2025-09-18.

Shipment events move from RabbitMQ to Kafka on the `events-bus` cluster.
Consumers get replay, and a topic keyed by `shipment_id` keeps each
shipment's events in order. Retention on the cluster is 7 days.
