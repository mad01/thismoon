# webkit, shared web-component UI kit

Private Go module + TS package that supplies the **shared theme/header/controls
and content components** for the mad01 local web tools (`present`, `csl`,
`deps`, `events`, `moondb`, `speak`, `status`). Single source of truth for how
they look. Authored in TypeScript/CSS, compiled to committed `dist/`, embedded
in consumers via `go:embed` and served at `GET /webkit/`.

## Layout

```
src/webkit.ts        # custom element definitions, the theme API, Webkit.bootSnippet FOUC guard
src/a11y.ts          # the keyboard path's decisions (page heading, filter badge keys, skip target)
src/format.ts        # pure text formatters: Webkit.timeAgo, formatDuration, formatDate, formatClock, dayName
src/url-state.ts     # Webkit.urlState: the query string as view state, with back/forward
src/settings-sync.ts # settings that follow the reader between .this hosts (d-man's /__this/settings.json)
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
Renders the topbar, sticky above 1024px. Key attributes: `brand`, `brand-href`, `back-label`,
`back-href`, `title`, `controls` (CSV, default `cmdk,font,fixation,size,reload,theme,help`),
`page-width` (default 1080), `skip-link` (`off` drops the skip link).

Its first child is a visually hidden "Skip to content" link, the page's first
tab stop; it moves focus to the first `main`, else `.wrap`, else the element
after the header. The bar wraps at any width: when the left group and the
controls do not fit one row, the controls drop to a second row on the right,
and a control set wider than the bar wraps inside itself, so the header never
widens the page; a header that fits stays one row, as before. Under 720px the
controls take one row of their own under the left group, starting under the
brand (the left group still wraps, so many nav links take more rows), the
`.ctrl-label` text of Fixation, Audio and Reload and the ⌘K hint hide (each
control keeps its `aria-label`). The controls row there never wraps: a set
wider than the screen scrolls sideways, edge to edge, and Tab scrolls each
control into view with room for its focus ring. Under 480px the icon controls
tighten to 28px squares with no gap, so the default set plus one framed
button fits 390px without scrolling; present's full set with Slides and Share
scrolls. Up to 1024px the bar scrolls with the page instead of sticking, so a
wrapped bar never holds part of the screen.

Light-DOM children it relocates:
- `<a data-nav>…</a>` → nav links area.
- `<button data-extra id=…>…</button>` → controls area (app wires its own handler by `id`).
- `<template data-wk-help>…</template>` → its innerHTML is appended into the `help`
  control's feature-guide modal, so a consumer adds app-specific sections without JS
  (e.g. speak's "read a document" note). The base modal already documents the shared
  controls plus a read-aloud/speed section when `speed` is present.

On theme toggle dispatches `new CustomEvent('wk-themechange', {detail:{theme, palette, mode}})`
on `document`. Consumers listen there (present recolors its Cytoscape graphs).
The same event fires when another tab of the origin changes the theme (the
themes page beside a brief) and when the system appearance flips in system mode.

### Content components (CSS-only)
All use light DOM only (no Shadow DOM) — styled by tag + attribute selectors in
`webkit.css`. Two carry a little behaviour, each in its own class: `wk-title`
sets `role="heading"` + `aria-level="1"` when it has no role and sits outside
`wk-card`, `.wk-card` and `wk-panel`; `wk-badge variant="filter"` without its
own `tabindex` or `role`, outside any link or button, gets `role="button"` +
`tabindex="0"`, and Enter and Space click it (badges that set either are the
page's to wire). Each connect checks again, so a badge moved into a link or
button hands both back.

Layout/content: `<wk-page-header>` / `<wk-title>` / `<wk-subtitle>`,
`<wk-card>` (`--accent` left bar; the lift on hover only for `a.wk-card`,
`[href]`, and `[interactive]`, which a page sets on a card it made clickable),
`<wk-panel>` / `<wk-panel-title>` / `<wk-panel-subtitle>`,
`<wk-button variant="primary|ghost|danger" [size="md"]>` (no or an unknown
variant gets the ghost look; `size="md"` or `.btn-md` is the 2.25rem dialog
size), `<wk-table>`.

Interactive: `<wk-search>` (optional first-child `<svg>` as leading icon),
`<wk-seg>` (`<button class="active">` marks selected segment).

Badges: `<wk-badge variant="a|b|c|outline|stat|accent|filter">`, and the
status variants `info|ok|warn|error` on the `status-<s>-bg`/`-text` roles;
`variant="filter"` is a clickable pill, `[active]` fills it with
`--chip-active-bg`, and a `<small>` inside holds a count; `accent` binds
`var(--accent, --primary)`, mixed by the status rule in the stylesheet with the
variant's `--accent-ink-share`, so its text reads 4.5:1; under a page's own
`--accent` the ink takes 65%, since the share is picked for `--primary`. `<wk-chips>` lays
filter badges out in a wrapping row.

Shared looks: one focus ring for everything the keyboard reaches except text
inputs and `tabindex="-1"` containers, at zero specificity (`:where(...)`). Plain links (`a:not([class])`) in
content blocks take the primary ink and a soft underline, also at zero
specificity. The header's controls rest quiet, scoped to `.topbar-controls`: selects as
wide as their option, a gapless size stepper, and with no visible `.btn` the
row ends on the content's right edge.
`color-scheme` follows `data-theme`.

Content blocks: `<wk-kv>` / `<wk-kv-row>` / `<wk-kv-label>` / `<wk-kv-value>`
(`variant="card"` adds border + rounded corners + uppercased labels),
`<wk-section>` / `<wk-section-heading>` /
`<wk-section-subheading>` (ruled heading), `<wk-callout variant="info|warn|ok|error">`,
`<wk-progress>` / `<wk-progress-bar>` / `<wk-progress-fill>` /
`<wk-progress-label>`, `<wk-toc>` / `<wk-toc-title>`, `<wk-columns cols="2|3">` /
`<wk-col>` (equal columns, one under 700px), `<wk-stat>` / `<wk-stat-value>` /
`<wk-stat-label>` / `<wk-stat-sub>` (figure over a label; the value is skipped by
fixation), `<wk-figure>` / `<wk-figcaption>` (an image with a caption; read-aloud
reads the image's `alt` in its place), and inside a section the native
`blockquote` (+ `cite`) and `details` (+ `summary`, closed by default; read-aloud
opens it while a part inside plays). `<wk-empty [variant="error"]>` /
`<wk-empty-title>` is the empty, loading or failed state of a list or page;
`variant="error"` takes the callout's frame at the full width of its block.

Spacing and measure: the header's `2rem` bottom margin is the only space above
a page's first line (`wk-page-header` has no top margin, `.wrap` no top
padding). `--wk-measure` (`76ch`, on `:root`) caps prose straight in a
`wk-section` or `.wk-prose`; the `wk-callout` box and a `details` straight in a
section end at the same right edge (their caps scale the `ch` measure by the
paragraph's font over their own). `--wk-measure: none` opts a subtree out.

Tokens (MAD-567), on the same `:root` block: radius by role
`--wk-radius-mark|control|field|surface|overlay|pill` (4, 6, 8, 10, 14,
999px), spacing `--wk-space-N` (N × 0.25rem, N in `0-5 1 1-5 2 2-5 3 4 5 6 8
10`), `--wk-control-h` (1.75rem: every `.btn`/`wk-button` min-height, each
header control, `wk-seg`) and motion `--wk-dur-fast|--wk-dur|--wk-dur-theme`
(0.15s, 0.2s, 0.3s). Rules read them; service CSS should too, rather than
copying near-values. Spacing tokens are rem, so only rem literals map to them
(the size control moves the root font size; px spacing stays px). Values off
the scale are literals on purpose: the header layout rules and its 55px bar
(the controls row keeps a 30px min-height for it), `.wrap`, `wk-page-header`,
the `wk-search` input (csl mirrors it), `wk-badge` padding (moondb overrides
it), the `wk-stat` and `wk-section-heading` box (present restyles them), and
snaps that would move content by more than 2px. `embed_test.go` pins each
token's name and value.

Overlays: `<wk-modal [hidden]>` / `<wk-modal-panel>` / `<wk-modal-head>` /
`<wk-modal-actions>` (fade+slide in; toggle `[hidden]` to open/close),
`<wk-form>` / `<wk-form-row>` / `<wk-form-hint>`,
`<wk-toast-host>` + `<wk-toast variant="ok|warn|err">` (fixed bottom-right stack).

Full spec with markup examples: `COMPONENTS.md`.

## Format helpers

`src/format.ts` holds the time, duration and date text the consumers used to
write by hand (MAD-595, MAD-604), on the global beside `el`, `poll` and
`escapeHtml`. All are pure: no DOM, and the clock is read only when the caller
leaves `now` out.

- `Webkit.timeAgo(value, opts?)`: how long before `opts.now` a moment was.
  `value` and `now` take a Date, an ISO string or epoch ms; a number is always
  ms, so a caller with epoch seconds multiplies by 1000. `style` is `short`
  (default; csl, events: `42s ago`, `5m ago`, `3h ago`, `2d ago`), `medium`
  (moondb: `just now`, `5 min ago`, `3 h ago`, `2 d ago`) or `long` (deps:
  `just now`, `1 minute ago`, `5 minutes ago`). `daysAfterHours` (default 24)
  is where hours give way to days; moondb passes 48. Counts round down, a
  future moment reads as no time, and there are no weeks or dates.
- `Webkit.formatDuration(ms, opts?)`: `style` is `precise` (default; `850ms`,
  `4.2s`, `3m12s`, `1h02m`) or `coarse` (`<1m`, `12m`, `1h 05m`, `2d 3h`). A
  negative span counts as zero.
- `Webkit.formatDate(value, opts?)` (MAD-604): a moment in the local zone.
  `style` is `stamp` (default; moondb: `Wed Oct 28 15:04`, Go's
  `Mon Jan 2 15:04`), `day` (moondb's and events' day headings:
  `Wed Oct 28`, plus `, 2025` when the year is not `opts.now`'s), `ymd-hm`
  (status, present's index: `2026-10-28 15:04`) or `ymd-hms` (status:
  `2026-10-28 15:04:05`). Weekday and month names are fixed English, never
  the browser's locale.
- `Webkit.formatClock(value, opts?)`: `15:04:05`, or `15:04` with
  `seconds: false` (status, moondb).
- `Webkit.dayName(value, opts?)`: `Today` or `Yesterday` by local calendar
  day against `opts.now`, `''` for any other day; days count between local
  midnights, so a daylight saving day is one day. events composes
  `dayName(t) || formatDate(t, { style: 'day' })`.

A missing, unparseable or Go zero time (year 1), and a duration that is not a
finite number, return `opts.fallback` (default `''`), never `NaN` text and
never a throw. `test/format.test.mjs` pins the strings each old copy printed,
and runs the old date copies beside the helpers over two years of moments in
three zones. Money, token counts and byte sizes stay in moondb, the only
service that writes them; fold one in when a second service needs the same
text.

## URL state

`src/url-state.ts` is `Webkit.urlState`, the query string as a page's view
state (MAD-548), folded from the readURL/syncURL pairs in deps, events,
moondb and csl:

- `read()` parses `location.search` into an object: the first value per
  param, an absent one `undefined`, no prototype. `list(value)` splits a
  comma list param into its non-empty items.
- `write(values, { push?, defaults? })` sets the whole query, keeping the path
  and hash. `''`, `null`, `undefined`, `false`, a non-finite number and an
  empty list are left out, `true` is `1`, a list joins with commas, and a
  value equal to its `defaults` entry is left out. It replaces the current
  history entry unless `push` is set, and does nothing (returns `false`) when
  the query is already the one in the address bar, compared as a set of
  params, so a re-render or an old link in another order adds no entry.
- `subscribe(fn)` calls `fn(params)` on a `popstate` that lands on another
  query of the same page, so the page shows that view again; a step that
  only moves the hash (an in-page anchor) is not one. It returns a stop
  function. Pages that subscribe write through `write`, which keeps the
  query it last showed.
- `parse`, `serialize` and `nextHref` are the pure parts; `test/url-state.test.mjs`
  covers them, and runs read, write and subscribe against a stand-in tab.

The rule the consumers follow: a discrete change (a chip, a segment, a
submitted search) pushes, typing replaces. Param names and encoding are the
ones each page wrote before, so old links still load. brain and present keep
their own URL code: brain's view (graph data, camera, popup, local graph)
has no restore path to hook back/forward into, and present's deck already
follows its `#N` slide hash on `hashchange`.

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
`src/themes.mjs`: tags, the status fills and inks (`status-<info|ok|warn|error>-bg`
and `-text`), code tokens, the Cytoscape box and tone colours, chart
grid and text, `text-body`, the alpha surfaces (`topbar-bg`, `scrim`,
`focus-ring`), and the `on-<colour>` inks. An `on-<colour>` ink is white or
the variant's deepest ink (`text-1` in light, `bg` in dark), whichever reads
with more contrast on that fill.

`themes_contrast_test.go` holds every ink, light and dark, to 4.5:1 on its
fill. Each mode has a named exception map. In dark it lists solarized's red,
orange, and violet. In light it lists the fills whose family ink is too pale
to win and which would have to become a different colour for white to reach
the bar. Those are catppuccin's green, peach, and yellow, One's red, green,
and yellow, and solarized's green and yellow. Where a fill sat just under
the bar, the family moved it a few points of lightness with the hue held. It pinned the roles derived from that
fill to the published values, so only the fill and its ink changed (MAD-372,
MAD-376, MAD-378).

The same file holds each `status-<s>-text` to 4.5:1 on its `status-<s>-bg`
(the semantic badges and callout tints read them). The fill is paper 14%
toward the colour; the ink is the colour moved toward `text-1` by the first of
45%, 55% and 65% at which all four inks clear the bar, so each variant keeps
as much hue as it can (the default family takes 45%), and
`TestStatusInkShareIsTheFirstThatReads` holds that pick. Solarized light and
dark and Tokyo Night Day clear at no share: their own `text-1` only just
clears 4.5 on paper. They take 65% and sit in `statusFloorExceptions` with
their measured floors (MAD-566). The accent badge mixes at run time, so the
build makes the same pick for it on `primary` and emits it as the
`accent-ink-share` percentage, held by `TestAccentBadgeInkReadsOnItsFill`. The
status roles are not in `Roles()`; no page names them as a reference.

A variant can pin any derived role under `overrides`. The default family
does that for every role whose derived value differs from the old
hand-written stylesheet, so it stays byte-identical (pinned by
`TestDefaultFamilyPinsTodaysPalette`).

The build emits every colour role as a literal hex or rgba, never a `var()`
or a `color-mix()` (the shadows and `accent-ink-share` are literal non-colour
values). Cytoscape and Chart.js parse colour strings themselves, and
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
ramp value. The aliases are gone too. `--green-light` went when csl, its
last reader, moved its query tokens to the semantic colour roles.
`--terracotta` went when present started rendering the panel accent stored
under that name as `var(--primary)`; present's own shell declares the old
name for pages a shared instance never rerenders.

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
make test         # SANDBOXED: in the container VM, fail if the committed dist/ differs from a fresh build,
                  # then npm test + npm run test:dom against it; then go test on the host
make test-local   # fallback: the same on the host (test:dom needs `npm ci` there first)
npm test          # node --test test/*.test.mjs; pure-function unit tests, no dependencies, safe on the host
npm run test:dom  # node --test test/dom/*.test.mjs; the DOM tests (needs happy-dom from npm ci)
go test ./...     # asserts Handler() serves /webkit/webkit.css + .js + the themes page; pins the default palette
```

### DOM tests

`test/dom/` renders the custom elements. `page.mjs` opens a happy-dom
(`happy-dom`, pinned exact in `devDependencies`) window per test, runs the
built `dist/webkit.js` in it as the classic script a consumer loads, and
answers the version poll and the site list fetch with a 404, so nothing
reaches the network. The tests cover the header's controls, skip link,
relocated children and theme toggle, the theme API and `wk-themechange`,
`wk-title`'s heading role, the filter `wk-badge` keys and their hand-back,
and `Webkit.urlState` against a real history. They read the committed
`dist/webkit.js`, the file the services embed. `make test` builds the sources
in the VM and fails when the committed `dist/` differs from that build, so a
source edit without `make build` fails there. It then runs the unit and DOM
tests against the committed files. On the host, run `make build` before them.

They run in the container because happy-dom is npm code: the host keeps its
dependency-free `npm test`, and only the throwaway VM installs and runs it.
The export stage never reads the test stage, so the dependency cannot reach
`dist/`.

Add a DOM test when the behaviour lives in a connected element (a callback,
an attribute the kit sets, an event it fires, focus it moves); keep the
decision itself a pure function under `test/`, as `a11y.ts` is. Assert what a
page or a screen reader sees (attributes, focus, events, storage), not
private fields. happy-dom has no layout, so widths, rows and contrast are
screenshot and Go-test territory, not DOM-test territory.

**Why the container build:** `npm ci` runs arbitrary install-time code
(postinstall scripts and the dep tree itself: esbuild, tsc, prismjs +
transitive). The VM keeps a poisoned npm package off the host; nothing
host-side is mounted into the build, only `dist/webkit.js` + `dist/webkit.css`
come out. Output is byte-identical to a host build (bundle output is
platform-independent). Review `git diff dist/` before committing — the bundle
rides into every consumer Go binary via `go:embed`. The general when-to guide
and the extraction-pattern rules are in
[docs/CONTAINER-BUILDS.md](../docs/CONTAINER-BUILDS.md); this package is its
reference implementation.

### Adding a component

1. Add CSS rules in `src/webkit.css` (tag + attribute selectors; CSS-first).
2. Register the element as a no-op in `src/webkit.ts` — one fresh class per tag
   (no shared superclass, so each element stays independently removable).
3. Add an example to `examples/gallery.html`.
4. Document it in `COMPONENTS.md` and `docs/components.md`.
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
  `webkit-ra-speed`). They persist across a tool's own pages, and a theme
  change in one tab reaches the others through the `storage` event. The one exception
  is the audio toggle. Its key is `webkit-audio` by default, but a shell may
  name a per-view key and default on `<html>` (`data-audio-key`,
  `data-audio-default`), and boot.js and the header both read them. That is
  how present's deck opens with the read-aloud controls hidden under
  `webkit-audio-deck` while its brief keeps its own choice.
- **Six keys follow the reader between `.this` hosts** (`src/settings-sync.ts`):
  the theme, both palettes, font, size and fixation, not `webkit-audio` or
  `webkit-ra-speed`. d-man serves one document on every host at
  `/__this/settings.json`, same-origin, with the localStorage keys as its wire
  keys. Only on a hostname ending in `.this`, with site storage the page can
  use, `webkit.js` pulls it at load and again on focus, on the page turning
  visible and on a back/forward restore (at most once per 2s). Unless that
  first answer is a `{"v":1,"settings":{...}}` JSON document within 1.5s,
  sync stays off for the page load and sends nothing more, which keeps the
  kit safe behind an old d-man. localStorage stays the first-paint cache:
  boot.js is unchanged, so a pull that changes something repaints after
  first paint. The server wins for every key it holds; a key it lacks is
  seeded from a local value that is not the default. The server also wins
  over a newer value another tab of the same host wrote moments earlier,
  before that tab's PATCH landed. That shows as a brief flicker, which
  converges on the next focus. The reader's own changes (the mode and palette setters, reset,
  the header's font, size and fixation controls) go out as one PATCH per
  200ms, one in flight. On pagehide the queue goes at once with keepalive,
  even beside a PATCH in flight or before the first pull has answered. Two
  PATCHes with the same key can then land out of order, which leaves d-man on
  the older value until the reader changes that key again. After a jump to
  another host, the new page's first pull can beat the old page's last push,
  so the new page shows the old value until the next focus. A failed PATCH
  is resent before the next pull. A 400 on a patch of several keys is resent
  key by key, all at once, so the valid keys land. A refused value is not
  resent: a key refused on its own, or any key of a patch another 4xx
  refuses. Pulls leave its local value alone until the reader changes that
  key again or the page reloads. Absent keys read as their defaults
  (`light`, `default`, `Fira Code`, `16`, `false`). The endpoint cannot delete a key, so a reset
  sends those defaults, and a pulled `default` palette removes the local key;
  every other pulled value is stored as sent.
- **A theme is a file; a rule reads a role.** Never put a colour literal or a
  ramp name back into `src/webkit.css`: add or derive a role in
  `src/themes.mjs` so every family gets it. A role the graph or chart code
  reads must resolve to a hex literal, and the build refuses one that doesn't.
- **The default family is pinned.** `TestDefaultFamilyPinsTodaysPalette` holds
  the literal values the old stylesheet and present's colour code carried;
  change the default on purpose or not at all.
- **`data-extra` buttons are not wired by webkit.** The consuming app keeps its
  own click handler (e.g. present's `shareBtn`).
- **Light DOM only.** No Shadow DOM — everything is styled by tag + attribute
  selectors in `webkit.css`, so CSS vars and fixation text-walk work normally.

## See also

- Catalog entry: `service-info.yaml` (System `webkit`, Component `webkit-ui`).
- Consumers live under `services/` as they migrate into this repo.
- Full component spec: `COMPONENTS.md`.
