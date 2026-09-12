# ADR-0017: csl reads catalog descriptors from checkouts, not from the catalog service

**Date:** 2026-09-12
**Status:** Accepted

**Scope:** `services/csl` (repo discovery, `csl repo`, `csl_repo_lookup`),
`services/csl/internal/repo/catalogspec`.

## Context

`csl repo` and `csl_repo_lookup` resolve a repo by its org/repo name. The
name is often not what a person has in mind: they think "the platform
team's repos" or "everything in the shop system", and the catalog service
already holds that identity, as Backstage-shaped Component entities read
from `service-info.yaml` files. The question was where csl should get owner
and system from: ask the running catalog service over HTTP, or read the
descriptor from each checkout during the discovery walk it already does.

## Decision

csl reads the descriptor itself. A repo's identity is the first Component
in the root `catalog-info.yaml` (Backstage's name) or `service-info.yaml`
(the catalog service's), and a repo whose descriptor is not at the root can
point csl at it with a root `.csl-catalog.yaml` (`descriptor: <repo-relative
path>`). The reader is a csl-internal file-format package with no dependency
on the catalog service's code and no validation beyond what csl uses.

Three things drove it. Discovery must work with nothing running: csl's
discovery feeds every search, index, and MCP path, and a dependency on a
service at lookup time would turn "which repo" into a distributed question
with a new failure mode. Freshness comes for free: the descriptor is read
from the same working tree the walk found, so a renamed system shows up on
the next `csl repo` with no refresh step. And the two catalogs disagree on
scope: the catalog service indexes what its registry names, while csl indexes
what is checked out under `dirs`; a checkout the catalog does not know still
carries its descriptor, and reading the file answers for it too.

The cost is a second, lenient parser of the same file shape next to the
strict one in `services/catalog`. That is deliberate: csl shows whatever
identity a descriptor carries, including one the catalog would reject for a
missing owner, and the two components release separately. Lifting the reader
into `kit/` waits for a third consumer.

## Consequences

- csl knows one Component per repo: the first one the root descriptor
  declares, or the one `.csl-catalog.yaml` names. A monorepo's many
  components are not enumerated; the pointer file chooses which identity the
  repo shows.
- A malformed descriptor drops that repo's identity, not the repo: discovery
  never fails on a descriptor. `csl doctor` (`catalog-descriptors`) is where
  the drop becomes visible.
- Only the `backstage.io` and `catalog.mad01` apiVersion groups are read,
  any version; a descriptor with a missing apiVersion reads as the catalog
  service's default.
- The catalog service is unaffected: it still scans `service-info.yaml`
  only, so `.csl-catalog.yaml` and a root `catalog-info.yaml` never register
  anything there.
