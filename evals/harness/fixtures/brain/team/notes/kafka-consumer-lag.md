---
title: Kafka consumer lag runbook
tags:
    - runbook
    - kafka
    - oncall
type: note
status: validated
created: "2025-10-06"
updated: "2026-05-11"
---
# Kafka consumer lag runbook

Alert: `KafkaConsumerLagHigh` fires when a consumer group on the
`events-bus` cluster is more than 50,000 messages behind for 10 minutes.

1. Find the group: `kafka-consumer-groups --describe --group <group>`.
2. If one partition lags, look for a stuck consumer and restart that pod.
3. If every partition lags, the consumer is too slow: scale it out, up to
   one consumer per partition.

The group for tracking is `parcel-tracker-v2`, see [[parcel-tracker]].
How lag is computed: [[kafka-consumer-groups-docs]]. Why shipment events
are on Kafka: [[adr-0011-kafka-for-shipment-events]].
