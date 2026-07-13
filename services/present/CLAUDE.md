# present, CLI + MCP server for served briefing pages

Go CLI + MCP server. Manages single-page HTML presentations as create / read / update / list; delete exists only in the web index (confirm dialog → `DELETE /p/{id}`), not as an MCP tool. Content is authored as structured JSON (Doc format) and compiled to HTML at authoring time (`internal/render` `RenderDoc`/`RenderGraph`, run by `present_create`/`present_update`/`present rerender`). **The page view renders client-side**: `GET /p/{id}` serves a static chrome-only shell (`internal/server/shell.html`), and `app.js` (served at `GET /app.js`) fetches the page as JSON from `GET /api/p/{id}` and builds the DOM in the browser, replacing the old live-reload script with `Webkit.poll` on `/p/{id}/version`. The **index** view (`GET /`) renders client-side the same way: it serves a static shell (`internal/server/index_shell.html`) and `index.js` (served at `GET /index.js`) fetches the page list as JSON from `GET /api/pages` and builds the list in the browser. Pages are served over localhost. See `docs/adr/0005-webkit-client-side-rendering.md`.

## Module layout

```
present/
  cmd/present/         - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra command tree: root, serve, mcp, version (Version via ldflags)
    store/             - filesystem CRUD over pages/<id>/ (id.go, store.go); Delete is web-index-only, not exposed via MCP
    render/            - Doc-to-HTML renderer (doc.go), Graph-to-JS renderer (graph.go), legacy raw-HTML upgrade (upgrade.go); RenderDoc/RenderGraph run at authoring time
    server/            - HTTP handlers: GET / (static index shell), /api/pages (page list as JSON), /index.js (client index renderer), /p/{id} (static shell.html), /api/p/{id} (page as JSON), /app.js (embedded client renderer), /p/{id}/version, DELETE /p/{id}; embeds index_shell.html, shell.html, index.js, app.js
    mcpserver/         - MCP wiring + present_* tools
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- **`present mcp`** (stdio MCP server): tools write/read pages under the workdir. Never serves HTTP.
- **`present serve`** (HTTP server): serves the static shell + `app.js` for page views (the browser fetches `GET /api/p/{id}` and renders), and the static index shell + `index.js` for the index (the browser fetches `GET /api/pages` and renders).
- Both share the workdir (`~/.config/present` by default) and port (`7423`), set via `--workdir`/`PRESENT_WORKDIR` and `--port`/`PRESENT_PORT`. URLs the MCP returns point at the serve port.

## Data model & storage

```
~/.config/present/
  pages/<id>/
    meta.json              # {id,title,version,has_graph,has_refs,has_doc,created_at,updated_at}
    content.html           # rendered HTML body fragment
    doc.json               # canonical Doc source (only when content was given as Doc JSON)
    graph.js               # optional rendered cytoscape init (only when has_graph)
    graph.json             # canonical GraphInput source (only when graph was given as JSON)
    refs.json              # optional [{title,url},...] (only when has_refs)
```

`doc.json` and `graph.json` are the editable sources: `present_source` returns
them for mutation from a later session, and `present rerender` re-renders
`content.html`/`graph.js` from them after renderer or template changes (e.g. a
webkit bump). Pages created from raw HTML/JS have no source files; rerender
falls back to a deterministic legacy-HTML upgrade and leaves legacy JS graphs
untouched.

## Build / install / test

```bash
make build    # ./present binary
make install  # build + cp to ~/code/bin/present + adhoc codesign
make test     # go test ./...
make tidy     # go mod tidy
make resign   # re-apply adhoc signature (macOS)
```

## Commands

`present rerender [id...]` (all pages when no ids) re-renders each page through
the current renderer: from `doc.json`/`graph.json` when present, otherwise a
deterministic legacy-HTML class→wk-* upgrade. Unchanged pages are skipped;
changed ones get a version bump so open tabs live-reload. Run it after a webkit
bump or renderer change that alters emitted markup.

## MCP tools

- `present_create(title, content, graph?, references?)` → `{id, url, version}`
  - `content`: Doc JSON object `{summary?, meta?, chips?, sections:[{h, blocks}]}` or legacy HTML string (auto-detected)
  - `graph`: Graph JSON object `{nodes, edges, layout?}` or legacy JS string (auto-detected)
  - `references`: `[{title, url}]`
- `present_read(id)` → full page with rendered HTML content (includes references)
- `present_source(id)` → editable source: `{content_format: "doc"|"html", content, graph_format: "json"|"js"|"", graph, references}`: outputs round-trip directly into `present_update` (it auto-detects both formats); use this to mutate a page from a new/restored session
- `present_update(id, title?, content?, graph?, references?)` → patch (omitted fields unchanged; empty graph clears it; empty array clears refs); bumps version. Doc/graph JSON input refreshes `doc.json`/`graph.json`; raw HTML/JS input deletes the now-stale source file
- `present_list()` → all pages (metadata only, no content), newest first; `has_doc` flags source-editable pages
- `present_open(id)` → macOS `open` (call once per page; updates auto-reload)

## Shared UI: webkit

The chrome (header, theme toggle, font/size/bionic controls) comes from the
in-module package **`github.com/mad01/thismoon/webkit`**, which embeds compiled
TypeScript/CSS web components. Do NOT re-add palette, topbar, or theme CSS
locally; those live in webkit only. There is no pin or bump step: the binary
compiles against the webkit committed alongside it.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // mux.Handle("GET /webkit/", webkit.Handler()) - serves webkit.css/.js + /webkit/boot.js
```

Templates load the FOUC guard (served by webkit at `/webkit/boot.js`) and the
webkit assets:

```html
<head>
  <script src="/webkit/boot.js"></script>
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

The boot script is blocking (no `defer`/`async`) on purpose: it must set
`data-theme` before first paint to avoid a flash of the wrong theme.

### Header markup (present)

The brief template uses:
```html
<wk-header back-href="/" back-label="← All" title="Brief"></wk-header>
```

The index template uses:
```html
<wk-header brand="present"></wk-header>
```

`webkit.js` injects the full control set (font · bionic · size ± · reload ·
theme). Do not add those controls manually.

### Per-repo changes

- Header markup lives in `internal/server/shell.html` (page view) and
  `internal/server/index_shell.html` (index listing).
- Available content components (CSS-only, no extra JS):
  `<wk-page-header>`, `<wk-card>`, `<wk-badge>`, `<wk-search>`,
  `<wk-table>`, `<wk-panel>` (+ `<wk-panel-title>` / `<wk-panel-subtitle>`),
  and the briefing-block components: `<wk-section>` (+ `<wk-section-heading>` /
  `<wk-section-id>` / `<wk-section-subheading>`), `<wk-toc>` (+ `<wk-toc-title>`),
  `<wk-kv>` (+ `<wk-kv-row>` / `<wk-kv-label>` / `<wk-kv-value>`),
  `<wk-progress>` (+ `<wk-progress-bar>` / `<wk-progress-fill>` /
  `<wk-progress-label>`), `<wk-callout>` (`variant="info|warn"`), and
  `pre.wk-code-block` (standalone code block, the `t=code` Doc block renders
  to it; webkit.js adds the language badge, copy button, and Prism highlight).
- The index listing uses `<a class="wk-card">` + `<wk-page-header>` for the
  page list entries.
- Theme changes fire `document` event `wk-themechange` with `detail.theme`:
  present's brief template listens there to recolor the Cytoscape graph and
  rebuild any metric charts (`initPresentCharts`).
- Present's briefing blocks (sections, toc, kv, progress, callout) are NOW
  webkit components: the Doc renderer emits `<wk-section>` / `<wk-toc>` /
  `<wk-kv>` / `<wk-progress>` / `<wk-callout>` (alongside `<wk-table>` /
  `<wk-panel>` / `<wk-badge>`), all styled by `/webkit/webkit.css`. Only present-
  specific chrome stays local in `internal/server/shell.html`: the hero (`.brief-title` /
  `.brief-meta` / `.brief-summary`), the `.chip-row` flex container, the
  Cytoscape graph chrome (`.cy-*`), the metric-chart chrome (`.present-chart`),
  `.brief a` link styling, `.brief-list`, and the `.refs-*` references block.
- **Metric charts are present-local, not webkit.** The `t=chart` Doc block
  (`kind` = `bar`/`area`/`sparkline`) renders a `<div class="present-chart">`
  with a JSON spec in a `<script type="application/json">` island
  (`chartSpec` returns `template.JS` so html/template emits it verbatim instead
  of re-encoding it inside the script context). The `initPresentCharts`
  bootstrap in `app.js` reads each spec and builds a Chart.js instance,
  mirroring how the Cytoscape graph works: a vendored lib in `assets/js/`
  (`chart-4.4.6.umd.min.js`, fetched by `scripts/cache-assets.sh`) plus a
  theme-aware color function. Charts are inline blocks, many per page,
  unlike the single `graph` argument.

### Webkit

present is a webkit consumer like the other services in this repo. The chrome
comes from the in-module package `github.com/mad01/thismoon/webkit`; there is
no pin or bump step; the binary compiles against the webkit committed alongside
it, so a webkit change ships at the next build.

### Version check

`GET /webkit/version` → `{"module":"github.com/mad01/thismoon/webkit","version":"<asset hash>"}`:
confirms which embedded webkit assets the running present server serves.

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe (wave 1) registers it.
- **Two processes.** `present mcp` only touches files; pages don't render until `present serve` runs (as a t-man agent, see the recipe).
- **Shared workdir is load-bearing.** The MCP and serve must resolve the *same* workdir/port or updates land where nothing serves them. `serve` pins `--workdir/--port`; the MCP is pinned via `env` in the consuming repo's `recipes/claude-mcp/servers.json` (`PRESENT_WORKDIR`/`PRESENT_PORT`). A leading `~` in either is expanded in Go (`expandTilde`, `root.go`) since neither shell nor `sync_mcp.py` expands env values. Both processes log their resolved `workdir=… port=…` at startup; diff those first if updates don't appear.
- **Request logging → stderr.** `serve` logs one line per request (`method path -> status bytes (dur)`) to stderr: `t-man logs present --stderr`. A reload-loop (repeated `GET /p/<id>` every ~1s) means the served HTML was cached; pages set `Cache-Control: no-store` to prevent exactly that.
- **Page view is a static shell + client render.** `GET /p/{id}` serves `internal/server/shell.html` (chrome only: `<wk-header>` + webkit assets), and `app.js` builds the body in the browser from `GET /api/p/{id}` JSON (`{id,title,version,has_graph,content,graph,references}`): it mounts `content` as innerHTML, runs the `graph` field as a `<script>`, then inits the Cytoscape graph, metric charts, references, read-aloud, and theme-recolor. The old `{{CONTENT}}`/`{{GRAPH_SCRIPT}}`/`{{REFERENCES}}`/`{{LIVE_RELOAD}}` template substitutions (`render.Render` + `template.html`) have been removed; the index renders client-side from `index_shell.html` + `index.js` the same way. The header/theme/font/size/bionic chrome lives in the in-module `webkit` package (served at `/webkit/`; see *Shared UI: webkit* above).
- **Client render uses webkit's shared helpers.** `app.js` builds the DOM with `Webkit.el` / `Webkit.escapeHtml` and polls `/p/{id}/version` for live-reload via `Webkit.poll` (webkit shared helpers). Decision recorded in `docs/adr/0005-webkit-client-side-rendering.md`.
- **Theme/controls state is global (webkit), not per-page.** Light/dark/font/size/bionic are stored under global `localStorage` keys (`webkit-theme`/`webkit-font`/`webkit-size`/`webkit-bionic`), shared across all present pages, default light. (The old per-page `brief-theme:<pathname>` keys are gone.) **The page view is served from the embedded `shell.html`, not the workdir**: there is no on-disk template. `serve` and `mcp` no longer seed `~/.config/present/template.html` — the file and the `render.Render`/`EnsureTemplate` layer have been removed. A shell or `app.js` change ships by rebuild + `t-man restart present`.
- **Codesign for MCP.** macOS kills adhoc-signed binaries with stale provenance xattrs; `make install` re-signs.
- **Version probe.** `present version -o json` → `{"version":"<sha>"}` (the cross-tool convention `ralph outdated` uses). The recipe bakes this sha into the t-man service env so a new build reloads the running `serve` agent automatically.

## See also

- Recipe: `recipes/present/recipe.toml` (+ `recipes/present/CLAUDE.md`)
- Skill: `recipes/claude/skills/present/SKILL.md`
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json`
