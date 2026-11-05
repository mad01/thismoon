---
title: 'Kafka docs: consumer groups and lag'
tags:
    - external
    - kafka
    - vendor-docs
type: source
status: validated
source: https://kafka.apache.org/documentation/#consumerconfigs
evidence: A
created: "2025-10-06"
updated: "2025-10-06"
---
# Kafka docs: consumer groups and lag

Summary of the Apache Kafka consumer documentation.

- Consumers with the same `group.id` share a topic's partitions; each
  partition goes to one consumer in the group.
- Lag is the log end offset minus the group's committed offset, per
  partition.
- `max.poll.interval.ms` (5 minutes by default) is how long a consumer may go
  between polls before the group rebalances without it.
