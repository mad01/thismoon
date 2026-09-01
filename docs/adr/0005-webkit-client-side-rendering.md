# ADR-0005: webkit consumers render client-side from JSON APIs

> Imported from the dotfiles repo (`adr/0017`) with the present service
> migration (MAD-191). Written pre-monorepo: module paths and `make
> update-webkit` references describe the old layout; webkit is now the
> in-module `github.com/mad01/thismoon/webkit` package (see ADR-0002).

**Date:** 2026-06-29
**Status:** Accepted (rollout in progress)

**Scope:** the local `*.this` web tools that mount `github.com/mad01/webkit`:
`present`, `status`, `pr`, `reminder`, `deps`, `speak` (server-rendered today),
plus `csl`, `catalog`, `events` (already client-side). Tracked in Linear
MAD-142; `present` is the pilot (MAD-143).

## Context

The webkit consumers split into two camps. `csl`, `catalog`, and `events`
already render client-side: the backend serves a static shell plus JSON at
`/api/*`, and a hand-written `app.js` builds the DOM. The other six render
server-side: Go `html/template` (or string injection, in `present`) bakes data
into webkit-component markup, coupling presentation logic to the backend.

We want one model: **backend serves data (JSON APIs), frontend owns all
rendering.** Two sub-questions had real trade-offs:

1. **Where do the shared render primitives live?** The three client-side tools
   each hand-roll their own helpers: `csl` and `catalog` carry *identical*
   `escapeHtml`, `catalog` has a composable `el(tag, attrs, children)` DOM
   builder, `events` has a `setInterval` poll. The duplication already meets the
   rule of three.
2. **For `present` specifically, where does the Doc→HTML compile run?** The
   `render.go`/`graph.go` renderers are shared with the MCP (`present_read`).
   Porting them to JS would duplicate ~600 lines of rendering in two languages;
   retiring the Go side would break the MCP and `present rerender`.

## Decision

- **Extract the shared primitives into webkit, not each consumer.** `webkit`
  ships `Webkit.escapeHtml`, `Webkit.el`, and `Webkit.poll` (in `src/render.ts`,
  re-exported on the `Webkit` global). They are small and vanilla; webkit
  provides chrome, components, and these primitives, **not** a rendering
  framework (no React/lit). Consumers stay node-free: each writes a plain-JS
  `app.js` that uses `Webkit.*`.
- **The backend serves a static shell + JSON; the frontend renders.** Each tool
  serves a chrome-only `shell.html` and exposes its data at `/api/*`; `app.js`
  fetches and builds the DOM.
- **Content compiled once at authoring time stays compiled; it is delivered as
  data, not re-rendered in the browser.** For `present`, the Doc→HTML compile
  remains in Go at create/update time (used by the MCP and `present rerender`).
  `GET /api/p/{id}` returns the stored content fragment and the Cytoscape init
  script as fields; `app.js` mounts the content and executes the graph script,
  then initializes charts, references, read-aloud, and theme-recolor
  client-side. **No renderer is duplicated.**

## Consequences

- Presentation logic moves out of Go into per-tool `app.js`; backends shrink to
  data APIs. The page route serves a cacheable-by-revalidation static shell.
- `Webkit.el` builds children via `textContent`, so interpolated data is
  XSS-safe by construction; `Webkit.escapeHtml` covers `innerHTML` paths.
- `present`'s legacy pages (pre-`doc.json`) still work: their stored
  `content.html` is delivered and mounted as-is. The graph arrives in the
  `graph` field (a `<script>` the frontend executes), never as inline
  `<script>` in the content fragment, which `innerHTML` would not run.
- The `present` MCP surface is unchanged; `render.go`'s template-injection
  layer (`Render`, `IndexHTML`, `LoadTemplate`, `LiveReloadScript`) becomes
  unused by the server and is a follow-up prune.
- Pilot scope: `present`'s page view (`/p/{id}`) is client-side; its index (`/`)
  stays server-rendered for now. The remaining five server-side tools migrate to
  the same shell + `/api` + `app.js` pattern, reusing `Webkit.el`/`escapeHtml`/
  `poll`; `csl`/`catalog`/`events` later drop their hand-rolled copies.
- A webkit change ships to all consumers in one session (`make
  update-webkit-all`); adding these helpers is backward compatible, so existing
  consumers take a no-op version bump.

**Repo:** the shared helpers live in `github.com/mad01/webkit` (`src/render.ts`).
