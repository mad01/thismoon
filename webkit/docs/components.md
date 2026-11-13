# webkit component reference

A field guide for adding or restyling `<wk-*>` elements without rereading the source.

All components use **light DOM** only — no Shadow DOM. CSS vars and fixation text-walk work normally. Most components are pure CSS on the tag name. `<wk-header>` and `<wk-read-aloud>` have JavaScript behaviour, and `<wk-title>` and `<wk-badge>` set a few ARIA attributes on connect (see their sections).

---

## `<wk-header>` — sticky topbar (JS custom element)

The only interactive custom element. Renders the topbar with brand, nav, controls, and optional extra buttons; it sticks to the top above 1024px. Reads and persists all state in `localStorage`.

**Attributes:**

| Attribute | Default | Notes |
|-----------|---------|-------|
| `brand` | — | Wordmark text; a trailing `.` or `·` is wrapped in `.dot` for styling |
| `brand-href` | `/` | Brand link target |
| `back-label` | — | Optional back link label (e.g. `← All`) |
| `back-href` | — | Back link target; required when `back-label` is set |
| `title` | — | Small muted label rendered next to the brand |
| `controls` | `cmdk,font,fixation,size,reload,theme,help` | CSV of controls to render; omit any you don't want |
| `page-width` | `1080` | Sets `--page-width` on `:root` |
| `fixation-targets` | `[data-fixation], wk-panel-title, wk-panel-subtitle, wk-card, .callout, main p, main li, main td` | CSS selector for fixation text-walk |
| `skip-link` | on | `off` leaves out the skip link |

The fixation text-walk works in place: each text node under a target that holds a word becomes a `span.wk-fixation-run` with a `<b>` around the first half of every word. Toggling fixation off unwraps those spans again. Elements, attributes, classes and listeners under a target are left as they are, so content another component injected (read-aloud's buttons and badges, its `data-ra-chunk` stamps and highlights) survives the toggle. The walk skips `code`, `b`, `strong`, `button`, `select`, `textarea`, `wk-badge`, `svg`, `script` and `style` subtrees: chips and control labels aren't prose. A `MutationObserver` re-walks new content while fixation is on, so client-rendered pages need no extra call. Read-aloud plans a live session on plain text: it takes the runs off the section before wrapping sentences. The observer bolds the text inside the whole-sentence spans again before the next paint.

**Skip link.** The header's first child is a "Skip to content" link, so it is the page's first tab stop. It stays visually hidden until it has focus, then shows as a pill in the top-left corner. Activating it moves focus to the first `main`, else the `.wrap` container, else the element after the header; a target that cannot take focus gets `tabindex="-1"` (and no focus ring). Set `skip-link="off"` on a page that has no content to skip to.

**Wrapping.** At any width, when the left group and the controls do not fit one row, the controls drop to a second row, still on the right, and a control set wider than the bar wraps inside itself, so the header never widens the page. A header that fits stays one row, laid out as before.

**Narrow screens.** Under 720px wide the bar is always two rows: the back link, brand, title and nav, then the controls, starting under the brand. The Fixation, Audio and Reload labels hide behind their icons and the ⌘K hint goes; each control keeps its `aria-label`. Text in the first row never wraps mid-label, a long `title` ends in an ellipsis, and a row that still does not fit wraps onto another row. A `data-extra` button keeps its text. Up to 1024px wide the bar scrolls away with the page instead of sticking, so a wrapped bar of two or three rows never holds part of the screen.

**Quiet controls.** The header's own controls rest without a fill or a visible border (the 1px border stays, transparent, so nothing moves) and take the chip fill on hover. An active toggle and a pressed size button keep the primary fill, and the selects keep their arrow, drawn in `--text-3`. A select is as wide as the option it shows, so its arrow sits by the text, and the size stepper's two buttons sit together with a 1rem glyph. The dividers between control groups are hidden. Above 720px a `.btn` passed as `data-extra` sits 0.5rem from the controls, and with no visible `.btn` the row pulls out by the last control's padding, so its icon ends on the content's right edge. The quiet look is scoped to `.topbar-controls`, so `.size-btn` and `.btn` elsewhere on a page keep their frame.

**Light-DOM children relocated by the element:**

- `<a data-nav [class="active"] href="…">…</a>` — rendered in the nav area
- `<button data-extra id="…">…</button>` or `<wk-button data-extra id="…">` — rendered in the controls area; the app wires click handlers by `id`

**Controls (CSV values for the `controls` attribute):**

| Value | What it renders |
|-------|----------------|
| `cmdk` | ⌘K / Ctrl+K site-picker button |
| `font` | Font family selector (Fira Code, Inter, Lexend, Work Sans) |
| `fixation` | Fixation reading toggle |
| `size` | Font size −/+ buttons (12–24 px, 2 px steps) |
| `audio` | Read-aloud controls toggle: shows or hides every control `<wk-read-aloud>` injects by setting `data-audio` on `<html>`; remembered per view (see below) |
| `speed` | Speech speed selector (0.75×, 1×, 1.25×, 1.5×, 2×); dims while `audio` is off |
| `reload` | Page reload button |
| `theme` | Light/dark toggle |
| `help` | `?` button — opens the feature-guide modal (X / Esc / click-outside to close) |

**Events dispatched on `document`:**

- `wk-themechange` — `CustomEvent<{ theme: 'light' | 'dark' }>` — fires when the theme toggle is clicked. Listen on `document` to recolor dynamic visuals (Cytoscape graphs, charts).
- `wk-speedchange` — `CustomEvent<{ speed: number }>` — fires when the speech speed selector changes. `<wk-read-aloud>` listens for this automatically; subsequent TTS clips use the new speed.
- `wk-audiochange` — `CustomEvent<{ audio: 'on' | 'off' }>` — fires when the audio toggle changes, or another tab changes the same key. `<wk-read-aloud>` ends a playing session on `off`.

**Audio toggle scope.** The `audio` control keeps its choice under the storage key a shell names on `<html>` with `data-audio-key` (default `webkit-audio`). When nothing is stored, `data-audio-default` applies (`on` unless `off`). `boot.js` resolves the same two before paint and sets `data-audio`, so the controls never flash. The attributes sit on `<html>` rather than on the header because the boot script runs before the header exists. present's deck shell sets `webkit-audio-deck` and `off`, and its brief uses the defaults. So a deck opens with the controls hidden while a brief keeps its own choice.
- `wk-cmdk:open` — dispatched by the ⌘K button to ask the global site-picker controller to open. Dispatch this event yourself to open the picker programmatically.

**`Webkit.init(config)` shim:** the legacy `Webkit.init()` call is still supported and builds a `<wk-header>` from a config object. Prefer the markup form for new pages.

**Example:**

```html
<wk-header brand="present." brand-href="/">
  <a data-nav class="active" href="/">Pages</a>
  <a data-nav href="/webkit/themes">Themes</a>
  <button data-extra id="shareBtn">Share</button>
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
| `prepare` | off | Boolean. Registers the page's text with speak on connect, so parts are synthesized ahead of playback and the page shows how far that got |
| `name` | page title | The document name speak files the page under |

On load it probes `GET {endpoint}/` with a 1.5s timeout. If unreachable it injects nothing — pages degrade gracefully on hosts without speak.

Speech is fetched in parts that start at one sentence and grow to a few hundred characters, two ahead of the one playing. On a page whose sections list their part keys in `data-ra-parts` (prepared mode stamps them), the parts are prepared audio from `GET {endpoint}/audio/{key}`. The playing part is highlighted with `.wk-ra-active` (background `--ra-highlight`), on its sentence spans (`.wk-ra-sentence`) or on the `data-ra-chunk` blocks it reads, and the play button pulses (`.wk-ra-wait`) while that part is still loading. Starting a second section stops the first. While a section plays, a restart button appears beside its play/pause control to rewind to the section start. Esc clears selection playback.

With `prepare`, the element registers the page with speak itself (`POST {endpoint}/read`, one list of block texts per target section). It stamps the keys speak answers onto the blocks (`data-ra-chunk`) and sections (`data-ra-parts`). It also renders a status bar inside itself: how many parts are ready, with "Generate all TTS for page" and "Retry failed" buttons. Each section gets a state badge (`wk-badge.wk-ra-state`) beside its play buttons, with "Retry" while it has failed parts. The status is polled every 2s while parts are in progress, backing off to once a minute while speak doesn't answer. If registration fails the page reads live as before, with no toast. If speak later forgets the document (it restarted), the page registers again. Should that fail too, the stamps come off, one toast says so, and the page reads live from then on.

---

## Layout

### `<wk-page-header>` / `<wk-title>` / `<wk-subtitle>`

Hero block at the top of a page. A `<wk-title>` outside a `wk-card`, `.wk-card` or `wk-panel` is the page heading: on connect it gets `role="heading"` and `aria-level="1"`, unless it already has a role (an `aria-level` it carries is kept).

The header's bottom margin (`2rem`) is the only space above the page's first line: `wk-page-header` has no top margin and `.wrap` no top padding, so every page's title sits at the same height. A shell adds no top padding of its own.

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

### Reading measure (`--wk-measure`)

A token on `:root` beside `--page-width`, `76ch` by default: the widest a line of prose runs. It caps paragraphs, lists and quotes that sit straight in a `wk-section` or a `.wk-prose` body, and a `wk-callout` and a `details` straight in a section, whose boxes end where a section paragraph does (the measure is in `ch`, which each reads in its own font, so their caps scale by the paragraph's font over theirs; a callout's text is usually a bare text node, so the cap sits on its box). Tables, figures, charts, code blocks, stats and columns keep the full width. A shell's own prose can read the token too (`max-width: var(--wk-measure)`). A page or a subtree that wants full-width prose opts out:

```css
.deck { --wk-measure: none; }
```

---

## Cards and panels

### `<wk-card>`

Content card with an optional `--accent` left bar. Every card's border darkens on hover. Only a card you can act on lifts: `wk-card[href]`, `<a class="wk-card">`, and a `<wk-card interactive>` the page made clickable with its own handler. The `interactive` attribute adds the lift and the pointer cursor and nothing else; the page wires the click and any `tabindex`.

```html
<wk-card>
  <wk-title>present</wk-title>
  <p>Serves HTML briefings over localhost.</p>
</wk-card>

<!-- Clickable card -->
<a class="wk-card" href="/p/my-brief">
  <wk-title>My Brief</wk-title>
</a>

<!-- Clickable without a link: the page handles the click -->
<wk-card interactive tabindex="0" data-id="evt-42">
  <wk-title>deploy finished</wk-title>
</wk-card>
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
| `accent` | Uses `var(--accent, --primary)`, mixed by the status rule: the ink moves `--accent-ink-share` (45%, 55% or 65%, per variant) toward `--text-1`, so its text reads 4.5:1 on its tint where the variant allows |
| `info` / `ok` / `warn` / `error` | Status badge: `--status-<s>-bg` fill, `--status-<s>-text` ink (blue, green, amber, red) |
| `filter` | Clickable pill; `[active]` fills with `--chip-active-bg` |

```html
<wk-badge variant="a">go</wk-badge>
<wk-badge variant="filter" active>All</wk-badge>
<wk-badge variant="filter">Systems</wk-badge>
```

A filter badge is keyboard-operable without page code. When it has `variant="filter"`, neither a `tabindex` nor a `role` attribute, and no link or button around it, webkit gives it `role="button"` and `tabindex="0"`, and Enter or Space dispatches a `click` on it, so the page's click handler serves the keyboard too. A variant set after the badge is in the page counts as well, and a badge that stops being a filter loses the two attributes again. A badge that sets its own `tabindex` or `role` is left to the page, which then wires its own keys (csl and moondb do).

A count inside a filter badge goes in a `<small>`: smaller, tabular figures, in the muted ink.

```html
<wk-badge variant="filter">go <small>12</small></wk-badge>
```

### `<wk-chips>`

A row of filter badges above the list they filter: wraps, `0.5rem` apart, `1.25rem` below. CSS only.

```html
<wk-chips>
  <wk-badge variant="filter" active>All <small>128</small></wk-badge>
  <wk-badge variant="filter">Tools <small>12</small></wk-badge>
  <wk-badge variant="filter">Services <small>9</small></wk-badge>
</wk-chips>
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

A `<wk-button>` or `.btn` with no variant, or one the kit does not know, gets the ghost look, so it still reads as a button. The fallback sits at zero specificity (`:where()`), so the named variants and any page rule win over it.

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
    <wk-kv-value><code>7433</code></wk-kv-value>
  </wk-kv-row>
</wk-kv>
```

### `<wk-section>` / `<wk-section-heading>` / `<wk-section-subheading>`

Document section with a ruled, primary-underlined heading.

```html
<wk-section>
  <wk-section-heading>
    Overview
  </wk-section-heading>
  <p>Body paragraph.</p>
  <wk-section-subheading>Details</wk-section-subheading>
  <p>More text.</p>
</wk-section>
```

### `<wk-callout>`

Callout block framed in `--border`, with a 3px bar in the variant's colour on the left. A status variant also takes that status's tinted fill; the text is `--text-1`.

| `variant` | Bar | Fill |
|-----------|-----|------|
| (none) | `--primary` | `--card-bg` |
| `info` | `--blue` | `--status-info-bg` |
| `warn` | `--amber` | `--status-warn-bg` |
| `ok` | `--green` | `--status-ok-bg` |
| `error` | `--red` | `--status-error-bg` |

```html
<wk-callout variant="info">Remember to validate before committing.</wk-callout>
<wk-callout variant="warn">Destructive — cannot be undone.</wk-callout>
```

A callout's box ends where a paragraph beside it in a section does. With `--wk-measure: none` it runs full width.

### `<wk-empty>` / `<wk-empty-title>`

What a list or a page shows in place of its content while it is empty, loading, or failed to load: a muted (`--text-2`), left-aligned block with one padding. An optional `<wk-empty-title>` leads with a short headline over the detail. `variant="error"`, for a fetch that failed, takes the callout's frame: a `--border` box with a 3px `--red` bar on the left, on `--status-error-bg` with its text in `--status-error-text`, as wide as the block it sits in. Put the raw error under the title.

```html
<wk-empty>No events match these filters.</wk-empty>
<wk-empty variant="error">
  <wk-empty-title>Could not load sessions</wk-empty-title>
  GET /api/sessions: read sessions.jsonl: permission denied
</wk-empty>
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
    <li><a href="#overview">01 Overview</a></li>
    <li><a href="#usage">02 Usage</a></li>
  </ul>
</wk-toc>
```

### `.wk-prose`

A CSS class (not a custom element) for rich-markdown containers. Put it on any block wrapping server-rendered HTML — headings, paragraphs, lists, tables, blockquotes, links, inline and fenced code. Its direct paragraphs, lists and blockquotes stop at the reading measure (`--wk-measure`); tables, code blocks and images keep the full width.

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
<pre class="wk-code-block"><code class="language-bash">moondb why ./main.go:12</code></pre>
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
| `ok` | Green (`--green` / `--on-green`) |
| `warn` | Amber (`--amber` / `--on-amber`) |
| `err` | Red (`--red` / `--on-red`) |

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

| Role | Light | Dark |
|------|-------|------|
| `--bg` | `#FAF9F7` | `#1A1916` |
| `--paper` | `#FFFFFF` | `#252320` |
| `--chip` | `#EEECE8` | `#35322C` |
| `--border` | `#D8D4CD` | `#35322C` |
| `--border-hover` | `#B8B2A7` | `#4A453D` |
| `--text-1` / `--text-2` / `--text-3` | `#252320` / `#6B6459` / `#8C8578` | `#D8D4CD` / `#B8B2A7` / `#8C8578` |
| `--primary` / `--on-primary` | `#C4704B` / `#FFFFFF` | `#E8956A` / `#FFFFFF` |
| `--red` `--green` `--amber` `--yellow` `--blue` `--purple` | the semantic set | the same values (decision 7), except `--purple` `#B294D8`, brightened for contrast on chips |
| `--series-1` to `--series-4` | `#C4704B` `#5B8EC4` `#4A9E6B` `#8B6BB0` | `#E8956A` `#7AAAE8` `#6BC48A` `#8B6BB0` |

Those are the default family's values. Every other family declares the same
roles under its own `[data-palette="<name>"][data-theme="<mode>"]` block, so a
rule that reads a role follows the reader's choice.

**Derived roles (generated, literal, available for bespoke CSS):**

- surfaces and fills: `--card-bg`, `--chip-bg`, `--chip-active-bg`,
  `--chip-active-text`, `--progress-bg`, `--progress-fill`, `--primary-soft`
- inks: `--text-body`, and `--on-<colour>` for each semantic colour (white or
  the variant's deepest ink, whichever reads with more contrast on that fill)
- tags: `--tag-a`, `--tag-b`, `--tag-c` and their `-text` partners
- status: `--status-<s>-bg` and `--status-<s>-text` for `info` (blue), `ok`
  (green), `warn` (amber) and `error` (red): paper moved 14% toward the colour,
  and the colour moved toward `--text-1` by the first of 45%, 55% and 65% at
  which all four inks read 4.5:1 on their fills, so each variant keeps as much
  hue as the bar allows (the default family takes 45%). Solarized light and dark
  and Tokyo Night Day, whose own body text barely clears 4.5 on paper, reach it
  at no share and take 65%; `themes_contrast_test.go` names them with their
  measured floors. `--accent-ink-share` is the same pick for the accent badge on
  `--primary`, emitted as a percentage because that badge mixes at run time.
- overlays: `--ra-highlight`, `--focus-ring`, `--topbar-bg`, `--scrim`
- code: `--code-bg`, `--code-header-bg`, `--code-border`, `--code-keyword`,
  `--code-string`, `--code-number`, `--code-symbol`
- canvas: the `--graph-*` box and edge colours, the
  `--tone-<name>-bg` / `-border` / `-text` triples, `--chart-grid`,
  `--chart-text`, `--chart-label`

The old ramp primitives (`--cream`, `--off-white`, `--wg100` to `--wg900`,
`--terracotta-light`) are gone, and so are the aliases. `--green-light` went
when csl, its last reader, moved its query tokens to the semantic colour
roles. `--terracotta` went when present started rendering the panel accent
stored under that name as `var(--primary)`; present's own shell declares the
old name for pages a shared instance never rerenders.

**`localStorage` state keys (global, shared across a tool's pages):**

| Key | Values |
|-----|--------|
| `webkit-theme` | `light` \| `dark` \| `system` (follows `prefers-color-scheme`) |
| `webkit-palette-light` | a family name from `themes.json`; absent = `default` |
| `webkit-palette-dark` | a family name from `themes.json`; absent = `default` |
| `webkit-font` | `Fira Code` \| `Inter` \| `Lexend` \| `Work Sans` |
| `webkit-size` | integer 12–24 |
| `webkit-fixation` | `true` \| `false` |
| `webkit-audio`, or the key a shell names in `data-audio-key` on `<html>` | `on` \| `off` (per view; absent = `data-audio-default`, `on` unless `off`) |

**Focus, links and native controls.** Everything the keyboard reaches (links, buttons, selects, summaries, anything with a `tabindex` other than `-1`, `wk-button`, filter badges) shows one focus ring: a 2px `--primary` outline, 2px out. Text inputs show a `--primary` border with a `--focus-ring` halo instead. A plain link (an `<a>` with no class) inside `wk-section`, `wk-callout`, `wk-modal-body`, `wk-panel`, `wk-card`, `wk-kv-value`, `wk-figcaption` or the feature guide takes the `--primary` ink over a soft underline. Both rules have zero specificity, so a page's own rule wins. `color-scheme` follows the mode and `accent-color` is `--primary`, so scrollbars, select menus and checkboxes match the theme.

**Theme-change event:** `wk-themechange` fires on `document` with `detail.theme` (the resolved mode), `detail.palette` (the family for it), and `detail.mode` (what the reader chose, `system` included). Listen there to recolor dynamic visuals. It fires on the header toggle, on a change made in another tab of the origin (the themes page at `/webkit/themes`), and on a system appearance change in system mode.

```js
document.addEventListener('wk-themechange', e => {
  buildGraph(e.detail.theme); // recolor Cytoscape, charts, etc.
});
```
