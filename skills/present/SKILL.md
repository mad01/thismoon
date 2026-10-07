---
name: present
description: Build a scrollable briefing page, a slide deck beside it, or both with the present MCP, from structured Doc JSON with fixation reading, one Cytoscape graph, inline charts, and architecture diagrams, for digesting a work summary or research or for talking it through in a room
---

# Present: briefing pages and slide decks

Generate scrollable HTML briefing pages and slide decks served live over localhost by the **present MCP** (Model Context Protocol server), or on a shared instance others reach by link. You pass structured JSON. The server renders it into the full page under a reader-chosen palette (light and dark mode, several theme families picked on the Themes page). The page carries a fixation-reading toggle and font and size controls. The page draws one Cytoscape graph, Chart.js and D3 charts, and diagrams laid out by the Eclipse Layout Kernel (ELK). Never put a colour literal in a page: name a role (see **Colors available**) and the page follows whatever palette the reader picked.

This file is the entry point. It says which block, chart kind, graph option, and deck setting answers which question, points at the worked examples beside it, and names the decisions behind the design. The service's own docs are for changing present, not for using it.

## Trigger

When the user asks to present, summarize, or brief on a topic (e.g., "present the incident summary", "brief me on our infra stack", "summarize the PR changes"). Or when the user asks for slides or a deck on it ("make a deck of this", "slides for the review").

## How it works

- **You pass structured JSON**: a Doc object with sections and typed blocks. The server renders it to HTML with the correct CSS classes, `data-fixation` attributes, and structure. You never write HTML.
- **The MCP stores and serves it.** `present_create` returns an `id` and a `url`. Keep the id for later edits.
- **Updates auto-reload.** `present_update` bumps the version; open browser tabs refresh. Call `present_open` **once**, never again for the same page.
- **A page carries a brief, a deck, or both** under one id: the brief is the `content` Doc, the deck a second Doc passed as `deck`, one section per slide. See **Slide decks**.

### Worked examples

Three complete pages sit beside this file in `examples/`. Each file is the argument object of one `present_create` call (`title`, `content`, `deck`, `graph`, `references`), so you can read it and pass it through; the tool takes `content`, `deck`, and `graph` as JSON strings. On a machine where they were created, `present_list` finds the same pages by title, and `present_open` shows them.

| File | Title | Shows |
|------|-------|-------|
| [stepped-charts.json](examples/stepped-charts.json) | Stepped charts in present | a stacked bar, a line, a sankey, and a ribbon that each build up one step per Next, a stepped diagram, toned sections, a `statement` opener, `transition: "slide"` |
| [diagram-block.json](examples/diagram-block.json) | Diagram block test: boxes, boundaries, arrows | a system context, two levels of groups, a top-to-bottom pipeline, a request walkthrough with `focus`, a framed chart beside a frameless one, a borderless image, a `section` divider |
| [slides-without-boxes.json](examples/slides-without-boxes.json) | Slides without boxes | every slide layout, `section` dividers with tones, a row of stats, a lone stat, a lone quote, a chart beside its caption in `columns` |

The repo's `services/present/internal/render/testdata/sample-deck.json` is a fourth: a deck that sets every layout, tone, notes, reveal, block, and chrome field once, kept compiling by a test.

### Where the decisions are recorded

Three architecture decision records (ADRs) explain why the format is shaped the way it is. Each format section below names the ones that apply, and the deeper reasons live there, not here.

- [ADR-0019][adr-0019]: a page carries a deck beside its brief. The deck is a second rendition under the same id, authored on its own and never converted from the brief. The two share the title, the one graph, the references, and the share link.
- [ADR-0020][adr-0020]: slides carry layout, notes, and reveal; the deck carries its chrome; the Doc gained the blocks both renditions share. Amended for the image block, chart steps, the diagram block, and frameless slides.
- [ADR-0022][adr-0022]: the browser libraries (Cytoscape, Chart.js, D3, ELK, the fonts) are fetched at install and pinned by version. A page loads offline without graphs, charts, or diagrams, and a missing library shows a note in the block's place.
- The service's [CLAUDE.md][svc-claude] holds the renderer's gotchas: what each validator refuses, how the deck view splits a page into slides, how charts and diagrams build in the browser. The markdown import mapping is there too. Read it when a call is refused and the message is not enough.
- The writing rules follow Stanford's communication teaching. The reader-and-ask step and the named orders come from Matt Abrahams ([three guiding principles][gsb-principles], [the Think Faster, Talk Smarter masterclass][gsb-masterclass]). The bottom line up front comes from [Abrahams and Kramon, Writing to Win][gsb-writing]. The number comparison comes from [Heath and Abrahams, Make Numbers Count][gsb-numbers]. The claim heading, the one idea per slide, and the chart cut to its message come from the Stanford Engineering [Technical Communication Program's visual aids notes][stanford-visual-aids].

## Start here

1. **Gather content.** Research the topic with code search, git log, PR data, or whatever sources apply. Every number and claim on the page comes from a real source.
2. **Name the reader and the ask.** One line each: who will read or hear it, what they should know afterwards, and what they should do. The `summary` states the know; the brief's first section or the deck's Next slide states the do. Anything that serves neither is cut.
3. **Pick the shape.** A brief for a reader alone, a deck for a room, both when the room will want the detail afterwards. See **Brief or deck**.
4. **Pick the visuals.** For each set of facts, find the block that answers its question in **Which block answers which question**. Numbers over categories or time are a `chart`; how parts fit is a `diagram`; a map of many nodes is the page's one `graph`.
5. **Build the Doc** (and the Graph when the data is a map). Use the formats below.
6. **Create and open.** `present_create` with `title`, `content`, optional `deck`, `graph`, and `references`. Hold the returned `id`. `present_open` once.
7. **Iterate.** `present_update` with the same id and only the fields that changed. The open tab reloads.

To edit a page created in an earlier session: `present_list` to find it (`has_doc: true` means the source is available), `present_source` to get the Doc, deck, and graph JSON, modify, then `present_update`.

## Type contract

The shapes at a glance; full field tables sit in the format sections below.

```
Doc = {                                  // the `content` argument, and the `deck` argument
  summary: string,                       // the one line a reader acts on (the deck's title-slide sentence)
  meta: string,                          // date · category · key stats
  chips: [{text: string, style: "stat" | "a" | "b" | "c" | "outline"}],
  sections: Section[],
  // deck only, ignored by the brief:
  logo?, logo_position?, progress?, presenter?, footer?, transition?
}

Section = {                              // one slide in a deck
  h: string,                             // heading (TOC anchor; on a slide, the claim)
  blocks: Block[],
  tone?: string,                         // a palette role: a band in the brief, the accent on a slide
  layout?: "default" | "center" | "statement" | "section",   // deck only
  notes?: string,                        // deck only: speaker notes
  reveal?: boolean                       // deck only: items appear one per Next
}

Block =                                  // discriminated on `t`
  | {t: "p", text}                       | {t: "h3", text}
  | {t: "callout", text, sev?}           | {t: "table", cols, rows}
  | {t: "kv", kv}                        | {t: "list", items, ordered?}
  | {t: "panel", title, sub?, accent?}   | {t: "progress", pct, label?}
  | {t: "graph"}                         | {t: "chart", kind, series | flows, title?, unit?, xunit?, steps?, order?, frame?}
  | {t: "code", text, lang?}             | {t: "html", text}
  | {t: "columns", cols}                 | {t: "stat", value, label, sub?}
  | {t: "quote", text, cite?}            | {t: "details", summary, blocks}
  | {t: "image", src, alt, caption?, frame?} | {t: "diagram", nodes, edges?, groups?, direction?, caption?, steps?}

Graph = {                                // the `graph` argument, one per page, shared by brief and deck
  nodes: [{id, label, type?: "center" | "module" | "leaf" | "registry", color?: 0..3,
           tone?: "neutral" | "green" | "red" | "blue" | "amber" | "purple"}],
  edges: [{from, to, type?: "consumes" | "publishes", label?, weight?, flow?}],
  layout: "dagre" | "elk" | "elk-layered" | "elk-mrtree" | "elk-stress" | "elk-radial" | "elk-force" | "cose",
  direction?: "TB" | "LR"
}

PageRef = {id, url, has_deck, deck_url?, version}   // present_create return; hold `id` all session
```

## Which block answers which question

Pick by the question the facts answer, not by what looks rich. The right column is the block that pattern-matches but reads worse.

| The facts are | Use | Not |
|---------------|-----|-----|
| the one thing the reader should do or know | `summary`, then a `p` | a background section first |
| one number the reader should keep | `stat`, alone | a `kv` with one row, a number buried in prose |
| two to four labelled figures | `kv` in a brief; two or three `stat` blocks in `columns` on a slide | a table |
| a comparison where the grid itself is the point | `table` (on a slide at most four columns and five rows) | nested lists |
| numbers over categories or over time | `chart`, see **Which kind** | a table of numbers |
| how a few parts fit: boxes, boundaries, arrows | `diagram` | the graph, a bulleted architecture |
| items that cause each other, branch, or nest: a cycle, a request path, a hierarchy | `diagram` | a bulleted list that hides the arrows |
| a map of many nodes and their links | the page's one `graph` | a diagram past twelve boxes |
| parallel items, or a plain sequence with no branching | `list` (`ordered: true` when the order matters) | a paragraph with "first, then, then" |
| the one risk or blocker | one `callout` with `sev: "warn"` | several callouts |
| a line someone said | `quote` with `cite` | italics in a `p` |
| long raw material: a timeline, logs, every number | `details`, closed by default, in the brief | a slide, a long `p` |
| two things read side by side: a chart and its caption | `columns` | two sections |
| an exact command or code | `code` with `lang` | backticks in a `p`, `html` with a `<pre>` |
| a screenshot or a figure | `image` with an `alt` a listener can use | `html` with an `<img>` |
| a titled card with an accent | `panel`, in a brief | a slide (deck rule 17) |
| work in flight | `progress` plus `@chip(b:done)` chips | a sentence with a percentage |

### Brief or deck

A **brief** is read alone and scrolled. It holds the detail, the timeline, the alternatives, the raw numbers in `details`, and a table of contents when there are two or more sections. A **deck** is talked through in a room, one slide at a time. It says less on purpose, one idea per slide, and points at the brief for everything else. When the material will be read and talked through, make both under one id: the deck view has a Brief link and the brief a Slides link. Never make a second page for the deck of an existing brief; pass `deck` to `present_update` on the same id ([ADR-0019][adr-0019]).

The brief's first section answers the question. How you found it, the background, and the history come after it or go in `details`. Write `summary` first, then cut any section that does not support it: tell the reader the time, not how the clock was built.

### Graph, diagram, or chart

- The **graph** is the page's one map: many nodes, their links, laid out by an engine you choose. Use it when the shape of the network is the point (a dependency tree, a traffic map with flow edges, a module map). One per page, shared by the brief and the deck.
- A **diagram** is the picture a slide explains. It has a few boxes with a name and a one-line text, labelled boundaries around what shares a host or a trust level, and labelled arrows. Steps can walk a request through it. As many per page as the story needs. Keep each to about twelve boxes.
- A **chart** is numbers: over categories, over time, as shares, as flows of volume. Never draw structure with a chart or numbers with a graph.

### Stat, kv, or table

- One figure the reader should remember: `stat`. On a slide it is the big-number slide when alone, a row of figures when two or three sit in `columns`.
- Two to four labelled figures that belong together (TTL, timeout, retries): `kv` in a brief. On a slide, stats in a row read better than a kv.
- A grid where the reader compares across both axes (three options against four criteria): `table`. Read-aloud skips tables and kv blocks, so when the page will be listened to, say the comparison in a list with one full sentence per item.
- Give the one number a comparison the reader already knows, in the `stat`'s `sub` line or the next sentence. A ratio for a percentage ("2 of every 5 checkouts", not "40%"), a familiar unit for a big count ("a full day of Tuesday's traffic"). A number without a comparison is read once and lost.

### Columns or details

- `columns` puts two or three things side by side at one glance. That is a chart and the paragraph that says what to look at, the graph and one sentence, three stats, or an image and its explanation. One column under 700 px wide.
- `details` hides what most readers skip and some need: a full timeline, the log lines, the raw table. Closed by default, read aloud when reached. Brief only: a slide that needs one has too much on it.
- Neither holds the other, and `details` never holds the graph or a stepped chart.

## MCP tools

| Tool | Use |
|------|-----|
| `present_create(title, content?, deck?, graph?, references?)` | Create a page. `content` is a Doc JSON object (the brief). `deck` is a second Doc JSON object (the slides). At least one of the two. `graph` is a Graph JSON object. `references` is `[{title, url}]`. Returns `deck_url` when there is a deck. |
| `present_source(id)` | Get the editable source: Doc JSON (`content_format: "doc"`), the deck's Doc JSON (`deck`, empty without one), and graph JSON (`graph_format: "json"`), ready to modify and pass back to `present_update`. **Use this to edit a page from a new session.** Legacy pages return raw HTML/JS instead. |
| `present_read(id)` | Fetch the rendered page (HTML, plus the deck's HTML). For editing, prefer `present_source`. |
| `present_update(id, title?, content?, deck?, graph?, references?)` | Patch a page. Omit fields to leave unchanged. Pass `graph: ""` to remove a graph, `deck: ""` to remove the deck, `references: []` to clear refs. A page keeps at least one of content and deck. |
| `present_list()` | List all pages (id, title, url, version, updated, has_doc, has_brief, has_deck, deck_url). `has_doc: true` means the page is source-editable via `present_source`. |
| `present_open(id, deck?)` | Open in browser, **once per page**. `deck: true` opens the slide deck instead of the brief. |
| `present_deck(id, action, slide?)` | Drive the deck that is open in the browser: `start` or `stop` presenting, `next`, `prev`, `goto` a 1-based `slide`. Local instance only. |
| `present_share(id, ephemeral?)` | Push the page to the shared instance this machine is configured for and return `{url, ephemeral, expires_at, shared_at}`. Only registered when a shared instance is configured; sharing again replaces the copy under the same link, `ephemeral` makes it expire 30 days after the last share. |
| `present_doctor()` | Check the page store, the serve process, and version skew. Call it when a tool errors or a page URL does not load. |

> **Workflow rule:** `present_create` returns `{id, url, version}`. The `id` is
> required for every subsequent call (`present_update`, `present_open`,
> `present_source`, `present_read`). Hold it for the duration of the session.
> If `present_open` fails (sandbox or no `open` binary), the URL from
> `present_create` is the direct link; return it to the user instead.
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
        {"t": "image", "src": "~/Desktop/dashboard.png", "alt": "The on-call dashboard at 14:15, every checkout panel red", "caption": "The dashboard at 14:15."},
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
| `sections` | array | Content sections with headings and blocks |

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
| `callout` | `text`, `sev?` | Highlighted callout. Severity: `info` (blue border), `warn` (amber border), `ok` (green border), `error` (red border), or omit for default |
| `table` | `cols`, `rows` | Data table. Cells support inline markdown |
| `kv` | `kv: [{k, v}]` | Key-value pairs. Values support inline markdown |
| `list` | `items`, `ordered?` | Bulleted or numbered list. Items support inline markdown |
| `panel` | `title`, `sub?`, `accent?` | Titled card. Accent is a palette role name (`primary`, `blue`, `green`, `purple`, `amber`, `red`, `yellow`, `series-1` to `series-4`; `terracotta` still works as an alias of `primary`). An unknown name is refused |
| `progress` | `pct`, `label?` | Progress bar (0-100) |
| `graph` | (none) | Placement marker for the Cytoscape graph container |
| `chart` | `kind`, `series` or `flows`, `title?`, `unit?`, `xunit?`, `steps?`, `order?`, `frame?` | Metric chart (Chart.js, or D3 for `ribbon`). `kind` is one of `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`, `ribbon`. Inline: use as many as you like per page. `steps` makes a deck slide walk the chart one step per Next. See **Chart format** |
| `diagram` | `nodes`, `edges?`, `groups?`, `direction?`, `caption?`, `steps?` | Architecture picture laid out in the browser by ELK and drawn as SVG (D3): boxes with a name and a short text, labelled boundaries, labelled arrows. A node `kind` is one of `service`, `store`, `queue`, `person`, `external`; `direction` is one of `LR`, `TB`. Many per page, never the page graph. `steps` walks it one step per Next on a deck slide. See **Diagram format** |
| `code` | `text`, `lang?` | Fenced code block with language badge and copy button. `text` is verbatim code (NO inline markdown: backticks, `**`, `<` all render literally). `lang` sets the badge and syntax highlighting: `go`, `bash`, `json`, `python`, `typescript`, `yaml`, `sql` highlight; anything else (or omitted) renders plain with a `text` badge |
| `html` | `text` | Raw HTML passthrough for one-off custom content |
| `columns` | `cols: [[blocks], [blocks]]` | Two or three equal-width columns of blocks, one column under 700 px. A column holds any block but `columns` and `details`. Three `stat` blocks in it make a row of figures; a `chart` beside a `p` puts the caption next to the chart. The page's one `graph` may sit in a column too, beside the paragraph that says what to look at |
| `stat` | `value`, `label`, `sub?` | A large figure over a label, in a tile in the brief and plain on a slide. `value` is shown verbatim (no inline markdown) and stays out of fixation; `label` and `sub` take inline markdown |
| `quote` | `text`, `cite?` | A quotation with a left rule and the attribution under it |
| `details` | `summary`, `blocks` | A collapsible block, closed by default, with `summary` as the clickable line. Holds any block but `graph`, `columns`, and `details`. Read-aloud reads it and opens it while a part inside plays. The home for a long timeline or raw numbers |
| `image` | `src`, `alt`, `caption?`, `frame?` (`false` drops the border) | An image with a caption under it. `src` is an `http(s)` URL, or on this machine an absolute or `~` path to a png, jpeg, gif, or webp file of at most 2 MiB. The tool copies the file into the page store and rewrites `src` to the `/img/<hash>.<ext>` path it is served at, which is what `present_source` returns. A relative path is refused. `alt` is required: read-aloud reads it in the image's place, and the caption after it. Goes inside `columns` and `details`. A shared instance takes image URLs only, and a page with stored images can't be shared until they are URLs |

### Layout blocks

One example of each, as they go in a section's `blocks`:

```json
[
  {"t": "columns", "cols": [
    [{"t": "stat", "value": "41 min", "label": "checkout outage", "sub": "Tuesday 14:02 to 14:43"}],
    [{"t": "stat", "value": "3 min", "label": "to detect after the fix"}],
    [{"t": "stat", "value": "2", "label": "actions"}]
  ]},
  {"t": "columns", "cols": [
    [{"t": "chart", "kind": "area", "title": "Checkout 5xx per minute", "series": [{"points": [{"x": "14:00", "y": 0}, {"x": "14:15", "y": 940}]}]}],
    [{"t": "p", "text": "The spike starts eleven minutes before the first page."}]
  ]},
  {"t": "columns", "cols": [
    [{"t": "graph"}],
    [{"t": "p", "text": "The resolver sits between the gateway and payments; every request after 14:02 followed its stale record."}]
  ]},
  {"t": "columns", "cols": [
    [{"t": "image", "src": "~/Desktop/dashboard.png", "alt": "The on-call dashboard at 14:15, every checkout panel red", "caption": "The dashboard at 14:15."}],
    [{"t": "p", "text": "Every checkout panel went red at once while the gateway's own panels stayed green."}]
  ]},
  {"t": "stat", "value": "41 min", "label": "checkout outage", "sub": "Tuesday 14:02 to 14:43"},
  {"t": "quote", "text": "We never saw the resolver because nothing watched it.", "cite": "On-call engineer, retrospective"},
  {"t": "details", "summary": "Full timeline, 14:00 to 15:00", "blocks": [
    {"t": "list", "items": ["14:02 first 502s at the edge", "14:09 paged", "14:43 resolved"]}
  ]},
  {"t": "callout", "sev": "ok", "text": "The TTL cap is live in every region."}
]
```

### Section fields

| Field | Type | Description |
|-------|------|-------------|
| `h` | string | Section heading (used for TOC anchor) |
| `blocks` | array | Content blocks |
| `tone` | string? | A palette role name (see **Colors available**). In a brief it gives the section a band in that colour; on a slide it colours the heading rule and the accent bar (on a `statement` slide a centred bar under the heading), never a filled background. Pick a saturated role (`blue`, `green`, `amber`, `red`, `purple`, `primary`): a `-bg` role is nearly invisible as a rule and bar. An unknown name is refused |
| `layout` | string? | Deck only, ignored by the brief: `default`, `center` (centred at today's sizes), `statement` (the heading is the slide, large and centred, blocks as a line under it), `section` (a divider: a large centred heading with a short accent bar in the tone). A slide whose only block is a `stat` or a `quote` is the big-number or quote slide with no layout set |
| `notes` | string? | Deck only: speaker notes with inline markdown. Never on the slide, never read aloud; the deck shows them in a drawer on the N key or the Notes button |
| `reveal` | bool? | Deck only: the slide's list items and top-level blocks appear one per Next, Prev hides the last one, a jump lands with all shown. The progress dots and the URL hash track slides, not steps |

### Notes

- **The table of contents (TOC) is auto-generated** from section headings when there are 2+ sections. Do not write TOC markup.
- **`data-fixation`** is applied automatically to the title, summary, table of contents, section headings and subheadings, paragraphs, callouts, table bodies, stat labels and sub lines, quote text, details summaries, and image captions. The page shell adds kv values, list items, and panel titles by element. Chips, table headers, the meta line, a stat's value, and an image stay out of the fixation walk.
- **Unknown block types** produce an HTML comment error; they don't break the page.
- **Use `code` blocks for multi-line code**, not `p` with backticks (inline `code` is for short identifiers) and not `html` with a hand-written `<pre>`.
- **Read-aloud skips tables, kv blocks, and code blocks.** A `stat` reads as its value, label, and sub line; a `details` block reads like prose and opens while a part inside it plays. An `image` reads as its `alt` and then its caption, so write the `alt` as the sentence a listener needs in place of the picture. When the reader will listen to the page, put a comparison in a list with one full sentence per item instead of a table. Name a command in words in the prose and put the exact invocation in a code block beside it: an inline code span mid-sentence reads badly aloud.
- **Inline markup nests.** A code span, a link, or a chip inside `**bold**` renders as expected.

### Presenting multiple items

When presenting N related items (review findings, bugs, options, comparison points), **always put them in one page as N sections; never create one page per item.**

```json
{
  "summary": "3 issues found during assessment.",
  "meta": "2026-06-05 · Code Review · 3 findings",
  "chips": [{"text": "3 items", "style": "stat"}],
  "sections": [
    {
      "h": "Race condition in poll loop",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    },
    {
      "h": "Missing error propagation",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    },
    {
      "h": "Stale cache after leader change",
      "blocks": [{"t": "p", "text": "Description of the issue..."}]
    }
  ]
}
```

Use this pattern for: review findings, bug lists, options comparison, enumerable sets of any kind.

## ADHD briefing mode (opt-in)

**Off by default.** Build briefings normally unless the reader asks for this mode. Triggers: "adhd mode", "adhd briefing", "shape it for adhd", or the `I Have ADHD` output style is active. When none of those apply, ignore this section entirely.

When on, shape the Doc so an ADHD reader can act on it. This changes structure and ordering only. It never changes the facts, and it stays inside the page (it does not affect chat replies, commits, or docs):

1. **Lead with actions.** Make the `summary` the single most important next action in concrete terms (a command, a path, a decision), not background. Put context lower.
2. **First section is "Do now."** Give it a `list` block of bounded, numbered steps, each one action, no "and then" twice.
3. **Cap every list at 5 items.** If more, split into a "Now" section and a "Later" section rather than one long list.
4. **Make wins visible.** Use `@chip(b:done)` chips and a `progress` block for anything completed or in-flight, so finished work is not buried in prose.
5. **One callout, not many.** If there is a single blocker or risk, use one `callout` with `sev: "warn"`. Do not scatter warnings.
6. **End with the next action.** The last section names ONE concrete thing to do next, doable in under two minutes.
7. **Matter-of-fact tone.** State cause and fix for errors; no "uh oh" framing.

Skip preamble sections ("about this brief", "overview of overview"). Every section earns its place by being actionable or a visible win.

## Slide decks (the `deck` argument)

A deck is a second Doc under the same page id, shown one section at a time at `deck_url` (`/p/<id>/deck`). It uses the Doc schema above unchanged: `summary`, `meta`, and `chips` make the title slide, every entry in `sections` is one slide, and the page's `references` make the last slide. The page's one `graph` is shared with the brief, so a `{"t": "graph"}` block in a deck section shows the same graph on that slide. Pass `deck` to `present_create` beside `content`, or alone for a deck-only page, and to `present_update` to replace it (`deck: ""` removes it). The deck is never generated from the brief: you write it, and it says less ([ADR-0019][adr-0019], [ADR-0020][adr-0020]).

Worked examples: [slides-without-boxes.json](examples/slides-without-boxes.json) for the layouts and tones, [stepped-charts.json](examples/stepped-charts.json) for data slides that build up.

### When to make one

Make a deck when the material will be talked through: a review in a meeting, an incident retrospective, a decision the room has to make. A brief someone reads alone stays a brief. When both exist, the deck is the version for the room and the brief is where the detail lives. The deck view has a Brief link for exactly that hand-off, so nothing on a slide needs to be complete.

### Building a deck

The order that produces a deck rather than a shortened brief:

1. **Write the title slide last, but decide it first.** `summary` is the one sentence the room should remember. `meta` is the date, the occasion, and the audience. `chips` carry two or three headline numbers with `style: "stat"`. `presenter` is the byline, and the only place for a date beside `meta`.
2. **One claim per slide.** List the claims the room must accept, in the order the argument runs. Order them by one named structure and keep it. Problem, Solution, Benefit for a proposal. What, So What, Now What for a finding or a status. Cause, Effect, Solution for an incident. Comparison, Contrast, Conclusion for options. Each claim becomes a section whose `h` states it, "Cache cut p99 latency by 40%", never a topic, "Latency". Under it, a `list` of 3 to 5 items under ten words each or one `p` of at most two sentences. What you would say goes in `notes`.
3. **Give every number its own slide.** A `chart` whose heading states what the chart proves and whose `title` names the measure, a `diagram` when the slide explains how parts fit, the `graph` when the map is the point. Alone on the slide, or in `columns` beside one short `p`. When the picture should form while you talk, give it `steps`.
4. **Keep one figure and one line.** The number the room should carry out of the door is a lone `stat`; the sentence is a lone `quote` or a `statement` slide. Once or twice a deck.
5. **End on the ask.** The last authored slide is `h: "Next"` with an ordered list of at most three actions, each with an owner and a date. References follow by themselves.
6. **Cut.** Five to twelve slides. A longer deck opens its parts with `section` dividers, or is two decks. Everything cut goes in the brief.

### Deck rules

A good deck is not a shorter brief. Apply these when writing `deck`:

1. **One idea per slide.** The section heading `h` states the point as a claim, "Cache cut p99 latency by 40%", never a topic, "Latency". A data slide is no exception. Its heading is what the chart proves, "Errors held near 900 a minute for half an hour"; the chart's `title` names the measure, "Checkout 5xx per minute".
2. **Slide budget.** A `list` of 3 to 5 items under ten words each, or one `p` of at most two sentences. Never a long paragraph and a list on the same slide. Six lines of body is the ceiling; a slide that scrolls has too much on it.
3. **Deck length.** 5 to 12 slides. More than that is two decks, or material that belongs in the brief.
4. **Title slide.** `summary` is the one sentence the audience should remember. `meta` is date, occasion, and audience. `chips` carry two or three headline numbers with `style: "stat"`.
5. **One highlight per slide.** Bold exactly one phrase, or use one `@chip(stat:...)` for the number that matters. Two bold phrases highlight neither.
6. **Data gets its own slide.** One `chart` block per slide, its heading the claim the chart proves and its `title` the measure. Draw only the series the claim needs and fold or cut the rest. The eye then finds the comparison the heading names. A `kv` block for up to four figures. A `table` only when the comparison is the point, at most four columns and five rows.
7. **The graph gets its own slide.** A `{"t": "graph"}` block with nothing but the heading; it is the page's one visual, so let it fill the slide. When the room needs one sentence pointing at a node, put the graph in a `columns` block beside one short `p` and nothing else.
8. **Code only when the code is the point.** At most eight lines; otherwise name the file or function in prose.
9. **One callout per deck at most**, `sev: "warn"`, for the single risk or blocker.
10. **No agenda, no "questions?" slide.** Under nine slides an agenda is noise. The last authored slide is the ask: `h: "Next"` with a list of at most three actions. References follow automatically.
11. **Cut the spoken sentences.** If a line only makes sense when said aloud, it is the speaker's, not the slide's. The slide carries the claim; the speaker carries the argument. No sentence appears both on the slide and in `notes`.
12. **Which block when.** One `stat` or one `quote` alone on a slide for the one figure or the one line the room should keep, with a comparison the room already knows in the `sub` line ("2 of every 5 checkouts"). Two or three `stat` blocks in a `columns` block for a row of figures, and a `chart`, the `graph`, a `diagram`, or an `image` beside its caption paragraph in `columns`. An image alone on a slide fills it, bounded by the slide's height. A `details` block belongs in the brief: a slide that needs one has too much on it.
13. **Which layout when.** `statement` for the one sentence the deck exists to say, once or twice a deck. `section` to open a part of a longer deck, with a `tone`. `center` for a slide that is one short thing, a row of figures say. Everything else stays `default`; the heading and the blocks carry the slide.
14. **Notes carry the argument, reveal carries the pace.** Put what you would say, and only that, in `notes`. Set `reveal` on a list that is an argument built one line at a time, never on a list the room should read whole.
15. **A chart that builds up gets steps.** Give a `chart` `steps` when the room should watch the picture form: one caption per step, and `step` on each series for the step it joins at. Next walks the steps before leaving the slide, on a `reveal` slide right after the chart appears. A jump lands on the finished chart, and the brief lists the captions under it. Three to five steps, each caption one short sentence. A `ribbon` steps by period, one caption per column and no series `step`, so keep a stepped ribbon to three to five periods. See **Stepped charts**.
16. **An architecture gets a diagram, and a diagram alone fills its slide.** Use a `diagram` block, not the graph, when the slide explains how parts fit. That is boxes with a name and a short text, boundaries around the parts that share a host or a trust level, and labelled arrows. Alone on a slide it fills the slide with its caption under it. A request walkthrough gets `steps`: four or five, each caption one sentence, with `focus` on the box the step is about. At most about twelve boxes and two or three tones per slide; split a bigger picture over slides. See **Diagram format**.
17. **A slide draws no filled boxes.** The slide is already the container, so a box inside it reads as card UI, and two of them nest. `panel`, `details`, `code`, and a chart with `frame: true` draw a card, so keep them in the brief; the one warn callout of rule 9 is the only box a deck keeps. `tone` colours the heading rule and accent bar, a `section` divider is a heading with an accent bar, not a band, and a `stat` sits plain, alone or in a row.

### What each setting is for

| Setting | For | Not for |
|---------|-----|---------|
| `layout: "default"` | a heading over a list, a paragraph, a chart, a diagram: most slides | |
| `layout: "center"` | one short thing centred: a row of figures, a single line | a list, a chart |
| `layout: "statement"` | the one sentence the deck exists to say, the heading as the slide, once or twice a deck | a slide with body under it beyond one line |
| `layout: "section"` | a divider opening a part of a longer deck, with a `tone` | a deck under nine slides |
| `tone` | colouring a part's heading rule and accent bar so the room sees where it is; a saturated role, the same one for the whole part | decoration on every slide, a `-bg` role |
| `reveal: true` | a list that is an argument built one line at a time | a list the room should read whole, a slide with one block |
| chart or diagram `steps` | a picture that should form while you talk, three to five captions | a chart the room reads at a glance |
| `notes` | what you would say and only that; N opens the drawer, nothing reads it aloud | content the room must read |
| `transition` | `fade` (the default) for most decks; `slide` when the deck reads as a path through material; `none` when the room or the reader wants cuts | |
| `logo`, `logo_position` | a mark in the corner: the repo logo by default, an http(s) URL for another, `"none"` for a bare room | |
| `progress` | `dots` (the default) to 24 slides, where they give way to a `bar`; `"none"` when the count would distract | |
| `presenter`, `footer` | the byline under the meta line and the bottom strip; the footer defaults to the deck title once either is set | |
| `present_deck` | driving the open deck from the shell or the MCP: `start`, `next`, `prev`, `goto`, `stop`; every open tab follows within a second | a shared instance, which has no remote control |
| Reduce Motion | the reader's operating-system setting: every transition, reveal, draw-in, step, and flow animation becomes a cut or a static frame. Nothing to set; write the deck so it reads without motion | |

### Deck-level fields

Six optional fields sit at the top of the deck Doc beside `summary`, `meta`, and `chips`. They are the deck view's chrome rather than slide content, so the brief ignores them. A deck that sets none of them shows the repo logo in the bottom-right corner and one dot per slide under the bar, with no footer line.

| Field | Default | Values |
|-------|---------|--------|
| `logo` | shown (the embedded repo logo) | `"none"` hides it; an `http(s)` URL replaces it |
| `logo_position` | `bottom-right` | `bottom-left`, `top-left`, `top-right` |
| `progress` | `dots` | `bar`, `none`. One dot per slide, done ones filled, the current one ringed, each a button that jumps there. Above 24 slides the dots give way to a thin bar |
| `presenter` | none | Free text: a byline under the meta line on the title slide and in the footer. Put the date in it when wanted; there is no date field |
| `footer` | the deck title | Text on the left of the bottom strip; `"none"` suppresses it. The footer line appears only when `presenter` or `footer` is set |
| `transition` | `fade` | How the view moves between slides: `fade` (a 200 ms crossfade), `slide` (the crossfade with a 24 px nudge in the direction of travel), `none` (a cut). Reveal steps, the chart draw-in, and the graph fade follow it; Reduce Motion makes every change a cut |

```json
{
  "summary": "...",
  "presenter": "Alex, platform · 2026-10-01",
  "footer": "Incident review, checkout 502s",
  "progress": "bar",
  "sections": [ ... ]
}
```

### Example deck

An incident review, seven slides plus the references the page already carries, with a presenter byline. `services/present/internal/render/testdata/sample-deck.json` in the repo is a longer one that uses every layout, tone, notes, reveal, block, and chrome field:

```json
{
  "summary": "A stale DNS cache took checkout down for 41 minutes; the fix is a TTL cap and a health check that would have caught it in 3.",
  "meta": "2026-09-29 · Incident review · Platform and payments",
  "presenter": "Alex, platform",
  "chips": [
    {"text": "41 min outage", "style": "stat"},
    {"text": "3 min to detect after fix", "style": "stat"},
    {"text": "2 actions", "style": "stat"}
  ],
  "sections": [
    {
      "h": "Checkout returned 502s for 41 minutes on Tuesday",
      "reveal": true,
      "notes": "Pause after the first line. Ask who was on call before showing the rest.",
      "blocks": [
        {"t": "list", "items": [
          "14:02 first 502s at the edge",
          "14:09 paged, **checkout only**, other services fine",
          "14:43 resolved after a resolver restart"
        ]}
      ]
    },
    {
      "h": "Checkout was down for 41 minutes",
      "blocks": [
        {"t": "stat", "value": "41 min", "label": "checkout outage", "sub": "Tuesday 14:02 to 14:43, about 2 of every 5 lunchtime orders lost"}
      ]
    },
    {
      "h": "Errors held near 900 a minute for half an hour",
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
      "blocks": [
        {"t": "p", "text": "The payments gateway rotated its IP; our resolver cached the old record with a **3600 second TTL** and no health check noticed."},
        {"t": "kv", "kv": [{"k": "TTL cached", "v": "3600 s"}, {"k": "Upstream rotation", "v": "no notice"}, {"k": "Health check", "v": "none on DNS"}]}
      ]
    },
    {
      "h": "Every request after 14:02 followed the resolver's stale record",
      "blocks": [{"t": "graph"}]
    },
    {
      "h": "Two changes close the gap",
      "tone": "green",
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

The reader opens the deck from the brief's Slides link or at `deck_url`, and moves with Right, Space, or PageDown (next), Left, PageUp, or Backspace (previous), Home and End. On a `reveal` slide Next shows the next step and Prev hides the last one; Home, End, a dot, or a link lands with every step shown. N opens the speaker notes drawer. F or P starts presenting (chrome hidden, one slide filling the window, browser fullscreen when allowed). Pressed in fullscreen it ends, pressed while presenting without fullscreen (after a reload) it asks for fullscreen again; Escape always ends it. The URL's `#3` names the slide, so a link can open on one. An update to the deck reloads the open tab on the same slide, still presenting if it was, so you can edit a deck mid-talk. You can drive the open deck too: `present_deck(id, "start")`, then `"next"`, `"prev"`, `"goto"` with a `slide`, and `"stop"`; every open tab of the deck follows within a second. Open the deck first with `present_open(id, deck: true)`. The header's Audio toggle hides the read-aloud bar, play buttons, and badges. A deck opens with them hidden, so the room never sees them unless the reader turns them on; a brief keeps its own choice.

## Graph format (the `graph` argument)

Pass a structured object. The server stores the nodes and edges; the page styles and lays them out when it loads, so a style change reaches existing pages without a rerender. One graph per page, shared by the brief and the deck. The engines, the palette, and the flow animation are built in the page shell. That is why the stored graph carries no colours and no coordinates (the service's [CLAUDE.md][svc-claude], under *Shared UI: webkit*). The ELK engines need the vendored library ([ADR-0022][adr-0022]); without it the graph falls back to the Cytoscape built-ins.

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

### Which engine

Pick `layout` by the shape of the data:

- **A tree or a hierarchy** (a dependency tree, a call graph, an org of services): `dagre`, the default. Layered, accounts for node size, minimises crossings. Fine to about thirty nodes.
- **A long chain of steps** (a pipeline, an import flow, a release process): `elk`. ELK layered with wrapping, so a chain of twelve steps folds into rows (or columns, top-down) that fit the container instead of shrinking into a strip.
- **The same layered drawing without folding**, when the chain should stay one row however long: `elk-layered`.
- **One root fanning out** (a tree with many leaves): `elk-mrtree`, which spaces a wide fan-out better than dagre.
- **One centre with rings around it** (a hub and its callers): `elk-radial`.
- **No hierarchy, distances should mean something** (a service mesh, a cluster of related modules): `elk-stress`. `elk-force` is the organic variant; `cose` is the Cytoscape built-in that needs no vendored library.
- **A map with cycles** (a mesh with traffic both ways): `elk` or `elk-stress`; a layered engine draws a back-edge, which is acceptable for one or two.

`dagre` and `elk` take an optional `direction`: `TB` (top-down) or `LR` (left-to-right). Omit it for auto: small graphs (8 nodes or fewer) draw left-to-right to fill the wide container, larger ones top-down. A pipeline reads left to right; a hierarchy reads top-down.

The page's graph toolbar has an engine button that cycles the live graph through every engine, so a reader can compare them on any page without re-authoring it. The choice is not saved; set `layout` to keep it.

### Node types

| Type | Visual |
|------|--------|
| `center` | Highlighted root (accent border, bold label) |
| `module` | Coloured border from the palette. Set `color` (0-3) to pick one of four slots |
| `leaf` | Plain node (default if type omitted) |
| `registry` | Dashed border |

Use `module` with `color` to group nodes by subsystem the way a Mermaid `subgraph` would: every node of one subsystem gets the same slot, four slots in all. A tone (below) says what a node *is*; a module colour says which *group* it belongs to. Use one or the other on a graph, not both.

### Node tones

`tone` tints a node's box. Background, border, and text move together as one colour family, in both light and dark mode, so the reader can tell groups apart at a glance (inputs grey, the model's own steps green, a guard red). A tone overrides the type's colours and keeps its other traits (a centre node stays bold, a registry node stays dashed).

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

### Colour that reads

The palette is the reader's choice, so you never pick a colour; you pick what carries one. On a graph that is the node `type`, a `module` slot, or a `tone`. The rules that keep them readable in both modes:

- **One system carries the meaning.** `type` says the role in the map (the centre, a module, a leaf, a registry), `color` slots say which subsystem a node belongs to, `tone` says what kind of thing it is. Use slots or tones on a graph, never both: a module border under a tone is invisible.
- **Tone every node, or none.** An untoned node is the leaf colour, a plain box close to the page colour, and in dark mode those are two greys a shade apart. One plain node among toned ones reads as switched off; one toned node among plain ones reads as the point, which is right when it is.
- **Two or three tones with fixed meanings**, the same on every picture in the page. `neutral` is the outside, `green` the system's own parts, `blue` storage and the services it calls, `red` guards and failures, `amber` pending or manual, `purple` people. Say the key in the paragraph beside the graph.
- **The accent already highlights.** The centre node, the hot edges (the top third by `weight`), and the flow dashes are drawn in the accent colour. Do not spend `red` on "busy" when `weight` and `flow` show it.
- **Check both modes.** Flip the theme in the page header before sharing. Contrast between two tones holds in both modes; contrast between a plain node and the page is what runs out first in dark mode.

### Edge types and labels

| Type | Visual |
|------|--------|
| (omit) | Solid line |
| `consumes` | Solid line (explicit) |
| `publishes` | Dashed line |

Edges can have an optional `label` string. Keep it to a few words: labels sit on the edge and are not accounted for by the layout, so a long label on a dense graph overlaps a node.

### Edge weight and flow

Two optional edge fields turn a dependency graph into a traffic map:

| Field | Type | Effect |
|-------|------|--------|
| `weight` | number | Traffic volume on the edge (requests per second, messages, bytes; any one unit per graph). Line width scales with it, and edges in the top third of the range are tinted in the accent colour |
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

Put the number in `label` too when the reader should see it; the width alone only shows relative volume. Set `flow` on the edges that carry traffic and leave it off the rest, or the animation says nothing. The animation pauses while the graph is scrolled off screen or the tab is hidden, and it renders one static frame when the reader has Reduce Motion on.

### Placement

Place a `{"t": "graph"}` block in the section where you want the graph to appear. It goes at the top level or inside a column of a `columns` block, beside the paragraph that says what to look at. One graph block per page: a Doc that places it twice is refused, and so is a graph inside `details`. In a deck, the graph gets its own slide (deck rule 7); the brief and the deck show the same graph.

### From a Mermaid flowchart

A markdown file dropped on the index page (or posted to the import endpoint) becomes a page, and the first fenced `mermaid` flowchart in it becomes the page graph. Node labels are kept and shapes dropped. Dotted links become dashed `publishes` edges and every other link a solid arrow. `|text|` on a link becomes its label. A `subgraph` colours its members as `module` nodes, one colour slot per subgraph. `TD`, `TB`, and `BT` map to top-down, `LR` and `RL` to left-to-right. A sequence diagram, a second flowchart, or syntax the converter does not know stays a code block. When you already hold a flowchart, this is the fastest path to a graph; the full mapping and its losses are in the service's [CLAUDE.md][svc-claude] under *Importing markdown*.

## Chart format (the `chart` block)

Unlike the graph (one per page, passed as the `graph` argument), charts are **inline blocks**: drop a `{"t": "chart", ...}` block into a section's `blocks` array, as many as you need. The data lives in the block itself. Chart.js draws every kind but `ribbon`, which D3 draws; both are vendored at install ([ADR-0022][adr-0022]), and a missing library shows a note in the chart's place. Worked example: [stepped-charts.json](examples/stepped-charts.json).

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

### Which kind

Pick `kind` by the question the numbers answer:

- **How many per category?** `bar`. One series, or two to four side by side when the question is also "and how do the groups compare".
- **What is each category made of?** `stacked-bar`. Composition per category (responses by status class per day). Series stack in order; keep it to four.
- **Which ranks highest?** `horizontal-bar`. One series sorted by value, long labels (stages, repos, endpoints) readable at the left.
- **How did it move over time?** `line`. One or more trends on one axis. Never two value axes: split into two charts.
- **How much volume over time?** `area`. A single trend where the filled volume matters (requests per minute through an incident).
- **Is it trending, at a glance?** `sparkline`. A compact trend beside a `stat`, drawn without axes.
- **What share of the whole?** `doughnut`. One series, at most four slices; fold the rest into "other".
- **Do these two numbers move together?** `scatter`. One point per observation, numeric `x`, a `unit` for `y` and an `xunit` for `x`.
- **Where does the volume go?** `sankey`. Flows between stages or services, drawn from `flows`, not `series`. See **Sankey format**.
- **Who overtook whom?** `ribbon`. Ranks per period: one column per period, a ribbon per category, a crossing is a rank change. See **Ribbon format**.

The kind list, in the renderer's order: `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`, `ribbon`.

### Limits

The palette has four series colours, so four series is the ceiling for a readable chart of any kind; a fifth wraps to the first colour. Four is a ceiling, not a target: draw only the series the chart's claim needs and fold or cut the rest, so the eye finds the comparison the heading names. The renderer enforces the rest.

| Kind | Series | Categories, slices, or periods | Steps |
|------|--------|-------------------------------|-------|
| `bar`, `line`, `area`, `scatter` | one to four | about twelve categories; past that, a `horizontal-bar` or two charts | by series |
| `stacked-bar` | up to four | about twelve categories | by series |
| `horizontal-bar` | one | about fifteen labels, sorted | by series |
| `sparkline` | one, `y` only | any length | none: steps are refused |
| `doughnut` | one (only the first draws) | at most four slices, the rest folded into "other" | captions only, no series `step` |
| `sankey` | `flows`, not series | about twelve nodes | captions only |
| `ribbon` | one to four, each named, a fifth refused | about eight periods, every period in every series | by period: one caption per period, no series `step` |

A stepped chart never sits inside `details`, and a step without a caption is refused.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | One of `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`, `ribbon`. Defaults to `bar` |
| `title` | string? | Caption above the chart |
| `unit` | string? | Value-axis unit label (e.g. `ms`, `req`). Ignored for `sparkline`; shown in tooltips for `doughnut` |
| `xunit` | string? | X-axis unit label, `scatter` only (e.g. `payload KB`) |
| `series` | array | One or more `{name?, color?, points}` series. Every kind except `sankey` |
| `flows` | array | `{from, to, value}` links, `sankey` only. See **Sankey format** below |
| `steps` | array? | `[{caption}]`, one per step a deck slide walks through the chart. See **Stepped charts** below |
| `order` | string? | `ribbon` only. `rank` (the default) stacks the largest category on top of each column; `given` keeps the series order |
| `frame` | bool? | Every chart sits straight on the page or slide; `true` keeps a card (background, border, padding) around one that should stand apart in a brief. Do not set it on a deck slide (deck rule 17) |

Each series: `name` (legend label, shown when 2+ series), `color` (`series-1` to `series-4`, or the legacy names `terracotta`, `blue`, `green`, `purple` for the same four slots; omit to auto-assign by index), `step` (the 1-based step the series first shows at, on a chart with `steps`; omit for a series shown from the start), and `points`, an array of `{x, y}` where `x` is a category label (string) and `y` the value. Sparklines use only `y` (omit `x`). Scatter points take a numeric `x` (a JSON number or a numeric string).

Charts use the same palette as the graph and recolour automatically when the reader switches mode or family. Hover shows a tooltip on every kind but `sparkline`; the entry animation plays once per render and is skipped when the reader has Reduce Motion on.

### Frameless or framed

A chart sits straight on the page or the slide: the surface behind it is the page's own, and its gaps and labels read the page colour. That is the default because a card around a chart reads as UI, and on a slide it nests a box inside the slide's box. Set `frame: true` only in a brief, for one chart that should stand apart from the prose around it (a single key figure among paragraphs). The same switch exists on `image` the other way round: an image keeps a border unless `frame: false`, for a screenshot or an export that carries its own edge.

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

Node names are matched by exact string, so reuse the same spelling on every link. Each node takes the next palette colour in order of first appearance and every link fades from its source colour to its target colour.

### Ribbon format

```json
{
  "t": "chart",
  "kind": "ribbon",
  "title": "Signups by channel",
  "unit": "k",
  "series": [
    {"name": "search", "points": [{"x": "Q1", "y": 40}, {"x": "Q2", "y": 35}, {"x": "Q3", "y": 30}]},
    {"name": "social", "points": [{"x": "Q1", "y": 20}, {"x": "Q2", "y": 45}, {"x": "Q3", "y": 50}]},
    {"name": "direct", "points": [{"x": "Q1", "y": 20}, {"x": "Q2", "y": 25}, {"x": "Q3", "y": 10}]}
  ]
}
```

One series per category, `x` the period, `y` its value in that period. Each period is one column standing on the baseline. The categories stack inside it, the largest on top by default (`order: "rank"`), or in series order with `order: "given"`. A ribbon joins a category's segment to its segment in the next column, so two ribbons crossing is one category passing another. A category with a zero in a period has no segment there and its ribbon breaks. List every period in the first series, in order, with a zero for a gap. The columns follow the order the periods first appear in, and a later series that lists them the other way round is refused. Every series needs its own `name`, a period appears once per series, and values are zero or more. Hovering a segment or a ribbon shows its values and dims the other categories.

A ribbon takes at most four categories (the palette has four series colours, and a fifth is refused): fold the small ones into one named "other". Keep it to about eight periods; past that the ribbons turn into a hairball, so split the periods across slides.

### Stepped charts

A chart with `steps` walks through its data on a deck slide. Each press of Next takes one step: it shows the series whose `step` it is and puts that step's caption under the chart. The slide's counter shows the steps as `s/S` beside the slide number. The remote control's `next` and `prev` take them the way the keys do, and a jump (`goto`, a dot, the URL hash) lands on the finished chart. On a `reveal` slide the chart's steps follow right after the chart itself appears. The brief shows the finished chart with the captions as a numbered list, so the page still reads top to bottom and read-aloud speaks them.

```json
{
  "t": "chart",
  "kind": "stacked-bar",
  "title": "5xx by service per five minutes",
  "unit": "req",
  "steps": [
    {"caption": "The pricing service times out first."},
    {"caption": "Checkout retries pile on top of it."},
    {"caption": "The gateway sheds load and the spike peaks."}
  ],
  "series": [
    {"name": "pricing", "step": 1, "points": [{"x": "14:00", "y": 0}, {"x": "14:05", "y": 120}]},
    {"name": "checkout", "step": 2, "points": [{"x": "14:00", "y": 0}, {"x": "14:05", "y": 60}]},
    {"name": "gateway", "step": 3, "points": [{"x": "14:00", "y": 0}, {"x": "14:05", "y": 0}]}
  ]
}
```

A `diagram` walks its elements the same way, with `step` on nodes, groups, and edges and `focus` ids on a step; see **Diagram format**. A `ribbon` walks its periods instead of its series: one caption per period, in order, and step n shows the first n columns with the ribbons into them. The slide opens on the empty axis, and the first Next shows the first period. The step count has to equal the period count, and a series on a ribbon carries no `step`.

Rules the renderer enforces: every caption is non-empty, and a series `step` is between 1 and the number of steps. A `sparkline` carries no steps, a `doughnut` takes caption-only steps (it draws its first series only), and a stepped chart never sits inside `details`. A `sankey` draws flows, not series, so its steps are captions only. A series without `step` shows from the start. The value axis is pinned to the full data (its positive values), so a series joining later doesn't rescale the ones already shown. On a chart stepped by series the legend lists only the series shown so far. Reduce Motion and a deck with `transition: "none"` swap series without animating.

## Diagram format (the `diagram` block)

A `diagram` explains an architecture: boxes with a name and a short text under it, labelled boundaries around groups of boxes, and labelled arrows between them. The browser lays it out with ELK and draws it as SVG, so the author never places a box. It is an inline block like `chart`, as many per page as the story needs, and it is never the page's one `graph`. The graph stays the tool for a dependency or traffic map of many nodes; a diagram is for the picture a slide explains ([ADR-0020][adr-0020], amended for MAD-382). Worked example: [diagram-block.json](examples/diagram-block.json).

```json
{
  "t": "diagram",
  "direction": "LR",
  "caption": "The checkout request path",
  "groups": [
    {"id": "prod", "label": "prod cluster", "tone": "blue"}
  ],
  "nodes": [
    {"id": "web", "label": "Web app", "text": "Next.js, renders checkout", "kind": "person"},
    {"id": "api", "label": "Checkout API", "text": "Go, validates and prices", "group": "prod", "step": 2},
    {"id": "db", "label": "Orders DB", "text": "Postgres", "kind": "store", "group": "prod", "step": 3}
  ],
  "edges": [
    {"from": "web", "to": "api", "label": "POST /checkout", "flow": true, "weight": 120, "step": 2},
    {"from": "api", "to": "db", "label": "insert order", "step": 3}
  ],
  "steps": [
    {"caption": "The browser posts the cart."},
    {"caption": "The API validates and prices it.", "focus": ["api"]},
    {"caption": "The order lands in Postgres.", "focus": ["db"]}
  ]
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `nodes` | array | The boxes, at least one and at most 24. Each is `{id, label, text?, kind?, tone?, group?, step?}`; a label is at most 32 characters, a text 60 |
| `edges` | array? | The arrows, `{from, to, label?, flow?, weight?, step?}`, between two different nodes (never a group), one per pair and direction; a label is at most 24 characters |
| `groups` | array? | The boundaries, `{id, label, tone?, group?, step?}`, at most 12. `group` names the group this one sits in; two levels at most |
| `direction` | string? | How the layers run: one of `LR`, `TB`, left to right or top to bottom. Defaults to `LR` |
| `caption` | string? | Prose under the diagram, with inline markdown |
| `steps` | array? | `[{caption, focus?}]`, one per step a deck slide walks through the diagram. `focus` lists the node and group ids that light up at that step. See **Stepping a diagram** below |

A node's `label` is its name, in bold. `text` is the short line under it (the technology, a one-line purpose), wrapped to two lines. `kind` picks the shape: `service` (the default, a plain box), `store` (a cylinder), `queue` (a pipe), `person` (a box with a figure), `external` (a dashed box). `tone` tints a node or a group with one of the six node tones from the graph (`neutral`, `green`, `red`, `blue`, `amber`, `purple`; see **Node tones**). Use at most two or three tones on one diagram, each with a fixed meaning.

An edge's `label` sits on the line with a halo. `flow: true` marks an edge that carries traffic. The line is dashed, and dots move along it in the brief and on a slide, faster for a larger `weight` against the heaviest flow edge on the diagram. Under Reduce Motion the dots go and the dash stays. Ids are plain tokens (letters, digits, `_` and `-`) you only use to connect things; labels are what the room reads. Labels and texts are plain text, never inline markdown.

The diagram sits straight on the page or slide with no card of its own. Boxes carry their own border and fill, so they stand out on every theme. In a brief it shows its final state with the step captions as a numbered list under it. Alone on a slide it fills the slide with its caption under it.

### Colour that reads

A diagram of plain boxes inside toned boundaries is the common mistake. In dark mode every box is a grey on a grey, and the only colour is a faint wash the eye cannot attach to anything. The rules:

- **Boxes carry the tone, boundaries carry the structure.** A toned node is a filled box with a coloured border and label. A toned group is a half-transparent wash under a dashed line, drawn behind its members. So tone the nodes by what they are. Tone a group only when the boundary itself is the point (a trust zone, a namespace the room must see), in a tone none of its members use. A group in the same tone as its members swallows them.
- **No plain boxes in a toned picture.** Every node has a tone or none has. An untoned box is the card colour on the page colour, a shade apart in dark mode.
- **Two or three tones, fixed meanings, named in the caption.** `green` for the system's own services, `blue` for stores and queues, `neutral` for the edge and the outside, `purple` for people, `red` for guards, `amber` for pending or manual. `kind` says the shape (a store is a cylinder, a queue a pipe, an external system a dashed box). So the shape says what a box does and the tone says whose it is.
- **Nested groups alternate.** A group inside a group gets a different tone or none; two washes of one tone stack into one.
- **Focus and flow are colours already.** On a stepped diagram the focused box glows in the accent and the rest dim; a flow edge is dashed with accent dots moving on it. Do not spend a tone on "the current step" or "the busy path".
- **Check both modes** with the theme toggle in the page header before sharing. The worked example [diagram-block.json](examples/diagram-block.json) tones its nodes this way; the groups that keep a tone are the boundaries the story is about.

### Stepping a diagram

With `steps`, Next walks the diagram on a deck slide the way it walks a stepped chart. The counter shows `s/S`, the remote `next` and `prev` take the steps, and a jump lands on the last step with its focus. Leave the last step's `focus` empty when the finished picture should show plain. A node, group, or edge with `step` appears at that step and stays. Without one it is there from the start, so the slide opens on the parts every step shares. Give a group a `step` too, or it is an empty boundary until its first member arrives. An element never appears before what holds it: a node waits for its group, a group for its parent, and an edge for both of its ends. A step's `focus` may only name what has appeared by then. The renderer refuses the rest.

A step's `focus` lights up the named nodes and groups, everything inside a focused group, and the edges touching a focused node. The rest dim. A step without `focus` dims nothing. The layout is computed once for the whole diagram, so nothing moves between steps. Reduce Motion and a deck with `transition: "none"` cut between steps instead of fading.

### Which picture

- A `diagram` for how parts fit. That is a system context (the system, its users, the systems next to it), the containers inside a cluster boundary, trust zones as toned groups, or a request walking through four or five boxes.
- The `graph` for a map of many nodes and their links, where the shape of the network is the point.
- A `chart` for numbers over categories or time.

Keep a diagram to about twelve boxes, two levels of groups, and two or three tones. Past that the room reads nothing; split the picture over slides, one zoom level per slide.

## References (the `references` argument)

The `references` parameter takes `[{title, url}]`: source links rendered at the bottom of the page by the server, and the last slide of a deck. Always include references when the page draws on external sources. Inline links point at the line or PR a sentence is about; references list the sources the whole page drew on. The same URL can appear in both.

```json
[
  {"title": "dotfiles repo", "url": "https://github.com/mad01/dotfiles"},
  {"title": "PR #42", "url": "https://github.com/mad01/dotfiles/pull/42"}
]
```

## Colors available

Pages name colours by palette role, never by value, so a page looks right under every theme family the reader can pick (the Themes link in the page header). Roles a page may reference:

| Role | Use |
|------|-----|
| `primary` | The accent; `terracotta` is accepted as its alias on panel accents |
| `red`, `green`, `amber`, `yellow`, `blue`, `purple` | Semantic colours: panel accents, section and slide tones, callout meaning |
| `series-1` to `series-4` | Chart series, in the palette's order; the legacy chart names `terracotta`, `blue`, `green`, `purple` map to the same slots |

Graph and diagram node tones (`neutral`, `green`, `red`, `blue`, `amber`, `purple`) are named on the node's `tone` field, not here. A tone's background, border, and text come as one family in both modes, so a toned box stays readable when the reader flips the theme. A plain box, with no tone, sits closest to the page colour, where contrast runs out first in dark mode. See **Colour that reads** under **Graph format** and **Diagram format**.

[adr-0019]: https://github.com/mad01/thismoon/blob/main/docs/adr/0019-present-deck-beside-brief.md
[adr-0020]: https://github.com/mad01/thismoon/blob/main/docs/adr/0020-present-slide-layouts-and-chrome.md
[adr-0022]: https://github.com/mad01/thismoon/blob/main/docs/adr/0022-vendored-browser-assets.md
[svc-claude]: https://github.com/mad01/thismoon/blob/main/services/present/CLAUDE.md
[gsb-principles]: https://www.gsb.stanford.edu/insights/three-guiding-principles-successful-communication
[gsb-masterclass]: https://www.gsb.stanford.edu/insights/how-think-faster-talk-smarter-masterclass-matt-abrahams
[gsb-writing]: https://www.gsb.stanford.edu/insights/writing-win-how-quickly-capture-readers-keep-them-engaged
[gsb-numbers]: https://www.gsb.stanford.edu/insights/make-numbers-count-how-translate-data-your-audience
[stanford-visual-aids]: https://web.stanford.edu/~mvassar/handouts/Week%203%20-%203%20-%20Visual%20Aids%20Lecture%20Notes.pdf
