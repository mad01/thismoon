---
title: 'ADR-0009: gRPC for internal service APIs'
tags:
    - decision
    - api
type: note
status: validated
created: "2025-05-27"
updated: "2025-05-27"
---
# ADR-0009: gRPC for internal service APIs

Status: accepted, 2025-05-27.

New internal service-to-service APIs use gRPC with protobuf schemas in the
`bw-protos` repo. Public and driver-app APIs stay HTTPS and JSON. The first
service built this way is [[ledger-svc]].
