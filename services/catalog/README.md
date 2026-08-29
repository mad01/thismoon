# catalog

A minimal, self-hosted take on the Backstage software catalog for the repos and
tools in this fleet. catalog reads `service-info.yaml` files from your repos,
builds an in-memory index of **Systems** and **Components**, and serves a
localhost web UI (plus a CLI) to browse and search them by name or owner. It
borrows Backstage's entity shape (`kind`, `metadata`, `spec`) so the files read
familiarly. It's not Backstage.

## How it works

catalog walks the repos listed in your registry, reads each
`service-info.yaml`, and rebuilds an in-memory index of Systems and Components
on every `list`/`validate`/`web` run (or **Refresh** in the UI /
`POST /api/refresh`). Nothing is cached to disk between runs. See
[`CLAUDE.md`](CLAUDE.md) for the entity model and the full sync workflow.

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
catalog config              # print the registry file in use and how many sources it lists
```

The web UI lists Systems and Components, searches by name/owner, and has detail
pages showing full metadata. **Refresh** re-scans the source repos; **Add**
writes a new `service-info.yaml` into a registered repo (writes are restricted
to directories inside a registered source). Light/dark toggle, light by default.

## Endpoints

- `GET /`, `/systems/{name}`, `/components/{name}`: web UI (one SPA shell for
  all three views)
- `GET /api/entities` / `/api/systems` / `/api/components`: list entities
- `GET /api/systems/{name}` / `/api/components/{name}`: one entity's detail
- `GET /api/search?q=&owner=&kind=`: filtered search
- `GET /api/owners`: distinct owners
- `POST /api/refresh`: re-scan the registry
- `POST /api/entities`: write a new `service-info.yaml` (the Add form)
- `GET /healthz` / `GET /version`

## Configuration

`catalog` discovers entities by walking the repos listed in a registry file
(default `~/.config/catalog/registry.yaml`):

```yaml
sources:
  - path: ~/code/src/github.com/mad01/dotfiles
  - path: ~/code/src/github.com/mad01/thismoon
```

Paths may use `~`; catalog expands them at load time. Point any command at a
different registry with `catalog --registry <path> ...` or `CATALOG_REGISTRY`,
and set `XDG_CONFIG_HOME` to move the default. `catalog web` reads
`CATALOG_PORT` behind `--port`.

`catalog config` prints which registry file was resolved and how many sources it
lists, so you can tell an empty catalog from a registry that never loaded.

## Where things live

- Registry (config): `~/.config/catalog/registry.yaml`
- Binary: `~/code/bin/catalog`
- Entities: `service-info.yaml` files inside each registered repo. catalog
  reads and writes them in place; it never copies or owns them.

## Develop

```sh
make test    # go test ./...
make build   # build the binary
make lint    # golangci-lint
make fmt     # gofmt -w .
```

The functional core (`internal/catalog`) is pure and fully unit-tested:
parsing, scanning, indexing, querying, rendering and the uniqueness rule. All
I/O lives in `internal/cli` and `internal/web`. The web frontend is hand-written
vanilla HTML/CSS/JS embedded with `//go:embed`; it needs no build toolchain.

The only third-party JavaScript is **Cytoscape.js**, used for the dependency
graph on System pages. It is vendored at a pinned version,
`internal/web/assets/static/cytoscape-3.31.0.min.js` (same version as `present`),
and embedded into the binary instead of loaded from a CDN. To bump it, replace
that file and update the `<script>` tag in `assets/index.html`.

`.github/workflows/ci.yml` runs `go test` and `go vet` on every pull request
(per changed component, plus a repo-wide pass). Schema and global-uniqueness
checks are not wired into CI: run `catalog validate` locally before merging to
catch a duplicate name or a malformed entity.

See [`CLAUDE.md`](CLAUDE.md) for the domain model (System vs Component, global
namespace, adding/removing entities) and [`docs/deep-dive.md`](docs/deep-dive.md)
for a longer walkthrough (validate output, debugging failures).

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
