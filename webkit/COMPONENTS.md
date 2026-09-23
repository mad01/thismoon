# webkit components — design spec

Goal: a small, reusable UI kit shared by `present`, `csl`, `catalog`. React-like
**composition without React** via **web components (custom elements), light DOM**,
styled by the shared `webkit.css`. Consumers write declarative markup
(`<wk-card>…`) in their server-rendered HTML; `webkit.js` upgrades the interactive
ones. CSS-vars (palette) and the fixation text-walk keep working because everything
is light DOM (no Shadow DOM).

## Principles
- **Light DOM only.** No Shadow DOM. Components render into themselves / are styled
  by tag + attribute selectors in `webkit.css`.
- **CSS-first.** Most components are pure CSS on the tag name (`wk-card { … }`,
  `wk-badge[variant="a"] { … }`) with `display:block` defaults — no JS needed.
- **JS only for behavior.** Only `<wk-header>` needs a custom-element class
  (controls + persistence + event). Others may be defined as no-op elements for
  semantics but require no JS.
- **Palette unchanged.** Keep present's palette/vars already in `webkit.css`
  (`--terracotta`, warm-grays, `--page-width`, light+dark). The component visuals
  (`.panel`, `.data-table`, `.chip*`, `.callout`, etc.) were ported verbatim into
  `webkit.css` from present's original page template and from csl/catalog
  `internal/web/assets/static/app.css` (`.card`, `.search-bar`, `.kind-badge`).

## Components

### `<wk-header>`  (JS custom element)
Replaces `Webkit.init()`. Renders the sticky `.topbar` with `.topbar-inner`
(max-width `--page-width`, centered). Attributes:
- `brand`, `brand-href` (default `/`) — wordmark (wrap trailing `.`/`·` in `.dot`).
- `back-label`, `back-href` — optional back link (present's "← All").
- `title` — small muted label (present's "Brief").
- `controls` — CSV, default `cmdk,font,fixation,size,reload,theme,help`.
- `page-width` — number, sets `--page-width` (default 1080).
- `fixation-targets` — selector override.
- `help` — a `?` control that opens a feature-guide modal (dismiss on X, Esc, or
  a click outside). It documents the controls this header renders, adds a
  read-aloud/speed section when `speed` is present, and appends the innerHTML of
  any `<template data-wk-help>` in the page so a consumer can add its own
  sections (e.g. speak's file upload).

Light-DOM children it relocates into the bar:
- `<a data-nav [class=active]>…</a>` → nav links area.
- `<button data-extra id=…>…</button>` → controls area (rendered as-is, app keeps
  its own click handler — e.g. catalog's `refreshBtn`/`addBtn`).

Behavior (ported from the current `Webkit.init`): font/fixation/size/reload/theme
controls wired to GLOBAL keys `webkit-theme|font|size|fixation` (default light,
size 16 clamp 12–24, fixation persisted). On theme toggle it sets `data-theme`,
saves, and **dispatches `new CustomEvent('wk-themechange', {detail:{theme}})` on
`document`** (consumers listen to recolor graphs, replacing the `onThemeChange`
callback). Keep a thin `Webkit.init(cfg)` shim that creates a `<wk-header>` from a
config object for backward-compat, but markup is the primary API.

### `<wk-read-aloud>`  (JS custom element)
Per-section text-to-speech playback. One element per page; it renders nothing
itself and instead injects a floating play/pause button into every `targets`
match. Attributes:
- `targets` — CSS selector for the readable sections (required).
- `endpoint` — base URL of the speech service (default `http://speak.this`).
- `voice` — Kokoro voice id (default `af_heart`).
- `speed` — playback speed multiplier (default `1`).

Backend contract: the endpoint must serve an OpenAI-compatible
`POST {endpoint}/v1/audio/speech` accepting `{model, input, voice, speed}` and
returning a WAV body, with CORS allowed (the d-man `speak` route provides both
via mlx-audio + the route's `cors = true` flag). On connect the element probes
`GET {endpoint}/` with a 1.5s timeout — if unreachable it injects nothing, so
pages degrade gracefully on hosts without the speak service.

Playback: section text is extracted skipping `code/pre/table/svg/button`
subtrees, split into sentences (`Intl.Segmenter`), fetched one clip per
sentence with the next clip prefetched while the current plays. The active
sentence is wrapped in `.wk-ra-sentence.wk-ra-active` (soft `--ra-highlight`
background) and kept in view. One session at a time: starting another section
stops the current one; the section button toggles pause/resume. While a section
plays, a restart button appears next to its play/pause control and rewinds to
the start of the section; it hides again when playback ends.

Selection speaker: independent of `targets`, selecting any text on the page
(outside the header/controls) shows a floating play button at the selection's
edge that reads just the selected text — same pause/resume toggle, no
highlight spans (the selection itself is the visual). Esc clears the
selection, the float button, and any selection playback. One element on the
page enables this even with no `targets` attribute.

Caveat: toggling fixation mid-playback rewrites the section's innerHTML and
detaches the live highlight spans — audio keeps playing but highlighting stops
until that section is played again.

### Content components (CSS-only; define as no-op elements for semantics)
- `<wk-page-header>` wrapping `<wk-title>` + `<wk-subtitle>` — hero/page header
  (port `.brief-title`/`.brief-summary` + csl/catalog `.hero`).
- `<wk-card>` — content/entity card; `wk-card[href]` (or an `<a class=wk-card>`)
  gets the hover lift. Port present `.row` + catalog `.card`.
- `<wk-panel>` with `<wk-panel-title>` / `<wk-panel-subtitle>` — info panel (port
  present `.panel`).
- `<wk-badge variant="a|b|c|outline|stat|accent">` — chips/badges/kind-badge (port
  present `.chip*` + catalog `.kind-badge`). `a|b|c` are non-semantic tag hues.
- `<wk-badge variant="info|ok|warn|error">` — semantic status badges bound to
  `--blue|--green|--amber|--red`. Use these for event level / finding severity /
  health state, never the `a|b|c` hues (which make a warning look like a success).
- `<wk-badge variant="filter" [active]>` — clickable pill for filter toggles and
  query chips. `[active]` fills the pill with `--chip-active-bg`. Example:
  `<wk-badge variant="filter" active>All</wk-badge>`
- `<wk-button variant="primary|ghost|danger">` — align to the header `.btn`/`.btn-primary`/
  `.btn-ghost`/`.btn-danger` already in webkit.css (share the same rules). `danger` fills
  with the theme `--red` for destructive actions. `[hidden]` hides either one: the kit ships that
  override because the shared `display` rule would otherwise win.
- `<wk-search>` — search bar: styles a child `<input>` (+ optional `<button>`).
  Port csl/catalog `.search-bar`/`.search-input`/`.search-btn`. An optional first-child
  `<svg>` overlays the input's left edge as a leading icon; the input gains extra
  left padding automatically via `:has(> svg)`. Example with icon:
  ```html
  <wk-search>
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
    </svg>
    <input type="search" placeholder="Search…">
    <button>Search</button>
  </wk-search>
  ```
- `<wk-table>` — styles a descendant `<table>` as a data table (port present
  `.data-table`).
- `<wk-seg>` — segmented control. Wraps `<button>` children; `.active` class
  marks the selected segment. Example:
  `<wk-seg><button class="active">Files</button><button>Matches</button></wk-seg>`
- `<wk-kv>` with `<wk-kv-row>`, `<wk-kv-label>`, `<wk-kv-value>` — key/value row
  list. `variant="card"` frames the block with a border, rounded corners, and
  uppercased label style. Example:
  ```html
  <wk-kv variant="card">
    <wk-kv-row><wk-kv-label>Status</wk-kv-label><wk-kv-value>active</wk-kv-value></wk-kv-row>
    <wk-kv-row><wk-kv-label>Port</wk-kv-label><wk-kv-value><code>7423</code></wk-kv-value></wk-kv-row>
  </wk-kv>
  ```
- `<wk-section>` with `<wk-section-heading>`, `<wk-section-id>`, `<wk-section-subheading>` —
  document section with a ruled, primary-underlined heading. `<wk-section-id>` renders
  a monospaced numbered badge. Example:
  ```html
  <wk-section>
    <wk-section-heading><wk-section-id>01</wk-section-id>Overview</wk-section-heading>
    <p>Body paragraph.</p>
    <wk-section-subheading>Details</wk-section-subheading>
    <p>More text.</p>
  </wk-section>
  ```
- `<wk-callout [variant="info|warn|error"]>` — left-bordered callout block. Default border
  color is `--primary`; `variant="info"` uses `--blue`; `variant="warn"` uses
  `--amber`; `variant="error"` uses `--red`. Example:
  `<wk-callout variant="info">Info callout.</wk-callout>`
- `<wk-progress>` with `<wk-progress-bar>`, `<wk-progress-fill>`, `<wk-progress-label>` —
  horizontal progress bar. Consumer sets fill width inline. Example:
  `<wk-progress><wk-progress-bar><wk-progress-fill style="width:63%"></wk-progress-fill></wk-progress-bar><wk-progress-label>63%</wk-progress-label></wk-progress>`
- `<wk-toc>` with `<wk-toc-title>` and a plain `<ul>/<li>/<a>` — table of contents
  block. Links in `wk-toc` are styled in `--primary`; `<wk-section-id>` badges may
  appear inline in anchors. Example:
  ```html
  <wk-toc>
    <wk-toc-title>Contents</wk-toc-title>
    <ul>
      <li><a href="#overview"><wk-section-id>01</wk-section-id> Overview</a></li>
      <li><a href="#usage">02 Usage</a></li>
    </ul>
  </wk-toc>
  ```
- `.wk-prose` — rich-markdown container for rendered Markdown/HTML bodies
  (headings, paragraphs, lists, tables, blockquotes, links, images, inline and
  fenced code). It is a **class**, not a custom element — put it on any block
  wrapping server-rendered HTML. Uses the palette vars for hierarchy and line
  spacing. Fenced code blocks are decorated client-side: on DOM ready,
  `webkit.js` highlights every `.wk-prose pre > code[class*="language-"]` with
  Prism, then injects a `.wk-code-controls` row holding a `.wk-code-lang` badge
  (derived from the `language-*` class) and a `.wk-code-copy` button (clipboard
  write + brief "Copied" feedback). The pass is lazy (skipped entirely when the
  page has neither `.wk-prose` nor `pre.wk-code-block`) and idempotent (safe
  across htmx swaps). Bundled grammars: bash, json, go, python, typescript,
  yaml, sql; other languages render unhighlighted. Example:
  ```html
  <article class="wk-prose">
    <h2>Usage</h2>
    <p>Run the indexer, then query with <code>csl search</code>.</p>
    <pre><code class="language-go">func main() {
      fmt.Println("hello")
  }</code></pre>
  </article>
  ```
- `pre.wk-code-block` — standalone code block for use outside a `.wk-prose`
  body. Same look and the same client-side decoration (Prism highlight +
  language badge + copy button) as the prose `<pre>`; the enhancer matches
  `pre.wk-code-block > code[class*="language-"]`. Both render as a
  terminal-style window: a header bar with palette-toned macOS traffic dots,
  the language badge, and the copy button, over the code body. The block's
  `--code-*` palette follows the page theme — paper-on-cream in light mode,
  deep warm grays (`--wg900`/`--wg800`) in dark mode. Use `language-text` when the
  language is unknown — it still gets the badge and copy button, just no
  highlighting. This is what present's `{"t": "code"}` Doc block renders to.
  ```html
  <pre class="wk-code-block"><code class="language-bash">csl index ./repo</code></pre>
  ```
- `<wk-modal [hidden]>` with `<wk-modal-panel>`, `<wk-modal-head>`, `<wk-modal-body>`,
  `<wk-modal-actions>` — fixed overlay modal. `[hidden]` attribute controls open/closed
  state (removes it to open, adds it to close). Animates in with fade + slide. An `<h2>`
  in the head gets modal title typography. Pair with `<wk-form>` for input dialogs, or
  `<wk-modal-body>` for prose content (confirm dialogs) — both pad to the panel edges,
  and `<wk-modal-actions>` pads itself when used outside a form. Confirm example:
  ```html
  <wk-modal hidden>
    <wk-modal-panel>
      <wk-modal-head><h2>Delete entry?</h2><button class="btn btn-ghost">✕</button></wk-modal-head>
      <wk-modal-body><p><strong>my-entry</strong> will be permanently removed.</p></wk-modal-body>
      <wk-modal-actions>
        <wk-button variant="ghost">Cancel</wk-button>
        <wk-button variant="danger">Delete</wk-button>
      </wk-modal-actions>
    </wk-modal-panel>
  </wk-modal>
  ```
- `<wk-form>` with `<wk-form-row>`, `<wk-form-hint>` — form layout inside a modal or
  panel. `<wk-form-row>` is a two-column grid row. `<wk-form-hint>` renders muted helper
  text. Example:
  ```html
  <wk-form>
    <label>Name<input placeholder="my-service"></label>
    <wk-form-row>
      <label>Kind<input placeholder="system"></label>
      <label>Owner<input placeholder="team"></label>
    </wk-form-row>
    <wk-form-hint>Path is relative to <code>~/code</code>.</wk-form-hint>
  </wk-form>
  ```
- `<wk-toast-host>` + `<wk-toast [variant="ok|err"]>` — fixed bottom-right toast stack.
  Place one `<wk-toast-host>` in the document; append `<wk-toast>` children dynamically
  and remove them to dismiss. `variant="ok"` is green; `variant="err"` is red. Example:
  ```js
  var t = document.createElement('wk-toast');
  t.setAttribute('variant', 'ok');
  t.textContent = 'Saved ✓';
  toastHost.appendChild(t);
  setTimeout(() => t.remove(), 2500);
  ```

Update `<wk-header>` default `fixation-targets` to be component-aware, e.g.
`[data-fixation], wk-panel-title, wk-panel-subtitle, wk-card, .callout, main p, main li, main td`.

## FOUC guard — `Webkit.bootSnippet`
`webkit.js` is deferred, so theme + font-size are only applied after it runs —
persisted dark/large preferences flash on load otherwise. The bundle exports a
single canonical pre-paint snippet so every consumer inlines the SAME guard
instead of hand-rolling it:

```html
<head>
  <!-- inline ONE guard before any stylesheet; mirrors Webkit.bootSnippet -->
  <script>(function(){try{var t=localStorage.getItem('webkit-theme')||'light';document.documentElement.setAttribute('data-theme',t);var s=parseInt(localStorage.getItem('webkit-size'),10);if(!isNaN(s)){s=Math.max(12,Math.min(24,s));document.documentElement.style.fontSize=s+'px';}}catch(e){}})();</script>
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

It reads `webkit-theme` → `data-theme` and `webkit-size` (clamped 12–24) →
`html` font-size before paint. The same string is available at runtime as
`Webkit.bootSnippet` (single source of truth — server templates can inject it).

## Version check + cache busting
`GET /webkit/version` → JSON `{"module":"github.com/mad01/thismoon/webkit","version":"<hash>"}`,
used to confirm which embedded assets a running tool serves. `<hash>` is a short
SHA-256 digest over the embedded `dist/` bytes, computed once at package init. It
changes whenever the compiled assets change. The old pinned pseudo-version
stayed put while the bytes moved and let browsers hold stale CSS/JS until a
force-refresh. The same hash is the asset `ETag`. `Handler()` intercepts
`/webkit/version` before the file server and sets `Cache-Control: no-store`;
assets get `Cache-Control: no-cache` + the content-hash `ETag` and the existing
`If-None-Match` 304 fast path.

`webkit.js` polls `GET /webkit/version` (`cache: 'no-store'`) every ~5s, records
the first value seen, and calls `location.reload()` once a later response
differs, so an open page picks up a redeploy on its own. Errors are ignored and
retried next tick.

**`NoCacheHTML(w http.ResponseWriter)`** — Go helper in the `webkit` package that
sets `Cache-Control: no-cache` on an HTML response. Browsers heuristically cache
documents that carry no cache headers, which can race the version-poll
soft-reload from the back/forward cache. Call it when writing your own HTML so a
webkit asset bump shows up on the next navigation, not after a force-refresh.

## Build / test (unchanged pipeline)
- `npm run build` = `tsc --noEmit` + esbuild → `dist/webkit.js` (IIFE global
  `Webkit`, self-registers the custom elements on load) + copy `dist/webkit.css`.
- Go: `embed_test.go` still asserts Handler serves css/js. ADD assertions that
  `webkit.js` contains `wk-header` (customElements.define) and `webkit.css`
  contains `wk-card`. Keep `--page-width`/`--terracotta` assertions.
- Pure-fn unit tests (node, `npm test` → `node --test`) for `toFixation`/`clampSize`
  live in `test/`; the logic is imported from pure modules (`src/fixation.ts`,
  `src/size.ts`) shared with `webkit.ts` — not duplicated.

## Consumer usage (target)
```html
<wk-header brand="csl·search">
  <a data-nav class="active" href="/">Search</a>
  <a data-nav href="/examples">Examples</a>
</wk-header>
<main class="wrap">
  <wk-page-header><wk-title>Search your local code</wk-title>
    <wk-subtitle>zoekt across every indexed repo</wk-subtitle></wk-page-header>
  <wk-search><input placeholder="…"><button>Search</button></wk-search>
  <wk-card href="/p/x"><wk-title>…</wk-title></wk-card>
</main>
<script src="/webkit/webkit.js"></script>
```
present keeps its bespoke briefing blocks (sections/toc/kv/progress/callout) on its
own CSS for now — those are candidates for a later expansion. present's header
becomes `<wk-header back-href="/" back-label="← All" title="Brief">` and listens
for `wk-themechange` to recolor its graph.
