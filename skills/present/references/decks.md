# Decks: slides, settings, and the room

The deck rules in SKILL.md say what a slide holds. This file holds the JSON for each kind of slide, the deck's own fields, a full example deck, and how the deck behaves while you present it.

## One skeleton per slide

A slide is one entry in the deck Doc's `sections`. Copy the skeleton for the slide you need and replace every `<<...>>`. The slide's heading is the claim, and the block under it is the one thing that proves it. The numbers the placeholders name by id are in [limits.md](limits.md).

**Title.** The title slide is the deck Doc's own fields, not a section. `summary` is the sentence the room should remember; the chips carry two or three headline numbers. `presenter` is optional: leave it out when no speaker is named.

```json
{
  "summary": "<<the one sentence the room should remember>>",
  "meta": "<<date · occasion · audience>>",
  "presenter": "<<name, team>>",
  "chips": [
    {"text": "<<headline number>>", "style": "stat"},
    {"text": "<<headline number>>", "style": "stat"}
  ],
  "sections": []
}
```

**Claim over a chart.** The heading says what the chart proves; the chart's `title` names the measure. Draw only the series the claim needs.

```json
{
  "h": "<<what the chart proves, with its number>>",
  "blocks": [
    {"t": "chart", "kind": "line", "title": "<<the measure and its unit>>", "unit": "<<unit>>", "series": [
      {"name": "<<series>>", "points": [{"x": "<<period>>", "y": 0}, {"x": "<<period>>", "y": 0}]}
    ]}
  ]
}
```

**Claim over a diagram.** The diagram stands alone and fills the slide; it stays within `diagram.slide_boxes`, and its longest path within `diagram.slide_path`. Give it `steps` when the room should watch a request move through it. The slide opens with no step caption, and the first Next shows caption 1. End the walk on a step with no `focus`, so the whole picture lights under the conclusion.

```json
{
  "h": "<<how the parts fit, as a claim>>",
  "blocks": [
    {"t": "diagram", "caption": "<<what the picture shows>>", "nodes": [
      {"id": "a", "label": "<<box name>>", "text": "<<one line>>"},
      {"id": "b", "label": "<<box name>>", "text": "<<one line>>", "step": 2}
    ], "edges": [
      {"from": "a", "to": "b", "label": "<<what moves>>", "step": 2}
    ], "steps": [
      {"caption": "<<caption 1, shown on the first Next>>"},
      {"caption": "<<what happens next>>", "focus": ["b"]},
      {"caption": "<<the conclusion, with the whole picture lit>>"}
    ]}
  ]
}
```

**Claim beside a visual.** `layout: "split"` puts the heading and a few words in a left column, two fifths of the slide, and one visual in the right column. The words stay within `split.words`, and the heading wraps in its column, so keep it short. The visual is a `chart`, a `diagram`, the `graph`, an `image`, or a `terminal`: exactly one, at the top level, never in `details`, and the slide takes no `columns` block (`split.visuals`). It sizes to the column, and a diagram's longest path stays within the column's `diagram.slide_path`. A narrow window stacks the text above the visual.

```json
{
  "h": "<<what the visual proves, short>>",
  "layout": "split",
  "blocks": [
    {"t": "p", "text": "<<a line or two the visual does not say>>"},
    {"t": "chart", "kind": "bar", "title": "<<the measure and its unit>>", "series": [
      {"name": "<<series>>", "points": [{"x": "<<label>>", "y": 0}, {"x": "<<label>>", "y": 0}]}
    ]}
  ]
}
```

**Big number.** One `stat` alone is the big-number slide. The `sub` line gives it a comparison the room already knows.

```json
{
  "h": "<<the number as a claim>>",
  "blocks": [
    {"t": "stat", "value": "<<number with unit>>", "label": "<<what it counts>>", "sub": "<<a comparison the room already knows>>"}
  ]
}
```

**Stats in a row.** Two or three `stat` blocks in one `columns` block, centred. `deck.text_only` counts default-layout and split slides only, so this centred slide does not lower the count; a lone big-number slide, a figure on a default-layout slide, or a split slide does.

```json
{
  "h": "<<what the numbers say together, as a claim>>",
  "layout": "center",
  "blocks": [
    {"t": "columns", "cols": [
      [{"t": "stat", "value": "<<number>>", "label": "<<what it counts>>"}],
      [{"t": "stat", "value": "<<number>>", "label": "<<what it counts>>"}],
      [{"t": "stat", "value": "<<number>>", "label": "<<what it counts>>"}]
    ]}
  ]
}
```

**Statement.** The heading is the slide: the one sentence the deck exists to say, once or twice a deck.

```json
{
  "h": "<<the one sentence the deck exists to say>>",
  "layout": "statement",
  "blocks": []
}
```

**Divider.** Opens a part of a deck of nine slides or more. Its heading may be the part's name, and its tone colours the part.

```json
{
  "h": "<<the part's name>>",
  "layout": "section",
  "tone": "blue",
  "blocks": []
}
```

**List.** Three to five items under ten words each; `reveal` when the list is an argument built one line at a time. What you would say goes in `notes`.

```json
{
  "h": "<<the claim the items support>>",
  "reveal": true,
  "notes": "<<what you say while the items appear>>",
  "blocks": [
    {"t": "list", "items": ["<<item, under ten words>>", "<<item, under ten words>>", "<<item, under ten words>>"]}
  ]
}
```

**Quote.** One `quote` alone is the quote slide.

```json
{
  "h": "<<what the quote shows, as a claim>>",
  "blocks": [
    {"t": "quote", "text": "<<the line someone said>>", "cite": "<<who, and where>>"}
  ]
}
```

**Options table.** One sentence states what the table compares, and the table holds at most four columns and five rows (deck rule 6). The full comparison belongs in the brief.

```json
{
  "h": "<<the options differ on one thing, as a claim>>",
  "blocks": [
    {"t": "p", "text": "<<one sentence on what the table compares>>"},
    {"t": "table", "cols": ["<<option>>", "<<criterion>>", "<<criterion>>"], "rows": [
      ["<<option>>", "<<a phrase>>", "<<a phrase>>"],
      ["<<option>>", "<<a phrase>>", "<<a phrase>>"]
    ]}
  ]
}
```

**Next.** The last authored slide: at most three actions, each with its owner and date when the source names them. The references follow by themselves.

```json
{
  "h": "Next",
  "blocks": [
    {"t": "list", "ordered": true, "items": ["<<an action, with its owner and date when the source names them>>", "<<an action, with its owner and date when the source names them>>"]}
  ]
}
```

## What each setting is for

| Setting | For | Not for |
|---------|-----|---------|
| `layout: "default"` | a heading over a list, a paragraph, a chart, a diagram: most slides | |
| `layout: "center"` | one short thing centred: a row of figures, a single line | a list, a chart |
| `layout: "statement"` | the one sentence the deck exists to say, the heading as the slide, once or twice a deck | a slide with body under it beyond one line |
| `layout: "section"` | a divider opening a part of a longer deck, with a `tone` | a deck under nine slides |
| `layout: "split"` | a claim with a line or two beside the one visual that proves it; the visual takes three fifths of the slide | two visuals, a list longer than three short items, a diagram with a long path |
| `tone` | colouring a part's heading rule and accent bar so the room sees where it is; a saturated role, the same one for the whole part | decoration on every slide, a `-bg` role |
| `reveal: true` | a list that is an argument built one line at a time | a list the room should read whole, a slide whose one block is not a list |
| chart or diagram `steps` | a picture that should form while you talk, three to five captions | a chart the room reads at a glance |
| `notes` | what you would say and only that; N opens the drawer, nothing reads it aloud | content the room must read |
| `transition` | `fade` (the default) for most decks; `slide` when the deck reads as a path through material; `none` when the room or the reader wants cuts | |
| `logo`, `logo_position` | a mark in the corner: the repo logo by default, an http(s) URL for another, `"none"` for a bare room | |
| `progress` | `dots` (the default) up to 24 slides and a `bar` past that; `"none"` when the count would distract | |
| `presenter`, `footer` | the byline under the meta line and the bottom strip; the footer defaults to the deck title once either is set | |
| `present_deck` | driving the open deck from the shell or the MCP: `start`, `next`, `prev`, `goto`, `stop`; every open tab follows within a second | a shared instance, which has no remote control |
| Reduce Motion | the reader's operating-system setting: every transition, reveal, draw-in, step, and flow animation becomes a cut or a static frame. Nothing to set; write the deck so it reads without motion | |

## Deck-level fields

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
  "summary": "A stale DNS cache took checkout down for 41 minutes.",
  "presenter": "Alex, platform · 2026-10-01",
  "footer": "Incident review, checkout 502s",
  "progress": "bar",
  "sections": []
}
```

## Example deck

An incident review, seven slides plus the references the page already carries, with a presenter byline. The repo's [sample-deck.json](https://github.com/mad01/thismoon/blob/main/services/present/internal/render/testdata/sample-deck.json) is a longer one that uses every layout, tone, notes, reveal, block, and chrome field.

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
         "series": [{"name": "5xx", "color": "series-1", "points": [
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

## In the room

The reader opens the deck from the brief's Slides link or at `deck_url`, and moves with Right, Space, or PageDown (next), Left, PageUp, or Backspace (previous), Home and End. On a `reveal` slide Next shows the next step and Prev hides the last one; Home, End, a dot, or a link lands with every step shown. A chart or diagram with `steps` walks them the same way before the slide moves on, and a jump lands on the finished picture. N opens the speaker notes drawer. F or P starts presenting (chrome hidden, one slide filling the window, browser fullscreen when allowed). Pressed in fullscreen it ends, pressed while presenting without fullscreen (after a reload) it asks for fullscreen again; Escape always ends it. The URL's `#3` names the slide, so a link can open on one. An update to the deck reloads the open tab on the same slide, still presenting if it was, so you can edit a deck mid-talk. You can drive the open deck too: `present_deck(id, "start")`, then `"next"`, `"prev"`, `"goto"` with a `slide`, and `"stop"`; every open tab of the deck follows within a second. Open the deck first with `present_open(id, deck: true)`. The header's Audio toggle hides the read-aloud bar, play buttons, and badges. A deck opens with them hidden, so the room never sees them unless the reader turns them on; a brief keeps its own choice.

A deck holding a `terminal` needs the live terminal on. Run `present terminal on` before the talk and `present terminal off` after it; off ends every session the deck started. In browser fullscreen the browser takes Escape for itself and presenting ends. A Chromium browser on `localhost` or https lets a focused terminal keep Escape, and holding Escape still leaves fullscreen. So a demo that needs Escape, such as interrupting Claude, opens the deck on `localhost` in Chrome. Safari, Firefox, and a plain http host such as `http://present.this` leave fullscreen on Escape.

How a terminal session behaves while you talk:

- **A block that has not started yet keeps asking** every few seconds and starts by itself once the switch is on and its slide is the one showing; a block whose session ended waits for Run. `present terminal status` prints which. `present terminal off` ends every session within two seconds, and each open block shows the off note.
- **The block attaches when it is first seen**: scrolled into view in the brief, on its slide's first showing in the deck, or on Run. The first attach starts the session and types the `prefill`. Every later attach replays the last 1 MiB of output and takes input again, and so does the reload after every update.
- **What a session runs comes from the stored page.** Serve reads `cmd`, `cwd`, and `prefill` from the page's own block with that session name, never from the browser. A running session keeps what it started with, so a changed `cmd` takes effect after Kill and Run.
- **Run and Kill sit in the block's header.** Kill ends the process, and the block stays killed through that tab's reloads until Run. Run on a session that has ended starts it again.
- **`cmd` runs in an interactive login shell**, `zsh -l -i -c <cmd>`, so the PATH set in `~/.zshrc` applies. Without `cmd` the session is the login shell. A missing `cwd` directory stops the session from starting, and the block shows why.
- **A session ends by itself** after two hours with no tab attached, and when serve restarts. A `ralph up` that rebuilds present restarts serve, so run none during a talk.

## What the lint catches in a deck

Each pair below is generated from the lint's own test fixtures: a fragment that trips one rule, the warning the lint returns for it, and the same fragment repaired.

### Words on a slide

<!-- pair:slide.words -->
This trips `slide.words`:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "Five steps bring the fleet up, with one sudo",
        "blocks": [
          {
            "t": "list",
            "ordered": true,
            "items": [
              "Install ralph and run `ralph init`",
              "Create your private config repo from `examples/dotfiles/`",
              "Declare profiles: `ralph profile set personal`",
              "Add the thismoon source and run `ralph up`",
              "Write `routes.toml` and register d-man: the one sudo"
            ]
          },
          {
            "t": "callout",
            "sev": "warn",
            "text": "Set profiles **before the first `ralph up`**: a machine with none silently skips every profile-gated recipe and source, and only `ralph doctor` will say so."
          }
        ]
      },
      {
        "h": "One sudo is the whole cost of setup",
        "blocks": [
          {
            "t": "stat",
            "value": "1",
            "label": "sudo step in the whole setup"
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `deck.sections[0]` slide 2 "Five steps bring the fleet up, with one sudo": 60 words outside the heading. A room reads a slide at a glance; the detail belongs in the brief or the speaker notes. Aim for 50 or fewer.

Repaired:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "Five steps bring the fleet up, with one sudo",
        "notes": "A machine with no profile silently skips every profile-gated recipe and source, and only ralph doctor says so.",
        "blocks": [
          {
            "t": "list",
            "ordered": true,
            "items": [
              "Install ralph: `ralph init`",
              "Create your config repo from `examples/dotfiles/`",
              "Set profiles: `ralph profile set personal`",
              "Add the thismoon source, then `ralph up`",
              "Write `routes.toml`, register d-man: the one sudo"
            ]
          },
          {
            "t": "callout",
            "sev": "warn",
            "text": "Set profiles **before the first `ralph up`**."
          }
        ]
      },
      {
        "h": "One sudo is the whole cost of setup",
        "blocks": [
          {
            "t": "stat",
            "value": "1",
            "label": "sudo step in the whole setup"
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Words beside a split slide's visual

<!-- pair:split.words -->
This trips `split.words`:

```json
{
  "title": "Checkout after the cache",
  "deck": {
    "summary": "A read-through cache in front of pricing cut checkout latency by two fifths.",
    "sections": [
      {
        "h": "The pricing cache cut checkout p99 by 40%",
        "layout": "split",
        "blocks": [
          {
            "t": "p",
            "text": "We put a read-through cache in front of the pricing service on 2 October. Checkout p99 fell from 1.2 s to 720 ms within a day, and it has held there through two sales since."
          },
          {
            "t": "chart",
            "kind": "line",
            "title": "Checkout p99 latency",
            "unit": "ms",
            "series": [
              {
                "name": "p99",
                "points": [
                  {"x": "Sep 29", "y": 1210},
                  {"x": "Sep 30", "y": 1190},
                  {"x": "Oct 1", "y": 1230},
                  {"x": "Oct 2", "y": 940},
                  {"x": "Oct 3", "y": 730},
                  {"x": "Oct 4", "y": 720}
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `deck.sections[0]` slide 2 "The pricing cache cut checkout p99 by 40%": 35 words beside the visual. The left column is under two fifths of the slide's width. In the default face, 15 words under a four-line claim fill it at full size, and more shrink the claim and the visual with them. Aim for 15 or fewer, and move the rest to the speaker notes.

Repaired:

```json
{
  "title": "Checkout after the cache",
  "deck": {
    "summary": "A read-through cache in front of pricing cut checkout latency by two fifths.",
    "sections": [
      {
        "h": "The pricing cache cut checkout p99 by 40%",
        "layout": "split",
        "notes": "We put the cache in front of pricing on 2 October, and p99 has held at 720 ms through two sales since.",
        "blocks": [
          {
            "t": "p",
            "text": "Live since **2 October**: checkout p99 went from 1.2 s to 720 ms."
          },
          {
            "t": "chart",
            "kind": "line",
            "title": "Checkout p99 latency",
            "unit": "ms",
            "series": [
              {
                "name": "p99",
                "points": [
                  {"x": "Sep 29", "y": 1210},
                  {"x": "Sep 30", "y": 1190},
                  {"x": "Oct 1", "y": 1230},
                  {"x": "Oct 2", "y": 940},
                  {"x": "Oct 3", "y": 730},
                  {"x": "Oct 4", "y": 720}
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Slides without a picture

<!-- pair:deck.text_only -->
This trips `deck.text_only`:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "Hooks fire every time; prompt instructions get skimmed",
        "layout": "statement",
        "tone": "primary",
        "blocks": [
          {
            "t": "p",
            "text": "Standalone, each tool is a thing you invoke. Together, they invoke each other."
          }
        ]
      },
      {
        "h": "Three memory stores, one question each",
        "tone": "blue",
        "blocks": [
          {
            "t": "list",
            "items": [
              "agent-memory: durable facts about **you**",
              "worklog: where a task stands, keyed by ticket",
              "events: what actually happened, on one timeline"
            ]
          }
        ]
      },
      {
        "h": "Only t-man and d-man may be hard dependencies across the boundary",
        "blocks": [
          {
            "t": "list",
            "items": [
              "t-man runs every service as a launchd agent",
              "d-man gives each service its `<name>.this` address",
              "Everything else orders across the boundary **by wave**"
            ]
          }
        ]
      },
      {
        "h": "One sudo is the whole cost of setup",
        "blocks": [
          {
            "t": "stat",
            "value": "1",
            "label": "sudo step in the whole setup"
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `deck` the deck: 2 of 3 default-layout and split slides hold only prose or a list. A claim shown beside a picture is remembered better than the same claim in text alone. Give the strongest of them a chart, a diagram, or one big number.

Repaired:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "Hooks fire every time; prompt instructions get skimmed",
        "layout": "statement",
        "tone": "primary",
        "blocks": [
          {
            "t": "p",
            "text": "Standalone, each tool is a thing you invoke. Together, they invoke each other."
          }
        ]
      },
      {
        "h": "Three memory stores, one question each",
        "tone": "blue",
        "blocks": [
          {
            "t": "stat",
            "value": "3",
            "label": "stores: facts in agent-memory, task state in worklog, what happened in events"
          }
        ]
      },
      {
        "h": "Only t-man and d-man may be hard dependencies across the boundary",
        "blocks": [
          {
            "t": "list",
            "items": [
              "t-man runs every service as a launchd agent",
              "d-man gives each service its `<name>.this` address",
              "Everything else orders across the boundary **by wave**"
            ]
          }
        ]
      },
      {
        "h": "One sudo is the whole cost of setup",
        "blocks": [
          {
            "t": "stat",
            "value": "1",
            "label": "sudo step in the whole setup"
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Diagrams on a slide

<!-- pair:diagram.slide_boxes -->
This trips `diagram.slide_boxes`:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "A wired session uses tools nobody asked for",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "Purple: you and the agent. Red: guardrails. Blue: shared stores. Green: shared services.",
            "nodes": [
              {
                "id": "agent",
                "label": "Your session",
                "text": "you and your agent in a repo",
                "kind": "person",
                "tone": "purple"
              },
              {
                "id": "belt",
                "label": "belt",
                "text": "session hooks: guards and hints",
                "tone": "red"
              },
              {
                "id": "memory",
                "label": "agent-memory",
                "text": "facts you told any agent, any machine",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "moondb",
                "label": "moondb",
                "text": "other sessions' uncommitted edits",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "csl",
                "label": "csl",
                "text": "zoekt index, also at csl.this",
                "tone": "green"
              },
              {
                "id": "suspenders",
                "label": "suspenders",
                "text": "pre-commit hook, same name rules",
                "tone": "red"
              },
              {
                "id": "events",
                "label": "events",
                "text": "every deny and hint on one timeline",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "present",
                "label": "present",
                "text": "briefing pages at present.this",
                "tone": "green"
              }
            ],
            "edges": [
              {
                "from": "memory",
                "to": "belt",
                "label": "index"
              },
              {
                "from": "moondb",
                "to": "belt",
                "label": "edits"
              },
              {
                "from": "belt",
                "to": "agent",
                "label": "injects"
              },
              {
                "from": "agent",
                "to": "csl",
                "label": "search"
              },
              {
                "from": "agent",
                "to": "suspenders",
                "label": "commit"
              },
              {
                "from": "belt",
                "to": "events",
                "label": "denies and hints",
                "flow": true
              },
              {
                "from": "agent",
                "to": "present",
                "label": "findings"
              }
            ]
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `deck.sections[0].blocks[0]` slide 2 "A wired session uses tools nobody asked for", a diagram: 8 boxes. A slide shrinks a diagram to fit, so a fuller diagram draws its text under the size a room can read. Aim for 6 or fewer, and leave the full picture to the brief.

Repaired:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "A wired session uses tools nobody asked for",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "Purple: you and the agent. Red: guardrails. Blue: shared stores. Green: shared services.",
            "nodes": [
              {
                "id": "agent",
                "label": "Your session",
                "text": "you and your agent in a repo",
                "kind": "person",
                "tone": "purple"
              },
              {
                "id": "belt",
                "label": "belt",
                "text": "session hooks: guards and hints",
                "tone": "red"
              },
              {
                "id": "memory",
                "label": "agent-memory",
                "text": "facts you told any agent, any machine",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "moondb",
                "label": "moondb",
                "text": "other sessions' uncommitted edits",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "csl",
                "label": "csl",
                "text": "zoekt index, also at csl.this",
                "tone": "green"
              },
              {
                "id": "suspenders",
                "label": "suspenders",
                "text": "pre-commit hook, same name rules",
                "tone": "red"
              }
            ],
            "edges": [
              {
                "from": "memory",
                "to": "belt",
                "label": "index"
              },
              {
                "from": "moondb",
                "to": "belt",
                "label": "open edits"
              },
              {
                "from": "belt",
                "to": "agent",
                "label": "injects"
              },
              {
                "from": "agent",
                "to": "csl",
                "label": "search"
              },
              {
                "from": "agent",
                "to": "suspenders",
                "label": "commit"
              }
            ]
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

<!-- pair:diagram.slide_path -->
This trips `diagram.slide_path`:

```json
{
  "title": "How a change reaches the fleet",
  "deck": {
    "summary": "Every machine pulls main and rebuilds, so a merge is the release.",
    "sections": [
      {
        "h": "Merging to main deploys the change to every machine",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "Purple is the developer, blue is GitHub, green the fleet.",
            "nodes": [
              {
                "id": "dev",
                "label": "Developer",
                "text": "opens a pull request",
                "kind": "person",
                "tone": "purple"
              },
              {
                "id": "pr",
                "label": "Pull request",
                "text": "review and checks",
                "tone": "blue"
              },
              {
                "id": "main",
                "label": "main",
                "text": "squash merge",
                "tone": "blue"
              },
              {
                "id": "ralph",
                "label": "ralph up",
                "text": "pulls main and rebuilds",
                "tone": "green"
              },
              {
                "id": "svc",
                "label": "present service",
                "text": "restarted by t-man",
                "tone": "green"
              }
            ],
            "edges": [
              {
                "from": "dev",
                "to": "pr",
                "label": "push"
              },
              {
                "from": "pr",
                "to": "main",
                "label": "merge"
              },
              {
                "from": "main",
                "to": "ralph",
                "label": "pull"
              },
              {
                "from": "ralph",
                "to": "svc",
                "label": "restart"
              }
            ]
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `deck.sections[0].blocks[0]` slide 2 "Merging to main deploys the change to every machine", a diagram: its path of 5 boxes, "Developer" to "present service", is about 1093 px wide in a row and 534 px tall in a column, past the 899 by 303 px the slide holds at its smallest text. A slide shrinks a diagram until it fits. Every box and labelled arrow on the path adds a layer, and a group adds its border and lays out whole, so a path too long for a row and a column draws its labels under the size a room can read. The lint estimates the path from its box text, edge labels, and groups in the page's default face. A wider box beside the path adds to it, so a path it passes can still draw small. Drop or shorten the edge labels on that path, fold boxes together, or drop a group; else show part of the flow on this slide and leave the whole to the brief.

Repaired:

```json
{
  "title": "How a change reaches the fleet",
  "deck": {
    "summary": "Every machine pulls main and rebuilds, so a merge is the release.",
    "sections": [
      {
        "h": "Merging to main deploys the change to every machine",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "Purple is the developer, blue is GitHub, green the fleet.",
            "nodes": [
              {
                "id": "dev",
                "label": "Developer",
                "text": "opens a pull request",
                "kind": "person",
                "tone": "purple"
              },
              {
                "id": "pr",
                "label": "Pull request",
                "text": "review and checks",
                "tone": "blue"
              },
              {
                "id": "main",
                "label": "main",
                "text": "squash merge",
                "tone": "blue"
              },
              {
                "id": "fleet",
                "label": "ralph up",
                "text": "rebuilds and restarts",
                "tone": "green"
              }
            ],
            "edges": [
              {
                "from": "dev",
                "to": "pr"
              },
              {
                "from": "pr",
                "to": "main",
                "label": "merge"
              },
              {
                "from": "main",
                "to": "fleet",
                "label": "pull"
              }
            ]
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

<!-- pair:diagram.slide_columns -->
This trips `diagram.slide_columns`:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "Three layers keep the repo public and the setup personal",
        "blocks": [
          {
            "t": "columns",
            "cols": [
              [
                {
                  "t": "diagram",
                  "direction": "TB",
                  "caption": "Green is public code, purple is yours; the amber boundary is what stays private.",
                  "groups": [
                    {
                      "id": "public",
                      "label": "public"
                    },
                    {
                      "id": "private",
                      "label": "private",
                      "tone": "amber"
                    }
                  ],
                  "nodes": [
                    {
                      "id": "thismoon",
                      "label": "thismoon",
                      "text": "Components and recipes: the code and its install",
                      "tone": "green",
                      "group": "public"
                    },
                    {
                      "id": "ralph",
                      "label": "ralph",
                      "text": "Installer: reconciles machines against TOML recipes",
                      "tone": "green",
                      "group": "public"
                    },
                    {
                      "id": "config",
                      "label": "Your config repo",
                      "text": "Secrets, hosts, hooks, overlays",
                      "tone": "purple",
                      "group": "private"
                    }
                  ],
                  "edges": [
                    {
                      "from": "thismoon",
                      "to": "ralph",
                      "label": "consumed by"
                    },
                    {
                      "from": "ralph",
                      "to": "config",
                      "label": "configured by"
                    }
                  ]
                }
              ],
              [
                {
                  "t": "p",
                  "text": "thismoon's recipes carry **only what is portable**: build, install, service registration."
                }
              ]
            ]
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `deck.sections[0].blocks[0].cols[0][0]` slide 2 "Three layers keep the repo public and the setup personal", a diagram: it sits inside columns. In part of the slide's width the diagram's text draws under the size a room can read. Give it the slide to itself and move the text beside it to the speaker notes.

Repaired:

```json
{
  "title": "How the fleet fits together",
  "deck": {
    "summary": "Our tools invoke each other: hooks, hints and shared services fire on every session without anyone remembering.",
    "sections": [
      {
        "h": "Three layers keep the repo public and the setup personal",
        "notes": "thismoon's recipes carry **only what is portable**: build, install, service registration.",
        "blocks": [
          {
            "t": "diagram",
            "direction": "TB",
            "caption": "Green is public code, purple is yours; the amber boundary is what stays private.",
            "groups": [
              {
                "id": "public",
                "label": "public"
              },
              {
                "id": "private",
                "label": "private",
                "tone": "amber"
              }
            ],
            "nodes": [
              {
                "id": "thismoon",
                "label": "thismoon",
                "text": "Components and recipes: the code and its install",
                "tone": "green",
                "group": "public"
              },
              {
                "id": "ralph",
                "label": "ralph",
                "text": "Installer: reconciles machines against TOML recipes",
                "tone": "green",
                "group": "public"
              },
              {
                "id": "config",
                "label": "Your config repo",
                "text": "Secrets, hosts, hooks, overlays",
                "tone": "purple",
                "group": "private"
              }
            ],
            "edges": [
              {
                "from": "thismoon",
                "to": "ralph",
                "label": "consumed by"
              },
              {
                "from": "ralph",
                "to": "config",
                "label": "configured by"
              }
            ]
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->
