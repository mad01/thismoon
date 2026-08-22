# catalog, self-hosted systems catalog (CLI + web service)

A minimal, self-hosted take on the Backstage software catalog. catalog reads
`service-info.yaml` files from repos, builds an in-memory index of **Systems**
and **Components**, and serves a localhost web UI and CLI to browse and search
them by name or owner. It borrows Backstage's entity shape (`kind`, `metadata`,
`spec`) so the files read familiarly. It's not Backstage. This file is the
agent-facing guide: the domain model, and how to keep the catalog in sync as
tools and repos come and go.

## Module layout

```
catalog/
  cmd/catalog/          - entrypoint
  internal/
    catalog/             - pure functional core: parse, scan, index, query, render,
                            uniqueness, remote-URL derivation (fully unit-tested; the
                            only I/O is the scanner's file reads)
    cli/                  - Cobra commands (web, list, validate, config, version)
    web/                  - HTTP shell + JSON API + embedded vanilla frontend
                            (assets/, no build toolchain, //go:embed)
  registry.yaml           - sample registry shipped with the repo
  docs/deep-dive.md       - entity model, registry, validate flow, working on catalog by hand
  Makefile                - part of module github.com/mad01/thismoon (no own go.mod)
```

The System page renders a dependency graph with **Cytoscape.js**, the only
third-party JS in the frontend, vendored and pinned at
`internal/web/assets/static/cytoscape-3.31.0.min.js` (no CDN). To bump it,
replace that file and the `<script>` tag in `assets/index.html`.

## How it works

`catalog list`, `catalog validate`, and `catalog web` all load the catalog the
same way (`catalog.Load`): read the registry, recursively scan every source
root for `service-info.yaml` files (skipping `.git`, `node_modules`, `vendor`,
`.idea`, `dist`, and `build`), parse each into entities, and build an
in-memory index. `catalog web` holds that index behind a read-write mutex so
`POST /api/refresh` can swap in a freshly rescanned copy while requests are in
flight. One page, `index.html`, renders the list, System, and Component views.
The client reads the path and calls the JSON API: the same
client-side-rendering approach `csl` and `events` use (see
`docs/adr/0005-webkit-client-side-rendering.md`).

Every command accepts a persistent `--registry <path>` flag (default
`~/.config/catalog/registry.yaml`) to point at a different registry file.

### Registry

- `registry.yaml` in this repo is the **sample** registry shipped with the
  repo. The **live** registry on this host is
  `~/.config/catalog/registry.yaml`, symlinked from
  `dotfiles/recipes/catalog/registry.yaml`. Keep both in sync: editing one
  doesn't update the other.
- `catalog.Load` (used by `web` and `list`) does **not** enforce global name
  uniqueness, so the UI can still render a catalog that happens to contain a
  collision. Only `catalog validate` enforces it, by calling `CheckUnique`
  explicitly.

### System vs Component: how to decide

- **System** = a logical grouping, declared at a **repo root**. Every repo that
  holds catalogued tools is one System.
- **Component** = a buildable/deployable unit inside a System (a CLI, MCP
  server, library, app, or service), linked to its System via `spec.system`.
- A **monorepo** (e.g. `dotfiles`) is **one System** with **many Components**,
  one `service-info.yaml` per tool subdir plus one at the root for the System.
- A **single-tool repo** is its own System with one Component, both declared in
  a single root `service-info.yaml` (two YAML docs separated by `---`).
- An **iOS app repo** is its own System with the app as its Component.
- **Recipes / install descriptors are NOT entities.** Data-only repos (e.g. a
  note vault, a static site) are a System whose Component is typed `library` or
  `service`. There is no binary to model.

### Global namespace: names must be unique

Every `metadata.name` must be unique across **all Systems and Components
combined**. A System and a Component may not share a name. When a repo and its
binary would collide (repo `ralph`, binary `ralph`), keep the System as the repo
name and give the Component a distinct name: the binary name if it differs
(`kitty-session` → `ks`), otherwise a `-cli` suffix (`ralph` → `ralph-cli`).
`catalog validate` enforces this; run it before merging to catch a duplicate.

### Adding a System or Component

1. Write a `service-info.yaml` (see Data model & storage below for the
   schema):
   - **New tool in the `dotfiles` monorepo** → a `Component` file in the tool's
     subdir with `spec.system: dotfiles`. No registry change (dotfiles is already
     a source).
   - **New standalone repo** → a root `service-info.yaml` with both a `System`
     and its `Component`, then **register the repo path** in *both* registries
     (`catalog/registry.yaml` and `dotfiles/recipes/catalog/registry.yaml`).
2. Validate before committing, checking schema and global uniqueness:
   ```sh
   catalog validate <repo-path> [<repo-path>...]
   ```
   (or `make test` for the core). CI does not run this; validate by hand.
   Names must stay globally unique.
3. Refresh the running UI: the **Refresh** button, `POST /api/refresh`, or
   `t-man restart catalog-web`.

The UI **Add** form does steps 1 and 3 for you, writing the file into a
registered repo (writes are restricted to directories inside a registered
source). It doesn't register a new repo path. Do that by hand for a new repo.

### Removing / retiring

- **Tool deleted from a repo** → delete its `service-info.yaml`, then refresh.
- **Whole repo retired** → delete its `service-info.yaml` *and* remove its path
  from both registries, then refresh.
- **Temporary deprecation** (kept but winding down) → set
  `spec.lifecycle: deprecated` instead of deleting, so it still shows but reads
  as retired.

Whenever a tool or repo is added or removed elsewhere, treat the catalog as part
of "done": its `service-info.yaml` and the registry should reflect reality.

## Data model & storage

```go
Entity{ APIVersion, Kind (System | Component), Metadata{Name, Description, Tags}, Spec{Owner, Type, Lifecycle, System} }
```

- `Metadata.Name` must match `^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$` and
  be ≤63 characters (alphanumeric start/end, with `-`/`_`/`.` allowed in
  between).
- `Spec.Owner` is required on every entity. `Spec.System` is required on
  Components only (the link back to their System). `Spec.Type` and
  `Spec.Lifecycle` are Component-only conventions; `Entity.Validate` doesn't
  enforce their values.
- A `service-info.yaml` file may hold multiple entities separated by `---`,
  the usual shape for a single-tool repo declaring its System and Component
  together:

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

catalog doesn't own or copy this data: `service-info.yaml` files stay in
their source repos, and every `list`/`validate`/`web` run (or
`POST /api/refresh`) rescans them fresh into memory; nothing is cached to disk
between runs. The one write path is the UI's Add form (`WriteServiceInfo`),
which refuses to overwrite an existing `service-info.yaml` so an accidental
add can't clobber hand-written metadata, and is restricted to directories
inside a registered source root.

## Build / install / test

```sh
make build    # go build; build metadata via ldflags, from ../../buildinfo.mk
make install  # build + cp to ~/code/bin/catalog + codesign ("mad01 Local Signing" or adhoc)
make test     # go test -timeout 30s ./...
make lint     # golangci-lint run ./...
make fmt      # gofmt -w .
make tidy     # go mod tidy
```

## HTTP API

- `GET  /`, `/systems/{name}`, `/components/{name}`: SPA shell; one
  `index.html` renders the list, System, and Component views, client-routed
- `GET  /api/entities`: every entity
- `GET  /api/systems` / `GET /api/components`: every System / every Component
- `GET  /api/systems/{name}`: one System plus its Components
  (`{"system": ..., "components": [...]}`)
- `GET  /api/components/{name}`: one Component
- `GET  /api/search?q=&owner=&kind=`: filtered search
- `GET  /api/owners`: distinct owners
- `POST /api/refresh`: rescan; returns `{"systems": n, "components": n}`
- `POST /api/entities`: the Add form backend; body
  `{dir, kind, name, description, tags, owner, type, lifecycle, system}`; 400
  on bad JSON or missing `dir`, 403 if `dir` is outside every registered source
  root, 201 with `{"path": ..., "name": ...}` on success
- `GET  /healthz`: `200 ok` (plain text)
- `GET  /version`: the four-key build metadata object (`version`, `commit`,
  `tag`, `build_time`)
- `GET  /assets/`: embedded static assets
- `GET  /webkit/*`: shared chrome, mounted via `webkit.Mount(mux)`

## Commands

CLI surface beyond `web`:

```sh
catalog list [--owner O] [--system S] [--kind K] [--json] [query]
catalog validate [path...]
catalog config
catalog version [-o json]
```

- `catalog list`: prints Systems/Components as a table (or `--json`). Filter
  with `--owner`, `--system` (Components only), `--kind`; the positional
  argument substring-matches name, description, and tags.
- `catalog validate [path...]`: schema-checks each entity (`Entity.Validate`)
  then enforces global name uniqueness (`CheckUnique`). No arguments validates
  the whole registry; one or more directory/file paths validates just those
  (the mode you'd use to check specific repos before merging).
- `catalog config`: prints the resolved registry path and its status — `loaded,
  N sources`, `missing`, or `parse error: <err>`. Deliberately one line: the
  registry is a data file, and `catalog list` already prints what it produces.
  `catalog config --help` documents the registry format and `--registry`.
- `catalog version`: prints the bare version token (the git commit it was built
  from), the token sibling tools also print so ralph and status can probe any of
  them for the build they are running; `-o json` prints the full build metadata
  object.

## Shared UI: webkit

The chrome (header, theme toggle, font/size/bionic controls) comes from
**`github.com/mad01/thismoon/webkit`**, the in-module package at the repo root
(`webkit/`) that embeds compiled TypeScript/CSS web components. catalog
compiles against the webkit committed beside it; there is no version pin. Do
NOT re-add palette, topbar, or theme CSS locally; those live in webkit only.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // serves /webkit/ (webkit.css, .js, boot.js)
```

Templates load the FOUC guard from webkit (`boot.js`, blocking; no defer/async),
load webkit assets, and use `<wk-header>`:

```html
<head>
  <script src="/webkit/boot.js"></script>
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

### Header markup (catalog)

`internal/web/assets/index.html` uses:

```html
<wk-header brand="catalog.">
  <button data-extra id="refreshBtn">Refresh</button>
  <button data-extra id="addBtn">Add</button>
</wk-header>
```

`[data-extra]` children are rendered in the controls area as-is; `app.js` wires
their click handlers by `id`. `webkit.js` injects the full control set
(font · bionic · size ± · reload · theme). Don't add those controls manually.

### Per-repo changes

- Header markup lives in `internal/web/assets/index.html`.
- webkit components catalog uses:
  - `<wk-card>` with `--accent` kind-color left bar for entity cards
  - `<wk-badge variant="accent">` for kind labels bound to `--accent`
  - `<wk-badge variant="filter" [active]>` for clickable filter pills and
    query chips (the `[active]` attribute fills the pill)
  - `<wk-search>` with a leading `<svg>` icon
  - `<wk-seg>` for segmented controls (e.g. Systems / Components toggle)
  - `<wk-kv variant="card">` for entity detail key/value blocks
  - `<wk-modal>` / `<wk-form>` for the Add entity dialog
  - `<wk-toast-host>` + `<wk-toast>` for save/error feedback
  - `<wk-page-header>`, `<wk-table>`, `<wk-panel>` (+ sub-elements)
- Catalog-bespoke CSS (kept in `app.css`, not part of webkit):
  `--display`, `--sys`, `--comp` CSS variables, `.grain` texture overlay,
  `.section` / `.detail-*` layout classes, `.cy-*` Cytoscape chrome,
  `.icon-btn` icon-only buttons.
- Theme changes fire a `document` event, `wk-themechange`, with
  `detail.theme`. `app.js` listens there and calls `buildGraph()` to
  re-render the Cytoscape dependency graph.

### Version check

`GET /version` returns the four-key build metadata object (`version`, `commit`,
`tag`, `build_time`, every key present and `""` when unknown) from
`github.com/mad01/thismoon/buildinfo`, the consumer contract every
webkit-mounted tool implements.

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `catalog docs`): web-process supervision, registry resolution,
  missing-entity and name-collision triage, version-skew checks. Keep those
  facts there, not here.
- **Only `catalog validate` enforces the global-uniqueness rule.**
  `catalog.Load` (used by `web` and `list`) doesn't, so a name collision can
  sit in the running UI without complaint. Run `catalog validate` before
  merging.
- **Add refuses to overwrite.** `WriteServiceInfo` fails loud if
  `service-info.yaml` already exists in the target directory, so a second Add
  there needs a manual edit instead of clobbering the first.
- **Codesign for the binary.** `make install` strips quarantine xattrs and
  re-signs with the "mad01 Local Signing" identity (falls back to adhoc).

## See also

- Recipe: `recipes/catalog/recipe.toml` (+ `recipes/catalog/CLAUDE.md`)
- Deep dive: `docs/deep-dive.md`, covering entity model, validate output,
  debugging failures, and working on catalog by hand
- Live registry: `dotfiles/recipes/catalog/registry.yaml`
- Shared UI package: `webkit/` at the repo root
