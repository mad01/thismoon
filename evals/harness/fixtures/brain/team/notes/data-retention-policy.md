---
title: Data retention policy for shipment data
owner: data-governance
classification: internal-restricted
tags:
    - policy
    - data
type: note
status: validated
created: "2025-11-03"
updated: "2026-02-17"
---
# Data retention policy for shipment data

- GPS traces from driver devices: 90 days.
- Proof-of-delivery photos and signatures: 7 years, for customs and tax.
- Customer contact details on a parcel: deleted 30 days after delivery by
  the weekly purge job.
- Shipment events in Kafka: 7 days on the cluster; long-term copies live in
  the data warehouse under the same rules.

Questions go to the data governance group.
