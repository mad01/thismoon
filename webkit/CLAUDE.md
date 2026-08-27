# webkit, shared web-component UI kit

Private Go module + TS package that supplies the **shared theme/header/controls
and content components** for the mad01 local web tools (`present`, `csl`,
`catalog`). Single source of truth for how they look. Authored in
TypeScript/CSS, compiled to committed `dist/`, embedded in consumers via
`go:embed` and served at `GET /webkit/`.

## Layout

```
src/webkit.ts    # custom element definitions + Webkit.bootSnippet FOUC guard
src/webkit.css   # palette (light+dark), topbar chrome, controls, component styles, .wrap layout
build.mjs        # tsc --noEmit (via npm) then esbuild -> dist/webkit.js (IIFE, global Webkit)
dist/            # COMMITTED build artifacts (go:embed needs them; do not gitignore)
embed.go         # //go:embed dist + Handler() http.Handler + FS()
```

## Components

### `<wk-header>` (JS custom element — the only interactive one)
Renders the sticky topbar. Key attributes: `brand`, `brand-href`, `back-label`,
`back-href`, `title`, `controls` (CSV, default `cmdk,font,fixation,size,reload,theme,help`),
`page-width` (default 1080).

Light-DOM children it relocates:
- `<a data-nav>…</a>` → nav links area.
- `<button data-extra id=…>…</button>` → controls area (app wires its own handler by `id`).
- `<template data-wk-help>…</template>` → its innerHTML is appended into the `help`
  control's feature-guide modal, so a consumer adds app-specific sections without JS
  (e.g. speak's "read a document" note). The base modal already documents the shared
  controls plus a read-aloud/speed section when `speed` is present.

On theme toggle dispatches `new CustomEvent('wk-themechange', {detail:{theme}})` on
`document` — consumers listen there (catalog/present recolor their Cytoscape graphs).

### Content components (CSS-only)
All use light DOM only (no Shadow DOM) — styled by tag + attribute selectors in
`webkit.css`.

Layout/content: `<wk-page-header>` / `<wk-title>` / `<wk-subtitle>`,
`<wk-card>` (`--accent` left bar; `a.wk-card`/`[href]` gets hover lift),
`<wk-panel>` / `<wk-panel-title>` / `<wk-panel-subtitle>`,
`<wk-button variant="primary|ghost">`, `<wk-table>`.

Interactive: `<wk-search>` (optional first-child `<svg>` as leading icon),
`<wk-seg>` (`<button class="active">` marks selected segment).

Badges: `<wk-badge variant="a|b|c|outline|stat|accent|filter">`;
`variant="filter"` is a clickable pill, `[active]` fills it with
`--chip-active-bg`; `accent` binds `var(--accent, --primary)`.

Content blocks: `<wk-kv>` / `<wk-kv-row>` / `<wk-kv-label>` / `<wk-kv-value>`
(`variant="card"` adds border + rounded corners + uppercased labels),
`<wk-section>` / `<wk-section-heading>` / `<wk-section-id>` /
`<wk-section-subheading>` (ruled heading), `<wk-callout variant="info|warn">`,
`<wk-progress>` / `<wk-progress-bar>` / `<wk-progress-fill>` /
`<wk-progress-label>`, `<wk-toc>` / `<wk-toc-title>`.

Overlays: `<wk-modal [hidden]>` / `<wk-modal-panel>` / `<wk-modal-head>` /
`<wk-modal-actions>` (fade+slide in; toggle `[hidden]` to open/close),
`<wk-form>` / `<wk-form-row>` / `<wk-form-hint>`,
`<wk-toast-host>` + `<wk-toast variant="ok|err">` (fixed bottom-right stack).

Full spec with markup examples: `COMPONENTS.md`.

## FOUC guard — `/webkit/boot.js`
A blocking classic script that reads `localStorage` and sets `data-theme` + root
font size before first paint, so a persisted dark/large preference doesn't flash.
`Handler()`/`Mount()` serve it at `/webkit/boot.js`. Put it in `<head>` before the
stylesheet and before `webkit.js`. Three ways to reference it, one source:

- **Go-templated consumer:** inject `webkit.BootScript()` (returns `<script src="/webkit/boot.js"></script>`).
- **Static HTML consumer:** hardcode `<script src="/webkit/boot.js"></script>`.
- **At runtime in JS:** the same code is `Webkit.bootSnippet`.

`build.mjs` writes `dist/boot.js` from `src/boot.snippet.js`, and both `BootScript()`
and `Webkit.bootSnippet` resolve to that same string — they can't drift. Do not
hand-paste the IIFE into a consumer; that was the old pattern and it drifted.

## Version check + cache busting
`GET /webkit/version` → `{"module":"github.com/mad01/thismoon/webkit","version":"<hash>"}` —
served `no-store`. `<hash>` is a short SHA-256 over the embedded `dist/` bytes
(computed at package init), which is also the asset `ETag`. It changes whenever
the assets change, so `Cache-Control: no-cache` + content-hash ETag busts stale
CSS/JS. `webkit.js` polls `/webkit/version` every ~5s and reloads on change.
`webkit.NoCacheHTML(w)` sets `Cache-Control: no-cache` for consumer HTML.

## Consumer `/version` contract
Separate endpoint, separate meaning: don't confuse it with `/webkit/version`
above. Each consumer's own HTTP service exposes `GET /version` → the four-key
build metadata object from `github.com/mad01/thismoon/buildinfo`, served
`no-store`:

```json
{
  "version": "9f3c1ab",
  "commit": "9f3c1abf20e4c7d1b8a5e6003f2c9d47a1b6e850",
  "tag": "present/v1.2.3",
  "build_time": "2026-08-13T19:40:02Z"
}
```

The `version` key stays the bare service git sha (status compares that key
alone for binary drift); the other three describe the same build. Nothing
webkit-related appears in it: webkit is an in-module package, so a binary
always embeds the webkit committed alongside it and there is no module pin to
report or compare. Asset drift is `GET /webkit/version`'s job, the
embedded-asset hash, uniform across consumers built from the same commit.

## Build / test

```bash
make build        # SANDBOXED build (default): npm ci + npm run build inside a
                  # throwaway Apple-container VM; artifacts extracted via the
                  # BuildKit local exporter. Needs `container system start`.
make build-local  # fallback: npm run build on the host (container runtime down)
npm test          # node --test; pure-function unit tests (toFixation, clampSize) in test/
go test ./...     # asserts Handler() serves /webkit/webkit.css + .js; checks wk-header and wk-card presence
```

**Why the container build:** `npm ci` runs arbitrary install-time code
(postinstall scripts and the dep tree itself: esbuild, tsc, prismjs +
transitive). The VM keeps a poisoned npm package off the host; nothing
host-side is mounted into the build, only `dist/webkit.js` + `dist/webkit.css`
come out. Output is byte-identical to a host build (bundle output is
platform-independent). Review `git diff dist/` before committing — the bundle
rides into every consumer Go binary via `go:embed`. The general when-to guide
and the extraction-pattern rules live in the dotfiles repo:
[docs/container-builds.md](https://github.com/mad01/dotfiles/blob/main/docs/container-builds.md)
(this repo is its reference implementation).

### Adding a component

1. Add CSS rules in `src/webkit.css` (tag + attribute selectors; CSS-first).
2. Register the element as a no-op in `src/webkit.ts` — one fresh class per tag
   (no shared superclass, so each element stays independently removable).
3. Add an example to `examples/gallery.html`.
4. Document it in `COMPONENTS.md`.
5. Add an assertion in `embed_test.go` that the token appears in `webkit.css`.

## Rules / gotchas

- **Commit `dist/`.** Always `make build` (sandboxed; `make build-local` as
  fallback) before committing source changes so `dist/` stays in sync with
  `src/`, and review the `dist/` diff.
- **esbuild does not typecheck** — the build runs `tsc --noEmit` first. Keep TS strict.
- **The `go` directive lives in the repo root `go.mod`.** webkit carries no
  module files of its own; it compiles with whatever the repo pins.
- **A webkit change reaches every in-repo consumer at its next build.** No
  pins, no bump step: consumers compile against the committed webkit source,
  and CI's fanout runs every component job when `webkit/**` changes. Verify a
  running tool with `curl http://<tool>.this/webkit/version` — the asset hash
  is identical across consumers built from the same commit.
- **State keys are global per origin** (`webkit-theme`, `webkit-font`,
  `webkit-size`, `webkit-fixation`, `webkit-ra-speed`). They persist across a tool's own pages.
- **`data-extra` buttons are not wired by webkit.** The consuming app keeps its
  own click handler (e.g. catalog's `refreshBtn`/`addBtn`).
- **Light DOM only.** No Shadow DOM — everything is styled by tag + attribute
  selectors in `webkit.css`, so CSS vars and fixation text-walk work normally.

## See also

- Catalog entry: `service-info.yaml` (System `webkit`, Component `webkit-ui`).
- Consumers live under `services/` as they migrate into this repo (import
  provenance: `docs/MIGRATED-FROM.md` at the repo root).
- Full component spec: `COMPONENTS.md`.
