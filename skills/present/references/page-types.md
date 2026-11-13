# Page types: skeletons

One skeleton per page type in SKILL.md's **Page types** table. A skeleton is the `present_create` argument object for that type, with the block kinds in the order that carries it and placeholder text between `<<` and `>>` saying what goes in each place. Keep the structure exact and the wording yours:

1. Copy the skeleton for your type.
2. Replace every `<<...>>` with your content. A placeholder left in prints on the page as typed, and the lint warns about it (`placeholder.left`).
3. Repeat what the placeholder says to repeat ("one section per part"), and add or drop a stat, a table row, or a list item to fit the facts. Keep the order of the sections.
4. Leave out a placeholder the source cannot fill. Never invent its content. Simple arithmetic on the source's own numbers, such as a ratio or a count of its entries, is from the source.
5. Serialise `content` and `deck` as JSON strings when you call the tool.

The limits the placeholders name by id (`summary.words`, `diagram.boxes`) are in [limits.md](limits.md), with the reason for each.

## Rules every skeleton follows

- **A figure opens the first section**, so a picture or a number is on the first screen. A reader who stops after one screen still leaves with the point. The contents list sits between the summary and the first figure and takes about one line per two headings. A one-line title, a summary of three lines or fewer, and short headings keep the first figure on screen one.
- **One sentence before each table states its point.** Read-aloud skips tables and kv blocks, so a listener hears the point from the sentence or not at all.
- **The summary fits `summary.words`.** It sits above the first section, and a longer one pushes the first figure off the first screen.
- **Every heading is a claim** (SKILL.md, **Headings are claims**), apart from the closing "Next".
- **A run of sections without a figure stays within `brief.figure_gap`.** Give one of them a figure, or fold small sections together.

With the ADHD mode on, a "Do now" section comes first in any brief type, and the type's skeleton follows it whole. When the skeleton's first section holds a figure (stats, a chart, a diagram), that section stays as section two; nothing is dropped (SKILL.md, **ADHD briefing mode**).

## `decision`: a decision record, a proposal

The reader wants the decision and its price first, then the options and why this one won.

```json
{
  "title": "<<the decision, as a short phrase>>",
  "content": {
    "summary": "<<the decision and what it costs, in one or two sentences>>",
    "meta": "<<date · decision record · status>>",
    "sections": [
      {
        "h": "<<the decision as a claim, with its main number>>",
        "blocks": [
          {"t": "columns", "cols": [
            [{"t": "stat", "value": "<<number>>", "label": "<<what it counts>>", "sub": "<<a comparison the reader already knows>>"}],
            [{"t": "stat", "value": "<<number>>", "label": "<<what it counts>>"}],
            [{"t": "stat", "value": "<<number>>", "label": "<<what it counts>>"}]
          ]},
          {"t": "p", "text": "<<what was decided, and who it changes things for>>"}
        ]
      },
      {
        "h": "<<the winning option beats the rest on the criterion that matters>>",
        "blocks": [
          {"t": "p", "text": "<<one sentence that names the winner and what the table shows>>"},
          {"t": "table", "cols": ["<<option>>", "<<criterion>>", "<<criterion>>"], "rows": [
            ["<<the winner>>", "<<a phrase>>", "<<a phrase>>"],
            ["<<an option>>", "<<a phrase>>", "<<a phrase>>"]
          ]}
        ]
      },
      {
        "h": "<<why this one, as a claim>>",
        "blocks": [
          {"t": "diagram", "caption": "<<what the picture shows>>", "nodes": [
            {"id": "a", "label": "<<box name>>", "text": "<<one line>>"},
            {"id": "b", "label": "<<box name>>", "text": "<<one line>>"}
          ], "edges": [
            {"from": "a", "to": "b", "label": "<<what moves>>"}
          ]},
          {"t": "p", "text": "<<what to look at in the picture; or swap the diagram for a chart when the reason is a number>>"}
        ]
      },
      {
        "h": "<<what it costs, as a claim>>",
        "blocks": [
          {"t": "list", "items": ["<<one cost, one line>>", "<<one cost, one line>>"]}
        ]
      },
      {
        "h": "Next",
        "blocks": [
          {"t": "list", "ordered": true, "items": ["<<an action, with its owner and date when the source names them>>"]}
        ]
      }
    ]
  },
  "references": [{"title": "<<the decision record>>", "url": "https://example.com/<<its address>>"}]
}
```

The diagram stays within `diagram.boxes`: the parts the reason needs, not the whole system.

A small count taken from the source's prose (three options weighed, two services changed) is a fair stat. When the decision has no number, open with the diagram or the options table instead of the stats.

A record that makes more than one choice keeps one options table: add a verdict column, give each option one row, and mark the chosen rows. A record with no alternatives compares the thing chosen with what it replaces, or drops the table.

## `explainer`: how a system fits together

The reader wants the whole picture before the parts, then one request followed through it.

```json
{
  "title": "<<the system's name>>",
  "content": {
    "summary": "<<what the system does and who it serves, in one or two sentences>>",
    "meta": "<<date · explainer · scope>>",
    "sections": [
      {
        "h": "<<the whole system as a claim: what connects to what>>",
        "blocks": [
          {"t": "diagram", "caption": "<<the system and the systems next to it>>", "nodes": [
            {"id": "a", "label": "<<part>>", "text": "<<what it does, one line>>", "tone": "green"},
            {"id": "b", "label": "<<part>>", "text": "<<what it does, one line>>", "tone": "green"},
            {"id": "c", "label": "<<store>>", "text": "<<what it keeps>>", "kind": "store", "tone": "blue"}
          ], "edges": [
            {"from": "a", "to": "b", "label": "<<what moves>>"},
            {"from": "b", "to": "c", "label": "<<what moves>>"}
          ]},
          {"t": "p", "text": "<<what to look at first, and what the tones mean>>"}
        ]
      },
      {
        "h": "<<one part, as a claim about what it does; one section per part>>",
        "blocks": [
          {"t": "p", "text": "<<what the part does, and why it is separate, when the source says>>"},
          {"t": "kv", "kv": [{"k": "<<a setting>>", "v": "<<its value>>"}]}
        ]
      },
      {
        "h": "<<how one request moves, as a claim>>",
        "blocks": [
          {"t": "diagram", "caption": "<<the path of one request or one record, not the first diagram's boxes again>>", "nodes": [
            {"id": "in", "label": "<<entry>>", "text": "<<one line>>"},
            {"id": "work", "label": "<<part>>", "text": "<<one line>>", "step": 2},
            {"id": "out", "label": "<<part>>", "text": "<<one line>>", "step": 3}
          ], "edges": [
            {"from": "in", "to": "work", "label": "<<what moves>>", "step": 2},
            {"from": "work", "to": "out", "label": "<<what moves>>", "step": 3}
          ], "steps": [
            {"caption": "<<what happens first>>"},
            {"caption": "<<what happens next>>", "focus": ["work"]},
            {"caption": "<<where it ends>>", "focus": ["out"]}
          ]}
        ]
      },
      {
        "h": "<<where to look in the code, as a claim>>",
        "blocks": [
          {"t": "p", "text": "<<one sentence on what the table maps>>"},
          {"t": "table", "cols": ["<<part>>", "<<path>>"], "rows": [
            ["<<part>>", "[<<file:line>>](https://example.com/<<permalink>>)"]
          ]}
        ]
      }
    ]
  },
  "references": [{"title": "<<source>>", "url": "https://example.com/<<its address>>"}]
}
```

The first diagram stays within `diagram.boxes` and its longest path within `diagram.chain`. An ordered list can take the stepped diagram's place when the path has no branches.

When the system has more parts than `brief.figure_gap` allows between figures, fold the small parts into a neighbour's section as an `h3` each. The stepped diagram draws a different thing than the whole-system diagram: one request or the records it writes, not the same boxes again. A code-table row may link a directory instead of a `file:line` when the source names packages.

When the system is a map of many nodes, or the traffic on its links is the point, the page's `graph` takes the first diagram's place (SKILL.md, **Graph format**). The request path then becomes an ordered list that names the nodes on the graph, and no diagram is drawn: a reader should meet each node in one picture.

## `walkthrough`: a process or a how-to

The reader wants the outcome and the time it takes, then the steps with the one trap marked.

```json
{
  "title": "<<the process, as a short phrase>>",
  "content": {
    "summary": "<<what the reader has at the end, and how long it takes when the source gives a time>>",
    "meta": "<<date · how-to · who runs it>>",
    "sections": [
      {
        "h": "<<the path at a glance, as a claim>>",
        "blocks": [
          {"t": "columns", "cols": [
            [{"t": "stat", "value": "<<count>>", "label": "steps"}],
            [{"t": "stat", "value": "<<minutes, only when the source gives a time>>", "label": "<<end to end>>"}],
            [{"t": "stat", "value": "<<count>>", "label": "<<checks along the way>>"}]
          ]},
          {"t": "p", "text": "<<the order of the steps in one sentence; or swap the stats for a one-row diagram of the path, labels only>>"}
        ]
      },
      {
        "h": "<<what this step achieves, as a claim; one section per step>>",
        "blocks": [
          {"t": "p", "text": "<<what the step does, in words>>"},
          {"t": "code", "lang": "bash", "text": "<<the exact command>>"}
        ]
      },
      {
        "h": "<<the step with the trap, as a claim>>",
        "blocks": [
          {"t": "p", "text": "<<what the step does, in words>>"},
          {"t": "code", "lang": "bash", "text": "<<the exact command>>"},
          {"t": "callout", "sev": "warn", "text": "<<the one trap and how to avoid it>>"}
        ]
      },
      {
        "h": "<<how the reader knows it worked, as a claim>>",
        "blocks": [
          {"t": "p", "text": "<<one sentence on what the table checks>>"},
          {"t": "table", "cols": ["<<check>>", "<<expected>>"], "rows": [
            ["<<a check>>", "<<what it shows>>"]
          ]}
        ]
      },
      {
        "h": "<<what to do when it goes wrong, as a claim>>",
        "blocks": [
          {"t": "details", "summary": "<<the failure and its fix>>", "blocks": [
            {"t": "p", "text": "<<the cause and the fix>>"}
          ]}
        ]
      }
    ]
  },
  "references": [{"title": "<<source>>", "url": "https://example.com/<<its address>>"}]
}
```

One callout in the page, on the step with the trap. Steps without a figure count toward `brief.figure_gap`, so fold small steps into one section rather than giving each its own.

The path at a glance is the one place a plain sequence may be a diagram instead of a list: a single row of boxes with labels only. Give the time stat and the summary's time only when the source gives one. When the source has one check, the check section is a sentence and the command in a `code` block, not a table.

## `measurement`: a benchmark, an eval, a cost report

The reader wants the headline number first, then one finding per section, each carried by a chart.

```json
{
  "title": "<<what was measured>>",
  "content": {
    "summary": "<<the headline result with its number, and what it means>>",
    "meta": "<<date · measurement · runs or sample size>>",
    "sections": [
      {
        "h": "<<the headline result as a claim with its number>>",
        "blocks": [
          {"t": "columns", "cols": [
            [{"t": "stat", "value": "<<number>>", "label": "<<the measure>>", "sub": "<<a comparison the reader already knows>>"}],
            [{"t": "stat", "value": "<<number>>", "label": "<<the measure>>"}]
          ]},
          {"t": "chart", "kind": "bar", "title": "<<the measure and its unit>>", "unit": "<<unit>>", "series": [
            {"name": "<<arm>>", "points": [{"x": "<<category>>", "y": 0}, {"x": "<<category>>", "y": 0}]}
          ]}
        ]
      },
      {
        "h": "<<one finding as a claim with its number; one section per finding>>",
        "blocks": [
          {"t": "chart", "kind": "line", "title": "<<the measure and its unit>>", "unit": "<<unit>>", "series": [
            {"name": "<<series>>", "points": [{"x": "<<a period or an ordered amount, for a line>>", "y": 0}, {"x": "<<a period or an ordered amount, for a line>>", "y": 0}]}
          ]},
          {"t": "p", "text": "<<what to look at in the chart, in one paragraph>>"}
        ]
      },
      {
        "h": "<<how it was measured, as a sentence>>",
        "blocks": [
          {"t": "p", "text": "<<the method in two sentences>>"},
          {"t": "details", "summary": "<<the full method and the raw numbers>>", "blocks": [
            {"t": "p", "text": "<<setup, runs, and what was held equal>>"}
          ]}
        ]
      },
      {
        "h": "<<what the data cannot tell, as a claim>>",
        "blocks": [
          {"t": "list", "items": ["<<one limit of the data>>", "<<one limit of the data>>"]}
        ]
      },
      {
        "h": "Next",
        "blocks": [
          {"t": "list", "ordered": true, "items": ["<<an action, with its owner and date when the source names them>>"]}
        ]
      }
    ]
  },
  "references": [{"title": "<<the results>>", "url": "https://example.com/<<its address>>"}]
}
```

Pick each chart's kind with SKILL.md's **Which kind**, and every number from the source. The finding chart's `line` is a choice, not a fixed kind: a `line` needs an ordered axis (time, or an ordered amount such as input size), and a benchmark with arms and cases and no period is a `bar`. Points that share an `x` label are kept as separate points, not merged.

Two headline numbers as `stat` blocks in `columns` are right in a brief, as in the first section; a `kv` is for labelled values read together.

## `update`: a work summary or a status

The reader wants where it stands, what got done, and what comes next.

```json
{
  "title": "<<the work, as a short phrase>>",
  "content": {
    "summary": "<<where it stands, in one or two sentences>>",
    "meta": "<<date · status · period covered>>",
    "sections": [
      {
        "h": "<<where it stands now, as a claim>>",
        "blocks": [
          {"t": "columns", "cols": [
            [{"t": "stat", "value": "<<count>>", "label": "done"}],
            [{"t": "stat", "value": "<<count>>", "label": "open"}],
            [{"t": "stat", "value": "<<days>>", "label": "<<to the next milestone>>"}]
          ]},
          {"t": "p", "text": "<<the state in one or two sentences>>"}
        ]
      },
      {
        "h": "<<what got done, as a claim>>",
        "blocks": [
          {"t": "progress", "pct": 0, "label": "<<the milestone>>", "note": "<<n of m>>"},
          {"t": "list", "items": ["<<a finished item>> @chip(b:done)", "<<an item in flight>> @chip(a:in review)"]}
        ]
      },
      {
        "h": "<<the one risk, as a claim>>",
        "blocks": [
          {"t": "callout", "sev": "warn", "text": "<<the risk and what would trigger it>>"}
        ]
      },
      {
        "h": "Next",
        "blocks": [
          {"t": "list", "ordered": true, "items": ["<<an action, with its owner and date when the source names them>>"]}
        ]
      }
    ]
  }
}
```

The last section is always Next. The one risk callout sits beside the work it is about, before Next, never after it. Give the `progress` block and the `done` chips only when the source records what is done.

With the ADHD mode on, "Do now" comes first: the next actions as an ordered list of five at most, each one line. The skeleton follows it whole, so the stats section above becomes section two.

## `talk`: a deck for a room

The room wants one claim per slide, most of them carried by a picture, and the ask at the end. Each slide's JSON is in [decks.md](decks.md).

```json
{
  "title": "<<the talk's title>>",
  "deck": {
    "summary": "<<what the talk is about, in the one sentence the room should remember>>",
    "meta": "<<date · occasion · audience>>",
    "presenter": "<<name, team; leave the field out when no speaker is named>>",
    "chips": [{"text": "<<headline number>>", "style": "stat"}],
    "sections": [
      {
        "h": "<<the claim the whole talk makes, as one sentence that is not the summary>>",
        "layout": "statement",
        "blocks": []
      },
      {
        "h": "<<a point as a claim; one slide per point, each over one visual>>",
        "blocks": [
          {"t": "chart", "kind": "line", "title": "<<the measure and its unit>>", "series": [
            {"name": "<<series>>", "points": [{"x": "<<period>>", "y": 0}, {"x": "<<period>>", "y": 0}]}
          ]}
        ]
      },
      {
        "h": "<<a point as a claim, shown as parts that fit>>",
        "blocks": [
          {"t": "diagram", "caption": "<<what the picture shows>>", "nodes": [
            {"id": "a", "label": "<<box name>>"},
            {"id": "b", "label": "<<box name>>"}
          ], "edges": [
            {"from": "a", "to": "b", "label": "<<what moves>>"}
          ]}
        ]
      },
      {
        "h": "<<the number the room should carry out, as a claim>>",
        "blocks": [
          {"t": "stat", "value": "<<number>>", "label": "<<what it counts>>", "sub": "<<a comparison the room already knows>>"}
        ]
      },
      {
        "h": "Next",
        "blocks": [
          {"t": "list", "ordered": true, "items": ["<<an action, with its owner and date when the source names them>>"]}
        ]
      }
    ]
  }
}
```

Five to twelve slides. Slides that hold only prose or a list stay within `deck.text_only`, so most points get a chart, a diagram, a stat, or an image. The rule counts default-layout and split slides only: a centred row of stats does not lower the count, while a lone big-number slide, a figure on a default-layout slide, or a split slide does.

The title slide's `summary` says what the talk is about, and the statement slide makes the claim, so they are never the same sentence. `presenter` is optional: leave it out when the source names no speaker.

## `review`: a decision in a meeting

The room has to decide. It wants the ask first, the options side by side, and the evidence for the one you recommend.

```json
{
  "title": "<<the decision to make>>",
  "deck": {
    "summary": "<<the ask, in one sentence>>",
    "meta": "<<date · review · who decides>>",
    "sections": [
      {
        "h": "<<the ask, as one sentence>>",
        "layout": "statement",
        "blocks": []
      },
      {
        "h": "<<the options differ on one thing, as a claim>>",
        "blocks": [
          {"t": "p", "text": "<<one sentence on what the table compares>>"},
          {"t": "table", "cols": ["<<option>>", "<<criterion>>", "<<criterion>>"], "rows": [
            ["<<option>>", "<<a phrase>>", "<<a phrase>>"],
            ["<<option>>", "<<a phrase>>", "<<a phrase>>"]
          ]}
        ]
      },
      {
        "h": "<<the recommendation, with the number behind it>>",
        "blocks": [
          {"t": "chart", "kind": "bar", "title": "<<the measure and its unit>>", "series": [
            {"name": "<<option>>", "points": [{"x": "<<option>>", "y": 0}, {"x": "<<option>>", "y": 0}]}
          ]}
        ]
      },
      {
        "h": "<<the risk, as a claim>>",
        "blocks": [
          {"t": "callout", "sev": "warn", "text": "<<the risk and what would trigger it>>"}
        ]
      },
      {
        "h": "Next",
        "blocks": [
          {"t": "list", "ordered": true, "items": ["<<the decision asked for, with who makes it and by when, when the source names them>>"]}
        ]
      }
    ]
  }
}
```

The options table on a slide holds at most four columns and five rows (SKILL.md deck rule 6); the full comparison belongs in the brief.

## What the lint catches in a brief

Each pair below is generated from the lint's own test fixtures: a fragment that trips one rule, the warning the lint returns for it, and the same fragment repaired.

### Summary

<!-- pair:summary.words -->
This trips `summary.words`:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name. In exchange we accepted wider source reads, a weaker network isolation flag for Go services, and an egress policy that lives in two places. ADR-0027 builds on this decision, so a revert reaches it too.",
    "sections": [
      {
        "h": "srt replaces one hand-written profile per process",
        "blocks": [
          {
            "t": "p",
            "text": "srt replaces the one `.sb` file per process that ran under `sandbox-exec`."
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `content.summary` the summary: 61 words. The summary sits above the first section; a longer one pushes the first heading and figure off the first screen. Aim for 50 or fewer; the rest can open the first section.

Repaired:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "srt replaces one hand-written profile per process",
        "blocks": [
          {
            "t": "p",
            "text": "In exchange we accepted wider source reads, a weaker network isolation flag for Go services, and an egress policy that lives in two places. ADR-0027 builds on this decision, so a revert reaches it too."
          },
          {
            "t": "p",
            "text": "srt replaces the one `.sb` file per process that ran under `sandbox-exec`."
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Headings

<!-- pair:heading.label -->
This trips `heading.label`:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "srt replaces one hand-written profile per process",
        "blocks": [
          {
            "t": "p",
            "text": "srt replaces the one `.sb` file per process that ran under `sandbox-exec`."
          }
        ]
      },
      {
        "h": "Reverting",
        "blocks": [
          {
            "t": "p",
            "text": "Restore the deleted `.sb` files from git and point the wrappers back at `sandbox-exec`. ADR-0027 builds on this decision, so a revert reaches it too."
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `content.sections[1].h` section "Reverting": the heading is a label of 1 word. A reader who skims only the headings should leave with the argument; a label names the topic and nothing else. Write the claim the section proves as a sentence with a verb.

Repaired:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "srt replaces one hand-written profile per process",
        "blocks": [
          {
            "t": "p",
            "text": "srt replaces the one `.sb` file per process that ran under `sandbox-exec`."
          }
        ]
      },
      {
        "h": "A revert restores the old profiles and reaches ADR-0027 too",
        "blocks": [
          {
            "t": "p",
            "text": "Restore the deleted `.sb` files from git and point the wrappers back at `sandbox-exec`. ADR-0027 builds on this decision, so a revert reaches it too."
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Figures

<!-- pair:brief.figure_gap -->
This trips `brief.figure_gap`:

```json
{
  "title": "Releasing: from merged PR to signed artifact",
  "content": {
    "summary": "A merged PR reaches a signed artifact with no hand-made tag: release-please opens the release PR, and merging it cuts the tag.",
    "sections": [
      {
        "h": "The commit prefix and path decide the version bump",
        "blocks": [
          {
            "t": "p",
            "text": "A `feat:` commit bumps the minor version and a `fix:` the patch. A commit counts only toward the components whose registered path it changes."
          }
        ]
      },
      {
        "h": "release-please looks back only 200 commits",
        "blocks": [
          {
            "t": "p",
            "text": "A component whose last tag is further back than 200 commits on main is not seen. `commit-search-depth` in release-please-config.json sets the window."
          }
        ]
      },
      {
        "h": "What to check when a release is stuck",
        "blocks": [
          {
            "t": "p",
            "text": "Start from the last thing that exists: no release PR, a release PR that will not merge, or a tag without its assets."
          }
        ]
      },
      {
        "h": "Each release ships a tarball, a checksum file, and a signature",
        "blocks": [
          {
            "t": "p",
            "text": "The artifacts job builds one tarball per component on a macOS arm64 runner, then writes checksums.txt and its cosign bundle."
          }
        ]
      },
      {
        "h": "Two commands prove a release is genuine",
        "blocks": [
          {
            "t": "p",
            "text": "Check the cosign bundle against checksums.txt first, then the tarball against its line in checksums.txt."
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `content.sections[0..4]` section "The commit prefix and path decide the version bump" to section "Two commands prove a release is genuine": 5 sections in a row hold no chart, diagram, graph, image, or stat. A long run of text-only sections reads as a wall; a figure gives the reader who skims a place to land. Give one of them a figure, or fold the run into fewer sections.

Repaired:

```json
{
  "title": "Releasing: from merged PR to signed artifact",
  "content": {
    "summary": "A merged PR reaches a signed artifact with no hand-made tag: release-please opens the release PR, and merging it cuts the tag.",
    "sections": [
      {
        "h": "The commit prefix and path decide the version bump",
        "blocks": [
          {
            "t": "p",
            "text": "A `feat:` commit bumps the minor version and a `fix:` the patch. A commit counts only toward the components whose registered path it changes."
          }
        ]
      },
      {
        "h": "release-please looks back only 200 commits",
        "blocks": [
          {
            "t": "stat",
            "value": "200",
            "label": "commits on main that release-please reads back"
          },
          {
            "t": "p",
            "text": "A component whose last tag is further back than 200 commits on main is not seen. `commit-search-depth` in release-please-config.json sets the window."
          }
        ]
      },
      {
        "h": "What to check when a release is stuck",
        "blocks": [
          {
            "t": "p",
            "text": "Start from the last thing that exists: no release PR, a release PR that will not merge, or a tag without its assets."
          }
        ]
      },
      {
        "h": "Each release ships a tarball, a checksum file, and a signature",
        "blocks": [
          {
            "t": "p",
            "text": "The artifacts job builds one tarball per component on a macOS arm64 runner, then writes checksums.txt and its cosign bundle."
          }
        ]
      },
      {
        "h": "Two commands prove a release is genuine",
        "blocks": [
          {
            "t": "p",
            "text": "Check the cosign bundle against checksums.txt first, then the tarball against its line in checksums.txt."
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Paragraphs

<!-- pair:brief.paragraph -->
This trips `brief.paragraph`:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "A process moves onto srt in four steps",
        "blocks": [
          {
            "t": "p",
            "text": "Moving a process onto srt takes four steps, and each one has to land before the next. First write the settings file that names the hosts the process may reach and the paths it may write. Then swap the launcher so it starts the process under `srt-run` instead of `sandbox-exec`. Then run the process for a day and read every denial the log records, adding a host or a path only when the denial is a real need. Last, delete the old `.sb` profile so nothing can start the process unconfined."
          }
        ]
      }
    ]
  }
}
```

The lint says:

> `content.sections[0].blocks[0]` section "A process moves onto srt in four steps", a paragraph: 91 words. A reader who skims passes over a long block of text, and one who listens loses its thread before it ends. Aim for 70 or fewer: split it, or make the longer thought a list, a table, or a figure.

Repaired:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "A process moves onto srt in four steps",
        "blocks": [
          {
            "t": "p",
            "text": "Each step lands before the next one starts."
          },
          {
            "t": "list",
            "ordered": true,
            "items": [
              "Write the settings file that names the hosts the process may reach and the paths it may write.",
              "Swap the launcher so it starts the process under `srt-run` instead of `sandbox-exec`.",
              "Run the process for a day and read every denial the log records. Add a host or a path only for a real need.",
              "Delete the old `.sb` profile, so nothing can start the process unconfined."
            ]
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->

### Tables

<!-- pair:table.rows -->
This trips `table.rows`:

```json
{
  "title": "Releasing: from merged PR to signed artifact",
  "content": {
    "summary": "A merged PR reaches a signed artifact with no hand-made tag: release-please opens the release PR, and merging it cuts the tag.",
    "sections": [
      {
        "h": "When a release stalls, the step it stopped at tells you what to check",
        "blocks": [
          {
            "t": "p",
            "text": "Find the row for what you see, then check the thing it names."
          },
          {
            "t": "table",
            "cols": [
              "What you see",
              "Check"
            ],
            "rows": [
              [
                "A commit merged and nothing opened",
                "The commit type: `chore:` and `docs:` never release"
              ],
              [
                "release-please fails with \"Error adding to tree\"",
                "The GitHub App's secrets and permissions"
              ],
              [
                "A component is missing from the release PR",
                "Whether its last tag is within 200 commits"
              ],
              [
                "The release PR sits unmerged",
                "Its required checks"
              ],
              [
                "The PR opened for a different component",
                "The paths the commit touched"
              ],
              [
                "The release run fails on API timeouts",
                "`commit-search-depth` and `commit-batch-size`"
              ],
              [
                "The run has not started",
                "Another run in the `release` concurrency group"
              ],
              [
                "Checks are green and it still does not merge",
                "Whether auto-merge was turned off on the PR"
              ],
              [
                "The GitHub Release has no tarball",
                "The `artifacts` job of that run"
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

> `content.sections[0].blocks[1]` section "When a release stalls, the step it stopped at tells you what to check", a table: 9 body rows. Past that a reader scans instead of comparing; keep the rows the point needs and move the full list into details. Aim for 8 or fewer.

Repaired:

```json
{
  "title": "Releasing: from merged PR to signed artifact",
  "content": {
    "summary": "A merged PR reaches a signed artifact with no hand-made tag: release-please opens the release PR, and merging it cuts the tag.",
    "sections": [
      {
        "h": "When a release stalls, the step it stopped at tells you what to check",
        "blocks": [
          {
            "t": "p",
            "text": "Most stalls are one of these four; the five rarer ones are below."
          },
          {
            "t": "table",
            "cols": [
              "What you see",
              "Check"
            ],
            "rows": [
              [
                "A commit merged and nothing opened",
                "The commit type: `chore:` and `docs:` never release"
              ],
              [
                "release-please fails with \"Error adding to tree\"",
                "The GitHub App's secrets and permissions"
              ],
              [
                "A component is missing from the release PR",
                "Whether its last tag is within 200 commits"
              ],
              [
                "The release PR sits unmerged",
                "Its required checks"
              ]
            ]
          },
          {
            "t": "details",
            "summary": "Five rarer stalls",
            "blocks": [
              {
                "t": "table",
                "cols": [
                  "What you see",
                  "Check"
                ],
                "rows": [
                  [
                    "The PR opened for a different component",
                    "The paths the commit touched"
                  ],
                  [
                    "The release run fails on API timeouts",
                    "`commit-search-depth` and `commit-batch-size`"
                  ],
                  [
                    "The run has not started",
                    "Another run in the `release` concurrency group"
                  ],
                  [
                    "Checks are green and it still does not merge",
                    "Whether auto-merge was turned off on the PR"
                  ],
                  [
                    "The GitHub Release has no tarball",
                    "The `artifacts` job of that run"
                  ]
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

<!-- pair:table.cell -->
This trips `table.cell`:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "We pay for srt with wider reads and two places for policy",
        "blocks": [
          {
            "t": "table",
            "cols": [
              "Cost",
              "What bounds it"
            ],
            "rows": [
              [
                "A process that reads source can read any file under the code roots that isn't secret-shaped, a token in a YAML config included. srt globs cannot express \"prose and Go files only\".",
                "Egress is pinned to named hosts, and the calling agent reads the same files."
              ],
              [
                "The egress policy lives in two places: the settings file and the wrapper that names per-machine hosts.",
                "A wrapper change gets the same review as a policy change."
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

> `content.sections[0].blocks[0]` section "We pay for srt with wider reads and two places for policy", a table: 1 cell runs past 160 characters, the longest 179 (row 1, column 1). A cell is read across a row; a paragraph in it breaks the row, so the prose belongs beside the table. Cut each to a phrase and put the rest in a paragraph after the table.

Repaired:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "We pay for srt with wider reads and two places for policy",
        "blocks": [
          {
            "t": "table",
            "cols": [
              "Cost",
              "What bounds it"
            ],
            "rows": [
              [
                "Reads reach any file under the code roots that is not secret-shaped",
                "Egress is pinned to named hosts"
              ],
              [
                "The egress policy lives in two places: the settings file and the wrapper that names per-machine hosts.",
                "A wrapper change gets the same review as a policy change."
              ]
            ]
          },
          {
            "t": "p",
            "text": "srt globs cannot express \"prose and Go files only\", so a token in a YAML config under the code roots is readable too. The calling agent reads the same files."
          }
        ]
      }
    ]
  }
}
```
<!-- pair:end -->
