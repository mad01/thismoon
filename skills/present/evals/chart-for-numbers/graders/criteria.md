---
type: llm
weight: 2
---
The reply ends with one JSON object for present_create with `title`, `content`, and `references`. You see only the reply, so the fixture it was written from is repeated here.

Fixture: p95 checkout latency in ms per week: week 36: 910, week 37: 940, week 38: 620, week 39: 480, week 40: 455. p50 per week: 210, 215, 180, 150, 148. 5xx responses in week 40 by service: pricing 42, checkout-api 17, stock 9, mailer 3. The pricing connection pool shipped in week 38 (PR #418 at https://github.com/example/shop/pull/418). Follow-ups: raise the pool size on the two remaining replicas (platform, due Friday); add a p95 alert at 600 ms (checkout team). The "checkout latency" dashboard is at https://metrics.example.internal/d/checkout.

PASS if all of these hold:

1. `content` is a Doc: `summary`, `sections` with `h` and `blocks`, and every block has a `t` from the present block set (p, h3, callout, table, kv, list, panel, progress, graph, chart, diagram, code, html, columns, stat, quote, details, image).
2. The latency per week is a `chart` block of kind `line` or `area` with the five weeks on `x` and p95 and p50 as two `series`, values as in the fixture.
3. The 5xx count per service is a `chart` block of kind `bar` or `horizontal-bar` with one series, values as in the fixture.
4. No hex value and no rgb() value anywhere.
5. Every source number is one from the fixture, or is computed from fixture numbers (a total, a difference, a percentage, a ratio).
6. `references` holds the PR and the dashboard with their URLs.

FAIL if any of these hold: the reply tried to call an MCP tool instead of writing the JSON; there is no JSON object; the latency trend is a table, a bar chart, or prose instead of a line or area chart; the 5xx counts are not a chart; a hex or rgb() colour appears; a number appears that the fixture does not contain and that cannot be computed from it.

Do not fail for any of these: the follow-ups in a `table` or a `kv` rather than a `list`; derived totals or percentages; palette role names such as `series-1`, `blue`, `green`, `amber`, `terracotta` in a chart `color`, a section `tone`, or a panel `accent`; extra sections such as raw numbers in a `details` block; a chip or stat that repeats a fixture number.
