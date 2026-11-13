---
name: present
description: Build a scrollable briefing page, a slide deck beside it, or both with the present MCP, from structured Doc JSON with fixation reading, one Cytoscape graph, inline charts, and architecture diagrams, for digesting a work summary or research or for talking it through in a room. Use when the user asks to present or brief on a topic ("present the incident summary", "brief me on our infra stack"). Also use when the user asks for slides or a deck ("make a deck of this", "slides for the review").
---

# Present: briefing pages and slide decks

Generate scrollable HTML briefing pages and slide decks served live over localhost by the **present MCP** (Model Context Protocol server), or on a shared instance others reach by link. You pass structured JSON. The server renders it under a palette, a face, and a size the reader picks, with fixation reading on a toggle. The page draws one Cytoscape graph, Chart.js and D3 charts, and diagrams laid out by the Eclipse Layout Kernel (ELK). Never put a colour literal in a page: name a role (see **Colours available**) and the page follows whatever palette the reader picked.

This file holds the workflow and the rules every page follows. The files under `references/` hold the page skeletons and the detail for one rendition or block kind each; the workflow says which to read at which step.

**Who reads the pages.** The reader is dyslexic, has ADHD, and often listens to a page read aloud instead of reading it. The summary and the headings carry the argument, so a reader who skims or stops early still has it. A figure shows what a paragraph would make them hold in their head. Tables and key-value blocks are silent when read aloud, so the sentence before one states its point.

## How it works

- **You pass structured JSON**: a Doc object with sections and typed blocks. The server renders the HTML; you never write it.
- **`present_create` returns an `id` and a `url`.** Keep the id for edits.
- **Updates auto-reload** in open tabs. Call `present_open` **once**, never again for the same page.
- **A page carries a brief, a deck, or both** under one id: the brief is the `content` Doc, the deck a second Doc passed as `deck`, one section per slide. See **Slide decks**.
- **Every write is checked.** The renderer refuses a page it cannot draw, and the lint returns `warnings` for a page it stored but that passes a house limit. See **Checks**.

### Worked examples

Complete pages sit beside this file. Each file is the argument object of one `present_create` call (`title`, `content`, `deck`, `references`): read it, serialise `content` and `deck` as JSON strings, and call the tool. `gallery/` holds one page per page type, named in **Page types**. `examples/` holds four pages that each show one feature set (stepped charts, diagrams, slide layouts, a live terminal), and the format section for each feature names its file. The JSON under **Graph format** is the worked example for a graph.

## Workflow

1. **Gather the content.** Research the topic with code search, git log, PR data, or whatever sources apply. Every number and claim on the page comes from a real source.
2. **Name the reader and the ask.** One line each: who will read or hear it, what they should know afterwards, and what they should do. The `summary` states the know; the brief's first section or the deck's Next slide states the do. When the ask has two parts, the first two sections answer them, in the ask's order. Anything that serves neither is cut.
3. **Pick the page type** from **Page types**: a brief for a reader alone, a deck for a room, both when the room will want the detail afterwards (see **Brief or deck**). Before you write the Doc, read the type's skeleton in [references/page-types.md](references/page-types.md) and the limits in [references/limits.md](references/limits.md). The skeleton is the order of blocks that carries that kind of page, and the limits are the numbers its placeholders name and the lint checks. For a deck, also read [references/decks.md](references/decks.md) before you write the deck Doc: it holds one JSON skeleton per slide and the deck's own fields.
4. **Pick the pictures before the prose.** Decide which claim gets a chart, which a diagram, and which a lone number, with **Which block answers which question**. Numbers over categories or time are a `chart`; how parts fit is a `diagram`; a map of many nodes is the page's one `graph`. A picture picked after the prose is written only decorates it.
5. **Fill the skeleton.** Open the gallery page that **Page types** names for the type and follow how it is built, not its words; if the file is missing, follow the skeleton alone. Before you write a block kind that has a file of its own, read that file, because its fields and rules are there and not here:
   - a `diagram`: [references/diagrams.md](references/diagrams.md)
   - a `chart` with `steps`, a `sankey`, or a `ribbon`: [references/charts.md](references/charts.md)
   - the page's `graph`: [references/graph.md](references/graph.md)
   - `columns`, `details`, a `kv`, an `image`, or links to code: [references/blocks.md](references/blocks.md)
6. **Create the page.** `present_create` with `title`, `content`, optional `deck`, `graph`, and `references`. Hold the returned `id`.
7. **Read `warnings` in the result and revise once.** Fix what applies with one `present_update` and leave the rest. See **Checks**.
8. **Open the page once and look at it.** `present_open` once. Iterate with `present_update` on the same id, passing only the fields that changed; the open tab reloads. To change one section, use `present_patch_section`. That is also how parallel workers report into one page: each patches its own section, and no edit is lost. A hook that only reports progress runs `present progress <id> <label> <pct>`.

To edit a page created in an earlier session: `present_list` to find it (`has_doc: true` means the source is available), `present_source` to get the Doc, deck, and graph JSON, modify, then `present_update`.

## Page types

Pick the type whose reader and ask match yours. A page with a brief and a deck picks one type for each.

| Type | Rendition | For | Gallery page |
|------|-----------|-----|--------------|
| `decision` | brief | a decision record, a proposal | [gallery/decision.json](gallery/decision.json) |
| `explainer` | brief | how a system fits together | [gallery/explainer.json](gallery/explainer.json) |
| `walkthrough` | brief | a process or a how-to | [gallery/walkthrough.json](gallery/walkthrough.json) |
| `measurement` | brief | a benchmark, an eval, a cost report | [gallery/measurement.json](gallery/measurement.json) |
| `update` | brief | a work summary or a status | [gallery/update-adhd.json](gallery/update-adhd.json), with the ADHD mode on |
| `talk` | deck | 5 to 12 slides for a room | [gallery/talk.json](gallery/talk.json) |
| `review` | deck | a decision in a meeting | [gallery/talk.json](gallery/talk.json) for the statement, evidence, risk, and Next slides; the options-table and chart slides from [references/decks.md](references/decks.md) |

Every skeleton opens its first section with a figure, so a picture or a number is on the first screen (a short title, summary, and headings keep it there), and puts one sentence before each table that states its point.

### Headings are claims

Write each heading as the claim the section proves, a full sentence with a verb, because a reader who skims only the headings then leaves with the argument. The contents list shows the headings in order, so they should read as the summary of the page.

| A label says what the section is about | A claim says what it proves |
|---|---|
| Latency | The cache cut p99 latency by 40% |
| Architecture | Every request crosses the gateway twice |
| Results | Opus wrote the cleaner page in 11 of 15 runs |
| What changed | A chart on a slide now builds one step per Next |

The same holds for slide headings (deck rule 1). The few headings that stay short labels: the closing "Next", a `section` divider slide, and the ADHD mode's "Do now", and its "Now" and "Later" when no claim carries the split.

## Type contract

The shapes at a glance; full field tables sit in the format sections below.

```
Doc = {                                  // the `content` argument, and the `deck` argument
  summary: string,                       // the one line a reader acts on (the deck's title-slide sentence)
  meta: string,                          // date · category · key stats
  chips: [{text: string, style?: "stat" | "a" | "b" | "c" | "outline"}],
  sections: Section[],
  // deck only, ignored by the brief:
  logo?, logo_position?, progress?, presenter?, footer?, transition?
}

Section = {                              // one slide in a deck
  h: string,                             // heading (table of contents anchor; on a slide, the claim)
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
  | {t: "panel", title, sub?, accent?}   | {t: "progress", pct, label?, note?}
  | {t: "graph"}                         | {t: "chart", kind?, series | flows, title?, unit?, xunit?, steps?, order?, frame?}
  | {t: "code", text, lang?}             | {t: "html", text}
  | {t: "columns", cols}                 | {t: "stat", value, label, sub?}
  | {t: "quote", text, cite?}            | {t: "details", summary, blocks}
  | {t: "image", src, alt, caption?, frame?} | {t: "diagram", nodes, edges?, groups?, direction?, caption?, steps?}
  | {t: "terminal", session, cmd?, cwd?, prefill?, rows?}

Graph = {                                // the `graph` argument, one per page, shared by brief and deck
  nodes: [{id, label, type?: "center" | "module" | "leaf" | "registry", color?: 0..3,
           tone?: "neutral" | "green" | "red" | "blue" | "amber" | "purple"}],
  edges: [{from, to, type?: "consumes" | "publishes", label?, weight?, flow?}],
  layout?: "dagre" | "elk" | "elk-layered" | "elk-mrtree" | "elk-stress" | "elk-radial" | "elk-force" | "cose",
  direction?: "TB" | "LR"
}

PageRef = {id, url, has_deck, deck_url?, version, warnings?}   // present_create return; hold `id` all session
```

## Which block answers which question

Pick by the question the facts answer, not by what looks rich. The right column is the block that pattern-matches but reads worse.

| The facts are | Use | Not |
|---------------|-----|-----|
| the one thing the reader should do or know | `summary`, then a `p` | a background section first |
| one to three headline numbers the reader should keep | `stat`, alone or in `columns` | a `kv` with one row, a number buried in prose |
| two to four labelled values read together | `kv` in a brief, `stat` blocks in `columns` on a slide | a table |
| a comparison where the grid itself is the point | `table` (on a slide at most four columns and five rows) | nested lists |
| numbers over categories or over time | `chart`, see **Which kind** | a table of numbers |
| how a few parts fit: boxes, boundaries, arrows | `diagram` | the graph, a bulleted architecture |
| items that cause each other, branch, or nest: a cycle, a request path, a hierarchy | `diagram` | a bulleted list that hides the arrows |
| a map of many nodes and their links | the page's one `graph` | a diagram too big for one screen |
| parallel items, or a plain sequence with no branching | `list` (`ordered: true` when the order matters); only a walkthrough's opening path may be a one-row diagram of labels | a paragraph with "first, then, then" |
| the one risk or blocker | one `callout` with `sev: "warn"` | several callouts |
| a line someone said | `quote` with `cite` | italics in a `p` |
| long raw material: a timeline, logs, every number | `details`, closed by default, in the brief | a slide, a long `p` |
| two things read side by side: a chart and its caption | `columns` | two sections |
| an exact command or code | `code` with `lang` | backticks in a `p`, `html` with a `<pre>` |
| a screenshot or a figure | `image` with an `alt` a listener can use | `html` with an `<img>` |
| a live demo the talk starts once and comes back to | `terminal`, one session name per demo | a `code` block of pasted output, a screenshot of a terminal |
| a titled card with an accent | `panel`, in a brief | a slide (deck rule 17) |
| work in flight | `progress` plus `@chip(b:done)` chips | a sentence with a percentage |

### Brief or deck

A **brief** is read alone and scrolled. It holds the detail, the timeline, the alternatives, and the raw numbers in `details`. A **deck** is talked through in a room, one slide at a time. It says less on purpose, one idea per slide, and points at the brief for everything else. When the material will be read and talked through, make both under one id: the deck view has a Brief link and the brief a Slides link. Never make a second page for the deck of an existing brief; pass `deck` to `present_update` on the same id.

The brief's first section answers the question. How you found it, the background, and the history come after it or go in `details`. Write `summary` first, then cut any section that does not support it: tell the reader the time, not how the clock was built.

### Graph, diagram, or chart

- The **graph** is the page's one map: many nodes, their links, laid out by an engine you choose. Use it when the shape of the network is the point (a dependency tree, a traffic map with flow edges, a module map). One per page, shared by the brief and the deck. When the graph is the page's map, no diagram redraws a part of it: point at that part in a paragraph or an ordered list, so the reader meets each node once.
- A **diagram** is the picture a slide explains. It has a few boxes with a name and a one-line text, labelled boundaries around what shares a host or a trust level, and labelled arrows. Steps can walk a request through it. As many per page as the story needs. Keep each within the box limits, `diagram.boxes` in a brief and `diagram.slide_boxes` on a slide.
- A **chart** is numbers: over categories, over time, as shares, as flows of volume. Never draw structure with a chart or numbers with a graph.

### Stat, kv, or table

- One to three figures the reader should remember: `stat`, two or three in `columns`, in a brief as on a slide. Alone on a slide it is the big-number slide.
- Two to four labelled values read together (a cache's time to live, a timeout, a retry count): `kv` in a brief, each key about 15 characters at most. On a slide, stats in a row read better.
- A grid where the reader compares across both axes (three options against four criteria): `table`. Read-aloud skips tables and kv blocks, so when the page will be listened to, say the comparison in a list with one full sentence per item.
- Give the one number a comparison the reader already knows, in the `stat`'s `sub` line or the next sentence. A ratio for a percentage ("2 of every 5 checkouts", not "40%"), a familiar unit for a big count ("a full day of Tuesday's traffic"). A number without a comparison is read once and lost.

### Columns or details

- `columns` puts two or three things side by side at one glance. That is a chart and the paragraph that says what to look at, the graph and one sentence, three stats, or an image and its explanation. One column under 700 px wide.
- `details` hides what most readers skip and some need: a full timeline, the log lines, the raw table. Closed by default, read aloud when reached. Brief only: a slide that needs one has too much on it.
- Neither holds the other, and `details` never holds the graph, a stepped chart, or a stepped diagram.

## MCP tools

| Tool | Use |
|------|-----|
| `present_create(title, content?, deck?, graph?, references?)` | Create a page. `content` is a Doc JSON object (the brief). `deck` is a second Doc JSON object (the slides). At least one of the two. `graph` is a Graph JSON object. `references` is `[{title, url}]`. Returns `deck_url` when there is a deck, and `warnings` when the lint found something. |
| `present_source(id, version?)` | Get the editable source: the Doc, the deck's Doc, and the graph JSON, ready to modify and pass back to `present_update`. **Use this to edit a page from a new session.** `version` reads one of the newest 20 versions; pass that source to `present_update` to undo a bad edit. |
| `present_read(id)` | Fetch the rendered page (HTML, plus the deck's HTML). For editing, prefer `present_source`. |
| `present_update(id, title?, content?, deck?, graph?, references?)` | Patch a page. Omit fields to leave unchanged. Pass `graph: ""` to remove a graph, `deck: ""` to remove the deck, `references: []` to clear refs. A page keeps at least one of content and deck. Returns `warnings` for the whole page it wrote. |
| `present_patch_section(id, op, anchor?, section?, body_format?, rendition?, base_version?)` | Change one section. `op` is `replace`, `insert_before`, `insert_after`, or `remove` at `anchor`, or `append` at the end. `anchor` is the section's TOC anchor (the heading lowercased with dashes: "Next steps" is `next-steps`) or `index:N`, zero-based, when a heading repeats. `section` is one section as a JSON string, or markdown starting with a `#` or `##` heading when `body_format` is `markdown`. `rendition` is `content` (default) or `deck`. Pass the `version` you last read as `base_version` to be refused instead of overwriting a newer page. A refusal over a shared anchor lists the indices to use. Returns `warnings` for the patched section and counts the rest of the page in `elsewhere`. |
| `present_list()` | List all pages (id, title, url, version, updated, has_doc, has_brief, has_deck, deck_url). `has_doc: true` means the page is source-editable via `present_source`. |
| `present_open(id, deck?)` | Open in browser, **once per page**. `deck: true` opens the slide deck instead of the brief. |
| `present_deck(id, action, slide?)` | Drive the deck that is open in the browser: `start` or `stop` presenting, `next`, `prev`, `goto` a 1-based `slide`. Local instance only. |
| `present_share(id, ephemeral?)` | Push the page to the configured shared instance and return its `url`. Sharing again replaces the copy under the same link; `ephemeral` makes it expire 30 days after the last share. |
| `present_doctor()` | Check the page store, the serve process, and version skew. Call it when a tool errors or a page URL does not load. |

Hold the `id` from `present_create` for the whole session: every later call takes it. If `present_open` fails (sandbox or no `open` binary), return the URL from `present_create` to the user instead. When the user asks to share a page, call `present_share` with the id and return the link with any `warnings` message; if the tool is missing or fails on the network, point the user at the Share button or `present share <id>`.

## Checks

- A **refusal** comes from the renderer: the call fails, nothing is stored, and the error names the block and the rule. Fix that block and call again.
- A **warning** comes from the lint: the page is stored, and the result lists `warnings: [{rule, where, message}]`, with `more` counting any left out. `rule` is an id in [references/limits.md](references/limits.md). `where` is a zero-based path into your arguments: `content.sections[2].blocks[0]` is the first block of the brief's third section. `message` gives the place, the fault, the reason, and the fix.

Read the warnings once, fix what applies in one update, and stop. A warning is a house limit with its reason, not an order: a long table may be the point of its section, so keep it when it is. Never change a fact to clear a warning.

The lint does not judge whether a claim is true, whether a sentence heading really makes a claim, whether the chart kind fits the data, or anything that needs the drawn page, which is why step 8 opens the page and looks. `present lint --file <args.json>` runs the same checks before you send.

## Doc format (the `content` argument)

```json
{
  "summary": "Executive summary with **bold** and `code`.",
  "meta": "2026-05-29 · Category · Key stats",
  "chips": [{"text": "42 items", "style": "stat"}, {"text": "tag", "style": "a"}],
  "sections": [
    {
      "h": "The cache cut p99 latency by 40%",
      "blocks": [
        {"t": "p", "text": "Paragraph text."},
        {"t": "h3", "text": "Subsection heading"},
        {"t": "callout", "text": "Important note.", "sev": "warn"},
        {"t": "table", "cols": ["Name", "Status"], "rows": [["foo", "@chip(b:done)"]]},
        {"t": "kv", "kv": [{"k": "Label", "v": "Value"}]},
        {"t": "list", "items": ["First", "Second"], "ordered": false},
        {"t": "progress", "pct": 75, "label": "75%"},
        {"t": "code", "lang": "go", "text": "func main() {\n\tfmt.Println(\"hello\")\n}"},
        {"t": "image", "src": "https://example.com/dashboard.png", "alt": "The on-call dashboard at 14:15, every checkout panel red", "caption": "The dashboard at 14:15."}
      ]
    }
  ]
}
```

### Top-level fields

| Field | Type | Description |
|-------|------|-------------|
| `summary` | string | The point in one or two sentences, its key phrase in **bold** |
| `meta` | string | Meta line below title (date, category, stats) |
| `chips` | array | Chip tags below summary |
| `sections` | array | Content sections with headings and blocks |

### Inline markdown

Prose fields take inline markdown: the `summary`, paragraphs, callouts, list items, table cells, kv labels and values, quotes and their `cite`, a stat's `label` and `sub`, captions (of an image, a diagram, and each step), a panel's `sub`, a `details` summary, and speaker `notes`. The server renders these patterns in them:

| Pattern | Renders to |
|---------|------------|
| `**bold**` | `<strong>` |
| `*italic*` | `<em>` |
| `` `code` `` | `<code>` |
| `[text](url)` | `<a href>` |
| `@chip(style:text)` | inline chip |

Chip styles: `stat` (gray), `a` (warm), `b` (green), `c` (blue), `outline`.

Every other text field is plain text and prints the marks as typed: the title, `meta`, chips, headings, table headers, a stat's `value`, an image's `alt`, and every label inside a chart or a diagram.

A star inside a code span is literal. Outside one, a single `*` italicises only when it pairs with another star, so `*.pem, *.key` stays literal; put a glob or a pointer in a code span to be sure. The lint warns (`inline.stray_mark`) about a backtick or a pair of stars in a plain field and about an odd number of backticks in a prose field.

### Block types

| Type | Fields | Description |
|------|--------|-------------|
| `p` | `text` | Paragraph of at most 70 words; a longer thought becomes a list, a table, or a figure |
| `h3` | `text` | Subsection heading |
| `callout` | `text`, `sev?` | Highlighted callout. Severity: `info` (blue border), `warn` (amber border), `ok` (green border), `error` (red border), or omit for default |
| `table` | `cols`, `rows` | Data table. Cells support inline markdown |
| `kv` | `kv: [{k, v}]` | Key-value pairs. Labels and values support inline markdown |
| `list` | `items`, `ordered?` | Bulleted or numbered list. Items support inline markdown |
| `panel` | `title`, `sub?`, `accent?` | Titled card. Accent is a palette role name (`primary`, `blue`, `green`, `purple`, `amber`, `red`, `yellow`, `series-1` to `series-4`; `terracotta` still works as an alias of `primary`). An unknown name is refused |
| `progress` | `pct`, `label?`, `note?` | Progress bar (0-100); `note` is a short line after the label ("Index: 12 of 20") |
| `graph` | (none) | Placement marker for the Cytoscape graph container |
| `chart` | `kind`, `series` or `flows`, `title?`, `unit?`, `xunit?`, `steps?`, `order?`, `frame?` | Metric chart (Chart.js, or D3 for `ribbon`). `kind` is one of `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`, `ribbon`. Inline: use as many as you like per page. `steps` makes a deck slide walk the chart one step per Next. See **Chart format** |
| `diagram` | `nodes`, `edges?`, `groups?`, `direction?`, `caption?`, `steps?` | Architecture picture laid out in the browser by ELK and drawn as SVG (D3): boxes with a name and a short text, labelled boundaries, labelled arrows. A node `kind` is one of `service`, `store`, `queue`, `person`, `external`; `direction` is one of `LR`, `TB`. Many per page, never the page graph. `steps` walks it one step per Next on a deck slide. See **Diagram format** |
| `code` | `text`, `lang?` | Fenced code block with language badge and copy button. `text` is verbatim code with no inline markdown: backticks, `**`, and `<` all render literally. `lang` sets the badge and the highlighting: `go`, `bash`, `json`, `python`, `typescript`, `yaml`, `sql` highlight; any other `lang` shows in the badge as given (DOCKERFILE) over plain text, and none shows `text` |
| `html` | `text` | Raw HTML passthrough for one-off custom content |
| `columns` | `cols: [[blocks], [blocks]]` | Two or three equal-width columns of blocks, one column under 700 px. A column holds any block but `columns` and `details`. Three `stat` blocks in it make a row of figures; a `chart` beside a `p` puts the caption next to the chart. The page's one `graph` may sit in a column too, beside the paragraph that says what to look at |
| `stat` | `value`, `label`, `sub?` | A large figure over a label, in a tile in the brief and plain on a slide. `value` is shown verbatim (no inline markdown) and stays out of fixation; `label` and `sub` take inline markdown |
| `quote` | `text`, `cite?` | A quotation with a left rule and the attribution under it |
| `details` | `summary`, `blocks` | A collapsible block, closed by default, with `summary` as the clickable line. Holds any block but `graph`, `columns`, and `details`. Read-aloud reads it and opens it while a part inside plays. The home for a long timeline or raw numbers |
| `image` | `src`, `alt`, `caption?`, `frame?` (`false` drops the border) | An image with a caption under it. `src` is an `http(s)` URL, or on this machine an absolute or `~` path to an image file, which the tool copies into the page store; [references/blocks.md](references/blocks.md) has the formats and what sharing needs. `alt` is required: read-aloud reads it in the image's place, and the caption after it. Goes inside `columns` and `details` |
| `terminal` | `session`, `cmd?`, `cwd?`, `prefill?`, `rows?` | A live terminal running in the local `present serve`. `session` names the process with `a-z`, `0-9`, and `-`. Every block on the page that names it shows the same process, in the brief and the deck alike. `cmd` runs in place of a login shell, and `cwd` is where it starts (a `~` path, or the home directory when omitted). `prefill` is one line, typed on the prompt and left for Enter. `rows` is the height. Blocks naming one session give the same `cmd`, `cwd`, and `prefill`, or the call is refused. Goes inside `columns`, never inside `details`. Nothing runs until the live terminal is switched on; see **Terminal format** |

### Section fields

| Field | Type | Description |
|-------|------|-------------|
| `h` | string | Section heading (used for TOC anchor): the claim the section proves, see **Headings are claims** |
| `blocks` | array | Content blocks |
| `tone` | string? | A palette role name (see **Colours available**). In a brief it gives the section a band in that colour; on a slide it colours the heading rule and the accent bar, never a filled background. Pick a saturated role (`blue`, `green`, `amber`, `red`, `purple`, `primary`): a `-bg` role is nearly invisible as a rule. An unknown name is refused |
| `layout` | string? | Deck only: `default`, `center`, `statement` (the heading is the slide), or `section` (a divider). A slide whose only block is a `stat` or a `quote` is the big-number or quote slide with no layout set |
| `notes` | string? | Deck only: speaker notes with inline markdown. Never on the slide, never read aloud; the deck shows them in a drawer on the N key |
| `reveal` | bool? | Deck only: the slide's list items and top-level blocks appear one per Next; a jump lands with all shown |

### Notes

- **The table of contents (TOC) is auto-generated** from section headings when there are 2+ sections. Do not write TOC markup.
- **Fixation reading is applied for you** to the prose of the page; never write `data-fixation` yourself.
- **Unknown block types** produce an HTML comment error; they don't break the page.
- **Use `code` blocks for multi-line code**, not `p` with backticks (inline `code` is for short identifiers) and not `html` with a hand-written `<pre>`.
- **Read-aloud skips tables, kv blocks, code blocks, and terminals.** A `stat` reads as its value, label, and sub line; a `details` block reads like prose and opens while a part inside it plays. An `image` reads as its `alt` and then its caption, so write the `alt` as the sentence a listener needs in place of the picture. Name a command or path in code once, where the reader must see it exactly, in a code block when they will run it; after that, say it in words, because a code span mid-sentence reads badly aloud.
- **Inline markup nests.** A code span, a link, or a chip inside `**bold**` renders as expected.
- **Related items go in one page.** N review findings, bugs, or options are N sections of one page, never one page each; [references/blocks.md](references/blocks.md) has the pattern.

## ADHD briefing mode (opt-in)

**Off by default.** It is on when the user asks for it for this page ("adhd mode", "adhd briefing", "shape it for adhd"), or when the page is for the user themself and their session runs the `I Have ADHD` output style. A page for other readers, such as a gallery or shared page, never takes it from that style. Otherwise ignore this section.

When on, shape the Doc so the reader can act on it. The mode changes the page's structure and order, never its facts, and nothing outside the page:

1. **Lead with actions.** Make the `summary` the single most important next action in concrete terms (a command, a path, a decision), not background. Put context lower.
2. **First section is "Do now."** Give it a `list` block of bounded, numbered steps, each one action, no "and then" twice.
3. **Cap every list at 5 items.** Split a longer one into a section for now and one for later. A heading that carries the split is a claim ("Five items can wait, because skipping them breaks nothing"); the bare labels "Now" and "Later" are allowed.
4. **Make wins visible.** Use `@chip(b:done)` chips and a `progress` block for anything completed or in-flight, so finished work is not buried in prose.
5. **One callout, not many.** If there is a single blocker or risk, use one `callout` with `sev: "warn"`, beside the action or claim it is about. Do not scatter warnings.
6. **End with the next action.** The last section names one concrete thing to do next, doable in under two minutes.
7. **Matter-of-fact tone.** State cause and fix for errors; no "uh oh" framing.

Skip preamble sections ("about this brief", "overview of overview"). Every section earns its place by being actionable or a visible win.

How the mode composes with a page type:

| Page type | With the mode on |
|-----------|------------------|
| any brief type | "Do now" comes first: the next actions, at most five, each one line. The type's skeleton follows it whole, and a first section that holds a figure stays as section two. The first screen shows the "Do now" heading rather than a figure. |
| a deck | No change: the deck rules are already tighter. |

## Slide decks (the `deck` argument)

A deck is a second Doc under the same page id, shown one section at a time at `deck_url`. It uses the Doc schema above unchanged: `summary`, `meta`, and `chips` make the title slide, every entry in `sections` is one slide, and the page's `references` make the last slide. The page's one `graph` is shared with the brief, so a `{"t": "graph"}` block in a deck section shows the same graph. Pass `deck` beside `content`, or alone for a deck-only page. The deck is never generated from the brief: you write it, and it says less.

Before you write the deck Doc, read [references/decks.md](references/decks.md): it holds one JSON skeleton per kind of slide, the deck's chrome fields, what each setting is for, a full example deck, and how the deck behaves in the room. Worked examples: [slides-without-boxes.json](examples/slides-without-boxes.json) for the layouts and tones, [stepped-charts.json](examples/stepped-charts.json) for data slides that build up, [live-terminal-demo.json](examples/live-terminal-demo.json) for a demo the deck starts on one slide and comes back to.

### When to make one

Make a deck when the material will be talked through: a review in a meeting, an incident retrospective, a decision the room has to make. A brief someone reads alone stays a brief. When both exist, the brief is where the detail lives, so nothing on a slide needs to be complete.

### Building a deck

The order that produces a deck rather than a shortened brief:

1. **Write the title slide last, but decide it first.** `summary` is the one sentence the room should remember; `presenter` is the byline, and the only place for a date beside `meta`.
2. **One claim per slide.** List the claims the room must accept, in the order the argument runs. Order them by one named structure and keep it. Problem, Solution, Benefit for a proposal. What, So What, Now What for a finding or a status. Cause, Effect, Solution for an incident. Comparison, Contrast, Conclusion for options. Each claim becomes a slide whose `h` states it. What you would say goes in `notes`.
3. **Give the data its own slide.** A `chart` when the slide proves a number, a `diagram` when it explains how parts fit, the `graph` when the map is the point. A chart or the graph stands alone or in `columns` beside one short `p`; a diagram stands alone. When the picture should form while you talk, give it `steps`.
4. **Keep one figure and one line.** The number the room should carry out of the door is a lone `stat`; the sentence is a lone `quote` or a `statement` slide. Once or twice a deck.
5. **End on the ask.** The last authored slide is `h: "Next"` with an ordered list of at most three actions, each with its owner and date when the source names them.
6. **Cut.** Nine slides or more may open their parts with `section` dividers; past twelve it is two decks. Everything cut goes in the brief.

### Deck rules

A good deck is not a shorter brief. Apply these when writing `deck`:

1. **One idea per slide.** The section heading `h` states the point as a claim, "Cache cut p99 latency by 40%", never a topic, "Latency". A data slide is no exception. Its heading is what the chart proves, "Errors held near 900 a minute for half an hour"; the chart's `title` names the measure, "Checkout 5xx per minute".
2. **Slide budget.** A `list` of 3 to 5 items under ten words each, or one `p` of at most two sentences. Never a long paragraph and a list on the same slide. Six lines of body is the ceiling; a slide that scrolls has too much on it.
3. **Deck length.** 5 to 12 slides. More than that is two decks, or material that belongs in the brief.
4. **Title slide.** `summary` is the one sentence the audience should remember. `meta` is date, occasion, and audience. `chips` carry two or three headline numbers with `style: "stat"`.
5. **One highlight per slide.** Bold exactly one phrase, or use one `@chip(stat:...)` for the number that matters. Two bold phrases highlight neither.
6. **Data gets its own slide.** One `chart` block per slide, its heading the claim the chart proves and its `title` the measure. Draw only the series the claim needs and fold or cut the rest. The eye then finds the comparison the heading names. A `kv` block for up to four figures. A `table` only when the comparison is the point, at most four columns and five rows. Give at least half the slides a picture: a claim shown beside a picture is remembered better than the same claim in text alone (`deck.text_only`).
7. **The graph gets its own slide.** A `{"t": "graph"}` block with nothing but the heading; it is the page's one visual, so let it fill the slide. When the room needs one sentence pointing at a node, put the graph in a `columns` block beside one short `p` and nothing else.
8. **Code stays in the brief.** A code block draws a card (rule 17), so a slide names the file or function in prose and the brief holds the code. When the code is the point of the talk, the room reads it from the brief.
9. **One callout per deck at most**, `sev: "warn"`, for the single risk or blocker.
10. **No agenda, no "questions?" slide.** Under nine slides an agenda is noise. The last authored slide is the ask (**Building a deck**, step 5).
11. **Cut the spoken sentences.** If a line only makes sense when said aloud, it is the speaker's, not the slide's. The slide carries the claim; the speaker carries the argument. No sentence appears both on the slide and in `notes`.
12. **Which block when.** One `stat` or one `quote` alone on a slide for the one figure or the one line the room should keep. The `sub` line gives the figure a comparison the room already knows ("2 of every 5 checkouts"). Two or three `stat` blocks in a `columns` block for a row of figures, and a `chart`, the `graph`, or an `image` beside its caption paragraph in `columns`. A `diagram` stands alone (rule 16). An image alone on a slide fills it, bounded by the slide's height.
13. **Which layout when.** `statement` for the one sentence the deck exists to say, once or twice a deck. `section` to open a part of a longer deck, with a `tone`. `center` for a slide that is one short thing, a row of figures say. Everything else stays `default`; the heading and the blocks carry the slide.
14. **Notes carry the argument, reveal carries the pace.** Put what you would say, and only that, in `notes`. Set `reveal` on a list that is an argument built one line at a time, never on a list the room should read whole.
15. **A chart that builds up gets steps.** Give a `chart` `steps` when the room should watch the picture form: one caption per step, and `step` on each series for the step it joins at. Three to five steps, each caption one short sentence. A `ribbon` steps by period, so keep a stepped ribbon to three to five periods. The shapes are in [references/charts.md](references/charts.md).
16. **An architecture gets a diagram, and a diagram alone fills its slide.** Use a `diagram` block, not the graph, when the slide explains how parts fit. That is boxes with a name and a short text, boundaries around the parts that share a host or a trust level, and labelled arrows. Alone on a slide it fills the slide with its caption under it; inside `columns` its text draws too small for the room (`diagram.slide_columns`). A request walkthrough gets `steps`: four or five, each caption one sentence, with `focus` on the box the step is about. Keep it within `diagram.slide_boxes` and two or three tones per slide, and split a bigger picture over slides. See **Diagram format**.
17. **A slide draws no filled boxes.** The slide is already the container, so a box inside it reads as card UI, and two of them nest. `panel`, `details`, `code`, and a chart with `frame: true` draw a card, so keep them in the brief. A deck keeps two boxes: the one warn callout of rule 9, and a `terminal`, whose frame is the slide's content (rule 18). `tone` colours the heading rule and accent bar, a `section` divider is a heading with an accent bar, not a band, and a `stat` sits plain, alone or in a row.
18. **A live demo is a terminal, and its session name ties the slides together.** Give the demo's `terminal` block a slide of its own and leave `rows` out, so it fills the slide. Two slides naming the same `session` show one process. Start Claude on slide 3, talk through slides 4 to 7, and slide 8 with the same session shows what happened meanwhile and takes input again. Every block naming the session repeats its `cmd`, `cwd`, and `prefill` exactly. Put the first command you will run in `prefill`, so the room watches you press Enter instead of watching you type. See **Terminal format**.

## Graph format (the `graph` argument)

Pass a structured object. The server stores the nodes and edges; the page styles and lays them out when it loads, so the stored graph carries no colours and no coordinates. One graph per page, shared by the brief and the deck.

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

Pick `layout` by the shape of the data. `dagre`, the default, suits a tree or a hierarchy. Before you pick another engine or set `direction`, read [references/graph.md](references/graph.md): it matches each engine to a shape of data and lists what each node `type` draws.

### Node tones

`tone` tints a node's box. Background, border, and text move together as one colour family, in both light and dark mode, so the reader can tell groups apart at a glance (inputs grey, the model's own steps green, a guard red). A tone overrides the type's colours and keeps its other traits (a centre node stays bold, a registry node stays dashed).

| Tone | Reads as |
|------|----------|
| `neutral` | Grey: the outside, the edge, inputs and outputs |
| `green` | The system's own parts and steps; healthy |
| `red` | Guards, checks, failures |
| `blue` | Storage, queues, and the services it calls |
| `amber` | Pending, degraded, manual steps; on a diagram also the one tone for a zone boundary |
| `purple` | People, decisions, review |

Set it as `{"id": "check", "label": "Safety check", "tone": "red"}`. Use tones for meaning, not decoration: two or three per graph, the same tone for the same kind of thing. The meanings in the table hold for diagram nodes and groups too, so one key serves every picture on a page. An unknown tone fails the call.

### Colour that reads

The palette is the reader's choice, so you never pick a colour; you pick what carries one. On a graph that is the node `type`, a `module` slot, or a `tone`. The rules that keep them readable in both modes:

- **One system carries the meaning.** `type` says the role in the map (the centre, a module, a leaf, a registry), `color` slots say which subsystem a node belongs to, `tone` says what kind of thing it is. Use slots or tones on a graph, never both: a module border under a tone is invisible.
- **Tone every node, or none.** An untoned leaf node is a plain box close to the page colour, and in dark mode those are two greys a shade apart. A centre or registry node keeps its own look. One plain node among toned ones reads as switched off; one toned node among plain ones reads as the point, which is right when it is.
- **Two or three tones with the meanings in the table above**, the same on every picture in the page. Say the key in the paragraph beside the graph.
- **The accent already highlights.** The centre node's border and the hot edges (weight in the top third of the range) are drawn in the accent colour. A flow edge is dashed and animated, and takes the accent only when it is hot. Do not spend `red` on "busy" when `weight` and `flow` show it.
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

Put the number in `label` too when the reader should see it; the width alone only shows relative volume. Set `flow` on the edges that carry traffic and leave it off the rest, or the animation says nothing.

### Placement

Place a `{"t": "graph"}` block in the section where you want the graph to appear. It goes at the top level or inside a column of a `columns` block, beside the paragraph that says what to look at. One graph block per Doc, so the brief and the deck may each place it once: a Doc that places it twice is refused, and so is a graph inside `details`. In a deck, the graph gets its own slide (deck rule 7); the brief and the deck show the same graph.

## Chart format (the `chart` block)

Unlike the graph (one per page, passed as the `graph` argument), charts are **inline blocks**: drop a `{"t": "chart", ...}` block into a section's `blocks` array, as many as you need. The data lives in the block itself. Chart.js draws every kind but `ribbon`, which D3 draws. Worked example: [stepped-charts.json](examples/stepped-charts.json).

A chart block is just another entry in `sections[].blocks`, never a top-level argument. Putting it in the full Doc:

```json
{
  "sections": [
    {
      "h": "API traffic grew by half from Monday to Tuesday",
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
- **Which ranks highest?** `horizontal-bar`. One series, sorted by value before you pass it (the chart keeps the point order), long labels (stages, repos, endpoints) readable at the left.
- **How did it move over time, or over an ordered amount such as input size?** `line`. One or more trends on one axis. Never a line over unordered categories: compare those with `bar`. Never two value axes: split into two charts.
- **How much volume over time?** `area`. A single trend where the filled volume matters (requests per minute through an incident).
- **Is it trending, at a glance?** `sparkline`. A compact trend beside a `stat`, drawn without axes.
- **What share of the whole?** `doughnut`. One series, at most four slices; fold the rest into "other".
- **Do these two numbers move together?** `scatter`. One point per observation, numeric `x`, a `unit` for `y` and an `xunit` for `x`.
- **Where does the volume go?** `sankey`. Flows between stages or services, drawn from `flows`, not `series`. Its shape is in [references/charts.md](references/charts.md).
- **Who overtook whom?** `ribbon`. Ranks per period: one column per period, a ribbon per category, a crossing is a rank change. Its shape is in [references/charts.md](references/charts.md).

### Limits

The palette has four series colours, so four series is the ceiling for a readable chart of any kind; a fifth wraps to the first colour. Four is a ceiling, not a target: draw only the series the chart's claim needs and fold or cut the rest, so the eye finds the comparison the heading names. The renderer refuses a ribbon with more series than the palette has colours (`ribbon.series`), steps on a sparkline, and a series `step` on a doughnut, a sankey, or a ribbon; the other step refusals are in [references/charts.md](references/charts.md). Every other limit here is guidance.

| Kind | Series | Categories, slices, or periods | Steps |
|------|--------|-------------------------------|-------|
| `bar`, `line`, `area`, `scatter` | one to four | about twelve categories; past that, a `horizontal-bar` or two charts | by series |
| `stacked-bar` | up to four | about twelve categories | by series |
| `horizontal-bar` | one | about fifteen labels, in the order you pass them | by series |
| `sparkline` | one, `y` only | any length | none: steps are refused |
| `doughnut` | one (only the first draws) | at most four slices, the rest folded into "other" | captions only, no series `step` |
| `sankey` | `flows`, not series | about twelve nodes | captions only |
| `ribbon` | one to four, each named | about eight periods, every period in every series | by period: one caption per period, no series `step` |

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | One of `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`, `ribbon`. Defaults to `bar` |
| `title` | string? | Caption above the chart: the measure and its unit, in plain text |
| `unit` | string? | Value-axis unit label (e.g. `ms`, `req`). Ignored for `sparkline`; shown in tooltips for `doughnut` |
| `xunit` | string? | X-axis unit label, `scatter` only (e.g. `payload KB`) |
| `series` | array | One or more `{name?, color?, points}` series. Every kind except `sankey` |
| `flows` | array | `{from, to, value}` links, `sankey` only, see [references/charts.md](references/charts.md) |
| `steps` | array? | `[{caption}]`, one per step a deck slide walks through the chart, see [references/charts.md](references/charts.md) |
| `order` | string? | `ribbon` only. `rank` (the default) stacks the largest category on top of each column; `given` keeps the series order |
| `frame` | bool? | Every chart sits straight on the page or slide; `true` keeps a card (background, border, padding) around one that should stand apart in a brief. Do not set it on a deck slide (deck rule 17) |

Each series: `name` (legend label, shown when 2+ series), `color` (`series-1` to `series-4` or a legacy name, see **Colours available**; omit to auto-assign by index), `step` (the 1-based step the series first shows at, on a chart with `steps`; omit for a series shown from the start), and `points`, an array of `{x, y}` where `x` is a category label (string) and `y` the value. Sparklines use only `y` (omit `x`). Scatter points take a numeric `x` (a JSON number or a numeric string).

### Frameless or framed

A chart sits straight on the page or the slide, because a card around a chart reads as UI, and on a slide it nests a box inside the slide's box. Set `frame: true` only in a brief, for one chart that should stand apart from the prose around it. An `image` works the other way round: it keeps a border unless `frame: false`, for a screenshot that carries its own edge.

## Diagram format (the `diagram` block)

A `diagram` explains an architecture: boxes with a name and a short text under it, labelled boundaries around groups of boxes, and labelled arrows between them. The browser lays it out with ELK and draws it as SVG, so you never place a box. It is an inline block like `chart`, as many per page as the story needs, and it is never the page's one `graph`. Worked example: [diagram-block.json](examples/diagram-block.json).

The renderer picks the layout. It tries the diagram in a few forms and draws the first one whose text stays readable in the space it has, and your `direction` is its first choice, not a promise. Box text wraps and is never cut. What you decide is what goes in: the boxes the claim needs and no more, and the labels a reader must read. A brief diagram stays within `diagram.boxes`, a slide diagram within `diagram.slide_boxes`, and the longest path through either within `diagram.chain`; past those, the text draws too small to read or the picture runs past a screen.

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

Before you write a diagram block, read [references/diagrams.md](references/diagrams.md): it holds the fields and the lengths the renderer refuses past, the colour rules that keep boxes readable in dark mode, and how steps walk a diagram.

### Which picture

- A `diagram` for how parts fit. That is a system context (the system, its users, the systems next to it), the containers inside a cluster boundary, trust zones as toned groups, or a request walking through four or five boxes.
- The `graph` for a map of many nodes and their links, where the shape of the network is the point.
- A `chart` for numbers over categories or time.

Size a diagram to its claim, not to the system. When the whole does not fit, split it: one diagram per zoom level, or one per slide, each with the heading that says what it shows.

## Terminal format (the `terminal` block)

A `terminal` block puts a live terminal on the page. The local `present serve` runs the process in a pseudo-terminal, and the browser shows it with xterm.js. The process belongs to the page and the block's `session` name, not to the tab, so a demo survives your edits to the deck. Use it for a demo the talk starts once and comes back to. A command the room only needs to read is a `code` block. Worked example: [live-terminal-demo.json](examples/live-terminal-demo.json), a deck that starts a shell and Claude early and shows both again after the talk slides.

```json
{"sections": [
  {"h": "Claude starts on the flaky checkout test", "blocks": [
    {"t": "terminal", "session": "demo", "cmd": "claude", "cwd": "~/code/shop", "prefill": "fix the flaky checkout test"}
  ]},
  {"h": "Two tests read one fake clock", "blocks": [
    {"t": "list", "items": ["The order decides who sees the tick", "It fails one run in twenty"]}
  ]},
  {"h": "Claude found the same cause", "blocks": [
    {"t": "terminal", "session": "demo", "cmd": "claude", "cwd": "~/code/shop", "prefill": "fix the flaky checkout test"}
  ]}
]}
```

- **Nothing runs until the live terminal is on.** It is off by default, and the block then says "Live terminal is off. Run `present terminal on`, then press Run." A block that has not started yet keeps asking every few seconds and starts by itself once the switch is on and its slide is the one showing; a block whose session ended waits for Run. `present terminal status` prints which. `present terminal off` ends every session within two seconds, and each open block shows the off note.
- **The brief is the run sheet.** For a deck with a terminal, the brief tells the speaker to run `present terminal on` before the talk and `present terminal off` after it, each in a `code` block, so the switch is part of the plan rather than a surprise on stage.
- **The first time a block is seen, the session starts** and types the `prefill`; every later showing replays the recent output and takes input again. `cmd` runs in an interactive login shell, and `cwd` is an absolute or `~` path.
- **A shared instance runs nothing.** A page with terminal blocks shares as usual, and each block there says the live terminal is not available.

How a session behaves during the talk (Run and Kill, when it ends, what changes take effect) is under **In the room** in [references/decks.md](references/decks.md).

## References (the `references` argument)

The `references` parameter takes `[{title, url}]`: source links rendered at the bottom of the page by the server, and the last slide of a deck. Always include references when the page draws on external sources. Inline links point at the line or PR a sentence is about; references list the sources the whole page drew on. The same URL can appear in both.

```json
[
  {"title": "dotfiles repo", "url": "https://github.com/mad01/dotfiles"},
  {"title": "PR #42", "url": "https://github.com/mad01/dotfiles/pull/42"}
]
```

## Colours available

Pages name colours by palette role, never by value, so a page looks right under every theme family the reader can pick (the Themes link in the page header). Roles a page may reference:

| Role | Use |
|------|-----|
| `primary` | The accent; `terracotta` is accepted as its alias on panel accents |
| `red`, `green`, `amber`, `yellow`, `blue`, `purple` | Semantic colours: panel accents, section and slide tones |
| `series-1` to `series-4` | Chart series, in the palette's order; the legacy chart names `terracotta`, `blue`, `green`, `purple` map to the same slots |

Graph and diagram node tones (`neutral`, `green`, `red`, `blue`, `amber`, `purple`) are named on the node's `tone` field, not here; see **Node tones**.
