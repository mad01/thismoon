# Diagrams: fields, colour, and steps

SKILL.md's **Diagram format** says when a diagram is the right picture and how much goes in it. This file holds its fields, what makes it draw as a row or a column, the colour rules that keep its boxes readable in both modes, and how steps walk it on a slide. The limits named by id are in [limits.md](limits.md).

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `nodes` | array | The boxes, at least one. Each is `{id, label, text?, kind?, tone?, group?, step?}`. The renderer refuses past `diagram.nodes` boxes, a label past `diagram.label`, and a text past `diagram.text` |
| `edges` | array? | The arrows, `{from, to, label?, flow?, weight?, step?}`, between two different nodes (never a group), one per pair and direction; a label past `diagram.edge_label` is refused |
| `groups` | array? | The boundaries, `{id, label, tone?, group?, step?}`, up to `diagram.groups`. `group` names the group this one sits in, up to `diagram.depth` levels |
| `direction` | string? | One of `LR`, `TB`, left to right or top to bottom; `LR` when left out. The renderer's first choice of layout: when the diagram would not be readable that way in the space it has, it draws another form |
| `caption` | string? | Prose under the diagram, with inline markdown |
| `steps` | array? | `[{caption, focus?}]`, one per step a deck slide walks through the diagram. `focus` lists the node and group ids that light up at that step. See **Stepping a diagram** below |

A node's `label` is its name, in bold. `text` is the short line under it (the technology, a one-line purpose); it wraps to the box's width and is never cut. `kind` picks the shape, one of `service`, `store`, `queue`, `person`, `external`: `service` is the default plain box, `store` a cylinder, `queue` a pipe, `person` a box with a figure, and `external` a dashed box. `tone` tints a node or a group with one of the six node tones from the graph (`neutral`, `green`, `red`, `blue`, `amber`, `purple`; see **Node tones** in SKILL.md). Use at most two or three tones on one diagram, each with a fixed meaning.

An edge's `label` sits on the line with a halo, and the layout leaves room for it. `flow: true` marks an edge that carries traffic. The line is dashed, and dots move along it in the brief and on a slide, faster for a larger `weight` against the heaviest flow edge on the diagram. Under Reduce Motion the dots go and the dash stays. Ids are plain tokens (letters, digits, `_` and `-`) you only use to connect things; labels are what the room reads. Labels and texts are plain text, never inline markdown.

The diagram sits straight on the page or slide with no card of its own. Boxes carry their own border and fill, so they stand out on every theme. In a brief it shows its final state with the step captions as a numbered list under it. Alone on a slide it fills the slide with its caption under it.

## Which form it draws

`direction` is the renderer's first choice, not a promise. In a brief, a left-to-right row holds five boxes, each a short name over a `text` of three or four words, when its arrows carry no labels. Each edge label on the row adds its own width plus about 32 px, so a row of five with every arrow labelled may draw as a column. The lint estimates the row and warns (`diagram.row`) when it is wider than the brief; groups and branches widen a row past the estimate, so a row it passes can still turn. A diagram with a fan-out, or with more than about five boxes, usually draws top to bottom, so set `direction` to `TB` and write the boxes for that. A back edge (an arrow that returns) adds a lane around the boxes, and its label sits next to the box it leaves.

## Colour that reads

A diagram of plain boxes inside toned boundaries is the common mistake. In dark mode every box is a grey on a grey, and the only colour is a faint wash the eye cannot attach to anything. The rules:

- **Boxes carry the tone, boundaries carry the structure.** A toned node is a filled box with a coloured border and label. A toned group is a half-transparent wash under a dashed line, drawn behind its members. So tone the nodes by what they are. Tone a group only when the boundary itself is the point (a trust zone, a namespace the room must see), in a tone none of its members use. `amber` is the boundary tone in the worked examples, since few boxes in an architecture are pending. A group in the same tone as its members swallows them.
- **No plain boxes in a toned picture.** Every node has a tone or none has. An untoned box is the card colour on the page colour, a shade apart in dark mode.
- **Two or three node tones with the meanings in Node tones, plus at most one boundary tone, named in the caption.** `kind` says the shape (a store is a cylinder, a queue a pipe, an external system a dashed box). So the shape says what a box does and the tone says whose it is.
- **Nested groups alternate.** A group inside a group gets a different tone or none; two washes of one tone stack into one.
- **Focus and flow are colours already.** On a stepped diagram the focused box glows in the accent and the rest dim; a flow edge is dashed with accent dots moving on it. Do not spend a tone on "the current step" or "the busy path".
- **Check both modes** with the theme toggle in the page header before sharing. The worked example [diagram-block.json](../examples/diagram-block.json) tones its nodes this way; the groups that keep a tone are the boundaries the story is about.

## Stepping a diagram

With `steps`, Next walks the diagram on a deck slide the way it walks a stepped chart. The counter shows `s/S`, the remote `next` and `prev` take the steps, and a jump lands on the last step with no focus, so the whole picture shows. A node, group, or edge with `step` appears at that step and stays. Without one it is there from the start, so the slide opens on the parts every step shares, with no step caption; the first Next shows caption 1. End the walk on a step with no `focus`, so the whole picture lights under the conclusion. Give a group a `step` too, or it is an empty boundary until its first member arrives. An element never appears before what holds it: a node waits for its group, a group for its parent, and an edge for both of its ends. A step's `focus` may only name what has appeared by then. The renderer refuses the rest.

A step's `focus` lights up the named nodes and groups, everything inside a focused group, and the edges touching a focused node. The rest dim. A step without `focus` dims nothing. The layout is computed once for the whole diagram, so nothing moves between steps. Reduce Motion and a deck with `transition: "none"` cut between steps instead of fading.

## What the lint catches in a diagram

Each pair below is generated from the lint's own test fixtures: a fragment that trips one rule, the warning the lint returns for it, and the same fragment repaired. The slide rules for diagrams are in [decks.md](decks.md).

### Too many boxes

<!-- pair:diagram.boxes -->
This trips `diagram.boxes`:

```json
{
  "title": "The memory stack: six stores and the hooks that feed them",
  "content": {
    "summary": "Six stores keep what an agent learns; the agent writes four of them, and hooks fill the other two.",
    "sections": [
      {
        "h": "The agent writes four stores, hooks write the other two",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "What writes each store. The amber boundary holds the Claude Code hooks: they record without the agent deciding anything.",
            "groups": [
              {
                "id": "hooks",
                "label": "Claude Code hooks",
                "tone": "amber"
              }
            ],
            "nodes": [
              {
                "id": "session",
                "label": "Claude Code session",
                "text": "the agent picks the store when it writes",
                "tone": "neutral"
              },
              {
                "id": "brec",
                "label": "belt record",
                "text": "nine lifecycle events",
                "tone": "green",
                "group": "hooks"
              },
              {
                "id": "mrec",
                "label": "moondb record",
                "text": "every prompt, file edit and stop",
                "tone": "green",
                "group": "hooks"
              },
              {
                "id": "others",
                "label": "Guards, hints, services",
                "text": "every deny, every hint, the services' audit trails",
                "tone": "green"
              },
              {
                "id": "automem",
                "label": "auto memory",
                "text": "MEMORY.md index plus one file per fact",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "agentmem",
                "label": "agent-memory",
                "text": "same layout; belt never writes to it",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "worklog",
                "label": "worklog",
                "text": "task state keyed by ticket or topic",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "handoff",
                "label": "handoff",
                "text": "a one-off bridge document",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "events",
                "label": "events",
                "text": "one queryable timeline",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "moondb",
                "label": "moondb",
                "text": "each edit with its turn, prompt and later commit",
                "kind": "store",
                "tone": "blue"
              }
            ],
            "edges": [
              {
                "from": "session",
                "to": "automem",
                "label": "file tools"
              },
              {
                "from": "session",
                "to": "agentmem",
                "label": "write, commit, push"
              },
              {
                "from": "session",
                "to": "worklog",
                "label": "worklog_checkpoint"
              },
              {
                "from": "session",
                "to": "handoff",
                "label": "/handoff skill"
              },
              {
                "from": "session",
                "to": "brec",
                "label": "nine lifecycle events"
              },
              {
                "from": "session",
                "to": "mrec",
                "label": "prompt, edit, stop"
              },
              {
                "from": "brec",
                "to": "events",
                "label": "ledger"
              },
              {
                "from": "others",
                "to": "events",
                "label": "denies, audit trails"
              },
              {
                "from": "mrec",
                "to": "moondb",
                "label": "before and after"
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

> `content.sections[0].blocks[0]` section "The agent writes four stores, hooks write the other two", a diagram: 10 boxes. A bigger diagram draws taller than a screen at a size the reader can read; split it or show one part. Aim for 9 or fewer.

Repaired:

```json
{
  "title": "The memory stack: six stores and the hooks that feed them",
  "content": {
    "summary": "Six stores keep what an agent learns; the agent writes four of them, and hooks fill the other two.",
    "sections": [
      {
        "h": "The agent writes four stores, hooks write the other two",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "What writes each store. The amber boundary holds the Claude Code hooks: they record without the agent deciding anything. Guards, hints, and the services write to events too.",
            "groups": [
              {
                "id": "hooks",
                "label": "Claude Code hooks",
                "tone": "amber"
              }
            ],
            "nodes": [
              {
                "id": "session",
                "label": "Claude Code session",
                "text": "the agent picks the store when it writes",
                "tone": "neutral"
              },
              {
                "id": "brec",
                "label": "belt record",
                "text": "nine lifecycle events",
                "tone": "green",
                "group": "hooks"
              },
              {
                "id": "mrec",
                "label": "moondb record",
                "text": "every prompt, file edit and stop",
                "tone": "green",
                "group": "hooks"
              },
              {
                "id": "automem",
                "label": "auto memory",
                "text": "MEMORY.md index plus one file per fact",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "agentmem",
                "label": "agent-memory",
                "text": "same layout; belt never writes to it",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "worklog",
                "label": "worklog",
                "text": "task state keyed by ticket or topic",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "handoff",
                "label": "handoff",
                "text": "a one-off bridge document",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "events",
                "label": "events",
                "text": "one queryable timeline",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "moondb",
                "label": "moondb",
                "text": "each edit with its turn, prompt and later commit",
                "kind": "store",
                "tone": "blue"
              }
            ],
            "edges": [
              {
                "from": "session",
                "to": "automem",
                "label": "file tools"
              },
              {
                "from": "session",
                "to": "agentmem",
                "label": "write, commit, push"
              },
              {
                "from": "session",
                "to": "worklog",
                "label": "worklog_checkpoint"
              },
              {
                "from": "session",
                "to": "handoff",
                "label": "/handoff skill"
              },
              {
                "from": "session",
                "to": "brec",
                "label": "nine lifecycle events"
              },
              {
                "from": "session",
                "to": "mrec",
                "label": "prompt, edit, stop"
              },
              {
                "from": "brec",
                "to": "events",
                "label": "ledger"
              },
              {
                "from": "mrec",
                "to": "moondb",
                "label": "before and after"
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

### Too long a path

<!-- pair:diagram.chain -->
This trips `diagram.chain`:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "One settings file and one launch path confine every process",
        "blocks": [
          {
            "t": "diagram",
            "direction": "TB",
            "caption": "Launching one confined process. Green boxes are our launch and watch code, red ones enforce the policy, blue holds it, and grey is outside.",
            "groups": [
              {
                "id": "box",
                "label": "sandbox",
                "tone": "amber"
              }
            ],
            "nodes": [
              {
                "id": "wrapper",
                "label": "Wrapper",
                "text": "resolves secrets, env -i allow-list, cd /",
                "tone": "green"
              },
              {
                "id": "settings",
                "label": "Settings file",
                "text": "~/.config/srt/<label>.json",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "srtrun",
                "label": "srt-run",
                "text": "merges per-machine hosts, sets TMPDIR",
                "tone": "green"
              },
              {
                "id": "srt",
                "label": "srt",
                "text": "default-deny seatbelt profile",
                "tone": "red"
              },
              {
                "id": "proc",
                "label": "Confined process",
                "text": "MCP server, install step, Go service",
                "tone": "green",
                "group": "box"
              },
              {
                "id": "proxy",
                "label": "HTTP and SOCKS proxies",
                "text": "listed hosts only, no direct connections",
                "tone": "red"
              },
              {
                "id": "hosts",
                "label": "Allowed hosts",
                "text": "named in settings or at launch",
                "kind": "external",
                "tone": "neutral"
              },
              {
                "id": "watch",
                "label": "sandbox-watch",
                "text": "decodes the SRT_RUN label from denials",
                "tone": "green"
              }
            ],
            "edges": [
              {
                "from": "wrapper",
                "to": "srtrun",
                "label": "argv + domains"
              },
              {
                "from": "settings",
                "to": "srtrun",
                "label": "static policy"
              },
              {
                "from": "srtrun",
                "to": "srt",
                "label": "merged settings"
              },
              {
                "from": "srt",
                "to": "proc",
                "label": "spawns"
              },
              {
                "from": "proc",
                "to": "proxy",
                "label": "all egress",
                "flow": true
              },
              {
                "from": "proxy",
                "to": "hosts",
                "label": "allowlisted",
                "flow": true
              },
              {
                "from": "proc",
                "to": "watch",
                "label": "denials via log"
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

> `content.sections[0].blocks[0]` section "One settings file and one launch path confine every process", a diagram: a path of 6 boxes, "Wrapper" to "Allowed hosts". Each box on the path is one layer of the layout, and every layer adds height in any direction. Aim for 5 or fewer: fold steps together or split the flow over two diagrams.

Repaired:

```json
{
  "title": "ADR-0028: sandbox-runtime replaces hand-written seatbelt profiles",
  "content": {
    "summary": "The fleet now confines its processes with **sandbox-runtime (srt)** instead of hand-written seatbelt profiles: each process starts default-deny and reaches only the hosts its settings name.",
    "sections": [
      {
        "h": "One settings file and one launch path confine every process",
        "blocks": [
          {
            "t": "diagram",
            "direction": "TB",
            "caption": "Launching one confined process. Green boxes are our launch and watch code, red ones enforce the policy, blue holds it, and grey is outside.",
            "groups": [
              {
                "id": "box",
                "label": "sandbox",
                "tone": "amber"
              }
            ],
            "nodes": [
              {
                "id": "wrapper",
                "label": "Wrapper",
                "text": "resolves secrets, env -i allow-list, cd /",
                "tone": "green"
              },
              {
                "id": "settings",
                "label": "Settings file",
                "text": "~/.config/srt/<label>.json",
                "kind": "store",
                "tone": "blue"
              },
              {
                "id": "srt",
                "label": "srt-run, then srt",
                "text": "merges hosts, spawns under a default-deny profile",
                "tone": "red"
              },
              {
                "id": "proc",
                "label": "Confined process",
                "text": "MCP server, install step, Go service",
                "tone": "green",
                "group": "box"
              },
              {
                "id": "proxy",
                "label": "HTTP and SOCKS proxies",
                "text": "listed hosts only, no direct connections",
                "tone": "red"
              },
              {
                "id": "hosts",
                "label": "Allowed hosts",
                "text": "named in settings or at launch",
                "kind": "external",
                "tone": "neutral"
              },
              {
                "id": "watch",
                "label": "sandbox-watch",
                "text": "decodes the SRT_RUN label from denials",
                "tone": "green"
              }
            ],
            "edges": [
              {
                "from": "wrapper",
                "to": "srt",
                "label": "argv + domains"
              },
              {
                "from": "settings",
                "to": "srt",
                "label": "static policy"
              },
              {
                "from": "srt",
                "to": "proc",
                "label": "spawns"
              },
              {
                "from": "proc",
                "to": "proxy",
                "label": "all egress",
                "flow": true
              },
              {
                "from": "proxy",
                "to": "hosts",
                "label": "allowlisted",
                "flow": true
              },
              {
                "from": "proc",
                "to": "watch",
                "label": "denials via log"
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

### A row too wide for the brief

<!-- pair:diagram.row -->
This trips `diagram.row`:

```json
{
  "title": "The nightly cost report: five steps from the vendor feed to the channel",
  "content": {
    "summary": "The cost report runs once a night: it reads the vendor feed, types and tags each record, stores the rows, and posts a digest to the team channel.",
    "sections": [
      {
        "h": "Five steps turn the vendor feed into the morning digest",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "One nightly run, left to right.",
            "nodes": [
              {
                "id": "ingest",
                "label": "Ingest",
                "text": "reads the feed"
              },
              {
                "id": "parse",
                "label": "Parse",
                "text": "splits records"
              },
              {
                "id": "enrich",
                "label": "Enrich",
                "text": "adds the owner"
              },
              {
                "id": "store",
                "label": "Store",
                "text": "writes rows",
                "kind": "store"
              },
              {
                "id": "report",
                "label": "Report",
                "text": "daily digest"
              }
            ],
            "edges": [
              {
                "from": "ingest",
                "to": "parse",
                "label": "raw json lines"
              },
              {
                "from": "parse",
                "to": "enrich",
                "label": "typed record rows"
              },
              {
                "from": "enrich",
                "to": "store",
                "label": "tagged record rows"
              },
              {
                "from": "store",
                "to": "report",
                "label": "nightly sql query"
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

> `content.sections[0].blocks[0]` section "Five steps turn the vendor feed into the morning digest", a left-to-right diagram: its path of 5 boxes, "Ingest" to "Report", is about 1198 px wide in a row, past the brief's 1032. A wider row may draw top to bottom instead, a column as tall as the path is long; the reader's browser picks the form, so the lint can only estimate. It estimates the width from the box text and edge labels on the path in the page's default face; groups, branches, and return edges widen a row further, so a row it passes can still turn. To keep the row, cut the box text and edge labels on that path to a few words or fold boxes together; else set direction to TB and write the boxes for a column.

Repaired:

```json
{
  "title": "The nightly cost report: five steps from the vendor feed to the channel",
  "content": {
    "summary": "The cost report runs once a night: it reads the vendor feed, types and tags each record, stores the rows, and posts a digest to the team channel.",
    "sections": [
      {
        "h": "Five steps turn the vendor feed into the morning digest",
        "blocks": [
          {
            "t": "diagram",
            "direction": "LR",
            "caption": "One nightly run, left to right.",
            "nodes": [
              {
                "id": "ingest",
                "label": "Ingest",
                "text": "reads raw json"
              },
              {
                "id": "parse",
                "label": "Parse",
                "text": "types each record"
              },
              {
                "id": "enrich",
                "label": "Enrich",
                "text": "tags the owner"
              },
              {
                "id": "store",
                "label": "Store",
                "text": "writes the rows",
                "kind": "store"
              },
              {
                "id": "report",
                "label": "Report",
                "text": "posts the digest"
              }
            ],
            "edges": [
              {
                "from": "ingest",
                "to": "parse"
              },
              {
                "from": "parse",
                "to": "enrich"
              },
              {
                "from": "enrich",
                "to": "store"
              },
              {
                "from": "store",
                "to": "report"
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
