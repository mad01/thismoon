---
type: llm
weight: 2
---
The reply ends with one JSON object for present_create with `title`, `content`, `deck`, and `references`. You see only the reply, so the fixture it was written from is repeated here.

Fixture: search outage on 2026-09-30. Timeline: 11:04 latency alert, 11:09 paged, 11:12 index cluster at 100% CPU, 11:31 index rebuilt on the warm standby, 11:38 recovered; 34 minutes degraded, 6 of them a full outage. Cause: a catalog import of 2.1 million rows ran the reindex in the foreground with no rate limit. Error rate per five minutes: 11:00 0.2%, 11:05 4.1%, 11:10 18.5%, 11:15 22.0%, 11:20 21.4%, 11:25 19.8%, 11:30 9.3%, 11:35 1.1%, 11:40 0.2%. Actions: rate-limit the reindex to 5000 rows per second (catalog, PR #431 open at https://github.com/example/shop/pull/431); move imports to the standby first (platform, proposal due 2026-10-14); CPU alert at 80% on the index cluster (done). Risk: the standby is one node.

PASS if all of these hold for the `deck`:

1. `deck` is a Doc of its own with `summary` and `sections`, and `content` is a brief that holds more detail than the deck (the full timeline, the cause, the actions, the references).
2. The deck has between 5 and 12 sections.
3. Every section heading except the closing one ("Next" or a close equivalent) and any `layout: "section"` divider states a claim or a finding ("A foreground reindex took search down for 34 minutes"), not a topic ("Timeline", "Cause", "Background", "Actions").
4. Every slide holds one idea: a `list` of at most 5 items, or a `p` of at most two sentences, or one visual, or a `p` with a `kv` of at most four rows, or a `list` with one `callout`. A visual is a chart, a diagram, a graph, an image, a lone `stat` or `quote`, or a row of two or three `stat` blocks inside one `columns` block. A chart beside one short `p` in `columns` counts as one visual. A `layout: "section"` divider holds its heading and at most one line. No slide has both a paragraph of three or more sentences and a list.
5. The error rate is a `chart` block of kind `line` or `area` on its own slide, with the nine values from the fixture.
6. Every source number is one from the fixture, or is computed from fixture numbers (a duration between two times, a total, a percentage).
7. The closing section is the ask: an `h` of "Next" or a close equivalent, with a `list` of at most three actions.

FAIL if any of these hold: the deck is missing; the reply tried to call an MCP tool instead of writing the JSON; there is no JSON object; the deck has fewer than 5 or more than 12 sections; two or more headings other than the closing one are topics rather than claims; a slide holds a `panel`, a `details`, a `code` block, or a chart with `frame: true`; the deck has more than one `callout`; there is an agenda slide or a "questions" slide; the error rate is a table or a bar chart; a number appears that the fixture does not contain and cannot be computed from it.

Do not fail for any of these: the closing "Next" heading; a row of two or three stats in one `columns` block; a stat that does not stand alone; a `layout: "section"` divider whose heading is a part name; derived durations or percentages; tone names on sections (`blue`, `green`, `amber`, `red`, `purple`); `notes` or `reveal` fields; a `details` block in `content`.
