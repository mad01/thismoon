---
name: present
description: Generate a scrollable briefing page with fixation reading, Cytoscape.js graphs, and inline metric charts for digesting work summaries or research
---

# Present — Scrollable Briefing Pages

Generate scrollable HTML briefing pages served live over localhost by the **present MCP**. You pass structured JSON; the server renders it into the full page with warm-neutral theme, fixation-reading toggle, font/size controls, light/dark mode, and Cytoscape support.

## Trigger
When the user asks to present, summarize, or brief on a topic (e.g., "present the incident summary", "brief me on our infra stack", "summarize the PR changes").

## How it works

- **You pass structured JSON** — a Doc object with sections and typed blocks. The server renders it to HTML with the correct CSS classes, `data-fixation` attributes, and structure. You never write HTML.
- **The MCP stores and serves it.** `present_create` returns an `id` and a `url`. Keep the id for later edits.
- **Updates auto-reload.** `present_update` bumps the version; open browser tabs refresh. Call `present_open` **once**, never again for the same page.

## Message Types

Built on Pipeline Types (`recipes/claude/types/pipeline.md`, DocSection domain type). The type contract at a glance — full field tables in the format sections below.

```
Doc = {                                  // the `content` argument
  summary: string,                       // executive summary — Layer 1: the one line a reader acts on
  meta: string,                          // date · category · key stats
  chips: [{text: string, style: "stat" | "a" | "b" | "c" | "outline"}],
  sections: Section[]
}

Section = {
  h: string,                             // heading (TOC anchor)
  id: string | null,                     // 4-char ID badge (A001 / RC01) — required when items are referenced by ID
  blocks: Block[]
}

Block =                                  // discriminated on `t`
  | {t: "p", text}                       | {t: "h3", text}
  | {t: "callout", text, sev?}           | {t: "table", cols, rows}
  | {t: "kv", kv}                        | {t: "list", items, ordered?}
  | {t: "panel", title, sub?, accent?}   | {t: "progress", pct, label?}
  | {t: "graph"}                         | {t: "chart", kind, series | flows, title?, unit?, xunit?}
  | {t: "code", text, lang?}             | {t: "html", text}

Graph = {                                // the `graph` argument — one per page
  nodes: [{id, label, type?: "center" | "module" | "leaf" | "registry", color?}],
  edges: [{from, to, type?: "consumes" | "publishes", label?, weight?, flow?}],
  layout: "dagre" | "cose"
}

PageRef = {id, url, version}             // present_create return — hold `id` all session
```

**Flow:** gather content → build `Doc` (+ optional `Graph`) → `present_create` → hold `PageRef.id` → iterate via `present_update(id, ...)`.

## Process

1. **Gather content** — research the topic using code search, git log, PR data, or whatever sources are relevant. All data must come from real sources.

2. **Identify graph-worthy relationships** — look for dependency trees, data flows, architecture layers. Not every brief needs a graph — only add one when the data has meaningful connections.

3. **Build the Doc object** — structure the content using the JSON format below.

4. **Create the page** — call `present_create` with `title`, `content` (the Doc object), optional `graph`, and optional `references`.

5. **Open once** — call `present_open` with the id.

6. **Iterate** — call `present_update` with the same id and a new Doc object for the fields that changed. The open tab reloads automatically.

To edit a page created in an earlier session: `present_list` to find it (`has_doc: true` means source is available), `present_source` to get the Doc + graph JSON, modify, then `present_update`.

## ADHD briefing mode (opt-in)

**Off by default.** Build briefings normally unless the reader asks for this mode — triggers: "adhd mode", "adhd briefing", "shape it for adhd", or the `I Have ADHD` output style is active. When none of those apply, ignore this section entirely.

When on, shape the Doc so an ADHD reader can act on it. This changes structure and ordering only — it never changes the facts, and it stays inside the page (it does not affect chat replies, commits, or docs):

1. **Lead with actions.** Make the `summary` the single most important next action in concrete terms (a command, a path, a decision), not background. Put context lower.
2. **First section is "Do now."** Give it `id` `"D001"` and a `list` block of bounded, numbered steps — each one action, no "and then" twice.
3. **Cap every list at 5 items.** If more, split into a "Now" section and a "Later" section rather than one long list.
4. **Make wins visible.** Use `@chip(b:done)` chips and a `progress` block for anything completed or in-flight, so finished work is not buried in prose.
5. **One callout, not many.** If there is a single blocker or risk, use one `callout` with `sev: "warn"`. Do not scatter warnings.
6. **End with the next action.** The last section names ONE concrete thing to do next (`id` `"N001"`), doable in under two minutes.
7. **Matter-of-fact tone.** State cause and fix for errors; no "uh oh" framing.

Skip preamble sections ("about this brief", "overview of overview"). Every section earns its place by being actionable or a visible win.

## MCP tools

| Tool | Use |
|------|-----|
| `present_create(title, content, graph?, references?)` | Create a page. `content` is a Doc JSON object. `graph` is a Graph JSON object. `references` is `[{title, url}]`. |
| `present_source(id)` | Get the editable source: Doc JSON (`content_format: "doc"`) and graph JSON (`graph_format: "json"`), ready to modify and pass back to `present_update`. **Use this to edit a page from a new session.** Legacy pages return raw HTML/JS instead. |
| `present_read(id)` | Fetch the rendered page (HTML). For editing, prefer `present_source`. |
| `present_update(id, title?, content?, graph?, references?)` | Patch a page. Omit fields to leave unchanged. Pass `graph: ""` to remove a graph, `references: []` to clear refs. |
| `present_list()` | List all pages (id, title, url, version, updated, has_doc). `has_doc: true` means the page is source-editable via `present_source`. |
| `present_open(id)` | Open in browser — **once per page**. |

> **Workflow rule:** `present_create` returns `{id, url, version}`. The `id` is
> required for every subsequent call (`present_update`, `present_open`,
> `present_source`, `present_read`). Hold it for the duration of the session.
> If `present_open` fails (sandbox or no `open` binary), the URL from
> `present_create` is the direct link — return it to the user instead.

## Doc format (the `content` argument)

```json
{
  "summary": "Executive summary with **bold** and `code`.",
  "meta": "2026-05-29 · Category · Key stats",
  "chips": [{"text": "42 items", "style": "stat"}, {"text": "tag", "style": "a"}],
  "sections": [
    {
      "h": "Section Heading",
      "id": "A001",
      "blocks": [
        {"t": "p", "text": "Paragraph text."},
        {"t": "h3", "text": "Subsection Heading"},
        {"t": "callout", "text": "Important note.", "sev": "warn"},
        {"t": "table", "cols": ["Name", "Status"], "rows": [["foo", "@chip(b:done)"]]},
        {"t": "kv", "kv": [{"k": "Label", "v": "Value"}]},
        {"t": "list", "items": ["First", "Second"], "ordered": false},
        {"t": "panel", "title": "Title", "sub": "Subtitle", "accent": "terracotta"},
        {"t": "progress", "pct": 75, "label": "75%"},
        {"t": "graph"},
        {"t": "code", "lang": "go", "text": "func main() {\n\tfmt.Println(\"hello\")\n}"},
        {"t": "html", "text": "<custom>escape hatch</custom>"}
      ]
    }
  ]
}
```

### Top-level fields

| Field | Type | Description |
|-------|------|-------------|
| `summary` | string | Executive summary paragraph (rendered with fixation reading) |
| `meta` | string | Meta line below title (date, category, stats) |
| `chips` | array | Chip tags below summary |
| `sections` | array | Content sections with headings, optional IDs, and blocks |

### Inline markdown

All text fields support these patterns (the server renders them to HTML):

| Pattern | Renders to |
|---------|------------|
| `**bold**` | `<strong>` |
| `*italic*` | `<em>` |
| `` `code` `` | `<code>` |
| `[text](url)` | `<a href>` |
| `@chip(style:text)` | inline chip |

Chip styles: `stat` (gray), `a` (warm), `b` (green), `c` (blue), `outline`.

### Block types

| Type | Fields | Description |
|------|--------|-------------|
| `p` | `text` | Paragraph with fixation reading |
| `h3` | `text` | Subsection heading |
| `callout` | `text`, `sev?` | Highlighted callout. Severity: `info` (blue border), `warn` (amber border), or omit for default |
| `table` | `cols`, `rows` | Data table. Cells support inline markdown |
| `kv` | `kv: [{k, v}]` | Key-value pairs. Values support inline markdown |
| `list` | `items`, `ordered?` | Bulleted or numbered list. Items support inline markdown |
| `panel` | `title`, `sub?`, `accent?` | Titled card. Accent is a CSS variable name (terracotta, blue, green, purple, amber) |
| `progress` | `pct`, `label?` | Progress bar (0-100) |
| `graph` | (none) | Placement marker for the Cytoscape graph container |
| `chart` | `kind`, `series` or `flows`, `title?`, `unit?`, `xunit?` | Metric chart (Chart.js). `kind` is one of `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`. Inline — use as many as you like per page. See **Chart format** below |
| `code` | `text`, `lang?` | Fenced code block with language badge and copy button. `text` is verbatim code (NO inline markdown — backticks, `**`, `<` all render literally). `lang` sets the badge and syntax highlighting: `go`, `bash`, `json`, `python`, `typescript`, `yaml`, `sql` highlight; anything else (or omitted) renders plain with a `text` badge |
| `html` | `text` | Raw HTML passthrough for one-off custom content |

### Section fields

| Field | Type | Description |
|-------|------|-------------|
| `h` | string | Section heading (used for TOC anchor) |
| `id` | string? | 4-char visible ID badge: either one letter + three digits (e.g. `"A001"`) or two letters + two digits (e.g. `"RC01"`). Rendered as a pill next to the heading and in the TOC. Use when presenting multiple items that need to be referenced by ID. |
| `blocks` | array | Content blocks |

### Notes

- **TOC is auto-generated** from section headings when there are 2+ sections. Do not write TOC markup.
- **`data-fixation`** is applied automatically to paragraphs, summaries, callouts, kv-values, and panel titles.
- **Unknown block types** produce an HTML comment error — they don't break the page.
- **Use `code` blocks for multi-line code**, not `p` with backticks (inline `code` is for short identifiers) and not `html` with a hand-written `<pre>`.

### Presenting multiple items

When presenting N related items (review findings, bugs, options, comparison points), **always put them in one page as N sections — never create one page per item.**

Assign each section a 4-char `id`: one letter + three digits (e.g. `"A001"`) or two letters + two digits (e.g. `"RC01"`). This makes items easy to reference in conversation (e.g. "let's discuss A002").

```json
{
  "summary": "3 issues found during assessment.",
  "meta": "2026-06-05 · Code Review · 3 findings",
  "chips": [{"text": "3 items", "style": "stat"}],
  "sections": [
    {
      "h": "Race condition in poll loop",
      "id": "A001",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    },
    {
      "h": "Missing error propagation",
      "id": "A002",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    },
    {
      "h": "Stale cache after leader change",
      "id": "A003",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    }
  ]
}
```

Use this pattern for: review findings, bug lists, options comparison, enumerable sets of any kind.

## Graph format (the `graph` argument)

Pass a structured object — the server generates the full Cytoscape JS including theme-aware colors.

```json
{
  "nodes": [
    {"id": "core", "label": "Core Service", "type": "center"},
    {"id": "auth", "label": "Auth Module", "type": "module", "color": 0},
    {"id": "cache", "label": "Redis Cache", "type": "leaf"},
    {"id": "npm", "label": "npm Registry", "type": "registry"}
  ],
  "edges": [
    {"from": "core", "to": "auth"},
    {"from": "core", "to": "cache", "type": "consumes"},
    {"from": "auth", "to": "npm", "type": "publishes", "label": "v2.1"}
  ],
  "layout": "dagre"
}
```

### Node types

| Type | Visual |
|------|--------|
| `center` | Highlighted root (terracotta border, bold label) |
| `module` | Colored border from palette. Set `color` (0-3) for different colors |
| `leaf` | Plain node (default if type omitted) |
| `registry` | Dashed border |

### Edge types

| Type | Visual |
|------|--------|
| (omit) | Solid line |
| `consumes` | Solid line (explicit) |
| `publishes` | Dashed line |

Edges can have an optional `label` string.

### Edge weight and flow

Two optional edge fields turn a dependency graph into a traffic map:

| Field | Type | Effect |
|-------|------|--------|
| `weight` | number | Traffic volume on the edge (requests per second, messages, bytes; any one unit per graph). Line width scales with it, and edges in the top third of the range are tinted in the accent color |
| `flow` | boolean | Animates dashes moving from source to target. Speed follows `weight`; without weights every flow edge moves at a calm middle speed |

```json
{
  "nodes": [
    {"id": "ingress", "label": "ingress", "type": "center"},
    {"id": "api", "label": "api gateway", "type": "module", "color": 1},
    {"id": "db", "label": "postgres", "type": "registry"}
  ],
  "edges": [
    {"from": "ingress", "to": "api", "weight": 1200, "flow": true, "label": "1.2k rps"},
    {"from": "api", "to": "db", "weight": 600, "flow": true, "label": "600 rps"}
  ]
}
```

Put the number in `label` too when the reader should see it; the width alone only shows relative volume. The animation pauses while the graph is scrolled off screen or the tab is hidden, and it renders one static frame when the reader has Reduce Motion on.

### Layout

- `dagre` (default) — layered DAG layout for trees and hierarchies; accounts for node size and minimizes edge crossings
- `cose` — for general graphs with no clear hierarchy

Dagre takes an optional `direction`: `TB` (top-down) or `LR` (left-to-right). Omit it for auto — small graphs (≤8 nodes) draw left-to-right to fill the wide container, larger ones top-down.

Place a `{"t": "graph"}` block in the section where you want the graph to appear.

## Chart format (the `chart` block)

Unlike the graph (one per page, passed as the `graph` argument), charts are **inline blocks** — drop a `{"t": "chart", ...}` block into a section's `blocks` array, as many as you need. The data lives in the block itself.

A chart block is just another entry in `sections[].blocks`, never a top-level argument. Putting it in the full Doc:

```json
{
  "sections": [
    {
      "h": "Traffic",
      "blocks": [
        {
          "t": "chart",
          "kind": "bar",
          "title": "Requests per day",
          "unit": "req",
          "series": [
            {"name": "api", "color": "blue", "points": [{"x": "Mon", "y": 120}, {"x": "Tue", "y": 180}]},
            {"name": "web", "color": "green", "points": [{"x": "Mon", "y": 80}, {"x": "Tue", "y": 95}]}
          ]
        }
      ]
    }
  ]
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | One of `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`. Defaults to `bar` |
| `title` | string? | Caption above the chart |
| `unit` | string? | Value-axis unit label (e.g. `ms`, `req`). Ignored for `sparkline`; shown in tooltips for `doughnut` |
| `xunit` | string? | X-axis unit label, `scatter` only (e.g. `payload KB`) |
| `series` | array | One or more `{name?, color?, points}` series. Every kind except `sankey` |
| `flows` | array | `{from, to, value}` links, `sankey` only. See **Sankey format** below |

Each series: `name` (legend label, shown when 2+ series), `color` (one of `terracotta`, `blue`, `green`, `purple`; omit to auto-assign by index), and `points` — an array of `{x, y}` where `x` is a category label (string) and `y` the value. Sparklines use only `y` (omit `x`). Scatter points take a numeric `x` (a JSON number or a numeric string).

Charts use the same palette as the graph and recolor automatically on theme toggle. Hover shows a tooltip on every kind but `sparkline`; the entry animation plays once per render and is skipped when the reader has Reduce Motion on.

### Which kind

- `bar` — counts per category, one or more series side by side.
- `stacked-bar` — composition per category (for example responses by status class per day). Series stack in order; keep it to four series.
- `horizontal-bar` — a ranked list with long labels (stages, repos, endpoints). One series, sorted by value.
- `line` — one or more trends over time, compared on one axis. Never two value axes: split into two charts instead.
- `area` — a single trend where the filled volume matters.
- `sparkline` — a compact trend beside a stat, drawn without axes.
- `doughnut` — share of a whole. One series, at most four slices; fold the rest into "other".
- `scatter` — correlation between two numbers, one point per observation.
- `sankey` — volume flowing between stages or services.

### Sankey format

```json
{
  "t": "chart",
  "kind": "sankey",
  "title": "Requests per second",
  "flows": [
    {"from": "ingress", "to": "api gateway", "value": 1200},
    {"from": "api gateway", "to": "orders", "value": 800},
    {"from": "orders", "to": "postgres", "value": 600}
  ]
}
```

Node names are matched by exact string, so reuse the same spelling on every link. Each node takes the next palette color in order of first appearance and every link fades from its source color to its target color.

## References

The `references` parameter takes `[{title, url}]` — source links rendered at the bottom of the page by the server. Always include references when the brief draws on external sources.

```json
[
  {"title": "dotfiles repo", "url": "https://github.com/mad01/dotfiles"},
  {"title": "PR #42", "url": "https://github.com/mad01/dotfiles/pull/42"}
]
```

## Colors available

Accent names for panel borders and chip references: `terracotta`, `blue`, `green`, `purple`, `amber`, `red`, `yellow`.
