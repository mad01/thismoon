# Graph: engines, node types, and Mermaid

SKILL.md's **Graph format** holds the graph's shape, tones, colour rules, edges, and placement. This file holds the engine picker, what each node `type` draws, and how a Mermaid flowchart becomes a graph.

## Which engine

Pick `layout` by the shape of the data:

- **A tree or a hierarchy** (a dependency tree, a call graph, an org of services): `dagre`, the default. Layered, accounts for node size, minimises crossings. Fine to about thirty nodes.
- **A long chain of steps** (a pipeline, an import flow, a release process): `elk`. ELK layered with wrapping, so a chain of twelve steps folds into rows (or columns, top-down) that fit the container instead of shrinking into a strip.
- **The same layered drawing without folding**, when the chain should stay one row however long: `elk-layered`.
- **One root fanning out** (a tree with many leaves): `elk-mrtree`, which spaces a wide fan-out better than dagre.
- **One centre with rings around it** (a hub and its callers): `elk-radial`.
- **No hierarchy, distances should mean something** (a service mesh, a cluster of related modules): `elk-stress`. `elk-force` is the organic variant; `cose` is the Cytoscape built-in that needs no vendored library.
- **A map with cycles** (a mesh with traffic both ways): `elk` or `elk-stress`; a layered engine draws a back-edge, which is acceptable for one or two.

`dagre` and every ELK engine take an optional `direction`: `TB` (top-down) or `LR` (left-to-right). Omit it for auto: small graphs (8 nodes or fewer) draw left-to-right to fill the wide container, larger ones top-down. A pipeline reads left to right; a hierarchy reads top-down.

dagre and the ELK engines need their libraries, which the install fetches; an engine whose library is missing draws nothing. `cose` is built into Cytoscape itself. The page's graph toolbar has an engine button that cycles the live graph through every engine, so a reader can compare them on any page without re-authoring it. The choice is not saved; set `layout` to keep it.

## Node types

| Type | Visual |
|------|--------|
| `center` | Highlighted root (accent border, bold label) |
| `module` | Coloured border from the palette. Set `color` (0-3) to pick one of four slots |
| `leaf` | Plain node (default if type omitted) |
| `registry` | Dashed border |

Use `module` with `color` to group nodes by subsystem the way a Mermaid `subgraph` would: every node of one subsystem gets the same slot, four slots in all. A tone (SKILL.md, **Node tones**) says what a node *is*; a module colour says which *group* it belongs to. Use one or the other on a graph, not both.

## From a Mermaid flowchart

On a local instance, a markdown file dropped on the index page (or posted to the import endpoint) becomes a page. The first fenced `mermaid` flowchart in it becomes the page graph. Node labels are kept and shapes dropped. Dotted links become dashed `publishes` edges and every other link a solid arrow. `|text|` on a link becomes its label. A `subgraph` colours its members as `module` nodes, one colour slot per subgraph, cycling after four. `TD`, `TB`, and `BT` map to top-down, `LR` and `RL` to left-to-right. A sequence diagram, a second flowchart, or syntax the converter does not know stays a code block. When you already hold a flowchart, this is the fastest path to a graph; the full mapping and its losses are in the service's [CLAUDE.md](https://github.com/mad01/thismoon/blob/main/services/present/CLAUDE.md) under *Importing markdown*.
