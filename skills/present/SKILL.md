---
name: present
description: Generate a scrollable briefing page with fixation reading, Cytoscape.js graphs, and inline metric charts for digesting work summaries or research, or a slide deck beside it for talking the same material through
---

# Present — Scrollable Briefing Pages

Generate scrollable HTML briefing pages served live over localhost by the **present MCP**, or on a shared instance others reach by link. You pass structured JSON; the server renders it into the full page with warm-neutral theme, fixation-reading toggle, font/size controls, light/dark mode, and Cytoscape support.

## Trigger
When the user asks to present, summarize, or brief on a topic (e.g., "present the incident summary", "brief me on our infra stack", "summarize the PR changes"). Or when the user asks for slides or a deck on it ("make a deck of this", "slides for the review").

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
  nodes: [{id, label, type?: "center" | "module" | "leaf" | "registry", color?,
           tone?: "neutral" | "green" | "red" | "blue" | "amber" | "purple"}],
  edges: [{from, to, type?: "consumes" | "publishes", label?, weight?, flow?}],
  layout: "dagre" | "elk" | "elk-layered" | "elk-mrtree" | "elk-stress" | "elk-radial" | "elk-force" | "cose",
  direction?: "TB" | "LR"
}

PageRef = {id, url, has_deck, deck_url?, version}   // present_create return — hold `id` all session
```

A page carries a brief (the `content` Doc above), a deck (a second Doc passed as `deck`, one section per slide), or both under one id. See **Slide decks** below.

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
| `present_create(title, content?, deck?, graph?, references?)` | Create a page. `content` is a Doc JSON object (the brief). `deck` is a second Doc JSON object (the slides). At least one of the two. `graph` is a Graph JSON object. `references` is `[{title, url}]`. Returns `deck_url` when there is a deck. |
| `present_source(id)` | Get the editable source: Doc JSON (`content_format: "doc"`), the deck's Doc JSON (`deck`, empty without one), and graph JSON (`graph_format: "json"`), ready to modify and pass back to `present_update`. **Use this to edit a page from a new session.** Legacy pages return raw HTML/JS instead. |
| `present_read(id)` | Fetch the rendered page (HTML, plus the deck's HTML). For editing, prefer `present_source`. |
| `present_update(id, title?, content?, deck?, graph?, references?)` | Patch a page. Omit fields to leave unchanged. Pass `graph: ""` to remove a graph, `deck: ""` to remove the deck, `references: []` to clear refs. A page keeps at least one of content and deck. |
| `present_list()` | List all pages (id, title, url, version, updated, has_doc, has_brief, has_deck, deck_url). `has_doc: true` means the page is source-editable via `present_source`. |
| `present_open(id, deck?)` | Open in browser — **once per page**. `deck: true` opens the slide deck instead of the brief. |
| `present_deck(id, action, slide?)` | Drive the deck that is open in the browser: `start` or `stop` presenting, `next`, `prev`, `goto` a 1-based `slide`. Local instance only. |
| `present_share(id, ephemeral?)` | Push the page to the shared instance this machine is configured for and return `{url, ephemeral, expires_at, shared_at}`. Only registered when a shared instance is configured; sharing again replaces the copy under the same link, `ephemeral` makes it expire 30 days after the last share. |

> **Workflow rule:** `present_create` returns `{id, url, version}`. The `id` is
> required for every subsequent call (`present_update`, `present_open`,
> `present_source`, `present_read`). Hold it for the duration of the session.
> If `present_open` fails (sandbox or no `open` binary), the URL from
> `present_create` is the direct link — return it to the user instead.
> When the user asks to share a page, call `present_share` with the id and
> return the link. If the tool is missing or fails with a network error,
> tell the user to use the Share button on the page or run `present share <id>`.
> A brief and its deck share one id and one share link: never create a
> second page for the deck of an existing brief; pass `deck` to
> `present_update` on the same id.

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

### Linking to code and sources

Prose that names a file, function, PR, ticket, or commit links to it. The label is the short form the reader scans; the URL carries the repo and the path. Never write a bare `org/repo/path/to/file.go` in a text field.

```json
{"t": "p", "text": "The rank gap is widened in [app.js:26](https://github.com/mad01/thismoon/blob/1e2f309/services/present/internal/server/app.js#L26-L48), see [PR #164](https://github.com/mad01/thismoon/pull/164)."}
```

Rules:

1. **Label with the short form.** `graph.go:47`, `RenderGraph`, `PR #164`, `MAD-357`. The reader sees where, the link says exactly where.
2. **Permalink at a commit, not a branch.** Build GitHub links from `git remote get-url origin` and `git rev-parse HEAD`: `https://github.com/<org>/<repo>/blob/<sha>/<path>#L<start>-L<end>`. A branch link points at the wrong lines as soon as the file changes.
3. **Local pages can link into csl.** `csl_show_file(repo, file, start_line, end_line)` returns a `csl.this` link that opens those lines in the local code viewer. Use it for a page that stays on this machine; use GitHub links for a page that will be shared.

`http`, `https`, `mailto`, relative, and fragment URLs render as links. Anything else stays literal text.

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
- **`data-fixation`** is applied automatically to the title, summary, table of contents, section headings and subheadings, paragraphs, callouts, and table bodies. The page shell adds kv values, list items, and panel titles by element. Chips, section ids, table headers, and the meta line stay out of the fixation walk.
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

## Slide decks (the `deck` argument)

A deck is a second Doc under the same page id, shown one section at a time at `deck_url` (`/p/<id>/deck`). It uses the Doc schema above unchanged: `summary`, `meta`, and `chips` make the title slide, every entry in `sections` is one slide, and the page's `references` make the last slide. The page's one `graph` is shared with the brief, so a `{"t": "graph"}` block in a deck section shows the same graph on that slide. Pass `deck` to `present_create` beside `content`, or alone for a deck-only page, and to `present_update` to replace it (`deck: ""` removes it). The deck is never generated from the brief: you write it, and it says less.

### When to make one

Make a deck when the material will be talked through: a review in a meeting, an incident retrospective, a decision the room has to make. A brief someone reads alone stays a brief. When both exist, the deck is the version for the room and the brief is where the detail lives. The deck view has a Brief link for exactly that hand-off, so nothing on a slide needs to be complete.

### Deck rules

A good deck is not a shorter brief. Apply these when writing `deck`:

1. **One idea per slide.** The section heading `h` states the point as a claim, "Cache cut p99 latency by 40%", never a topic, "Latency".
2. **Slide budget.** A `list` of 3 to 5 items under ten words each, or one `p` of at most two sentences. Never a long paragraph and a list on the same slide. Six lines of body is the ceiling; a slide that scrolls has too much on it.
3. **Deck length.** 5 to 12 slides. More than that is two decks, or material that belongs in the brief.
4. **Title slide.** `summary` is the one sentence the audience should remember. `meta` is date, occasion, and audience. `chips` carry two or three headline numbers with `style: "stat"`.
5. **One highlight per slide.** Bold exactly one phrase, or use one `@chip(stat:...)` for the number that matters. Two bold phrases highlight neither.
6. **Data gets its own slide.** One `chart` block per slide, with the heading saying what the chart shows. A `kv` block for up to four figures. A `table` only when the comparison is the point, at most four columns and five rows.
7. **The graph gets its own slide.** A `{"t": "graph"}` block with nothing but the heading; it is the page's one visual, so let it fill the slide.
8. **Code only when the code is the point.** At most eight lines; otherwise name the file or function in prose.
9. **One callout per deck at most**, `sev: "warn"`, for the single risk or blocker.
10. **No agenda, no "questions?" slide.** Under nine slides an agenda is noise. The last authored slide is the ask: `h: "Next"` with a list of at most three actions. References follow automatically.
11. **Cut the spoken sentences.** If a line only makes sense when said aloud, it is the speaker's, not the slide's. The slide carries the claim; the speaker carries the argument.
12. **Keep the ids.** When the brief uses section `id` badges (A001, RC01), keep them on the matching slides so the room can refer to a finding by id.

### Example deck

An incident review, six slides plus the references the page already carries:

```json
{
  "summary": "A stale DNS cache took checkout down for 41 minutes; the fix is a TTL cap and a health check that would have caught it in 3.",
  "meta": "2026-09-29 · Incident review · Platform and payments",
  "chips": [
    {"text": "41 min outage", "style": "stat"},
    {"text": "3 min to detect after fix", "style": "stat"},
    {"text": "2 actions", "style": "stat"}
  ],
  "sections": [
    {
      "h": "Checkout returned 502s for 41 minutes on Tuesday",
      "id": "I001",
      "blocks": [
        {"t": "list", "items": [
          "14:02 first 502s at the edge",
          "14:09 paged, **checkout only**, other services fine",
          "14:43 resolved after a resolver restart"
        ]}
      ]
    },
    {
      "h": "Error rate by minute",
      "blocks": [
        {"t": "chart", "kind": "area", "title": "Checkout 5xx per minute", "unit": "req",
         "series": [{"name": "5xx", "color": "terracotta", "points": [
           {"x": "14:00", "y": 0}, {"x": "14:05", "y": 310}, {"x": "14:15", "y": 940},
           {"x": "14:30", "y": 890}, {"x": "14:45", "y": 12}, {"x": "15:00", "y": 0}
         ]}]}
      ]
    },
    {
      "h": "The resolver kept a dead upstream for 40 minutes",
      "id": "RC01",
      "blocks": [
        {"t": "p", "text": "The payments gateway rotated its IP; our resolver cached the old record with a **3600 second TTL** and no health check noticed."},
        {"t": "kv", "kv": [{"k": "TTL cached", "v": "3600 s"}, {"k": "Upstream rotation", "v": "no notice"}, {"k": "Health check", "v": "none on DNS"}]}
      ]
    },
    {
      "h": "Where the request died",
      "blocks": [{"t": "graph"}]
    },
    {
      "h": "Two changes close the gap",
      "blocks": [
        {"t": "list", "items": [
          "Cap resolver TTL at 60 s for external upstreams @chip(b:merged)",
          "Synthetic checkout probe every 30 s, pages after 3 failures @chip(a:in review)"
        ]},
        {"t": "callout", "sev": "warn", "text": "The probe pages the payments rota, not platform; confirm the rota is staffed before enabling."}
      ]
    },
    {
      "h": "Next",
      "id": "N001",
      "blocks": [
        {"t": "list", "ordered": true, "items": [
          "Payments approves the probe PR by Thursday",
          "Platform enables the TTL cap in all regions Friday"
        ]}
      ]
    }
  ]
}
```

The brief for the same incident holds the full timeline, the log excerpts, and the alternatives considered; the deck points at it.

### In the room

The reader opens the deck from the brief's Slides link or at `deck_url`, and moves with Right, Space, or PageDown (next), Left, PageUp, or Backspace (previous), Home and End. F or P starts presenting (chrome hidden, one slide filling the window, browser fullscreen when allowed). Pressed in fullscreen it ends, pressed while presenting without fullscreen (after a reload) it asks for fullscreen again; Escape always ends it. The URL's `#3` names the slide, so a link can open on one. An update to the deck reloads the open tab on the same slide, still presenting if it was, so you can edit a deck mid-talk. You can drive the open deck too: `present_deck(id, "start")`, then `"next"`, `"prev"`, `"goto"` with a `slide`, and `"stop"`; every open tab of the deck follows within a second. Open the deck first with `present_open(id, deck: true)`.

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

### Node tones

`tone` tints a node's box. Background, border, and text move together as one color family, in both light and dark mode, so the reader can tell groups apart at a glance (inputs grey, the model's own steps green, a guard red). A tone overrides the type's colors and keeps its other traits (a center node stays bold, a registry node stays dashed).

| Tone | Reads as |
|------|----------|
| `neutral` | Grey; surroundings, inputs, outputs |
| `green` | The system's own steps; healthy |
| `red` | Guards, checks, failures |
| `blue` | External services, storage |
| `amber` | Pending, degraded, manual steps |
| `purple` | Humans, decisions, review |

```json
{"nodes": [
  {"id": "game", "label": "Doom", "tone": "neutral"},
  {"id": "see", "label": "What Laya sees", "tone": "green"},
  {"id": "check", "label": "Safety check", "tone": "red"}
]}
```

Use tones for meaning, not decoration: two or three per graph, the same tone for the same kind of thing. An unknown tone fails the call.

### Edge types

| Type | Visual |
|------|--------|
| (omit) | Solid line |
| `consumes` | Solid line (explicit) |
| `publishes` | Dashed line |

Edges can have an optional `label` string. Keep it to a few words: labels sit on the edge and are not accounted for by the layout.

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

- `dagre` (default): layered DAG layout for trees and hierarchies; accounts for node size and minimizes edge crossings
- `elk`: ELK layered with wrapping, the same kind of layered drawing. But a long chain of steps folds into rows (or columns, top-down) until the drawing fits the container's aspect ratio instead of shrinking into a thin strip. Use it for pipelines and flows with many steps in sequence
- `elk-layered`, `elk-mrtree`, `elk-stress`, `elk-radial`, `elk-force`: the other ELK algorithms, without folding: plain layered, tree, stress-majorization, radial, and force-directed
- `cose`: for general graphs with no clear hierarchy

`dagre` and `elk` take an optional `direction`: `TB` (top-down) or `LR` (left-to-right). Omit it for auto — small graphs (≤8 nodes) draw left-to-right to fill the wide container, larger ones top-down.

The page's graph toolbar has an engine button that cycles the live graph through every engine, so a reader can compare them on any page without re-authoring it. The choice is not saved; set `layout` to keep it.

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

The `references` parameter takes `[{title, url}]` — source links rendered at the bottom of the page by the server. Always include references when the brief draws on external sources. Inline links point at the line or PR a sentence is about; references list the sources the whole page drew on. The same URL can appear in both.

```json
[
  {"title": "dotfiles repo", "url": "https://github.com/mad01/dotfiles"},
  {"title": "PR #42", "url": "https://github.com/mad01/dotfiles/pull/42"}
]
```

## Colors available

Accent names for panel borders and chip references: `terracotta`, `blue`, `green`, `purple`, `amber`, `red`, `yellow`.
