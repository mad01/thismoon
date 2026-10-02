# webkit, shared web-component UI kit

Private Go module + TS package that supplies the **shared theme/header/controls
and content components** for the mad01 local web tools (`present`, `csl`,
`catalog`). Single source of truth for how they look. Authored in
TypeScript/CSS, compiled to committed `dist/`, embedded in consumers via
`go:embed` and served at `GET /webkit/`.

## Layout

```
src/webkit.ts        # custom element definitions, the theme API, Webkit.bootSnippet FOUC guard
src/webkit.css       # topbar chrome, controls, component styles, .wrap layout (reads palette roles only)
src/themes/*.json    # the theme collection: one family per file, light + dark variants
src/themes.mjs       # validates the files, derives the full role set, emits palette CSS + themes.json
src/themes.html      # the picker page served at GET /webkit/themes
src/themes-page.ts   # its script -> dist/themes.js (uses the Webkit global)
src/boot.snippet.js  # the pre-paint FOUC guard -> dist/boot.js
build.mjs            # tsc --noEmit (via npm) then esbuild -> dist/webkit.js (IIFE, global Webkit),
                     # palette blocks prepended to dist/webkit.css, dist/themes.{json,html,js}
dist/                # COMMITTED build artifacts (go:embed needs them; do not gitignore)
embed.go             # //go:embed dist + Handler() http.Handler + FS() + the /webkit/themes route
themes.go            # Themes(), Roles(), IsRole() parsed from the embedded themes.json
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

On theme toggle dispatches `new CustomEvent('wk-themechange', {detail:{theme, palette, mode}})`
on `document` — consumers listen there (catalog/present recolor their Cytoscape graphs).
The same event fires when another tab of the origin changes the theme (the
themes page beside a brief) and when the system appearance flips in system mode.

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
`<wk-section-subheading>` (ruled heading), `<wk-callout variant="info|warn|ok|error">`,
`<wk-progress>` / `<wk-progress-bar>` / `<wk-progress-fill>` /
`<wk-progress-label>`, `<wk-toc>` / `<wk-toc-title>`, `<wk-columns cols="2|3">` /
`<wk-col>` (equal columns, one under 700px), `<wk-stat>` / `<wk-stat-value>` /
`<wk-stat-label>` / `<wk-stat-sub>` (figure over a label; the value is skipped by
fixation), `<wk-figure>` / `<wk-figcaption>` (an image with a caption; read-aloud
reads the image's `alt` in its place), and inside a section the native
`blockquote` (+ `cite`) and `details` (+ `summary`, closed by default; read-aloud
opens it while a part inside plays).

Overlays: `<wk-modal [hidden]>` / `<wk-modal-panel>` / `<wk-modal-head>` /
`<wk-modal-actions>` (fade+slide in; toggle `[hidden]` to open/close),
`<wk-form>` / `<wk-form-row>` / `<wk-form-hint>`,
`<wk-toast-host>` + `<wk-toast variant="ok|err">` (fixed bottom-right stack).

Full spec with markup examples: `COMPONENTS.md`.

## Themes

A theme is a file. `src/themes/<family>.json` names a family (`name`, `label`,
`source`, `license`) and gives it a `light` and a `dark` variant. A variant
has a `variant` name, optional `notes`, and the authored roles:

- surfaces: `bg`, `paper`, `chip`, `border`, `border-hover`
- inks: `text-1`, `text-2`, `text-3`
- accent: `primary` and `on-primary` (ink on an accent fill)
- semantic colours: `red`, `green`, `amber`, `yellow`, `blue`, `purple`
- `series`: four chart colours in order

Every other role is derived at build time by the fixed rules in
`src/themes.mjs`: tags, code tokens, the Cytoscape box and tone colours, chart
grid and text, `text-body`, the alpha surfaces (`topbar-bg`, `scrim`,
`focus-ring`), and the `on-<colour>` inks. A variant can pin any derived role
under `overrides`. The default family does that for every role whose derived
value differs from the old hand-written stylesheet, so it stays byte-identical
(pinned by `TestDefaultFamilyPinsTodaysPalette`).

The build emits every role as a literal hex or rgba, never a `var()` or a
`color-mix()`. Cytoscape and Chart.js parse colour strings themselves, and
`getComputedStyle` returns a custom property's declared text, so present reads
a role with one `cssVar` call. The generated blocks sit at the top of
`dist/webkit.css`:

```css
:root, [data-palette="default"][data-theme="light"] { --bg: #FAF9F7; ... }
[data-theme="dark"], [data-palette="default"][data-theme="dark"] { --bg: #1A1916; ... }
[data-palette="nord"][data-theme="light"] { ... }
[data-palette="nord"][data-theme="dark"]  { ... }
```

No selector is `:root`-scoped, so a subtree can carry its own palette; the
themes page renders every card that way. The rules in `src/webkit.css` read
roles only (`--primary`, `--on-primary`, `--topbar-bg`, `--code-bg`), never a
ramp value. One alias remains: `--terracotta`, declared as `--primary` for
the panel accents present pages stored under that name and still render as
`var(--terracotta)`. The default family pins it to the light-mode primary in
dark, where its primary is a lighter shade. `--green-light` is gone; csl, its
last reader, paints its query tokens with the semantic colour roles.

Selection is three `localStorage` keys. `webkit-theme` keeps `light` | `dark`
and gains `system` (follows `prefers-color-scheme`); `webkit-palette-light` and
`webkit-palette-dark` name the family per mode, absent meaning default. The
boot snippet resolves them before paint into `data-theme` (always `light` or
`dark`) and `data-palette` (absent for default) on `<html>`. The header toggle
stays two-state and writes an explicit mode, which is also the way out of
system mode. The JS API on the `Webkit` global: `themeState()`, `applyTheme()`,
`setThemeMode(mode)`, `setPalette(theme, family)`, `resetTheme()`.

The picker is `GET /webkit/themes`, served by `Handler()` on every consumer
(localStorage is per origin, so each tool needs the page at its own host). It
lists every family that ships a variant under Light and Dark as live sample
cards, with a follow-system checkbox and a reset button. Consumers link it
from their header as a nav link.

Go: `Themes()` returns the parsed collection, `Roles()` the names a page may
reference (`primary`, the six semantic colours, `series-1`..`series-4`, `bg`,
`paper`, `chip`, `tone-<name>-bg`), and `IsRole(name)` checks one. present
validates panel accents against them; the deck layout work validates slide
tones the same way.

Adding a family:

1. Drop a JSON file in `src/themes/`; the file name is the family name.
   `license` and `source` are required. Values come from the upstream
   palette, and anything mixed by hand goes in `notes`.
2. Run `make build` and review the new blocks in `git diff dist/webkit.css`
   and the family in `dist/themes.json`.
3. Add the name to `wantFamilies` in `themes_test.go`.

## FOUC guard — `/webkit/boot.js`
A blocking classic script that reads `localStorage` and sets `data-theme`,
`data-palette`, and the root font size before first paint, so a persisted
dark/large preference or a non-default family doesn't flash.
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
npm test          # node --test; pure-function unit tests (toFixation, clampSize, theme derivation) in test/
go test ./...     # asserts Handler() serves /webkit/webkit.css + .js + the themes page; pins the default palette
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
- **State keys are global per origin** (`webkit-theme`, `webkit-palette-light`,
  `webkit-palette-dark`, `webkit-font`, `webkit-size`, `webkit-fixation`,
  `webkit-ra-speed`). They persist across a tool's own pages, and a change in
  one tab reaches the others through the `storage` event. The one exception
  is the audio toggle. Its key is `webkit-audio` by default, but a shell may
  name a per-view key and default on `<html>` (`data-audio-key`,
  `data-audio-default`), and boot.js and the header both read them. That is
  how present's deck opens with the read-aloud controls hidden under
  `webkit-audio-deck` while its brief keeps its own choice.
- **A theme is a file; a rule reads a role.** Never put a colour literal or a
  ramp name back into `src/webkit.css`: add or derive a role in
  `src/themes.mjs` so every family gets it. A role the graph or chart code
  reads must resolve to a hex literal, and the build refuses one that doesn't.
- **The default family is pinned.** `TestDefaultFamilyPinsTodaysPalette` holds
  the literal values the old stylesheet and present's colour code carried;
  change the default on purpose or not at all.
- **`data-extra` buttons are not wired by webkit.** The consuming app keeps its
  own click handler (e.g. catalog's `refreshBtn`/`addBtn`).
- **Light DOM only.** No Shadow DOM — everything is styled by tag + attribute
  selectors in `webkit.css`, so CSS vars and fixation text-walk work normally.

## See also

- Catalog entry: `service-info.yaml` (System `webkit`, Component `webkit-ui`).
- Consumers live under `services/` as they migrate into this repo.
- Full component spec: `COMPONENTS.md`.
