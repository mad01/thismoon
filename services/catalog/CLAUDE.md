# CLAUDE.md — catalog

A minimal self-hosted systems catalog (a small take on the Backstage software
catalog). It reads `service-info.yaml` files from repos, indexes **Systems** and
**Components**, and serves a localhost web UI. See `README.md` for the full model,
schema, install and usage; this file is the agent-facing guide for **keeping the
catalog in sync** as tools and repos come and go.

## Architecture (where things live)

- `internal/catalog/` — pure functional core: parse, scan, index, query, render,
  uniqueness, and remote-URL derivation. Fully unit-tested, no I/O beyond the
  scanner's file reads. Add behaviour here with table-driven tests first.
- `internal/cli/` — Cobra commands (`web`, `list`, `validate`, `version`).
- `internal/web/` — HTTP shell + JSON API + embedded vanilla frontend
  (`assets/`, no build toolchain; `//go:embed`). The System page renders a
  dependency graph with **Cytoscape.js**, the only third-party JS — vendored
  pinned at `assets/static/cytoscape-3.31.0.min.js` (no CDN). To bump it,
  replace that file and the `<script>` tag in `assets/index.html`.
- `registry.yaml` — the **sample** registry shipped with the repo. The **live**
  registry on this host is `~/.config/catalog/registry.yaml`, symlinked from
  `dotfiles/recipes/catalog/registry.yaml`. Keep both in sync.

## System vs Component — how to decide

- **System** = a logical grouping, declared at a **repo root**. Every repo that
  holds catalogued tools is one System.
- **Component** = a buildable/deployable unit inside a System — a CLI, MCP
  server, library, app, or service — linked to its System via `spec.system`.
- A **monorepo** (e.g. `dotfiles`) is **one System** with **many Components**,
  one `service-info.yaml` per tool subdir plus one at the root for the System.
- A **single-tool repo** is its own System with one Component, both declared in
  a single root `service-info.yaml` (two YAML docs separated by `---`).
- An **iOS app repo** is its own System with the app as its Component.
- **Recipes / install descriptors are NOT entities.** Data-only repos (e.g. a
  note vault, a static site) are a System whose Component is typed `library` or
  `service` — there is no binary to model.

## Global namespace — names must be unique

Every `metadata.name` must be unique across **all Systems and Components
combined** — a System and a Component may not share a name. When a repo and its
binary would collide (repo `ralph`, binary `ralph`), keep the System as the repo
name and give the Component a distinct name: the binary name if it differs
(`kitty-session` → `ks`), otherwise a `-cli` suffix (`ralph` → `ralph-cli`,
`leroy` → `leroy-cli`). `catalog validate` enforces this and fails a PR on a
duplicate.

## Adding a System or Component

1. Write a `service-info.yaml` (see `README.md` for the schema):
   - **New tool in the `dotfiles` monorepo** → a `Component` file in the tool's
     subdir with `spec.system: dotfiles`. No registry change (dotfiles is already
     a source).
   - **New standalone repo** → a root `service-info.yaml` with both a `System`
     and its `Component`, then **register the repo path** in *both* registries
     (`catalog/registry.yaml` and `dotfiles/recipes/catalog/registry.yaml`).
2. Validate before committing — schema + global uniqueness:
   ```sh
   catalog validate <repo-path> [<repo-path>...]
   ```
   (or `make test` for the core). Names must stay globally unique.
3. Refresh the running UI: the **Refresh** button, `POST /api/refresh`, or
   `t-man restart catalog-web`.

The UI **Add** form does steps 1 and 3 for you, writing the file into a
registered repo (writes are restricted to directories inside a registered
source). It does not register a new repo path — do that by hand for a new repo.

## Removing / retiring

- **Tool deleted from a repo** → delete its `service-info.yaml`, then refresh.
- **Whole repo retired** → delete its `service-info.yaml` *and* remove its path
  from both registries, then refresh.
- **Temporary deprecation** (kept but winding down) → set
  `spec.lifecycle: deprecated` instead of deleting, so it still shows but reads
  as retired.

Whenever a tool or repo is added or removed elsewhere, treat the catalog as part
of "done": its `service-info.yaml` and the registry should reflect reality.

## Shared UI: webkit

The chrome (header, theme toggle, font/size/bionic controls) comes from
**`github.com/mad01/thismoon/webkit`** — the in-module package at the repo root
(`webkit/`) that embeds compiled TypeScript/CSS web components. catalog
compiles against the webkit committed beside it; there is no version pin. Do
NOT re-add palette, topbar, or theme CSS locally; those live in webkit only.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // serves /webkit/ (webkit.css, .js, boot.js)
```

Templates load the FOUC guard from webkit (`boot.js`, blocking — no defer/async),
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
their click handlers by `id`. The full control set (font · bionic · size ± ·
reload · theme) is injected by `webkit.js` — do not add those controls manually.

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
- Theme changes fire `document` event `wk-themechange` with `detail.theme` —
  `app.js` listens there to call `buildGraph()` for the Cytoscape dependency
  graph re-render.

### Version check

`GET /version` → `{"version":"<catalog git sha>"}` — the catalog build version
(ldflags-injected).

## Validate / test

```sh
make test    # go test ./... (race)
make lint    # golangci-lint
catalog validate <paths>   # schema + global name uniqueness (also runs in CI)
```
