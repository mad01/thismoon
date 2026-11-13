<!-- Generated from services/present/internal/limits by `make limits-doc` in services/present. Do not edit by hand. -->

# Limits

Every number present holds a page to. A **warning** comes from the lint: present_create, present_update, and present_patch_section return it in `warnings`, and `present lint` prints it. The page is stored either way. A **refusal** comes from the renderer: the page is not stored, and the error names the block and the rule.

The source says how strong a number is: **study** is a measured result, **guideline** a published style guide, **house** present's own number, and **renderer** a fact of how present draws a page.

## Warnings

In the order the lint reports them.

| Id | Limit | Applies to | Source |
|---|---|---|---|
| `summary.words` | 50 words | the summary of a brief or a deck | house |
| `heading.label` | 2 words | section headings in a brief, slide headings in a deck | guideline (assertion-evidence studies, one research group) |
| `brief.figure_gap` | 4 sections | the sections of a brief | house |
| `brief.paragraph` | 70 words | the paragraphs of a brief, in columns and details too | house |
| `table.rows` | 8 rows | every table, brief and deck | house |
| `table.cell` | 160 characters | every table cell, brief and deck | guideline (Google developer style: more than two sentences in a cell belong elsewhere) |
| `diagram.boxes` | 9 boxes | a diagram in a brief | house |
| `diagram.chain` | 5 boxes | a diagram, brief and deck | renderer |
| `diagram.row` | 1032 px | a left-to-right diagram in a brief | renderer |
| `diagram.slide_boxes` | 6 boxes | a diagram on a slide | house |
| `diagram.slide_path` | 899 px | a diagram on a slide | renderer |
| `diagram.slide_columns` | none | a diagram on a slide | renderer |
| `diagram.key_unused` | none | a diagram's key, brief and deck | house |
| `slide.words` | 50 words | a slide, outside its heading | house |
| `split.words` | 15 words | the left column of a split slide, outside its heading | house |
| `deck.text_only` | 50 percent | the default-layout and split slides of a deck | study (one study, 110 students) |
| `inline.stray_mark` | none | every text field, brief and deck | renderer |
| `placeholder.left` | none | every text field, brief and deck | house |

- `summary.words`: The summary holds at most 50 words. The summary sits above the first section; a longer one pushes the first heading and figure off the first screen.
- `heading.label`: A heading is the claim its section proves, a sentence with a verb, not a label of 2 words or fewer or a word on the label list. A reader who skims only the headings should leave with the argument; a label names the topic and nothing else.
- `brief.figure_gap`: At most 4 sections in a row go without a figure (a chart, diagram, graph, image, or stat). A long run of text-only sections reads as a wall; a figure gives the reader who skims a place to land.
- `brief.paragraph`: A paragraph (a `p` block) in a brief holds at most 70 words. A reader who skims passes over a long block of text, and one who listens loses its thread before it ends.
- `table.rows`: A table holds at most 8 body rows. Past that a reader scans instead of comparing; keep the rows the point needs and move the full list into details.
- `table.cell`: A table cell holds at most 160 characters. A cell is read across a row; a paragraph in it breaks the row, so the prose belongs beside the table.
- `diagram.boxes`: A diagram in a brief holds at most 9 boxes. A bigger diagram draws taller than a screen at a size the reader can read; split it or show one part.
- `diagram.chain`: The longest path through a diagram passes at most 5 boxes. Each box on the path is one layer of the layout, and every layer adds height in any direction.
- `diagram.row`: A left-to-right diagram's longest path, laid out in a row in the compact form, is at most 1032 px wide, the brief's widest. A wider row may draw top to bottom instead, a column as tall as the path is long; the reader's browser picks the form, so the lint can only estimate. It estimates the width from the box text and edge labels on the path in the page's default face; groups, branches, and return edges widen a row further, so a row it passes can still turn.
- `diagram.slide_boxes`: A diagram on a slide holds at most 6 boxes. A slide shrinks a diagram to fit, so a fuller diagram draws its text under the size a room can read.
- `diagram.slide_path`: A slide diagram's longest path fits the slide at its smallest text. A row holds at most 899 px, a column 345 px, or 303 px under steps or a caption. A slide shrinks a diagram until it fits. Every box and labelled arrow on the path adds a layer, and a group adds its border and lays out whole, so a path too long for a row and a column draws its labels under the size a room can read. The lint estimates the path from its box text, edge labels, and groups in the page's default face. A wider box beside the path adds to it, so a path it passes can still draw small.
- `diagram.slide_columns`: A diagram on a slide sits alone, not inside columns. In part of the slide's width the diagram's text draws under the size a room can read.
- `diagram.key_unused`: Every key item names a tone a box or group of its diagram carries. A swatch no box matches sends the reader looking for a colour the picture does not hold.
- `slide.words`: A slide holds at most 50 words outside its heading; of a stepped chart or diagram only the longest caption counts. A room reads a slide at a glance; the detail belongs in the brief or the speaker notes.
- `split.words`: A split slide holds at most 15 words beside its visual, outside its heading. The left column is under two fifths of the slide's width. In the default face, 15 words under a four-line claim fill it at full size, and more shrink the claim and the visual with them.
- `deck.text_only`: At most half (50 percent) of the default-layout and split slides hold only prose or a list. A claim shown beside a picture is remembered better than the same claim in text alone.
- `inline.stray_mark`: No backtick or pair of stars sits in a field drawn as plain text, and a field that takes inline markdown holds an even number of backticks. The marks print as they are, so the reader sees raw markdown instead of code or bold.
- `placeholder.left`: No text holds a skeleton placeholder between << and >>. A placeholder left in prints the note to the author on the page.

The label list for `heading.label`: overview, background, summary, introduction, context, details, conclusion, results, findings, analysis, discussion, problem, solution, approach, architecture, timeline, status, update, notes, other, what changed, how it works, key points, takeaways, next steps. Exempt: a heading that opens with "Next" or "References", the page title, a section divider slide (`layout: "section"`), and the ADHD mode's "Do now", "Now", and "Later".

A figure is a `chart`, `diagram`, `graph`, `image`, `stat` block at the top of a section or in a column; one inside `details` does not count, since the disclosure starts closed.

A slide that holds only prose or a list is made of `p`, `h3`, `callout`, `list` blocks, in columns too, or of nothing below its heading.

A slide's words are the words of its blocks' text: paragraphs, lists, tables, callouts, stats, quotes, captions, and chart titles. The labels drawn inside a chart or diagram, an image's alt text, and the speaker notes do not count.

## Reading a warning

A warning has three fields. `rule` is the rule id above. `where` is a path into the tool's arguments, zero-based like a section patch's `index:N`: `content.sections[2].blocks[0]` is the first block of the brief's third section, and `content.sections[3..6]` is a run of sections. `message` names the place the way a reader finds it, then the fault, the reason, and the fix. A slide number counts the title slide as slide 1, the way the deck view does.

Findings come by rule in the order of the table, then from the top of the page: the title, the brief, then the deck. A section patch lists the patched section's findings and counts the rest of the page in `elsewhere`.

## Refusals

| Id | Limit | Applies to | Source |
|---|---|---|---|
| `ribbon.series` | 4 series | a ribbon chart | renderer |
| `diagram.nodes` | 24 boxes | a diagram | renderer |
| `diagram.groups` | 12 groups | a diagram | renderer |
| `diagram.depth` | 2 levels | the groups of a diagram | renderer |
| `diagram.label` | 32 characters | a diagram box or group label | renderer |
| `diagram.text` | 60 characters | a diagram box's text | renderer |
| `diagram.edge_label` | 24 characters | a diagram edge label | renderer |
| `diagram.key_items` | 6 items | a diagram's key | renderer |
| `diagram.key_label` | 24 characters | a diagram key item's label | renderer |
| `terminal.rows_min` | 8 rows | a terminal block that sets rows | renderer |
| `terminal.rows_max` | 60 rows | a terminal block that sets rows | renderer |
| `terminal.session` | 32 characters | a terminal block's session name | renderer |
| `terminal.prefill` | 512 characters | a terminal block's prefill | renderer |
| `split.visuals` | 1 visual | a split slide | renderer |

- `ribbon.series`: A ribbon chart takes at most 4 series. The palette has four series colours; a fifth repeats the first, and ranks that reorder every column make two bands of one colour unreadable.
- `diagram.nodes`: A diagram holds at most 24 boxes. Past it the picture is a hairball, and the layout takes long enough to notice.
- `diagram.groups`: A diagram holds at most 12 groups. Past it the boundaries crowd out the boxes they hold.
- `diagram.depth`: Groups nest at most 2 levels deep: a group inside a group inside the diagram. Each level adds a boundary and its label around the boxes.
- `diagram.label`: A box or group label is at most 32 characters. A box label wraps inside its box, about 27 characters a line in a full box (240 px) and 20 in a compact one (190 px), so it keeps to two lines, three when a long word falls badly; a group label sets the group's least width.
- `diagram.text`: A box's text is at most 60 characters. It wraps under the label, and the cap keeps a box to a few lines.
- `diagram.edge_label`: An edge label is at most 24 characters. The widest label sets the gap between two layers.
- `diagram.key_items`: A key holds at most 6 items, one per tone, and no tone twice. There are six node tones, and a swatch says what one tone means.
- `diagram.key_label`: A key label is at most 24 characters. The key is a row under the diagram at the caption's size, and each long label pushes another item onto a second row; the rest of what a tone means goes in the caption.
- `terminal.rows_min`: A terminal block's rows is at least 8. With fewer a full-screen program has no room to draw.
- `terminal.rows_max`: A terminal block's rows is at most 60. With more the terminal outgrows a slide.
- `terminal.session`: A session name is 1 to 32 of a-z, 0-9, and -. It is a path segment of the attach route and the key serve holds the session under.
- `terminal.prefill`: A prefill is one line of at most 512 characters. It is typed into the session as keystrokes, so a newline would run it.
- `split.visuals`: A split slide holds exactly 1 visual (a chart, diagram, graph, image, or terminal) as a top-level block, and no columns block. The right column shows one visual at the column's size: a second has no room, one in details would start hidden, and a columns block would split the slide again.

## What the lint does not see

- whether a claim is true;
- whether a heading really is a claim: it catches labels only;
- whether the chart kind fits the data;
- anything that needs the drawn page, such as text size or overflow (the looks eval's probes measure that);
- what an `html` block holds.
