# catalog architecture

## Overview

catalog is one Go process that doubles as CLI and web service. `catalog web`
runs as the `catalog-web` launchd agent under t-man, listens on 127.0.0.1:7575
(loopback pinned; the UI is unauthenticated), and is reached as
`http://catalog.this/` when d-man routes the hostname. The service's boundary
is narrow: it reads `service-info.yaml` files from the repos listed in a
registry, holds the resulting entity index in memory, and writes exactly one
thing back to disk, a new `service-info.yaml` via the Add form.

## Structure

```
cmd/catalog/        entrypoint, delegates to internal/cli
internal/catalog/   functional core
internal/cli/       Cobra commands: web, list, validate, version
internal/web/       HTTP server, JSON API, embedded frontend (assets/)
```

`internal/catalog` is the pure core: registry loading, the recursive scan for
`service-info.yaml`, YAML parsing into `Entity` values, the index, query and
search, the global-uniqueness check (`CheckUnique`), remote-URL derivation,
and `WriteServiceInfo`. Its only I/O is the scanner's file reads; everything
else stays testable without a filesystem. `internal/cli` and `internal/web`
are the imperative shell around it. The frontend is hand-written vanilla
HTML/CSS/JS compiled in with `//go:embed`; shared chrome comes from the
in-module webkit Go package, mounted with `webkit.Mount(mux)` at `/webkit/`.
The one piece of third-party JavaScript is Cytoscape.js, vendored and pinned
under `internal/web/assets/static/` for the System dependency graph.

## Data flow

Every entry point loads the catalog the same way. `catalog.Load` reads the
registry, `ScanPaths` walks each source root (skipping `.git`,
`node_modules`, `vendor` and friends) collecting `service-info.yaml` files,
each file parses into one or more entities, and `NewCatalog` builds the
in-memory index. `Load` deliberately skips the uniqueness rule so the UI can
render a catalog containing a collision; only `catalog validate` calls
`CheckUnique`.

Web requests follow the client-side rendering model (docs/adr/0005): `GET /`,
`/systems/{name}`, and `/components/{name}` all serve the same embedded
`index.html`, and `app.js` reads the path and fetches `/api/*`. Handlers in
`internal/web/api.go` answer from a catalog snapshot taken under a read lock.

The rescan path is `POST /api/refresh` (the header's Refresh button): it
calls `Server.Reload`, which re-runs `catalog.Load` and swaps the fresh
catalog in under the write lock while in-flight requests finish against the
old one. CLI runs need no refresh; `list` and `validate` scan per invocation.

The write path is `POST /api/entities` (the Add form): the handler rejects a
target directory outside every registered source root with 403, then
`WriteServiceInfo` writes the file and refuses to overwrite an existing one.

## Storage

catalog persists no state of its own; the index is rebuilt in memory on every
run or refresh. On disk it touches two things: the registry it reads
(`~/.config/catalog/registry.yaml` by default, a YAML `sources` list of repo
paths with `~` expanded at load), and the `service-info.yaml` files inside
those repos, read on every scan and written only by the Add form.

## Interfaces

Web UI and JSON API on port 7575: the SPA shell routes above, `GET
/api/entities`, `/api/systems[/{name}]`, `/api/components[/{name}]`,
`/api/search?q=&owner=&kind=`, `/api/owners`, `POST /api/refresh`, `POST
/api/entities`, plus `GET /healthz`, `GET /version` (ldflags-injected sha),
`/assets/`, and `/webkit/*`.

CLI: `catalog web [--port]`, `catalog list [--owner] [--system] [--kind]
[--json] [query]`, `catalog validate [path...]` (schema plus global
uniqueness; the pre-merge check), and `catalog version`. All commands accept
the persistent `--registry <path>` flag.

Config is the registry file alone; there is no other config surface. The
sample registry ships in this directory as `registry.yaml`; the live one on a
machine is wired up by the consuming repo, not here.
