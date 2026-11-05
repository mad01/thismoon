---
title: 'ADR-0004: RabbitMQ for shipment events'
tags:
    - decision
    - messaging
type: note
status: validated
created: "2024-05-13"
updated: "2024-05-13"
---
# ADR-0004: RabbitMQ for shipment events

Status: accepted, 2024-05-13.

## Context

Shipment status events go from the carrier gateway to tracking, billing and
notifications. We need at-least-once delivery and per-consumer queues.

## Decision

Shipment events go through RabbitMQ on the `rmq-ship` cluster, one quorum
queue per consumer, bound to the `shipment.events` topic exchange.

## Consequences

The Platform team runs `rmq-ship`. Consumers ack after processing.
Replaying old events is not possible once a message is acked.
