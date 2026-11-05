---
title: 'ADR-0011: Move shipment events to Kafka'
tags:
    - decision
    - messaging
type: note
status: validated
created: "2025-09-22"
updated: "2026-02-02"
---
# ADR-0011: Move shipment events to Kafka

Status: accepted, 2025-09-22. Supersedes
[[adr-0004-rabbitmq-for-shipment-events]].

## Context

Tracking and billing both need to replay shipment events after bugs, and
RabbitMQ drops messages once they are acked. Event volume grew to about
4,000 messages per second at peak.

## Decision

Shipment events move to Kafka on the `events-bus` cluster: topic
`shipment-events`, 12 partitions, 7-day retention, keyed by parcel id.
Carrier scans go to `parcel-events` with the same settings.

## Consequences

Consumers track their own offsets. The `rmq-ship` RabbitMQ cluster was
decommissioned on 2026-01-31.
