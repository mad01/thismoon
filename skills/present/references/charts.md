# Charts: sankey, ribbon, and steps

SKILL.md's **Chart format** picks the kind and lists the fields. This file holds the three shapes that differ from a plain series chart: the `sankey`, the `ribbon`, and a chart with `steps`. The kinds, in the renderer's order: `bar`, `line`, `area`, `sparkline`, `stacked-bar`, `horizontal-bar`, `doughnut`, `scatter`, `sankey`, `ribbon`.

## Sankey format

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

## Ribbon format

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

A ribbon takes no more categories than the palette has series colours (`ribbon.series` in [limits.md](limits.md)): fold the small ones into one named "other". Keep it to about eight periods; past that the ribbons turn into a hairball, so split the periods across slides.

## Stepped charts

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

A `diagram` walks its elements the same way, with `step` on nodes, groups, and edges and `focus` ids on a step; see [diagrams.md](diagrams.md). A `ribbon` walks its periods instead of its series: one caption per period, in order, and step n shows the first n columns with the ribbons into them. The slide opens on the empty axis, and the first Next shows the first period. The step count has to equal the period count, and a series on a ribbon carries no `step`.

Rules the renderer enforces: every caption is non-empty, and a series `step` is between 1 and the number of steps. A `sparkline` carries no steps, a `doughnut` takes caption-only steps (it draws its first series only), and a stepped chart or diagram never sits inside `details`. A `sankey` draws flows, not series, so its steps are captions only. A series without `step` shows from the start. The value axis is pinned to the full data (its positive values), so a series joining later doesn't rescale the ones already shown. On a chart stepped by series the legend lists only the series shown so far. Reduce Motion and a deck with `transition: "none"` swap series without animating.
