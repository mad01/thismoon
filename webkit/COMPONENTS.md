# webkit components — design spec

Goal: a small, reusable UI kit shared by `present`, `csl` and the other `*.this`
services. React-like **composition without React** via **web components (custom
elements), light DOM**, styled by the shared `webkit.css`. Consumers write
declarative markup (`<wk-card>…`) in their server-rendered HTML; `webkit.js`
upgrades the interactive ones. CSS-vars (palette) and the fixation text-walk keep
working because everything is light DOM (no Shadow DOM).

## Principles
- **Light DOM only.** No Shadow DOM. Components render into themselves / are styled
  by tag + attribute selectors in `webkit.css`.
- **CSS-first.** Most components are pure CSS on the tag name (`wk-card { … }`,
  `wk-badge[variant="a"] { … }`) with `display:block` defaults — no JS needed.
- **JS only for behavior.** Only `<wk-header>` needs a custom-element class
  (controls + persistence + event). Others may be defined as no-op elements for
  semantics but require no JS.
- **Palette is generated.** The colour roles (`--bg`, `--paper`, `--primary`,
  `--on-primary`, the semantic colours, tags, code tokens, graph and chart
  roles) come from the theme collection under `src/themes/`, one block per
  family and mode at the top of `webkit.css`. Component rules read roles only;
  a literal or a ramp name in a rule would stop following the family. The
  component visuals (`.panel`, `.data-table`, `.chip*`, `.callout`, etc.) were
  ported verbatim into `webkit.css` from present's original page template,
  csl's `internal/web/assets/static/app.css` and the retired catalog service's
  stylesheet (`.card`, `.search-bar`, `.kind-badge`).
- **One look per job.** One focus ring (a 2px `--primary` outline, 2px out) on
  everything the keyboard reaches, at zero specificity (a `tabindex="-1"`
  container, focused only from script, gets none); text inputs keep a
  `--primary` border plus a `--focus-ring` halo instead. Filled buttons darken by
  `brightness(0.95)` on hover. Surfaces (card, panel, toc, stat, details, callout,
  kv card, code block) rest on a 1px `--border` at `--wk-radius-surface` (10px);
  all but the callout carry `--shadow-xs`. A plain link (no class) in a content block takes
  the `--primary` ink over a soft underline. The rule has zero specificity, so a
  page's own link rule wins.
  `color-scheme` follows the mode and `accent-color` is `--primary`, so native
  controls match.
- **Tokens for shape, rhythm and motion.** `:root` declares the radius roles
  `--wk-radius-mark|control|field|surface|overlay|pill` (4, 6, 8, 10, 14 and
  999px), the spacing scale `--wk-space-N` (N × `0.25rem`, N in `0-5`, `1`,
  `1-5`, `2`, `2-5`, `3`, `4`, `5`, `6`, `8`, `10`), the control height
  `--wk-control-h` (`1.75rem`: buttons, header controls, `wk-seg`) and the
  durations `--wk-dur-fast|--wk-dur|--wk-dur-theme` (0.15s, 0.2s, 0.3s). The
  rules read them, and service CSS should read them too instead of copying
  near-values. A literal off the scale is deliberate: other code depends on it
  (the header bar, `.wrap`, the `wk-search` input, `wk-badge` padding, the
  `wk-stat` and `wk-section-heading` box), or snapping it would move content by
  more than 2px.

## Components

### `<wk-header>`  (JS custom element)
Replaces `Webkit.init()`. Renders the `.topbar` with `.topbar-inner`
(max-width `--page-width`, centered), sticky above 1024px. The bar wraps at any
width: when the left group and the controls do not fit one row the controls
drop to a second row on the right, and a control set wider than the bar wraps
inside itself, so the header never widens the page. Under 720px the controls
take their own row under the brand, the Fixation, Audio and Reload labels and
the ⌘K hint hide. Up to 1024px the bar scrolls away with the page. Attributes:
- `brand`, `brand-href` (default `/`) — wordmark (wrap trailing `.`/`·` in `.dot`).
- `back-label`, `back-href` — optional back link (present's "← All").
- `title` — small muted label (present's "Brief").
- `controls` — CSV, default `cmdk,font,fixation,size,reload,theme,help`. Leaving
  `cmdk` out also turns the ⌘K shortcut off on that page: the picker lists local
  `.this` sites, which a network-facing page has none of.
- `page-width` — number, sets `--page-width` (default 1080).
- `fixation-targets` — selector override.
- `themes-link`: `off` drops the feature guide's Themes row, which links
  `/webkit/themes`, for a page with no server behind it (present's exported file).
- `help` — a `?` control that opens a feature-guide modal (dismiss on X, Esc, or
  a click outside). It documents the controls this header renders and adds a
  read-aloud/speed section when `speed` is present. It also appends the
  innerHTML of any `<template data-wk-help>` in the page so a consumer can add
  its own sections (e.g. speak's landing page).
- `audio` — a pressed/unpressed toggle that shows or hides every read-aloud
  control by setting `data-audio` on `<html>` (`off` hides `.wk-ra-btn`,
  `.wk-ra-bar`, and `wk-badge.wk-ra-state`); the speed selector dims while
  off and `<wk-read-aloud>` ends a playing session. The choice is per view: a
  shell names the storage key and default on `<html>` as `data-audio-key`
  (default `webkit-audio`) and `data-audio-default` (`on` unless `off`), which
  `boot.js` resolves before paint. present's deck shell opens off under
  `webkit-audio-deck`; its brief keeps webkit's defaults.

Light-DOM children it relocates into the bar:
- `<a data-nav [class=active]>…</a>` → nav links area.
- `<button data-extra id=…>…</button>` → controls area (rendered as-is, app keeps
  its own click handler, e.g. present's `shareBtn`). A `.btn` there keeps its
  frame and sits 0.5rem from the controls above 720px.

The header's own controls (`.ctrl-btn`, `.ctrl-select`, `.size-btn` inside
`.topbar-controls`) rest without fill or visible border and take the chip fill on
hover; `.active` and a pressed size button keep the primary fill. A select is as
wide as the option it shows, the size stepper's two buttons sit together with a
1rem glyph, and with no visible `.btn` the row pulls out by the last control's
padding so its icon ends on the content's right edge. `.ctrl-divider` is still
rendered but hidden. The same classes outside the header (present's graph
controls reuse `.size-btn`) keep the framed look.

Behavior (ported from the current `Webkit.init`): font/fixation/size/reload/theme
controls wired to GLOBAL keys `webkit-theme|font|size|fixation` (default light,
size 16 clamp 12–24, fixation persisted). The theme toggle flips between light
and dark and writes an explicit mode; `system` and the palette family per mode
(`webkit-palette-light`, `webkit-palette-dark`) are chosen on the themes page at
`/webkit/themes`. Every change goes through `applyTheme`, which sets
`data-theme` and `data-palette` on `<html>` and **dispatches
`new CustomEvent('wk-themechange', {detail:{theme, palette, mode}})` on
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
- `prepare` — boolean; registers the page's text with speak on connect so its
  parts are synthesized ahead of playback and the page shows how far that got
  (prepared mode, below).
- `name` — the document name speak files the page under (default: the page
  title).

Backend contract: the endpoint must serve an OpenAI-compatible
`POST {endpoint}/v1/audio/speech` accepting `{model, input, voice, speed}` and
returning a WAV body, with CORS allowed (the d-man `speak` route provides both
via mlx-audio + the route's `cors = true` flag). Cached mode (below) also needs
`GET {endpoint}/audio/{key}`, and prepared mode speak serve's document API
(`POST /read`, `GET /doc/{id}`, `POST /doc/{id}/prepare`); speak serve
provides all of them. On connect the element probes `GET {endpoint}/` with a
1.5s timeout — if unreachable it injects nothing, so pages degrade gracefully
on hosts without the speak service.

Playback: a session plays a list of parts, one speech request each, keeping
two parts in flight ahead of the one playing. While the playing part's clip is
still being fetched, the play button carries `.wk-ra-wait` (a slow pulse; a
still dim under reduced motion). So a long wait reads as work, not a hang.
One session at a time: starting another section stops the current one; the
section button toggles pause/resume. While a section plays, a restart button
appears next to its play/pause control and rewinds to the start of the
section; it hides again when playback ends.

- **Uncached mode** (present pages, any section without `data-ra-chunk`):
  section text is extracted skipping `pre/table/svg/button/wk-badge`
  subtrees, split into sentences (`Intl.Segmenter`), and grouped into parts
  that ramp up. One sentence goes first, so the first sound comes after one
  short request, then up to 250 characters, then up to 600 (`LIVE_RAMP` in
  `src/sentences.ts`, which mirrors speak's `internal/chunk`; change both
  together). A sentence without terminal punctuation (a heading, a list item)
  gets a period in the request so it doesn't run into the next. Each part is
  a `POST /v1/audio/speech` with the speed in the body. The playing part's
  sentences are wrapped in `.wk-ra-sentence.wk-ra-active` (soft
  `--ra-highlight` background) and kept in view.
- **Cached mode**: applies to a section whose blocks carry
  `data-ra-chunk="<key> [<key> ...]"` (the keys of the pre-synthesized parts
  that read the block, as prepared mode stamps them). Such a section plays the
  keys in its own `data-ra-parts` list (play order, repeats kept), or the
  blocks' keys in document order, each once, when that is absent. Each is
  fetched with `GET {endpoint}/audio/{key}`. Every block whose list holds the
  playing key gets `[data-ra-chunk].wk-ra-active`; no sentence spans. Cached
  parts are synthesized at the provider's default speed, so the speed control
  sets the audio element's `playbackRate` instead, live while a part plays.
  A section that holds text directly (present's summary block) counts as a
  block of its own when it carries `data-ra-chunk`.
- **Prepared mode** (`prepare` attribute; present pages): once the probe
  succeeds, the element collects each target section's readable blocks and
  posts them as one document. A block is the nearest block-level element
  around a run of text (`BLOCK_TAGS` in `src/read-aloud.ts`, among them
  `wk-section-heading`, `p`, `li`, `wk-callout`, the headings and
  `blockquote`), or the section itself when it holds text directly.
  `pre/table/svg/button/wk-badge` subtrees are left out as in uncached mode;
  each block's text has its whitespace collapsed, and a block left with no
  text is dropped. The request is
  `POST {endpoint}/read` with `{name, sections: [{blocks: [text, ...]},
  ...]}`; speak answers, per section, the keys that read each block and the
  section's play order. The element stamps `data-ra-chunk` and
  `data-ra-parts` itself, so cached mode takes over: `GET /audio/{key}`
  synthesizes a part on demand, and registration alone starts nothing. When
  registration fails (unreachable, non-2xx, an answer that doesn't mirror
  the request) nothing is stamped and the page reads live as before; the
  reason goes to `console.debug` only.

Prepared status: with the page registered, the element renders a bar inside
itself (`.wk-ra-bar`; `wk-read-aloud` turns visible where the consumer placed
it). The bar shows "Audio: N of M parts ready", what the rest are doing
(preparing, retrying, not prepared, failed), and the failure reason. Its
buttons are "Generate all TTS for page" (`POST /doc/{id}/prepare`, enabled
while idle or failed parts remain) and "Retry failed (N)" (`?failed=1`, shown
when something failed). Each section with parts gets a `wk-badge.wk-ra-state`
beside its play buttons (audio ready, generating i of n, queued, retrying,
failed with speak's reason, or not prepared). It also gets a "Retry" button
while it has failed parts (`?section=N&failed=1`).

The status is polled every 2s while any part is queued, generating or
retrying, and again after a prepare call and whenever a session fetches a
part. While speak doesn't answer, the poll backs off, doubling to once a
minute, and the next prepare click or fetched part brings the 2s cadence
back.

A 404 from `GET /doc/{id}`, or from a part during playback, means speak
restarted. The page then registers again, stamps anew (keys may differ) and
playback resumes from the same part. If that registration fails, the stamps
come off, one toast says so, and the page reads live from then on.

Selection speaker: independent of `targets`, selecting any text on the page
(outside the header/controls) shows a floating play button at the selection's
edge. It reads just the selected text, in parts like uncached mode, same
pause/resume toggle, no highlight spans (the selection itself is the visual).
Esc clears the selection, the float button, and any selection playback. One
element on the page enables this even with no `targets` attribute.

Failures: a clip that cannot be fetched or played ends the session and says
why, never silently. The button turns red (`.wk-ra-error`, reason in its
title) until the next click, and a `<wk-toast variant="err">` names the reason
(the element adds a `<wk-toast-host>` when the page has none). The element
also dispatches `wk-read-aloud-error` on `document` with `{detail:
{reason}}` so the page can react. The reason is the backend's
`{"error": {"message"}}` body when it sends one (speak does), else the status
code; an unreachable endpoint, empty audio, and a broken-off stream each get
their own message.

Fixation: the two work together, in any order. Fixation wraps each text node
that holds a word in `span.wk-fixation-run` with a `<b>` per word start, and
takes those spans off again on toggle-off; nothing else under a target moves.
So the play buttons, the badge and the text buttons injected into a section
that is also a fixation target (present's summary) keep their listeners. The
`data-ra-chunk`/`data-ra-parts` stamps and the `.wk-ra-active` highlight
stay, and a playing session, live or cached, carries on through the toggle.
Fixation's walk skips `button`, `select`, `textarea`, `wk-badge`, `svg`,
`code`, `b` and `strong` subtrees, so the controls' labels and the badge are
never half-bolded. A live session is planned on plain text: read-aloud takes
fixation's runs off the section before it wraps the sentences. Fixation's
observer bolds the text inside the whole-sentence spans again before the
next paint, so a sentence span never cuts through a word.

`Webkit.stopReadAloud()` ends any playing session. Removing the element does
not, so a page that replaces its sections (a client render swapping its
content) calls it first; otherwise the session plays on from the detached
section.

### Content components (CSS-only; define as no-op elements for semantics)
- `<wk-page-header>` wrapping `<wk-title>` + `<wk-subtitle>` — hero/page header
  (port `.brief-title`/`.brief-summary` + csl/catalog `.hero`). A `<wk-title>`
  outside `wk-card`, `.wk-card` and `wk-panel` gets `role="heading"` +
  `aria-level="1"` on connect unless it has a role. No top margin: the
  header's `2rem` bottom margin is the one space above a page's first line, and
  `.wrap` adds no top padding, so every page's title sits at the same height.
- `<wk-card>` — content/entity card; every card's border darkens on hover, and
  only a card you can act on lifts: `wk-card[href]`, an `<a class=wk-card>`, or a
  `<wk-card interactive>` the page made clickable itself (the attribute adds the
  lift and pointer, nothing else; the page wires the click and any `tabindex`).
  Port present `.row` + catalog `.card`.
- `<wk-panel>` with `<wk-panel-title>` / `<wk-panel-subtitle>` — info panel (port
  present `.panel`).
- `<wk-badge variant="a|b|c|outline|stat|accent">` — chips/badges/kind-badge (port
  present `.chip*` + catalog `.kind-badge`). `a|b|c` are non-semantic tag hues.
  `outline` draws its 1px `--border` ring inside the box (an inset shadow), so it
  stands as tall as a plain badge.
  `accent` mixes `--accent` (else `--primary`) by the status rule below, the ink
  moved `--accent-ink-share` toward `--text-1`, so its text reads 4.5:1 on its
  tint where the variant allows.
- `<wk-badge variant="info|ok|warn|error">` — semantic status badges on the
  `--status-<s>-bg` fill with `--status-<s>-text` ink, derived from
  `--blue|--green|--amber|--red` so the ink reads 4.5:1 on the fill. Use these for
  event level / finding severity / health state, never the `a|b|c` hues (which
  make a warning look like a success).
- `<wk-badge variant="filter" [active]>` — clickable pill for filter toggles and
  query chips. `[active]` fills the pill with `--chip-active-bg`. Example:
  `<wk-badge variant="filter" active>All</wk-badge>`. Without its own
  `tabindex` or `role`, and outside any link or button, it gets
  `role="button"` + `tabindex="0"` and Enter/Space click it. A `<small>` inside
  holds a count in the muted ink.
- `<wk-chips>` — a wrapping row of filter badges (`gap: 0.5rem`, `1.25rem` below).
- `<wk-button variant="primary|ghost|danger">` — align to the header `.btn`/`.btn-primary`/
  `.btn-ghost`/`.btn-danger` already in webkit.css (share the same rules). `danger` fills
  with the theme `--red` for destructive actions. `[hidden]` hides either one: the kit ships that
  override because the shared `display` rule would otherwise win. No variant, or an unknown
  one, falls back to the ghost look at zero specificity. Every button is at least
  `--wk-control-h` tall. `size="md"` (`.btn-md` on a `.btn`) is the larger size for a
  dialog's actions: `min-height: 2.25rem`, `padding: 0 1rem`, same text size.
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
  marks the selected segment. The group is `--wk-control-h` tall and the
  segments fill it. Example:
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
- `<wk-section>` with `<wk-section-heading>`, `<wk-section-subheading>` —
  document section with a ruled, primary-underlined heading. Example:
  ```html
  <wk-section>
    <wk-section-heading>Overview</wk-section-heading>
    <p>Body paragraph.</p>
    <wk-section-subheading>Details</wk-section-subheading>
    <p>More text.</p>
  </wk-section>
  ```
- `<wk-callout [variant="info|warn|ok|error"]>` — callout block framed in `--border`
  with a 3px bar on the left. Default bar color is `--primary` on the card fill;
  `variant="info"` uses `--blue`, `variant="warn"` `--amber`, `variant="ok"`
  `--green` and `variant="error"` `--red`, each on its `--status-<s>-bg` tint.
  The box ends where a section paragraph beside it does: the measure, read in
  the callout's smaller font, scaled by the paragraph's font over its own (its
  text is usually a bare text node, which no rule can size apart from the box).
  Example: `<wk-callout variant="info">Info callout.</wk-callout>`
- Reading measure: `--wk-measure` on `:root` (`76ch`) caps `wk-section > p|ul|ol|blockquote`,
  `.wk-prose > :is(p, ul, ol, blockquote)`, and `wk-callout` and `wk-section > details`
  at the same right edge. Tables, figures,
  charts, code blocks, stats and columns keep the full width. A page or subtree
  opts out with `--wk-measure: none` (present's deck does).
- `<wk-empty [variant="error"]>` with an optional `<wk-empty-title>` — the block
  a list or page shows while empty, loading, or failed: `--text-2` ink,
  left-aligned, `padding: 2rem 0`. `variant="error"` takes the callout's frame
  (a `--border` box with a 3px `--red` bar on the left) on `--status-error-bg`
  with `--status-error-text`, padded inside and as wide as its block; put the
  raw error under a short title. Example:
  `<wk-empty variant="error"><wk-empty-title>Could not load sessions</wk-empty-title>read sessions.jsonl: permission denied</wk-empty>`
- `<wk-columns [cols="2|3"]>` with `<wk-col>` children — two or three equal-width
  columns of blocks (two when `cols` is unset), with a grid gap, collapsing to one
  column under 700px. Transparent to the fixation and read-aloud walks; a column
  boundary ends a read-aloud sentence run. Example:
  ```html
  <wk-columns cols="3">
    <wk-col><wk-stat>…</wk-stat></wk-col>
    <wk-col><p>Prose beside it.</p></wk-col>
    <wk-col><ul><li>A list.</li></ul></wk-col>
  </wk-columns>
  ```
- `<wk-stat>` with `<wk-stat-value>`, `<wk-stat-label>`, and an optional
  `<wk-stat-sub>` — a framed tile with a large figure over a label. The value is
  skipped by the fixation walk (a figure is not prose); read-aloud reads value,
  label, and sub as three short sentences. Example:
  `<wk-stat><wk-stat-value>41 min</wk-stat-value><wk-stat-label>checkout outage</wk-stat-label><wk-stat-sub>Tuesday 14:02 to 14:43</wk-stat-sub></wk-stat>`
- `<wk-figure>` wrapping an `<img>` and an optional `<wk-figcaption>` — an image
  with a caption under it. The image is block-level, scales down to the width of
  whatever holds it (a column, say) and never up past its own pixels, and takes
  the kit's border and radius. The caption is prose in `--text-3` at the small
  size: fixation half-bolds it, and read-aloud reads it as a sentence of its own.
  Before the caption, read-aloud reads the image's `alt`, which stands in for the
  picture: an `<img>` with a non-blank `alt` is a block of its own to the walker,
  so give every image one. Example:
  ```html
  <wk-figure>
    <img src="/img/3a7f….png" alt="Error rate by minute, peaking at 14:15" loading="lazy">
    <wk-figcaption>The on-call dashboard at 14:15.</wk-figcaption>
  </wk-figure>
  ```
- `wk-section blockquote` with a `<p>` and an optional `<cite>` — a quotation
  with a primary-coloured left rule and the attribution under it in `--text-3`.
  Native elements, styled only inside a `<wk-section>` (rendered markdown has its
  own rule under `.wk-prose`). Example:
  `<blockquote><p>We never saw the resolver.</p><cite>On-call engineer</cite></blockquote>`
- `wk-section details` with a `<summary>` — a native disclosure framed like a
  panel, closed by default, with a kit-drawn marker that turns when open. One
  straight in a section ends at the callout's right edge. No JS;
  read-aloud opens it while a part inside plays so the highlight is visible.
  Example:
  `<details><summary>Full timeline</summary><ul><li>14:02 first 502s</li></ul></details>`
- `<wk-progress>` with `<wk-progress-bar>`, `<wk-progress-fill>`, `<wk-progress-label>` —
  horizontal progress bar. Consumer sets fill width inline. Example:
  `<wk-progress><wk-progress-bar><wk-progress-fill style="width:63%"></wk-progress-fill></wk-progress-bar><wk-progress-label>63%</wk-progress-label></wk-progress>`
- `<wk-toc>` with `<wk-toc-title>` and a plain `<ul>/<li>/<a>` — table of contents
  block. Links in `wk-toc` are styled in `--primary`. Example:
  ```html
  <wk-toc>
    <wk-toc-title>Contents</wk-toc-title>
    <ul>
      <li><a href="#overview">01 Overview</a></li>
      <li><a href="#usage">02 Usage</a></li>
    </ul>
  </wk-toc>
  ```
- `.wk-prose` — rich-markdown container for rendered Markdown/HTML bodies
  (headings, paragraphs, lists, tables, blockquotes, links, images, inline and
  fenced code). It is a **class**, not a custom element — put it on any block
  wrapping server-rendered HTML. Uses the palette vars for hierarchy and line
  spacing. Fenced code blocks are decorated client-side. On DOM ready,
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
  `--code-*` roles come from the generated palette: paper-on-cream in the
  default light variant, the page background in dark, and each family's own
  token colours. Use `language-text` when the
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
        <wk-button variant="ghost" size="md">Cancel</wk-button>
        <wk-button variant="danger" size="md">Delete</wk-button>
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
- `<wk-toast-host>` + `<wk-toast [variant="ok|warn|err"]>` — fixed bottom-right toast stack.
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

## FOUC guard — `/webkit/boot.js`
`webkit.js` is deferred, so theme + font-size are only applied after it runs —
persisted dark/large preferences flash on load otherwise. The kit serves one
canonical pre-paint snippet so every consumer loads the SAME guard instead of
hand-rolling it:

```html
<head>
  <!-- ONE blocking guard before any stylesheet; built from src/boot.snippet.js -->
  <script src="/webkit/boot.js"></script>
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

It resolves `webkit-theme` (`system` through `prefers-color-scheme`) into
`data-theme`, the family key for that mode into `data-palette`, and
`webkit-size` (clamped 12–24) into the `html` font-size before paint. The same
string is available at runtime as `Webkit.bootSnippet` and in Go as
`webkit.BootScript()`; all three come from `src/boot.snippet.js`, so never
paste the IIFE inline (the gallery's hand-pasted copy drifted once).

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
  contains `wk-card`. Keep the `--page-width` assertion.
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
