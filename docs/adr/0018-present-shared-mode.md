# ADR-0018: present ships a shared mode and a container image

**Date:** 2026-09-22
**Status:** Accepted

**Scope:** `services/present` (serve modes, the page store, the deploy
manifests), `.github/workflows/ci.yml` and `release.yml` (the image jobs),
and the artifact rule in `docs/adr/0007`.

## Context

present pages are agent-written briefings that lived on one machine, served
on loopback with no authentication, which is what ADR-0011's binding rule
and ADR-0007's darwin-only artifacts assume. The pages turned out to be
worth showing to other people: a research summary or a review page is
handed over by link, and a link to localhost is no link at all. The
question was how to put a page somewhere others can reach it without
turning present into a publishing platform with accounts, and without a
second codebase to keep in step with the local one.

## Decision

present gains a shared mode, `present serve --shared`, and it is the same
binary: one process that serves pages by id alone, takes writes only from
callers presenting an author key, and mounts the MCP tools over HTTP. It
runs as a Kubernetes Deployment with several stateless replicas, so
present is the one component that ships a container image
(`ghcr.io/mad01/present`, linux/amd64 and linux/arm64, cosign-signed) as a
second artifact kind; ADR-0007 still governs the tarballs, which stay
darwin/arm64. Pages persist as `Page` custom resources in the pod's
namespace: every replica talks to the API server directly, resourceVersion
guards every write, and each replica sweeps expired ephemeral pages
on a timer, so there is no controller and no leader, and the only
dependency is the cluster itself. A shared page is reachable only by its
32-hex random id, a capability URL; shared mode serves no index and no
listing, on the web or as an MCP tool. Authorship is the SHA-256 of the
bearer key presented at create, stored on the resource and required on
update and delete, so there are no accounts and nothing to revoke. An
ephemeral page carries an expiry 30 days after its last write and the
sweeper deletes it; any other page stays until its author deletes it.
`--bind` keeps ADR-0011's loopback default, and shared mode refuses to
start until a bind is named, so exposure is always a stated choice.

Two alternatives were weighed for the store. A cloud object bucket would
have handled expiry with a lifecycle rule and carried no size cap, at the
price of cloud credentials and a fake for tests; a relational database
would have been the most infrastructure for the least gain at this scale.
Custom resources won because they add no dependency beyond the
cluster, test against kind in CI, and make `kubectl get pages` the
debugging surface. Their cost is the object size cap: a page over 1 MiB is
refused on write.

## Consequences

- `go.mod` gains client-go and its transitive set (roughly sixteen modules),
  and every present build, local included, grows by about 12 MB. The deps
  scanner will surface advisories from that tree that no present code
  path compiles; they are resolved as phantoms, not upgraded away.
- The custom resource definition (CRD) is a public shape. A field change
  needs a new served version and a conversion story; a test keeps the
  embedded copy and `deploy/base/crd.yaml` byte-identical.
- A lost author key orphans its pages until they expire or an operator
  deletes the resources; a leaked key is the author until every page is
  re-shared under a new one. Reads need no key by design, so the
  instance belongs on an internal network and its access logs, which carry
  page ids, are secrets.
- The image build runs on an ubuntu runner with `packages: write`, apart
  from the macOS artifacts job, and the kind end-to-end job adds a few
  minutes to any PR that touches present.
- Machine-private wiring for the shared instance, the hostname, the
  gateway, and each machine's author key, stays in the consuming repo per
  ADR-0006; this repo ships the base manifests and example overlays only.
