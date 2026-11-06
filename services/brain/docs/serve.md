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
`name|fingerprint`, and the topics flag. ForceAtlas2 runs in the browser.
On a hit, `/api/graph` returns the positions cached under the key with
`"cached":true`. On a miss it returns the positions of the newest layout
file, an older key's, with `"cached":false`, so notes already placed keep
their place. The browser then:

1. Takes the positions `/api/graph` returned for nodes it still knows.
2. Places new nodes near their neighbours with `circular`.
3. Runs ForceAtlas2 in its worker.
4. POSTs the positions with the key it was given.

The server writes the file atomically and keeps the newest three.

A node dragged on the global graph stays where it is dropped: ForceAtlas2
settles the rest around it for about a second, then the browser POSTs the
positions under the current key, so the node keeps its place on reload. A
drag on a local graph is never posted.

## Browser

- `assets/brain-graph.bundle.js` sets `window.BrainSigma` to
  `{Graph, Sigma, forceAtlas2, FA2Layout, circular, MarkdownIt}`.
- `graph.js` sets `window.BrainGraph` to pure helpers with no DOM access:
  filters, color maps, depth search, node sizes, and the orphan and
  dangling toggles.
- `app.js` is an IIFE over `window.Webkit`, `window.BrainSigma` and
  `window.BrainGraph`. It polls `/api/state` every 10 seconds and refetches
  when the key changes.
- The UI fetches the whole graph (`/api/graph` with only `topics`) and
  applies the vault filter and the local graph around a note in the
  browser. It never sends `/api/graph` a `vault`, `center` or `depth`; the
  API still takes them. Search and the notes list do send `vault`.
- serve sets no Content-Security-Policy. One that is added must allow
  `worker-src blob:`: the ForceAtlas2 worker starts from a Blob URL.

## Doctor checks

`brain doctor` gains `serve-reachable`, `version-skew`, `serve-vaults` and
`serve-state-dir`. The last one fails while the CLI and serve resolve
different state dirs.
