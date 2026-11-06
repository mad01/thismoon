---
title: 'Vendor call notes: CarrierLink API, October 2026'
tags:
    - vendor
    - carrierlink
type: note
status: raw
created: "2026-10-09"
updated: "2026-10-09"
---
# Vendor call notes: CarrierLink API, October 2026

Raw notes from the call with CarrierLink, our parcel carrier API vendor.
Not yet checked against their docs.

- Rate limit is 600 requests per minute per API key.
- Webhooks retry 5 times with exponential backoff, then drop.
- A v3 API with batch label creation is planned for Q2 2027.
