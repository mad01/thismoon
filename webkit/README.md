# webkit

Shared web-component UI kit for the mad01 local tools (`present`, `csl`,
`catalog`): one palette, one header, one full component library. Author once,
embed everywhere — so the tools look and behave the same.

## What it ships

- **`dist/webkit.css`** — palette (light + dark), sticky header chrome, layout
  container (`.wrap`, `max-width: var(--page-width, 1080px)`), and all component
  styles.
- **`dist/webkit.js`** — registers `<wk-header>` as a custom element; wires
  controls and persists state in `localStorage`; exports `Webkit.bootSnippet`
  (FOUC (Flash of Unstyled Content) guard) and a thin `Webkit.init(config)` shim
  for backward compatibility.

Both are compiled from `src/` (TypeScript + CSS) and **committed** so Go
consumers can `go:embed` them without a node toolchain.

## Components

See [`COMPONENTS.md`](COMPONENTS.md) for the full design spec. Summary:

| Component | Type | Notes |
|-----------|------|-------|
| `<wk-header>` | JS custom element | Brand/nav/back/title + font·fixation·size·reload·theme controls; fires `wk-themechange` on `document` |
| `<wk-page-header>`, `<wk-title>`, `<wk-subtitle>` | CSS-only | Hero/page header block |
| `<wk-card>` | CSS-only | Content card; `a.wk-card` / `[href]` gets hover lift; `--accent` left bar |
| `<wk-panel>`, `<wk-panel-title>`, `<wk-panel-subtitle>` | CSS-only | Info panel |
| `<wk-badge variant="a\|b\|c\|outline\|stat\|accent\|filter">` | CSS-only | Chips / badges; `variant="filter"` is a clickable pill; `[active]` fills it |
| `<wk-button variant="primary\|ghost">` | CSS-only | Action button |
| `<wk-search>` | CSS-only | Search bar; optional first-child `<svg>` becomes a leading icon |
| `<wk-seg>` | CSS-only | Segmented control; `<button class="active">` marks selected |
| `<wk-table>` | CSS-only | Wraps a descendant `<table>` as a data table |
| `<wk-kv>`, `<wk-kv-row>`, `<wk-kv-label>`, `<wk-kv-value>` | CSS-only | Key/value list; `variant="card"` adds border + rounded corners |
| `<wk-section>`, `<wk-section-heading>`, `<wk-section-id>`, `<wk-section-subheading>` | CSS-only | Document section with a ruled heading |
| `<wk-callout variant="info\|warn">` | CSS-only | Left-bordered callout block |
| `<wk-progress>`, `<wk-progress-bar>`, `<wk-progress-fill>`, `<wk-progress-label>` | CSS-only | Horizontal progress bar |
| `<wk-toc>`, `<wk-toc-title>` | CSS-only | Table of contents block |
| `.wk-prose` | CSS + JS | Rich-markdown container; fenced code is Prism-highlighted client-side with a language badge + copy button |
| `<wk-modal>`, `<wk-modal-panel>`, `<wk-modal-head>`, `<wk-modal-actions>` | CSS-only | Fixed overlay modal; `[hidden]` toggles open/closed |
| `<wk-form>`, `<wk-form-row>`, `<wk-form-hint>` | CSS-only | Form layout, typically inside a modal |
| `<wk-toast-host>`, `<wk-toast variant="ok\|err">` | CSS-only | Fixed bottom-right toast stack |

All components use **light DOM** only (no Shadow DOM) — CSS vars and fixation
text-walk work normally.

## Consume it (Go)

```go
import "github.com/mad01/thismoon/webkit"

webkit.Mount(mux) // serves /webkit/webkit.css + .js + boot.js at "GET /webkit/"
```

`Mount` is the one-liner; it wraps `mux.Handle("GET /webkit/", webkit.Handler())`.

### Head snippet

Load the FOUC guard and assets in `<head>`. The guard sets `data-theme` and
font size before paint so persisted dark/large preferences don't flash. It is
served as a standalone classic script at `/webkit/boot.js` — inject the tag
with `webkit.BootScript()` (it must come before any paint and before webkit.js):

```html
<head>
  {{ .BootScript }}                          <!-- = <script src="/webkit/boot.js"></script> -->
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

Static pages can hardcode `<script src="/webkit/boot.js"></script>`. The same
snippet is also available at runtime as `Webkit.bootSnippet`; all three
(boot.js, `BootScript()`, `Webkit.bootSnippet`) come from one source,
`src/boot.snippet.js`, so they can't drift.

### Markup

```html
<wk-header brand="csl·search">
  <a data-nav class="active" href="/">Search</a>
  <a data-nav href="/examples">Examples</a>
</wk-header>
<main class="wrap">
  <wk-page-header>
    <wk-title>Search your local code</wk-title>
    <wk-subtitle>zoekt across every indexed repo</wk-subtitle>
  </wk-page-header>
  <wk-search><input placeholder="…"><button>Search</button></wk-search>
  <wk-card href="/p/x"><wk-title>…</wk-title></wk-card>
</main>
<script src="/webkit/webkit.js"></script>
```

`webkit.js` injects the full control bar (font · fixation · size ± · reload ·
theme) automatically — do not add those manually.

### Version check + cache busting

`GET /webkit/version` → JSON metadata for the embedded assets:

```json
{"module":"github.com/mad01/thismoon/webkit","version":"3f8a1c0e9b2d4a76"}
```

The version is a short SHA-256 over the embedded `dist/` bytes — it changes
whenever the assets change. It's also the asset `ETag`, so browsers
(`Cache-Control: no-cache`) revalidate and pick up new bytes immediately instead
of holding a stale bundle. `/webkit/version` is served `no-store`. `webkit.js`
polls it every ~5s and reloads the page once the hash changes, so open pages
catch a redeploy on their own.

To stop browsers heuristically caching your own HTML documents, call
`webkit.NoCacheHTML(w)` before writing them — it sets `Cache-Control: no-cache`
so a webkit bump shows up on the next navigation.

### Service version

Each consumer reports its own build at `GET /version`. webkit is an in-module
package: a binary always embeds the webkit committed alongside it, so there is
no separate webkit pin to report. Use the `/webkit/version` asset hash (above)
when checking whether a running tool serves the expected assets.

### Shipping to consumers

There is no bump step: consumers are packages of the same module and compile
against the committed webkit source. Rebuild and reinstall the consumer binary
and the new assets ship with it.

## State

Global `localStorage` keys shared across a tool's pages, default theme light:

| Key | Meaning |
|-----|---------|
| `webkit-theme` | `light` or `dark` |
| `webkit-font` | font family choice |
| `webkit-size` | font size (12–24 px) |
| `webkit-fixation` | fixation reading on/off |

Theme changes dispatch `new CustomEvent('wk-themechange', {detail:{theme}})` on
`document` — listen there to recolor graphs or other dynamic visuals.

## Gallery

`examples/gallery.html` shows every component in one page. Open it in a browser
directly (no server needed) or serve it locally.

## Develop

```bash
make build        # sandboxed build: npm ci + npm run build in an Apple-container VM
make build-local  # fallback: build on the host
npm test          # node --test (pure-function unit tests in test/)
go test ./...     # asserts Handler() serves embedded assets; checks wk-header and wk-card presence
```

`make build` is the default path: npm install-time code (postinstall scripts
and the dependency tree) runs inside a throwaway Linux VM (Apple `container`),
not on your machine. Only `dist/webkit.js` + `dist/webkit.css` come out, and
the output is byte-identical to a host build. One-time setup: install
[apple/container](https://github.com/apple/container/releases) and run
`container system start`. The general containerized-build guide is
[docs/container-builds.md](https://github.com/mad01/dotfiles/blob/main/docs/container-builds.md)
in the dotfiles repo; this repo is its reference implementation.

Edit `src/webkit.ts` and `src/webkit.css`, rebuild, review `git diff dist/`,
**commit `dist/`** (it's intentionally tracked). Consumers pick the change up
at their next build.

### Adding a component

1. Add CSS rules in `src/webkit.css` (tag + attribute selectors; CSS-first).
2. Register the element as a no-op in `src/webkit.ts` (one class per tag — no
   shared superclass, to keep each element independently tree-shakeable).
3. Add an example to `examples/gallery.html`.
4. Document it in `COMPONENTS.md`.
5. Add an assertion in `embed_test.go` that the token appears in `webkit.css`.

## Docs

- [`docs/components.md`](docs/components.md) — full component reference: every `<wk-*>` element with props, variants, events, and markup examples. Read this before adding or restyling a component.
- [`docs/working-on-it.md`](docs/working-on-it.md) — build pipeline, dev loop, adding a new component step-by-step, the `go:embed` contract, and the ship-to-all-consumers flow.
