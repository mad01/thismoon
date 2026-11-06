---
title: 'Idea: Kafka tiered storage for parcel events'
tags:
    - kafka
    - idea
type: note
status: raw
created: "2026-10-14"
updated: "2026-10-14"
---
# Idea: Kafka tiered storage for parcel events

Unverified. Tracking wants 30 days of `parcel-events` for replays, the
cluster keeps 7 (see [[adr-0011-kafka-for-shipment-events]]). Tiered storage
could keep old segments in object storage. Need to check broker version and
what [[parcel-tracker]] replays actually read.
