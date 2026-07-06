# catalog

A minimal systems catalog for your tools — a small, self-hosted take on the
Backstage software catalog. It reads `service-info.yaml` files from your repos,
builds an in-memory index of **Systems** and **Components**, and serves a
localhost web UI to browse and search them by name or owner.

Not Backstage — but it borrows Backstage's entity shape (`kind`, `metadata`,
`spec`) so the files read familiarly.

## Model

- **System** — a logical grouping, declared at a repo root. A monorepo is one
  System with many Components; a single-tool repo is a System with one.
- **Component** — a buildable/deployable unit (a CLI, MCP server, library, app)
  that links to its System via `spec.system`.
- **Global namespace** — every name must be unique across *all* Systems and
  Components combined. A System and a Component may not share a name.
- **Recipes are not entities** — install descriptors stay out of the catalog.

## service-info.yaml

A file may hold multiple entities separated by `---` (e.g. a single-tool repo
declaring its System and Component together).

```yaml
apiVersion: catalog.mad01/v1alpha1
kind: System
metadata:
  name: dotfiles
  description: Personal dev-tooling monorepo
  tags: [monorepo, tooling]
spec:
  owner: mad01
---
apiVersion: catalog.mad01/v1alpha1
kind: Component
metadata:
  name: present
  tags: [go, cli, mcp]
spec:
  type: cli            # cli | mcp-server | library | app | service
  lifecycle: production
  owner: mad01
  system: dotfiles
```

## Registry

`catalog` discovers entities by walking the repos listed in a registry file
(default `~/.config/catalog/registry.yaml`):

```yaml
sources:
  - path: ~/code/src/github.com/mad01/dotfiles
  - path: ~/code/src/github.com/mad01/thismoon
```

## Install

```sh
make install        # builds and copies to ~/code/bin/catalog
mkdir -p ~/.config/catalog && cp registry.yaml ~/.config/catalog/registry.yaml
```

## Usage

```sh
catalog web                 # serve the UI on http://127.0.0.1:7575 (loopback only)
catalog list                # print all entities
catalog list --owner mad01  # filter by owner
catalog list --kind System  # filter by kind
catalog validate            # schema-check + enforce global name uniqueness
catalog validate ./path     # validate specific dirs/files (used by CI)
```

The web UI lists Systems and Components, searches by name/owner, and has detail
pages showing full metadata. **Refresh** re-scans the source repos; **Add**
writes a new `service-info.yaml` into a registered repo (writes are restricted
to directories inside a registered source). Light/dark toggle, light by default.

## PR check

`.github/workflows/ci.yml` runs `go test -race`, `go vet`, and
`catalog validate` on every pull request. Validation enforces the schema and
the global-uniqueness rule, so a PR that introduces a duplicate name fails.
Cross-repo uniqueness checking is enabled by adding a read-only
`CATALOG_RO_TOKEN` secret; without it, only this repo's entities are checked.

## Development

```sh
make test    # go test ./...
make build   # build the binary
make lint    # golangci-lint
make fmt     # gofmt -w .
```

The functional core (`internal/catalog`) is pure and fully unit-tested:
parsing, scanning, indexing, querying, rendering and the uniqueness rule. All
I/O lives in `internal/cli` and `internal/web`. The web frontend is hand-written
vanilla HTML/CSS/JS embedded with `//go:embed` — no build toolchain.

The only third-party JavaScript is **Cytoscape.js**, used for the dependency
graph on System pages. It is vendored at a pinned version,
`internal/web/assets/static/cytoscape-3.31.0.min.js` (same version as `present`),
and embedded into the binary — no CDN, no floating version. To bump it, replace
that file and update the `<script>` tag in `assets/index.html`.

## Docs

[`docs/deep-dive.md`](docs/deep-dive.md) — entity model detail, service-info.yaml spec, registry layout, validate output, adding/removing entities, debugging failures, and architecture overview.
