# brain serve: HTTP API and UI contract

`brain serve` is the read-only web view of the vaults at brain.this. This
page is the contract the server, the UI and the docs build against. Change
it here first when a shape changes. `brain docs serve` prints it.

## Server

- Listens on `127.0.0.1:<port>`. The port comes from `--port`, then
  `BRAIN_PORT`, then `DefaultPort` (7427) in facts.go; port 0 picks a free
  one.
- Never writes a vault, and never runs git or gh. It writes only under the
  state directory: `POST /api/layout` into `<state dir>/layout/`, the probe
  file `/api/doctor` creates and removes there, and the index refresh a
  search runs first, through library under the vault's index lock.
- Loads the config once, at start. A change to the config file, or a vault
  directory that appears later, needs a restart.
- Starts with no vault configured, and reports that through `/api/doctor`
  and the UI instead of exiting, so a service manager does not restart it
  in a loop.
- Every `/api/` response is JSON with `Cache-Control: no-store`. An error is
  `{"error":{"code","message"}}`:

| Status | `code` | When |
|---|---|---|
| 400 | `usage` | a bad or missing parameter or body, an unknown tier or vault |
| 400 | `ambiguous` | a bare path names notes in more than one vault |
| 400 | `invalid_query` | the zoekt query does not parse |
| 404 | `not_found` | no such note, or no such `/api/` route |
| 409 | `stale_key` | `POST /api/layout` with a key that is no longer current |
| 413 | `too_large` | a layout body over 8 MiB |
| 503 | `semantic_unavailable` | hybrid search while `semantic.enabled` is false |
| 500 | `error` | anything else |

## Static routes

| Route | Serves |
|---|---|
| `GET /` | `shell.html` from the `ui` package |
| `GET /app.js`, `GET /graph.js` | the UI scripts from the `ui` package |
| `GET /assets/{file}` | the graph bundle and its `SHA256SUMS` from the `ui` package |
| `/webkit/...` | `webkit.Mount` |
| `GET /healthz` | `ok` |
| `GET /version` | buildinfo JSON, as the other services serve it |

The `ui` package is `services/brain/internal/server/ui`. It exports
`func Files() fs.FS` over `shell.html`, `app.js`, `graph.js` and `assets/`.
The server takes it as an `fs.FS` option, so handler tests use
`fstest.MapFS`.

## Vaults

serve opens the configured vaults whose directory exists at start. A vault
it opened but cannot list (a sandbox that does not grant the path) stays in
`/api/vaults` with `notes` 0, and its fingerprint is `unreadable`. A vault
whose directory was missing at start appears only in `/api/doctor`. The
notes, graph, stale queue, check and promotions read every vault serve can
list, from one snapshot that is rebuilt when a fingerprint changes.

## API

Where a route says "as the CLI", it calls the same library function as that
command and returns the same JSON as its `-o json` output.

| Route | Query or body | Response |
|---|---|---|
| `GET /api/vaults` | none | `{"vaults":[{"name","shared","default","notes"}]}` |
| `GET /api/state` | none | `{"key","vaults":[{"name","fingerprint"}]}`; `key` is the layout key with topics off; fingerprints are memoized for 2 seconds |
| `GET /api/graph` | `vault`, `center` (a note ref), `depth` (1 to 5, default 1), `topics` (0 or 1) | `{"key","layout":{"cached"},"nodes":[...],"edges":[...]}` |
| `POST /api/layout` | `{"key","positions":{"<node id>":[x,y]}}`, at most 8 MiB | 204; 409 when `key` is not the current key with topics off or on; ids not in the current graph are dropped |
| `GET /api/notes` | `tier` (required), `vault`, `tag`, `status`, `limit` (default 100) | as `brain list` |
| `GET /api/note` | `ref` | `{"ref","vault","path","frontmatter","body","sha","status","banner","links":[{"text","target","resolved"}]}` |
| `GET /api/backlinks` | `ref` | as `brain backlinks` |
| `GET /api/search` | `q`, `mode` (literal or hybrid), `vault`, `tier`, `include_revoked` (0 or 1), `limit` (default 50 literal, 20 hybrid) | literal: as `brain search`, in `files_with_matches` mode; hybrid: as `brain hybrid-search` |
| `GET /api/stale` | none | as `brain stale`, without drift; the response adds `"drift":false` |
| `GET /api/check` | none | as `brain check`: one report over every vault serve can list, each finding naming its `vault` |
| `GET /api/promotions` | none | `{"promotions":[{"vault","path","id","title","shared_pr","status_at"}]}` for notes with status promoting |
| `GET /api/doctor` | none | `{"state_dir","layout_dir_writable","config","config_error","vaults":[{"name","path","resolved","readable","error"}]}` |

`/api/graph` with `vault` walks only that vault's notes; notes they link to
in other vaults still appear. `center` must be in that vault then. Hybrid
search reranks when the config's `semantic.rerank_default` is on. With
Ollama down it answers 200 with the literal hits and a `warnings[]` entry,
as the CLI does.

`/api/doctor` probes the vaults on each call, not from the snapshot, so a
vault serve cannot read is named. `config` is the config file serve was
pointed at, and `config_error` (omitted when empty) says why serve runs
with no vault. `brain doctor` compares it with what the CLI sees.

### Graph nodes and edges

A node is `vault.GraphNode` plus three fields:

- `stale` (bool): the note is in the stale queue (past `review_by`, or a
  raw note past `expires_at`; disputed; resting on a revoked note).
- `x`, `y` (numbers, omitted when unknown): the position from the layout
  cache.

An edge is `vault.GraphEdge`. `kind` is one of:

- `link`: a wikilink.
- `tag`: note to tag node, only with `topics=1`.
- `supersedes`: from a note to the note in its `superseded_by`.
- `source`: from a note to a note its `sources[]` names.
- `moc`: a link out of a note in `MOCs/`.

A `supersedes` or `source` edge appears only for a ref that resolves to
exactly one note.

### Notes and links

`body` is the markdown as stored. The browser renders it. `links` lists each
wikilink in the body as written (`text`, for example `[[pg-failover|Failover]]`),
the node id it resolves to (`target`), and `resolved`. The browser turns
each one into an anchor by matching `text`, so link resolution stays in Go.
`banner` is the one-line notice for a superseded, retracted or disputed
note, and is empty otherwise.

## Layout

The layout key is the sha256 of the layout version, each vault's
`name|fingerprint`, and the topics flag. d3-force runs in the browser.
On a hit, `/api/graph` returns the positions cached under the key with
`"cached":true`. On a miss it returns the positions of the newest layout
file, an older key's, with `"cached":false`, so notes already placed keep
their place. The browser then:

1. Takes the positions `/api/graph` returned for nodes it still knows, or
   the ones it last drew.
2. Groups the notes into communities and gives each community an island
   (see Islands below).
3. Starts each new node on a small spiral next to the neighbours it links
   to, or on a spiral at its island's centre when it links to no placed
   node, as every node does when nothing is placed yet.
4. Runs the simulation for up to 250 ms before the first draw, hotter the
   larger the share of new nodes, then live until it comes to rest.
5. POSTs the positions with the key it was given once it rests.

Only the global layout is posted: a run over every node the default
filters show (dangling targets stay hidden by default), with no search and
no community hidden in the legend. A
page opened with a vault filter, or on a local graph, settles what it
shows and posts nothing; the next run over the whole graph picks up the
settling that is left and posts once it rests. With every node cached the
simulation starts cold, nothing moves and nothing is posted. The server writes the file atomically and keeps the
newest three. Layout version 3 is the d3-force layout with islands; files
of versions 1 and 2 are not read.

### Islands

The global graph shows each community of notes as an island, with empty
space between the islands:

- Louvain (`graphology-communities-louvain`, with a fixed seed) splits the
  notes the default filters show into communities, numbered by size. Tags
  and dangling targets join the community most of their neighbours are in.
  Notes with no link share one island of their own.
- Each island is a disc sized to its notes. The discs are packed around the
  middle with a gap of half their mean radius, or at least 120 units,
  between any two. An island whose notes are already placed stays at their
  centroid.
- A force holds every node on its island: a pull toward the centre, and a
  stronger one back from beyond the edge. A link between two communities
  pulls at 2% of its strength, and the global graph draws it faint.
- A poll keeps every note's community, so the islands stay where they are.
  A new note joins the community most of its neighbours are in; new notes
  linked only among themselves form a new island.
- A reload reads the islands back out of the positions: nodes whose
  circles are less than 100 units apart share an island. A graph that
  changed while the page was closed keeps its islands, even when Louvain
  would now split it another way.
- A filter, a search or a community hidden in the legend keeps the
  communities and the islands: the nodes still shown gather on their own
  islands, and the hidden ones keep their places.
- A local graph has no islands: its notes spread around the centre with the
  plain forces.

The simulation stays live. Dragging a node on the global graph pulls its
linked neighbours along; on release the node springs back among them, and
once the graph rests the browser POSTs the positions under the current key.
From the start of a drag until the graph rests, the physics run slower
(more friction, less heat), so nodes drift instead of jumping.
A local graph, or a graph a filter or a search reduced, is never posted;
a drag or a filter that moved the global graph is posted after the next
run over the whole graph rests.

## Browser

- `assets/brain-graph.bundle.js` sets `window.BrainSigma` to `{Graph,
  Sigma, MarkdownIt}`, the sigma 4 drawing pieces the UI composes
  (`sdfCircle`, `layerFill`, `layerBorder`, `pathLine`, `pathCurved`,
  `pathLoop`, `extremityArrow`, `layerPlain`) and the d3-force pieces
  (`forceSimulation`, `forceLink`, `forceManyBody`, `forceCollide`,
  `forceX`, `forceY`), and `louvain`, which finds the communities.
- `graph.js` sets `window.BrainGraph` to pure helpers with no DOM access:
  filters, colour roles, node radii and captions, depth search, the
  simulation settings, where a new node starts, camera framing, search
  hits, community names, the legend's community toggles and the side
  panel's facts and neighbour list.
- `app.js` is an IIFE over `window.Webkit`, `window.BrainSigma` and
  `window.BrainGraph`. It polls `/api/state` every 10 seconds and refetches
  when the key changes.
- The page is one toolbar row over the graph: Global or Local with the
  depth, the search with its Literal or Hybrid toggle, a Filters button
  and a Review button. Filters opens a popover with the vault, colour,
  tier, tag and status selects and the Orphans, Dangling, Size by links
  and Topics toggles; the button counts the filters set away from their
  defaults. Colour is by tier by default, or by vault, evidence, status,
  staleness or cluster: each community in its own colour, named in the
  legend after its note with the most links. Review counts what needs action: promotions, the stale queue
  and check errors. Check warnings show in its Check tab only.
- In the cluster colour mode each community in the legend is a toggle
  button: a press hides that community's notes and their edges and dims
  the entry, the next press shows them again. The entry for the
  communities past the twelfth colour toggles them all, and Show all
  brings back every hidden community. A note shows only when the filters
  and the legend both let it; topic nodes and dangling targets follow the
  notes still shown, as they do for a filter. The camera stays where it
  is. Another colour mode shows every community and keeps the hidden set
  for when cluster comes back. The set lasts until the page reloads.
- A click on a note opens it in a popup over the dimmed graph, which Esc,
  the close button or a click outside closes; a double click opens its
  local graph. A press that moves 5 px or more is a drag, never a click.
  The review queues open in the same popup. When the vault changes, the
  open note is read again in place, keeping its scroll position.
- A click on any node also opens the side panel, docked on the right of
  the graph, and a double click moves the panel to the node it makes the
  local graph's centre. The panel shows the node's title, its tier (or,
  for a topic node or a dangling node, its kind), its links (its degree:
  the edges that touch it, so a link both ways counts twice), the name of
  its community, and its neighbours sorted by title, with a count of
  those the filters, the legend, a search or the local graph keep out of
  view. When the graph does not draw the node itself, the panel says it
  is not in the current view. A click on a topic node still toggles the
  tag filter, and on a dangling node still names its link in a toast.
  The panel is no dialog: while the popup is open its backdrop covers the
  panel, and the panel stays open when the popup closes, with the node's
  selected halo. A neighbour's button moves the camera onto that node at
  the zoom it has, selects it and shows it in the panel, without opening
  the popup; focus moves to the first of the new neighbours, or to the
  close button when there is none. Open note opens it in the popup. A
  link followed inside the popup moves the panel too. Esc inside the
  panel, or its close button, closes it. The panel reads the graph the
  page already holds and makes no request of its own.
- A search reduces the graph to the notes it finds and the edges among
  them; Esc in the search box clears it.
- The UI fetches the whole graph (`/api/graph` with only `topics`) and
  applies the vault filter and the local graph around a note in the
  browser. It never sends `/api/graph` a `vault`, `center` or `depth`; the
  API still takes them. Search does send `vault`.
- The URL keeps `vault`, `color`, `note` (the note the popup shows),
  `view=local` with `center` (the local graph's note), `depth` and
  `topics`. A link with `view=local` and no `center` centres on `note`.
- serve sets no Content-Security-Policy. The UI starts no worker, so one
  that is added needs no `worker-src`.

## Doctor checks

`brain doctor` gains `serve-reachable`, `version-skew`, `serve-vaults` and
`serve-state-dir`. The last one fails while the CLI and serve resolve
different state dirs.
