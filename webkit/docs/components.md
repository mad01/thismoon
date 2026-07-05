# webkit component reference

A field guide for adding or restyling `<wk-*>` elements without rereading the source.

All components use **light DOM** only — no Shadow DOM. CSS vars and bionic text-walk work normally. Most components are pure CSS on the tag name; only `<wk-header>` and `<wk-read-aloud>` have JavaScript behaviour.

---

## `<wk-header>` — sticky topbar (JS custom element)

The only interactive custom element. Renders the sticky topbar with brand, nav, controls, and optional extra buttons. Reads and persists all state in `localStorage`.

**Attributes:**

| Attribute | Default | Notes |
|-----------|---------|-------|
| `brand` | — | Wordmark text; a trailing `.` or `·` is wrapped in `.dot` for styling |
| `brand-href` | `/` | Brand link target |
| `back-label` | — | Optional back link label (e.g. `← All`) |
| `back-href` | — | Back link target; required when `back-label` is set |
| `title` | — | Small muted label rendered next to the brand |
| `controls` | `cmdk,font,bionic,size,speed,reload,theme` | CSV of controls to render; omit any you don't want |
| `page-width` | `1080` | Sets `--page-width` on `:root` |
| `bionic-targets` | `[data-bionic], wk-panel-title, wk-panel-subtitle, wk-card, .callout, main p, main li, main td` | CSS selector for bionic text-walk |

**Light-DOM children relocated by the element:**

- `<a data-nav [class="active"] href="…">…</a>` — rendered in the nav area
- `<button data-extra id="…">…</button>` or `<wk-button data-extra id="…">` — rendered in the controls area; the app wires click handlers by `id`

**Controls (CSV values for the `controls` attribute):**

| Value | What it renders |
|-------|----------------|
| `cmdk` | ⌘K / Ctrl+K site-picker button |
| `font` | Font family selector (Fira Code, Inter, Lexend, Work Sans) |
| `bionic` | Bionic reading toggle |
| `size` | Font size −/+ buttons (12–24 px, 2 px steps) |
| `speed` | Speech speed selector (0.75×, 1×, 1.25×, 1.5×, 2×) |
| `reload` | Page reload button |
| `theme` | Light/dark toggle |

**Events dispatched on `document`:**

- `wk-themechange` — `CustomEvent<{ theme: 'light' | 'dark' }>` — fires when the theme toggle is clicked. Listen on `document` to recolor dynamic visuals (Cytoscape graphs, charts).
- `wk-speedchange` — `CustomEvent<{ speed: number }>` — fires when the speech speed selector changes. `<wk-read-aloud>` listens for this automatically; subsequent TTS clips use the new speed.
- `wk-cmdk:open` — dispatched by the ⌘K button to ask the global site-picker controller to open. Dispatch this event yourself to open the picker programmatically.

**`Webkit.init(config)` shim:** the legacy `Webkit.init()` call is still supported and builds a `<wk-header>` from a config object. Prefer the markup form for new pages.

**Example:**

```html
<wk-header brand="catalog." brand-href="/">
  <a data-nav class="active" href="/">Systems</a>
  <a data-nav href="/components">Components</a>
  <button data-extra id="refreshBtn">Refresh</button>
</wk-header>
```

---

## `<wk-read-aloud>` — per-section text-to-speech (JS custom element)

Place one element per page. It injects a floating play/pause button into each element matching `targets`, and a selection speaker for any selected text on the page.

**Attributes:**

| Attribute | Default | Notes |
|-----------|---------|-------|
| `targets` | — | CSS selector for readable sections |
| `endpoint` | `http://speak.this` | Base URL of an OpenAI-compatible speech service |
| `voice` | `af_heart` | Kokoro voice id |
| `speed` | `1` | Playback speed multiplier |

On load it probes `GET {endpoint}/` with a 1.5s timeout. If unreachable it injects nothing — pages degrade gracefully on hosts without speak.

The active sentence is highlighted with `.wk-ra-sentence.wk-ra-active` (background `--ra-highlight`). Starting a second section stops the first. While a section plays, a restart button appears beside its play/pause control to rewind to the section start. Esc clears selection playback.

---

## Layout

### `<wk-page-header>` / `<wk-title>` / `<wk-subtitle>`

Hero block at the top of a page.

```html
<wk-page-header>
  <wk-title>Search your local code</wk-title>
  <wk-subtitle>zoekt across every indexed repo</wk-subtitle>
</wk-page-header>
```

### `.wrap`

Layout container class — not a custom element. Centers content at `max-width: var(--page-width, 1080px)` with horizontal padding. Put it on `<main>` or another block wrapper.

```html
<main class="wrap">…</main>
```

---

## Cards and panels

### `<wk-card>`

Content card with an optional `--accent` left bar. `wk-card[href]` and `<a class="wk-card">` get a hover lift.

```html
<wk-card>
  <wk-title>present</wk-title>
  <p>Serves HTML briefings over localhost.</p>
</wk-card>

<!-- Clickable card -->
<a class="wk-card" href="/p/my-brief">
  <wk-title>My Brief</wk-title>
</a>
```

Set a colored left bar with the `--accent` CSS variable:

```html
<wk-card style="--accent: var(--blue)">…</wk-card>
```

### `<wk-panel>` / `<wk-panel-title>` / `<wk-panel-subtitle>`

Info panel with a bordered box.

```html
<wk-panel>
  <wk-panel-title>Dependencies</wk-panel-title>
  <wk-panel-subtitle>3 components</wk-panel-subtitle>
  <p>…</p>
</wk-panel>
```

---

## Badges

### `<wk-badge>`

Chips and kind labels. Variant controls color and behavior.

| `variant` | Appearance |
|-----------|-----------|
| `a` | Warm amber tag (`--tag-a` / `--tag-a-text`) |
| `b` | Green tag (`--tag-b` / `--tag-b-text`) |
| `c` | Blue tag (`--tag-c` / `--tag-c-text`) |
| `outline` | Border-only, no fill |
| `stat` | Muted stat chip |
| `accent` | Uses `var(--accent, --primary)` |
| `filter` | Clickable pill; `[active]` fills with `--chip-active-bg` |

```html
<wk-badge variant="a">go</wk-badge>
<wk-badge variant="filter" active>All</wk-badge>
<wk-badge variant="filter">Systems</wk-badge>
```

---

## Interactive controls

### `<wk-button>`

Action button.

| `variant` | Appearance |
|-----------|-----------|
| `primary` | Filled, `--primary` background |
| `ghost` | Outlined, transparent background |
| `danger` | Filled with `--red` for destructive actions |

```html
<wk-button variant="primary">Save</wk-button>
<wk-button variant="ghost">Cancel</wk-button>
<wk-button variant="danger">Delete</wk-button>
```

### `<wk-search>`

Search bar. Styles a child `<input>`. Optional first-child `<svg>` becomes a leading icon with extra left padding added automatically via `:has(> svg)`.

```html
<wk-search>
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
    <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
  </svg>
  <input type="search" placeholder="Search…">
</wk-search>
```

### `<wk-seg>`

Segmented control. Wrap `<button>` children; `class="active"` marks the selected segment.

```html
<wk-seg>
  <button class="active">Systems</button>
  <button>Components</button>
</wk-seg>
```

---

## Content blocks

### `<wk-table>`

Wraps a descendant `<table>` as a styled data table.

```html
<wk-table>
  <table>
    <thead><tr><th>Name</th><th>Kind</th></tr></thead>
    <tbody><tr><td>present</td><td>Component</td></tr></tbody>
  </table>
</wk-table>
```

### `<wk-kv>` / `<wk-kv-row>` / `<wk-kv-label>` / `<wk-kv-value>`

Key/value list. `variant="card"` adds a border, rounded corners, and uppercased label style.

```html
<wk-kv variant="card">
  <wk-kv-row>
    <wk-kv-label>Status</wk-kv-label>
    <wk-kv-value>production</wk-kv-value>
  </wk-kv-row>
  <wk-kv-row>
    <wk-kv-label>Port</wk-kv-label>
    <wk-kv-value><code>7575</code></wk-kv-value>
  </wk-kv-row>
</wk-kv>
```

### `<wk-section>` / `<wk-section-heading>` / `<wk-section-id>` / `<wk-section-subheading>`

Document section with a ruled, primary-underlined heading. `<wk-section-id>` renders a monospaced numbered badge.

```html
<wk-section>
  <wk-section-heading>
    <wk-section-id>01</wk-section-id>Overview
  </wk-section-heading>
  <p>Body paragraph.</p>
  <wk-section-subheading>Details</wk-section-subheading>
  <p>More text.</p>
</wk-section>
```

### `<wk-callout>`

Left-bordered callout block.

| `variant` | Border color |
|-----------|-------------|
| (none) | `--primary` |
| `info` | `--blue` |
| `warn` | `--amber` |

```html
<wk-callout variant="info">Remember to validate before committing.</wk-callout>
<wk-callout variant="warn">Destructive — cannot be undone.</wk-callout>
```

### `<wk-progress>` / `<wk-progress-bar>` / `<wk-progress-fill>` / `<wk-progress-label>`

Horizontal progress bar. Set fill width inline on `<wk-progress-fill>`.

```html
<wk-progress>
  <wk-progress-bar>
    <wk-progress-fill style="width:63%"></wk-progress-fill>
  </wk-progress-bar>
  <wk-progress-label>63%</wk-progress-label>
</wk-progress>
```

### `<wk-toc>` / `<wk-toc-title>`

Table of contents block. Links inside are styled in `--primary`.

```html
<wk-toc>
  <wk-toc-title>Contents</wk-toc-title>
  <ul>
    <li><a href="#overview"><wk-section-id>01</wk-section-id> Overview</a></li>
    <li><a href="#usage">02 Usage</a></li>
  </ul>
</wk-toc>
```

### `.wk-prose`

A CSS class (not a custom element) for rich-markdown containers. Put it on any block wrapping server-rendered HTML — headings, paragraphs, lists, tables, blockquotes, links, inline and fenced code.

Client-side, `webkit.js` highlights every `.wk-prose pre > code[class*="language-"]` with Prism and injects a `.wk-code-controls` row holding a language badge and a copy button. Bundled Prism grammars: bash, json, go, python, typescript, yaml, sql. Other languages render unhighlighted.

```html
<article class="wk-prose">
  <h2>Usage</h2>
  <p>Run the indexer, then query.</p>
  <pre><code class="language-go">func main() {
  fmt.Println("hello")
}</code></pre>
</article>
```

### `pre.wk-code-block`

Standalone code block for use outside a `.wk-prose` body. Gets the same Prism highlight, language badge, and copy button. Use `language-text` when the language is unknown.

```html
<pre class="wk-code-block"><code class="language-bash">catalog validate ./path</code></pre>
```

---

## Overlays

### `<wk-modal>` / `<wk-modal-panel>` / `<wk-modal-head>` / `<wk-modal-body>` / `<wk-modal-actions>`

Fixed overlay modal. Toggle the `[hidden]` attribute to open and close. Animates in with fade and slide.

```html
<wk-modal hidden id="confirmModal">
  <wk-modal-panel>
    <wk-modal-head>
      <h2>Delete entry?</h2>
      <button class="btn btn-ghost" id="closeModal">✕</button>
    </wk-modal-head>
    <wk-modal-body>
      <p><strong>my-entry</strong> will be permanently removed.</p>
    </wk-modal-body>
    <wk-modal-actions>
      <wk-button variant="ghost" id="cancelBtn">Cancel</wk-button>
      <wk-button variant="danger" id="deleteBtn">Delete</wk-button>
    </wk-modal-actions>
  </wk-modal-panel>
</wk-modal>

<script>
  document.getElementById('openBtn').addEventListener('click', () => {
    document.getElementById('confirmModal').removeAttribute('hidden');
  });
  document.getElementById('closeModal').addEventListener('click', () => {
    document.getElementById('confirmModal').setAttribute('hidden', '');
  });
</script>
```

### `<wk-form>` / `<wk-form-row>` / `<wk-form-hint>`

Form layout, typically inside a modal. `<wk-form-row>` is a two-column grid row. `<wk-form-hint>` renders muted helper text.

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

### `<wk-toast-host>` / `<wk-toast>`

Fixed bottom-right toast stack. Place one `<wk-toast-host>` in the document body; append `<wk-toast>` children dynamically.

| `variant` | Color |
|-----------|-------|
| `ok` | Green |
| `err` | Red |

```js
const host = document.querySelector('wk-toast-host');
const t = document.createElement('wk-toast');
t.setAttribute('variant', 'ok');
t.textContent = 'Saved';
host.appendChild(t);
setTimeout(() => t.remove(), 2500);
```

---

## ⌘K site picker

A global overlay built by `webkit.js` with no markup required. Opens when the user presses ⌘K (macOS) or Ctrl+K (other platforms), or when the `<wk-header>` ⌘K button is clicked, or when `wk-cmdk:open` is dispatched on `document`.

The site list comes from d-man at `/__this/sites.json` (same-origin). A new route added to d-man's `routes.toml` shows up in the picker on every site with no rebuild. Fuzzy-matches by name; arrow keys / Ctrl+N / Ctrl+P move the selection; Enter jumps to the site; Esc closes.

Open programmatically:

```js
document.dispatchEvent(new CustomEvent('wk-cmdk:open'));
// or, if you import the module directly:
Webkit.openCmdK();
```

---

## Theming

webkit.css defines the palette as CSS custom properties on `:root` and overrides them under `[data-theme="dark"]`. The FOUC (Flash of Unstyled Content) guard in `<head>` sets `data-theme` before paint so persisted preferences don't flash.

**Semantic tokens (use these in consumer CSS):**

| Token | Light | Dark |
|-------|-------|------|
| `--primary` | `--terracotta` (`#C4704B`) | `#E8956A` |
| `--bg` | `--cream` (`#FAF9F7`) | `--wg900` (`#1A1916`) |
| `--paper` | `#FFFFFF` | `--wg800` (`#252320`) |
| `--text-1` | `--wg800` | `--wg200` |
| `--text-2` | `--wg500` | `--wg300` |
| `--text-3` | `--wg400` | `--wg400` |
| `--border` | `--wg200` | `--wg700` |
| `--chip-active-bg` | `--terracotta` | `#E8956A` |
| `--progress-fill` | `--terracotta` | `#E8956A` |

**Palette primitives (available for bespoke CSS):**

`--cream`, `--off-white`, `--wg100`…`--wg900` (warm gray scale), `--terracotta`, `--amber`, `--green`, `--yellow`, `--red`, `--blue`, `--purple`

**`localStorage` state keys (global, shared across a tool's pages):**

| Key | Values |
|-----|--------|
| `webkit-theme` | `light` \| `dark` |
| `webkit-font` | `Fira Code` \| `Inter` \| `Lexend` \| `Work Sans` |
| `webkit-size` | integer 12–24 |
| `webkit-bionic` | `true` \| `false` |

**Theme-change event:** `wk-themechange` fires on `document` with `detail.theme` set to the new value. Listen there to recolor dynamic visuals.

```js
document.addEventListener('wk-themechange', e => {
  buildGraph(e.detail.theme); // recolor Cytoscape, charts, etc.
});
```
