# brain serve: HTTP API and UI contract

`brain serve` is the read-only web view of the vaults at brain.this. This
page is the contract the server, the UI and the docs build against. Change
it here first when a shape changes.

## Server

- Listens on `127.0.0.1:<port>`. The port comes from `--port`, then
  `BRAIN_PORT`, then `DefaultPort` (7427) in facts.go.
- Never writes a vault, and never runs git or gh. Its one write is
  `POST /api/layout`, into `<state dir>/layout/`.
- Starts with no vault configured, and reports that through `/api/doctor`
  and the UI instead of exiting, so a service manager does not restart it
  in a loop.
- Every `/api/` response is JSON with `Cache-Control: no-store`. An error is
  `{"error":{"code","message"}}` with HTTP 400 (bad parameter), 404 (no such
  note or route), 409 (stale layout key), 413 (body too large), 503
  (semantic search unavailable) or 500.

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

## API

Where a route says "as the CLI", it calls the same library function as that
command and returns the same JSON as its `-o json` output.

| Route | Query or body | Response |
|---|---|---|
| `GET /api/vaults` | none | `{"vaults":[{"name","shared","default","notes"}]}` |
| `GET /api/state` | none | `{"key","vaults":[{"name","fingerprint"}]}`; fingerprints are memoized for 2 seconds |
| `GET /api/graph` | `vault`, `center` (a note ref), `depth` (1 to 5, default 1), `topics` (0 or 1) | `{"key","layout":{"cached"},"nodes":[...],"edges":[...]}` |
| `POST /api/layout` | `{"key","positions":{"<node id>":[x,y]}}`, at most 8 MiB | 204; 409 when `key` is not the current key; ids not in the current graph are dropped |
| `GET /api/notes` | `vault`, `tier`, `tag`, `status`, `limit` | as `brain list` |
| `GET /api/note` | `ref` | `{"ref","vault","path","frontmatter","body","sha","status","banner","links":[{"text","target","resolved"}]}` |
| `GET /api/backlinks` | `ref` | as `brain backlinks` |
| `GET /api/search` | `q`, `mode` (literal or hybrid), `vault`, `tier`, `include_revoked` (0 or 1), `limit` | as `brain search` or `brain hybrid-search`; 503 `semantic_unavailable` when hybrid has no semantic index |
| `GET /api/stale` | none | as `brain stale`, without drift; the response adds `"drift":false` |
| `GET /api/check` | none | as `brain check` over every vault |
| `GET /api/promotions` | none | `{"promotions":[{"vault","path","id","title","shared_pr","status_at"}]}` for notes with status promoting |
| `GET /api/doctor` | none | `{"state_dir","layout_dir_writable","vaults":[{"name","path","resolved","readable","error"}]}` |

### Graph nodes and edges

A node is `vault.GraphNode` plus three fields:

- `stale` (bool): the note is in the stale queue (past review_by, disputed,
  or resting on a revoked note).
- `x`, `y` (numbers, omitted when unknown): the cached layout position for
  the current key.

An edge is `vault.GraphEdge`. `kind` is one of:

- `link`: a wikilink.
- `tag`: note to tag node, only with `topics=1`.
- `supersedes`: from a note to the note in its `superseded_by`.
- `source`: from a note to a note its `sources[]` names.
- `moc`: a link out of a note in `MOCs/`.

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
On a cache miss the browser:

1. Takes the positions `/api/graph` returned for nodes it still knows.
2. Places new nodes near their neighbours with `circular`.
3. Runs ForceAtlas2 in its worker.
4. POSTs the positions with the key it was given.

The server writes the file atomically and keeps the newest three.

## Browser globals

- `assets/brain-graph.bundle.js` sets `window.BrainSigma` to
  `{Graph, Sigma, forceAtlas2, FA2Layout, circular, MarkdownIt}`.
- `graph.js` sets `window.BrainGraph` to pure helpers with no DOM access:
  filters, color maps, depth search, node sizes, and the orphan and
  dangling toggles.
- `app.js` is an IIFE over `window.Webkit`, `window.BrainSigma` and
  `window.BrainGraph`. It polls `/api/state` every 10 seconds and refetches
  when the key changes.

## Doctor checks

`brain doctor` gains `serve-reachable`, `version-skew`, `serve-vaults` and
`serve-state-dir`. The last one fails while the CLI and serve resolve
different state dirs.
