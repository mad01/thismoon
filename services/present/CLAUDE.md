# present, CLI + MCP server for served briefing pages

Go CLI + MCP server. Manages single-page HTML presentations as create / read / update / list. A page carries a brief (the scrollable content), a slide deck, or both under one id, each authored as its own Doc (see *Decks* under *How it works*). Delete exists only in the web index (confirm dialog → `DELETE /p/{id}`), not as an MCP tool. The index can also import a markdown file as a new page (`POST /api/import`, local mode only; see *Importing markdown*). Content is authored as structured JSON (Doc format) and compiled to HTML at authoring time. The compile step is `internal/render` `Compile`/`CompileDoc` around `RenderDoc`, plus `RenderGraph`, run by `present_create`/`present_update`/`present rerender` and the import.

**The page view renders client-side**: `GET /p/{id}` serves a static chrome-only shell (`internal/server/shell.html`). Then `app.js` (served at `GET /app.js`) fetches the page as JSON from `GET /api/p/{id}` and builds the DOM in the browser. It replaces the old live-reload script with `watchVersion`: a server-sent event stream on `/p/{id}/events` where the server has a change feed, else `Webkit.poll` on `/p/{id}/version`. The **index** view (`GET /`) renders client-side the same way: it serves a static shell (`internal/server/index_shell.html`). Then `index.js` (served at `GET /index.js`) fetches the page list as JSON from `GET /api/pages` and builds the list in the browser. Pages are served over localhost, except on a shared instance (`serve --shared`), which is network-facing. See `docs/adr/0005-webkit-client-side-rendering.md`.

## Module layout

```
present/
  cmd/present/         - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra command tree: root, serve, mcp, version (build metadata from the shared buildinfo package);
                         share.go adds share, unshare, key new, and sharer(), which turns --shared-url/--author-key into a
                         sharedclient.Client (nil, with a stderr warning, when only one is set)
    store/             - Store interface (create/read/update/list + GetMeta + sources + SetShared + Ping) with FS, the filesystem
                         implementation over pages/<id>/ (store.go, doc.go, id.go); deck.go adds the deck remote control
                         (DeckController, implemented by FS alone over deck-command.json); WithoutExpired wraps any
                         Store to hide expired pages; Delete is web-index-only, not exposed via MCP
    store/k8sstore/    - Store over Page custom resources for the shared instance: dynamic client + unstructured, no codegen; crd.yaml embedded and copied to deploy/base/crd.yaml (a test enforces byte equality); the kustomize base installs it, and the test-only installCRD in crd_install_test.go puts it in a cluster for the kind suite; cache.go is the metadata-only page cache (an informer built on `cache.NewSharedIndexInformerWithOptions`, not `dynamicinformer`, which imports all of k8s.io/api) that RunCache starts and GetMeta and the sweeper read once it has synced; feed.go fans the informer's events out to WatchPage subscribers, the server's change feed for event streams; SweepExpired/RunSweeper purge expired ephemeral pages; RESTConfig (in-cluster, else kubeconfig) and ResolveNamespace
    author/            - author keys for the shared instance: NewKey, Hash, FromHeader (bearer), Check (ErrMissing→401, ErrMismatch→403)
    baseurl/           - public base URL from X-Forwarded-Proto/Host: Middleware fills them from the request, FromHeader derives scheme://host
    sharedclient/      - client a local present uses against a shared instance: Create/Replace/Delete/WhoAmI over its JSON API with the
                         author key as a bearer token; Share pushes a page bundle (title, content, graph, references, doc, graph_source,
                         ephemeral) and records the result via store.SetShared, Unshare deletes the copy and clears the record
    render/            - Doc-to-HTML renderer (doc.go), Graph-to-JS renderer (graph.go), legacy raw-HTML upgrade (upgrade.go); compile.go
                         holds Compile (Doc JSON in), CompileDoc (parsed Doc in), and CompileGraph (parsed GraphInput in), the one
                         parse → render → canonical JSON step the MCP tools, present rerender, and the import all call
    mdimport/          - markdown file → title + Doc + optional graph (goldmark, GFM): Convert(name, src); pure, no HTTP; the mapping
                         and its losses are under *Importing markdown* below
    mermaid/           - Mermaid flowchart source → render.GraphInput: Convert(src); pure; the import calls it for a fenced mermaid
                         block, anything it cannot parse (a sequence diagram, unsupported syntax) stays a code block
    server/            - HTTP handlers per Mode: local = GET / (static index shell), /api/pages (page list as JSON), /index.js, POST /api/import (import.go; markdown to page), GET /img/{name} (images.go; a stored image, immutable), POST /p/{id}/share (share.go; only when a shared instance is configured); shared = GET / (how-to shell), POST /api/pages, PUT /api/p/{id}, GET /api/whoami, /mcp; both = /p/{id} (static shell.html; a deck-only page redirects to its deck), /p/{id}/deck (deck.go; the same shell, redirects to the brief when the page has no deck), /api/p/{id} (page as JSON, plus a share block in local mode), /app.js, /viz.js (the pure step helpers, window.PresentViz, loaded before app.js and tested under node in internal/server/test), /logo.png (logo.go; the embedded repo logo the deck chrome shows, served no-cache with a content ETag; a test keeps it byte-identical to docs/assets/logo.png), /p/{id}/version, DELETE /p/{id} (author-checked when shared), GET /p/{id}/events (events.go) whenever Options.Watcher is set, which serve does for the k8s store, and GET /p/{id}/deck/command (deck.go) whenever the store is a DeckController, which only the raw filesystem store is; embeds index_shell.html, shared_index_shell.html, shell.html, index.js, app.js, viz.js
    mcpserver/         - MCP wiring + present_* tools; Mode picks the tool set and whether the bearer/forwarded headers on req.Extra.Header are read; present_share is registered only when Config.Sharer is set, present_deck only when the store is a DeckController; images.go turns an image block's local file into a stored image before the Doc compiles (local mode only)
    images/            - the local image store under <workdir>/images/: content-addressed names (sha256 hex plus png, jpg, gif, or webp), ValidName/NameOf/Referenced for the /img/ path rule the renderer, the MCP tools, the server, and the share guard all use, Sniff for the content-type check, Sweep for the delete-time cleanup
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- **`present mcp`** (stdio MCP server): tools write/read pages under the workdir. Never serves HTTP.
- **Decks.** A page id carries up to two renditions: the brief (`content` → `content.html` + `doc.json`) and the deck (`deck` → `deck.html` + `deck.json`), each a Doc authored on its own, never converted from the other (docs/adr/0019). At least one must exist; `present_create` refuses neither and `present_update` refuses stripping the last one. Both share the title, the one graph, the references, and the share record. `GET /p/{id}` shows the brief, `GET /p/{id}/deck` the deck, and a deck-only page redirects from the first to the second. The deck view splits `deck.html` client-side: the nodes before the first section make the title slide, every `wk-section` is one slide, `wk-toc` is dropped, references are the last slide. The deck Doc's chrome fields (`logo`, `logo_position`, `progress`, `presenter`, `footer`) travel as a `script.deck-chrome` JSON island at the head of `deck.html`, emitted only when one is set; `app.js` pulls it out of the hero and builds a bottom strip (footer line left in `--text-3`, dots or bar centre, logo right; a bottom-left logo swaps sides with the footer, a top corner logo is its own fixed element). Defaults with no island: logo shown from `/logo.png`, one dot per slide (a bar above 24), no footer line. `show()` updates the marker; a dot click jumps. The strip stays visible while presenting at a z-index above the fixed slide, whose bottom padding becomes `max(5vh, 3.25rem)` through `html.has-deck-strip` (not named after the strip's own class, or the strip rules would hit the html element). `decorateSlide` copies `data-layout`, `data-tone` (with `--slide-accent`), and `data-reveal` from the `wk-section` to the slide, adds `solo-stat`/`solo-quote` when the section's only block is one, reads the notes template, and builds the slide's steps: on a reveal slide each `li` of a top-level list and every other top-level block is one `.fragment`, and a chart with `steps` (`data-steps`) adds its own steps right after the fragment holding it, on a slide without reveal too (`PresentViz.stepPlan` in `viz.js` orders them; `applySteps` writes `data-step-at` on the block, calls its `_presentStep` hook when built, and marks the current caption `li.current`); `show(i, stepsMode)` wraps its body in `apply()` and runs it through `document.startViewTransition` (the active slide carries `view-transition-name: deck-slide`; `deck-next`/`deck-prev` on html pick the 24 px keyframes for `transition: slide`; `deck-vt` turns the fallback `briefEnter` off) unless the transition is none, Reduce Motion is on, the API is missing, or it is the first showing; `next`/`previous` step fragments first, and the remote next/prev go through them. Charts on a slide build on its first showing (`buildChart(b, {duration: 400})`), a theme change rebuilds only built ones, and the cytoscape shim adds `ready` to the container on the first `layoutstop` so the shell fades it in. The notes drawer (`.deck-notes-drawer`, N key, a bar button when any slide has notes) sits outside every slide; the renderer's inert `template.deck-notes` keeps its own class. The deck view is served from `deckShell`, the page shell with the audio toggle's per-view key and off default on `<html>`, so a deck opens without the read-aloud controls while the brief shows them. Keys: Right, Space, or PageDown next; Left, PageUp, or Backspace previous; Home and End. F or P start presenting (chrome hidden, one slide filling the window, browser fullscreen requested best-effort). They ask for fullscreen again when pressed while presenting without it, and end presenting when pressed in fullscreen. Escape always ends it. A `#<n>` hash names the slide, and the tab keeps its slide and presenting mode in `sessionStorage` (`present-deck:<id>`) so the reload an update triggers resumes where it was. The browser-fullscreen request is refused without a gesture, so a resumed presentation fills the window until the reader presses F, which asks again with the gesture the request needs. Remote control (`present_deck`, `present deck`) writes `deck-command.json`, which keeps the last 32 commands. Every open deck tab polls `GET /p/{id}/deck/command?after=<seq>` once a second and applies what came after the sequence number it last ran. A fresh tab runs only a command sent within the last ten seconds, so present_open followed by present_deck start works. Every tab remembers that number with the slide. Remote control exists on the filesystem store only, so a shared instance serves decks without it.
- **`present serve`** (HTTP server): serves the static shell + `app.js` for page views (the browser fetches `GET /api/p/{id}` and renders), and the static index shell + `index.js` for the index (the browser fetches `GET /api/pages` and renders).
- **`present serve --shared`** (shared instance): one process, meant for several replicas in Kubernetes. No index or listing; pages by 32-hex capability id; every write needs an author key as a bearer token (`internal/author`). Ephemeral pages expire 30 days after their last write (`present.SharedTTL`, hidden by `store.WithoutExpired` until the sweeper deletes them). The tool server is mounted at `/mcp` via `mcp.NewStreamableHTTPHandler` in stateless mode with one `*mcp.Server` for all requests. Requires an explicit `--bind`; local mode refuses any bind but loopback (`checkBind` in `serve.go`).
- **Sharing a local page** (`--shared-url` + `--author-key`, env `PRESENT_SHARED_URL`/`PRESENT_AUTHOR_KEY`; both persistent, both required, one alone warns on stderr and stays off): the page view's Share button (`POST /p/{id}/share`), `present share <id> [--ephemeral]`, and the `present_share` tool all call `sharedclient.Share`. It pushes the page bundle to the shared instance (`POST /api/pages`, or `PUT /api/p/{id}` to replace an earlier copy under the same link; Share recreates a purged copy). It also records `{id,url,ephemeral,expires_at,shared_at}` in the local `meta.json` `shared` field. `present unshare <id>` deletes the copy and clears the record. `present key new` mints the author key; the shared instance keeps only its hash, so after losing a key you mint a new one and re-share.
- Both share the workdir and port (`7423`), set via `--workdir`/`PRESENT_WORKDIR` and `--port`/`PRESENT_PORT`. URLs the MCP returns point at the serve port. The workdir default is sticky: `~/.config/present` while that directory exists, else `~/.local/state/present` (`confdir.StateDir`) — pages are never migrated, so an existing store keeps being read. The recipe and the MCP sandbox wrapper both set it explicitly anyway.
- **Read-aloud goes through speak** (`--speak-url`/`PRESENT_SPEAK_URL`, serve only, default `present.DefaultSpeakURL` = `http://speak.this`; empty turns it off). The URL reaches the page as `speak_url` in `GET /api/p/{id}`. Then `app.js` creates `<wk-read-aloud targets=".brief-summary, wk-section" prepare name="<title>" endpoint="<speak url>">` right after `.brief-summary` (before the first `wk-toc`/`wk-section` on a page without one), so the status bar webkit renders inside the element sits under the summary. webkit does the rest: it registers the page's readable blocks with `POST {speak}/read` and stamps them with the part keys. It shows "Audio: N of M parts ready" with Generate all TTS for page and Retry failed, and gives each section a state badge plus Retry. It polls `GET /doc/{id}` while parts are in progress, and registers again on a 404 (speak restarted). Fallbacks, all webkit's: registration refused (a non-speak endpoint, say) means live per-sentence reading with no bar; speak unreachable means no read-aloud at all; an empty `speak_url` means `app.js` creates no element. Code blocks and tables are never registered. The shared manifest sets `PRESENT_SPEAK_URL=""` because no speak runs beside the instance.

## Data model & storage

```
~/.config/present/
  pages/<id>/
    meta.json              # {id,title,version,has_graph,has_refs,has_brief,has_deck,has_doc,created_at,updated_at,shared?}; shared = {id,url,ephemeral,expires_at,shared_at} once pushed
    content.html           # rendered HTML body fragment of the brief (empty file on a deck-only page)
    doc.json               # canonical Doc source (only when content was given as Doc JSON)
    deck.html              # rendered HTML fragment of the deck (only when has_deck)
    deck.json              # canonical deck Doc source (only when has_deck)
    deck-command.json      # the last 32 remote commands [{seq,action,slide?,at}] for open deck tabs (only after present_deck or `present deck`)
    graph.js               # optional rendered cytoscape init (only when has_graph)
    graph.json             # canonical GraphInput source (only when graph was given as JSON)
    refs.json              # optional [{title,url},...] (only when has_refs)
  images/<sha256>.<ext>    # image files the image blocks of any page brought in from this machine (png, jpg, gif, webp), served at /img/<name>
```

`doc.json`, `deck.json`, and `graph.json` are the editable sources: `present_source` returns
them for mutation from a later session, and `present rerender` re-renders
`content.html`/`deck.html`/`graph.js` from them after renderer or template changes (e.g. a
webkit bump). A local `present serve` runs the same sweep in the background at
startup (`--rerender`, on by default). So a binary upgrade refreshes every stored
page once the service restarts, and the command stays for on-demand use. Pages
created from raw HTML/JS have no source files; rerender falls back to a
deterministic legacy-HTML upgrade and leaves legacy JS graphs untouched.

A section `id` in a stored source is accepted and ignored. MAD-377 removed the
badge it used to render, and the field stays only so older sources and the
callers that still send it keep parsing. The legacy-HTML upgrade drops the
badge element too.

## Shared instance in Kubernetes

`Dockerfile` (context = repo root) builds the image; `deploy/` holds the
kustomize base and the kind, ingress, and istio overlays. `make image`
builds for the host arch. `make kind-test` loads it into the kind cluster
named `KIND_CLUSTER` (default `kind`, reused when it exists), applies
`deploy/overlays/kind`, rolls the deployment, and runs `scripts/kind-e2e.sh`
(the k8sstore conformance suite with `PRESENT_KIND_TEST=1` plus
`internal/e2e` over a port-forward). `make kind-down` removes the `present`
namespace and leaves the CRD. CI runs the same in the `present-kind` job on
every PR touching present, pushes `main`/`sha-*` tags on main, and the
release workflow pushes `X.Y.Z`/`latest` (`docs/adr/0018`,
`docs/RELEASING.md`). Machine-private overlays (hostname, gateway, author
keys) live in the consuming repo.

## Build / install / test

```bash
make build    # ./present binary
make install  # build + cp to ~/code/bin/present + adhoc codesign
make test     # go test ./... then node --test on internal/server/test (skipped without node)
make tidy     # go mod tidy
make resign   # re-apply adhoc signature (macOS)
```

## Commands

`present rerender [id...]` (all pages when no ids) re-renders each page through
the current renderer: from `doc.json`/`deck.json`/`graph.json` when present, otherwise a
deterministic legacy-HTML class→wk-* upgrade. Unchanged pages are skipped;
changed ones get a version bump so open tabs live-reload (outcomes name what
moved: `re-rendered-from-doc`, `re-rendered-deck`, `+deck`, `+graph`). A local
`present serve` runs the same sweep in the background when it starts
(`rerenderSweep`; `--rerender=false` turns it off) and logs one summary line.
The fleet needs no manual run after an upgrade. A shared instance never sweeps:
its pages arrive rendered and it runs several replicas.

`present deck <id> <start|stop|next|prev|goto> [slide]` drives the deck open in
the browser: it writes the page's `deck-command.json` with the next sequence
number and prints `<id>: <action> seq <n>`; `goto` takes a 1-based slide and the
other actions refuse a third argument. It needs only the workdir, so it works
with serve down, and a tab opened later ignores commands sent before it loaded.

`present share <id> [--ephemeral]` pushes a page to the shared instance named
by `--shared-url` and prints its link (the expiry goes to stderr when
ephemeral); `present unshare <id>` removes the copy; `present key new` prints a
fresh 64-hex author key. share and unshare need both `--shared-url` and
`--author-key`; key new needs nothing.

## Importing markdown

The index has an "Import markdown" button (a hidden `<input type=file>` behind a `wk-button`) and takes a file dropped anywhere on the page. `index.js` reads the file in the browser and posts `{"name": "notes.md", "markdown": "..."}` to `POST /api/import`. The server runs `mdimport.Convert`, then `render.CompileDoc` (and `render.CompileGraph` when the file held a mermaid flowchart), then `store.Create`. That last call stores the same fields an MCP-created page gets: rendered `content.html` plus `doc.json`, and the graph script plus `graph.json` when there is a graph. After that the server emits the same `page created` event and answers `201 {"id", "url"}`. The browser then navigates to `/p/{id}`.

Errors come back as `{"error": "..."}` and land in a `wk-toast`. The status is 415 without `Content-Type: application/json`. It is 403 when `Sec-Fetch-Site` is `cross-site` or an `Origin` is present that is not http(s) on localhost, a loopback address, or a `.this` host. The request's own Host is never consulted: a DNS name pointed at 127.0.0.1 would agree with itself. The status is 413 when the request body or the stored page passes `present.MaxPageBytes`. The page is measured the way a shared instance measures it, HTML plus `doc.json`, so an imported page can always be shared later. That is about 250 to 500 KiB of markdown, less for markup-heavy files. The status is 400 for an empty file or one that yields no blocks.

Shared mode does not register the route: writes there need an author key, and there is no index to import from.

Mapping (goldmark, GFM plus footnotes so both halves can be dropped):

| Markdown | Doc |
|----------|-----|
| first `#` | page title; later `#` behave like `##`. No `#`: the file name without extension, else "Untitled" |
| `##` | new section (`h` = heading as plain text) |
| `###` to `######` | `h3` block, plain text |
| text before the first `##` | a leading paragraph becomes `summary` when anything else follows it; the rest becomes an "Introduction" section. With no `##` at all the page has one section named after the title |
| paragraph | `p` with inline markup rewritten: `*x*`/`_x_` → `*x*`, `**x**`/`__x__` → `**x**`, code span → backticks. `[t](u)` → `[t](u)` when `u` is http, https, mailto, relative, or a fragment (`render.LinkHrefAllowed`, which the renderer enforces for every Doc's inline links; anything else, `javascript:` above all, renders as literal text). Otherwise the label alone. `)` in a URL encoded as `%29`, bare URLs and `<u>` → `[u](u)`, emails → `[a](mailto:a)`, strikethrough → its words, image → its alt text, line breaks → a space |
| list (bullet, ordered, task) | `list` with flat `items`; nested items follow their parent, paragraphs of one item join with a space, task boxes become `[x] `/`[ ] `. A code block, table, or quote inside an item ends the list block, is emitted on its own, and a new list block follows (an ordered list restarts at 1) |
| fenced or indented code | `code` with `lang` from the fence |
| fenced `mermaid` | the first block `mermaid.Convert` accepts (a `graph`/`flowchart` header) becomes the page graph: a `graph` placement block where the fence was, plus the page-level Graph JSON. Present has one graph per page. So a later flowchart, and any mermaid block that is not a flowchart or uses syntax the converter does not know, stays a `code` block with `lang: mermaid`. Flowchart mapping: node shapes are dropped and labels kept (`<br>` becomes a line break, `#quot;`-style entity codes are decoded). Dotted links become dashed (`publishes`) edges. Every other link becomes a solid arrow (open, thick, cross, circle, and bidirectional links lose their distinction; `<-->` is one edge). `\|text\|` and inline link text become edge labels. `A & B --> C` fans out. A `subgraph` colours its members as `module` nodes (one colour per subgraph, four cycle; a node belongs to the first subgraph whose body mentions it) since present has no compound nodes. A flowchart past 2000 nodes or 5000 edges is refused so a fan-out like `a&a&a --> b&b&b` cannot blow up the import. `TD`/`TB`/`BT` map to top-down and `LR`/`RL` to left-right. A `label:` key in an `@{...}` shape block names the node while the rest of the block is dropped. YAML front matter, `accTitle`/`accDescr`, edge ids (`e1@-->`), `classDef`, `class`, `style`, `linkStyle`, `click`, and `:::class` are ignored. `internal/mermaid/testdata` holds every flowchart from mermaid's own docs and demo pages; `TestConvertCorpus` requires all of them to parse |
| table | `table`: header cells plain, body cells through the inline mapping |
| blockquote | one `callout` `info` with all its blocks flattened into one text; `[!WARNING]`/`[!CAUTION]` on the first line make it `warn`, `[!NOTE]`/`[!TIP]`/`[!IMPORTANT]` stay `info`, the marker is removed |
| raw HTML, `---`, footnotes | dropped |

Limits, all consequences of present's inline syntax having no escape: a bold span that holds a code span or a link loses the bold. (The renderer would print a stray token.) Emphasis inside emphasis keeps only the outer mark, a link label is plain text, and a code span holding a backtick comes out as words. Literal `](` or `@chip(` in an assembled run of prose gets a space inserted so it cannot turn into a link or chip. The run joins text nodes, code spans and dropped nodes included, so nothing can glue the halves together. A literal `*` in the source still italicises when rendered, inside a code span too. Name normalization applies to imported prose like to any Doc (` & ` reads "and", ` - ` reads "minus"); code blocks are exempt.

## MCP tools

- `present_create(title, content?, deck?, graph?, references?)` → `{id, url, has_deck, deck_url?, version}`; at least one of `content` and `deck` is required, and `url` is the deck URL when the page has no content
  - `content`: Doc JSON object `{summary?, meta?, chips?, sections:[{h, blocks}]}` or legacy HTML string (auto-detected)
  - `deck`: a second Doc JSON object with the same schema, Doc JSON only (a non-object is refused); every section is one slide, summary/meta/chips make the title slide, references the last one. The tool description carries the authoring rules (claim headings, 3 to 5 short items or two sentences per slide, one highlight per slide, 5 to 12 slides). Detail stays in the brief; the skill has the full list. The deck Doc may also set the chrome fields `logo`, `logo_position`, `progress`, `presenter`, `footer` (all optional; `render.validateChrome` refuses an unknown position or progress value and a logo that is neither `none` nor an http(s) URL)
  - Section fields beside `h` and `blocks`: `tone` (a role from `webkit.Roles()`, no aliases; rendered as `data-tone` plus `style="--slide-accent: var(--<role>)"`, banding the section in the brief and tinting the slide), and deck-only `layout` (default|center|statement|section, emitted as `data-layout` when not default), `notes` (an inert `<template class="deck-notes">` at the end of the section, outside both walkers by construction), `reveal` (`data-reveal`). Deck-level `transition` (fade|slide|none) rides in the chrome island only when set and not fade. `render.validateSection` refuses an unknown layout or tone; a Doc without any of them renders byte-identically (`render/testdata/golden-stage1.html`). `render/testdata/sample-deck.json` exercises all of it and `TestSampleDeckRenders` keeps it compiling
  - Doc blocks: `p`, `h3`, `callout` (`sev` info|warn|ok|error), `table`, `kv`, `list`, `panel`, `progress`, `graph`, `chart` (`steps: [{caption}]` and a series `step` for a chart a deck walks one step per Next; `render.validateChart` refuses an unknown kind, a step without a caption, a series step out of range, steps on a sparkline, a series step on a doughnut, and a stepped chart inside `details`), `code`, `html`, `image` (`src`, `alt`, `caption?`; see *Images* under *Gotchas*), plus the layout blocks `columns` (`cols`: 2 or 3 arrays of blocks), `stat` (`value`, `label`, `sub?`), `quote` (`text`, `cite?`), and `details` (`summary`, `blocks`). A `columns` or `details` block holds any block but `columns` and `details`. The page's one `graph` may sit in a column but not in `details`; the deck splitter and `initGraphAndFlow` find `#cy-graph` wherever it is, and `validateDoc` refuses a Doc that places it twice. `render.validateBlocks` refuses the rest and `renderBlockAt` carries a depth guard. `cols` is one wire key for a table's header strings and a columns block's arrays, told apart by `t` in `Block.UnmarshalJSON`/`MarshalJSON`, so a stored table's canonical JSON never moves. A golden fixture (`render/testdata/golden-brief.html`) pins that a Doc without the new fields renders byte-identically
  - `graph`: Graph JSON object `{nodes, edges, layout?}` or legacy JS string (auto-detected); one per page, shared by both renditions
  - `references`: `[{title, url}]`
- `present_read(id)` → full page with rendered HTML content and `deck` (includes references, `has_deck`, `deck_url`)
- `present_source(id)` → editable source: `{content_format: "doc"|"html", content, deck?, graph_format: "json"|"js"|"", graph, references, deck_url?}`: outputs round-trip directly into `present_update` (it auto-detects both formats; `deck` is the canonical deck Doc JSON, empty when the page has none). Use this to mutate a page from a new/restored session
- `present_update(id, title?, content?, deck?, graph?, references?)` → patch (omitted fields unchanged; empty graph clears it; empty deck removes the deck and its source; empty array clears refs); bumps version. Doc/graph JSON input refreshes `doc.json`/`deck.json`/`graph.json`; raw HTML/JS input deletes the now-stale source file. An update that would leave neither content nor deck is refused before anything is written
- `present_list()` → all pages (metadata only, no content), newest first; `has_doc` flags source-editable pages, `has_brief`/`has_deck` say which renditions exist, `deck_url` is set when there is a deck
- `present_open(id, deck?)` → macOS `open` (call once per page; updates auto-reload); `deck: true` opens `deck_url` and errors when the page has no deck
- `present_deck(id, action, slide?)` → `{seq, action, slide?, deck_url}`: remote control for the deck open in the browser: `start`/`stop` presenting, `next`, `prev`, `goto` a 1-based `slide`. Registered in local mode only, and only when the store is the raw filesystem store (the one that relays commands); every open tab of the deck follows within a second
- `present_share(id, ephemeral?)` → `{url, ephemeral, expires_at?, shared_at}`: push the page to the configured shared instance; registered only when `--shared-url` and `--author-key` are set. Sharing again replaces the copy under the same link; `ephemeral` makes it expire 30 days after the last share
- `present_doctor()` → the `kit/doctor` report: store readable, serve reachable, no version skew, shared instance answering `GET /api/whoami` (skipped without one); for a client that can call a tool but has no shell

On a shared instance the set is `present_create` (plus an `ephemeral` bool; the bearer becomes the page's author), `present_read`, `present_source`, `present_update` (author-checked; resets an ephemeral page's expiry), and `present_doctor` (store ping only). No `present_list`, no `present_open`, no `present_deck` (the cluster store relays no commands; decks there are keyboard-driven). The deck fields on create, read, source, and update are the same as locally. URLs come from the request's forwarded headers unless `--base-url` overrides them.

## Shared UI: webkit

The chrome (header, theme toggle, font/size/fixation controls) comes from the
in-module package **`github.com/mad01/thismoon/webkit`**, which embeds compiled
TypeScript/CSS web components. Do NOT re-add palette, topbar, or theme CSS
locally; those live in webkit only. There is no pin or bump step: the binary
compiles against the webkit committed alongside it.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // mux.Handle("GET /webkit/", webkit.Handler()) - serves webkit.css/.js + /webkit/boot.js
```

Templates load the FOUC guard (served by webkit at `/webkit/boot.js`) and the
webkit assets:

```html
<head>
  <script src="/webkit/boot.js"></script>
  <link rel="stylesheet" href="/webkit/webkit.css">
</head>
```

The boot script is blocking (no `defer`/`async`) on purpose: it must set
`data-theme` and `data-palette` before first paint to avoid a flash of the
wrong theme or family.

### Header markup (present)

The brief template uses:
```html
<wk-header back-href="/" back-label="← All" title="Brief"><a data-nav href="/webkit/themes">Themes</a></wk-header>
```

The index template uses:
```html
<wk-header brand="present"><a data-nav href="/webkit/themes">Themes</a></wk-header>
```

The Themes nav link opens webkit's palette picker, served by this binary at
`/webkit/themes` like the other webkit assets; every shell carries it, the
shared instance's included.

`webkit.js` injects the full control set (font · fixation · size ± · reload ·
theme). Do not add those controls manually.

### Per-repo changes

- Header markup lives in `internal/server/shell.html` (page view) and
  `internal/server/index_shell.html` (index listing).
- Available content components (CSS-only, no extra JS):
  `<wk-page-header>`, `<wk-card>`, `<wk-badge>`, `<wk-search>`,
  `<wk-table>`, `<wk-panel>` (+ `<wk-panel-title>` / `<wk-panel-subtitle>`),
  and the briefing-block components: `<wk-section>` (+ `<wk-section-heading>` /
  `<wk-section-subheading>`), `<wk-toc>` (+ `<wk-toc-title>`),
  `<wk-kv>` (+ `<wk-kv-row>` / `<wk-kv-label>` / `<wk-kv-value>`),
  `<wk-progress>` (+ `<wk-progress-bar>` / `<wk-progress-fill>` /
  `<wk-progress-label>`), `<wk-callout>` (`variant="info|warn|ok|error"`),
  `<wk-columns cols="2|3">` (+ `<wk-col>`, the `t=columns` block),
  `<wk-stat>` (+ `<wk-stat-value>` / `<wk-stat-label>` / `<wk-stat-sub>`, the
  `t=stat` block), a section-scoped `blockquote` + `cite` (`t=quote`) and
  `details` + `summary` (`t=details`), and
  `pre.wk-code-block` (standalone code block, the `t=code` Doc block renders
  to it). webkit.js adds the language badge, copy button, and Prism highlight.
- The index listing uses `<a class="wk-card">` + `<wk-page-header>` for the
  page list entries.
- Theme changes fire `document` event `wk-themechange` with `detail.theme`,
  `detail.palette`, and `detail.mode`: present's brief template listens there
  to recolor the Cytoscape graph and rebuild any metric charts
  (`initPresentCharts`). The event fires on the header toggle, when the themes
  page in another tab changes a family or the mode, and when the system
  appearance flips in system mode, so an open brief recolours without a
  reload.
- **Colours are palette roles, read at call time.** `getGraphColors` and
  `getChartColors` in `app.js` hold no literals: each entry is one `cssVar`
  lookup of a role webkit emits as a literal hex for every family
  (`--graph-leaf-bg`, `--tone-green-border`, `--series-2`, `--chart-grid`,
  and so on; the full set is in `webkit/src/themes.mjs`). Stored pages carry
  no colours, so a theme change is a reload at most, never a rerender. A
  chart series names its colour by the legacy names (`terracotta`, `blue`,
  `green`, `purple`) or by role (`series-1` to `series-4`); both map to the
  same index. A panel's `accent` is validated against `webkit.Roles()`, plus
  `terracotta` as an alias of `primary`. It renders as `var(--<role>)` with
  the alias resolved to its role, so it follows the family too. Pages
  rendered before that resolution store `var(--terracotta)`. A local serve
  rewrites them in its startup rerender sweep. A shared instance never
  rerenders, so `shell.html` declares `--terracotta` as `--primary` for them
  (MAD-371). The body ink override in `shell.html` reads `--text-body`
  instead of a ramp step.
- Present's briefing blocks (sections, toc, kv, progress, callout) are NOW
  webkit components: the Doc renderer emits `<wk-section>` / `<wk-toc>` /
  `<wk-kv>` / `<wk-progress>` / `<wk-callout>` (alongside `<wk-table>` /
  `<wk-panel>` / `<wk-badge>`), all styled by `/webkit/webkit.css`. Only present-
  specific chrome stays local in `internal/server/shell.html`: the hero (`.brief-title` /
  `.brief-meta` / `.brief-presenter` / `.brief-summary`), the `.chip-row` flex container, the
  Cytoscape graph chrome (`.cy-*`), the metric-chart chrome (`.present-chart`),
  `.brief a` link styling, `.brief-list`, the `.refs-*` references block, and
  the deck chrome (`.deck-strip`, `.deck-dot`, `.deck-progress`, `.deck-logo`).
- **Metric charts are present-local, not webkit.** The `t=chart` Doc block
  (`kind` from `render.ChartKinds`: `bar`/`line`/`area`/`sparkline`/`stacked-bar`/
  `horizontal-bar`/`doughnut`/`scatter`/`sankey`; `validateChart` refuses any
  other, and tests pin the list to the MCP schema text and the skill) renders
  a `<div class="present-chart">` with a JSON spec in a
  `<script type="application/json">` island.
  `chartSpec` returns `template.JS` so html/template emits it verbatim instead
  of re-encoding it inside the script context. The `initPresentCharts`
  bootstrap in `app.js` reads each spec and builds a Chart.js instance,
  mirroring how the Cytoscape graph works: a vendored lib in `assets/js/`
  (`chart-4.4.6.umd.min.js`, fetched by `scripts/cache-assets.sh`) plus a
  theme-aware color function. `sankey` also needs the vendored
  `chartjs-chart-sankey-0.15.3.min.js` (same script, loaded by `shell.html`
  right after Chart.js); when that asset is missing the block shows a note
  instead of a chart. The graph's layout engines (dagre, the ELK
  algorithms, cose) are built in `app.js` (`presentGraphLayout`), not in the
  generated graph script, which only names the engine and the resolved
  direction. The ELK options take the container's aspect ratio at run time,
  and the graph toolbar's engine button cycles the live graph through every
  engine for comparison. ELK needs the vendored `elk-0.12.0.bundled.js` plus
  the `cytoscape-elk-2.3.0.js` adapter (same script, loaded by `shell.html`
  after cytoscape-dagre). The `elk` layout is ELK layered with wrapping,
  which folds a long chain of steps into rows to fit the container. Scatter points accept `x` as a JSON number
  (`ChartPoint.UnmarshalJSON` stores it as a decimal string; the client
  parses it back). Charts are inline blocks, many per page, unlike the
  single `graph` argument. The entry animation plays once per block and is
  skipped under `prefers-reduced-motion`; a theme recolor rebuilds silently.
  A chart with `steps` (`data-steps="N"`, the captions as
  `ol.present-steps` after the canvas) builds at the step `data-step-at`
  names. Series with a later `step` start hidden, the value axis is pinned to
  the full data, and the legend lists only the series shown and ignores
  clicks. Every build registers `block._presentStep(n)`, which the deck calls
  to move the chart (no animation under Reduce Motion or `transition: none`).
  `viz.js` (`window.PresentViz`, served at `/viz.js`) holds the pure helpers
  `stepPlan` and `chartStepVisibility`, tested under node. `d3-7.9.0.min.js`
  is vendored the same way as Chart.js (docs/adr/0022, `THIRD-PARTY.md`) for
  the renderers MAD-386 and MAD-382 add; nothing reads it yet.
- **Graph edge flow is split between graph.go and app.js.** A `GraphEdge` in
  the top third of the weight range gets a `hot: 1` data flag from `graph.go`,
  and `flow: true` emits `flow: 1`. `presentGraphStyle` gives hot edges the
  accent tint, weighted edges a `mapData` width over `[0, max weight]`, and
  flow edges the dashed pattern `[10, 6]`. `startGraphFlow` in `app.js` then
  marches `line-dash-offset` over that period (16) on `cy.edges('[flow]')`, at
  6 to 20 px/s by weight, pausing via IntersectionObserver and
  `visibilitychange`, and drawing a single static frame under
  `prefers-reduced-motion`. Change the pattern in one place and the other
  must follow.
- **The graph script is data only; the style lives in app.js.** `graph.go`
  emits `initGraph()` with the elements and a `presentGraphLayout(engine,
  direction)` call, nothing else. The `cytoscape()` shim in `app.js` fills in
  `presentGraphStyle(elements)`. That builds the palette (`getGraphColors`,
  one role lookup per colour), the node and edge rules, the four module border
  colors, and the six tone triples. It also sets the `mapData` width over the heaviest
  edge weight it finds in the elements. So a palette, box, or tone change
  reaches every stored graph the next time the page loads, with no rerender.
  The shim also replaces the inline style of a script an earlier template
  rendered, recognised by a layout from `presentGraphLayout`. A raw JS graph
  with its own style and layout is left alone. `graph.go` still validates
  tones (`graphTones`) and refuses an unknown one; keep that list and
  `GRAPH_TONES` in app.js in step.

### Webkit

present is a webkit consumer like the other services in this repo. The chrome
comes from the in-module package `github.com/mad01/thismoon/webkit`. There is
no pin or bump step; the binary compiles against the webkit committed alongside
it, so a webkit change ships at the next build.

### Version check

`GET /webkit/version` → `{"module":"github.com/mad01/thismoon/webkit","version":"<asset hash>"}`:
confirms which embedded webkit assets the running present server serves.

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe (wave 1) registers it.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `present docs`): the two-process split, shared-workdir/port
  mismatch, content rules that corrupt pages, failure modes, version-skew
  checks. Keep those facts there, not here. Dev-side notes that stay: the MCP
  is pinned via `env` in the consuming repo's `recipes/claude-mcp/servers.json`
  (`PRESENT_WORKDIR`/`PRESENT_PORT`), with leading `~` expanded in Go
  (`expandTilde`, `root.go`). `serve` request logs go to stderr (`t-man logs
  present --stderr`), and a reload-loop of `GET /p/<id>` means cached HTML.
  Pages set `Cache-Control: no-store` to prevent exactly that.
- **Page view is a static shell + client render.** `GET /p/{id}` serves `internal/server/shell.html` (chrome only: `<wk-header>` + webkit assets). `app.js` builds the body in the browser from `GET /api/p/{id}` JSON (`{id,title,version,has_graph,has_deck,content,deck,graph,references,share,speak_url,deck_control}`). It mounts `content` as innerHTML, runs the `graph` field as a `<script>`, then inits the Cytoscape graph, metric charts, references, read-aloud (when `speak_url` is set; see *How it works*), and theme-recolor. The old `{{CONTENT}}`/`{{GRAPH_SCRIPT}}`/`{{REFERENCES}}`/`{{LIVE_RELOAD}}` template substitutions (`render.Render` + `template.html`) have been removed; the index renders client-side from `index_shell.html` + `index.js` the same way. The header/theme/font/size/fixation chrome lives in the in-module `webkit` package (served at `/webkit/`; see *Shared UI: webkit* above).
- **The deck view is the same shell, split client-side.** `GET /p/{id}/deck` serves `shell.html` too. `app.js` tells the two apart by the `/deck` suffix and builds slides from `deck.html`. The top-level nodes before the first `wk-toc`/`wk-section` become the title slide, each `wk-section` one slide, `wk-toc` is dropped, and the references become the last slide. The page JSON carries `deck`, `has_deck`, and `deck_control` (true when the command route exists). Cytoscape needs a visible container, so the graph script is appended at mount. But `initGraph` runs the first time the slide holding `#cy-graph` shows (and again on a theme change only while that slide is on screen). A chart is built the first time its slide shows, with a draw-in that follows the deck's transition (400 ms, none for a cutting deck), and resized on later showings. `splitSlides` returns `{slides, chrome}`; the chrome island is dropped from the hero before the title slide is built. The brief view shows a Slides link when `has_deck`, the deck view a Brief link when the page has content; the deck view keeps the read-aloud controls behind the header's Audio toggle, off by default there.
- **The two rendition URLs redirect to each other.** `GET /p/{id}` answers 302 to `/p/{id}/deck` when the page has no content, and `GET /p/{id}/deck` answers 302 to `/p/{id}` when the page has no deck. So a tab reloading after an update that removed the rendition it showed lands on the one that is left. Only an unknown page is a 404, so a stale link never gets an empty shell.
- **Remote control is a file, filesystem store only.** `present_deck` and `present deck` write `pages/<id>/deck-command.json` through `store.DeckController`, which only `*store.FS` implements. The server registers `GET /p/{id}/deck/command` and the MCP registers `present_deck` only when the store type-asserts to it, which the wrapped shared-mode store and the cluster store never do. A deck tab polls that route once a second while visible, records the sequence number on its first answer, and applies later ones. So a command sent before a tab opened is not replayed and a tab that was hidden catches up on the latest command only.
- **Older peers drop the deck silently.** A shared instance built before decks reads a pushed bundle without its `deck` and `deck_source` fields and answers success, so a deck-only page shared there is blank. An older CRD prunes `spec.deck` and `spec.deckSource` the same way. A current instance answers every write with `has_deck`. `sharedclient.Share` returns `ErrDeckDropped` (the copy stays and is recorded, the link is in the message) when a bundle carried a deck the answer does not confirm. Roll the CRD and the shared instance forward before the local presents.
- **`has_brief` is persisted, and derived for old pages.** `meta.json` carries `has_brief` and `has_deck` so `GetMeta`/`ListMeta` (and the k8s metadata cache, which drops bodies) can say which renditions exist without loading them. A `meta.json` written before decks has no `has_brief` key, and the filesystem store reads it off `content.html`'s size until the next write persists it.
- **The shared bundle and the CRD grew.** `POST /api/pages` and `PUT /api/p/{id}` take `deck` and `deck_source` beside `content`/`doc` (`sharedclient.Bundle` mirrors them; `replaceSources` saves or deletes the deck source like the others). The size cap measures the deck and its source too, and the `Page` custom resource gained optional `spec.deck` and `spec.deckSource`. Existing objects stay valid, but apply `deploy/base` before a deck reaches a cluster or the API server drops the unknown fields.
- **Client render uses webkit's shared helpers.** `app.js` builds the DOM with `Webkit.el` / `Webkit.escapeHtml` and polls `/p/{id}/version` for live-reload via `Webkit.poll` (webkit shared helpers). Decision recorded in `docs/adr/0005-webkit-client-side-rendering.md`.
- **Theme/controls state is global (webkit), not per-page.** Mode, palette family, font, size, and fixation are stored under global `localStorage` keys (`webkit-theme`, `webkit-palette-light`, `webkit-palette-dark`, `webkit-font`, `webkit-size`, `webkit-fixation`), shared across all present pages, default light with the default family; `webkit-theme` may also be `system`. The read-aloud controls' visibility is the one per-view key: `webkit-audio` for the brief (default on) and `webkit-audio-deck` for the deck (default off), named on the deck shell's `<html>` as `data-audio-key`/`data-audio-default` by `deckShell` in `deck.go`, so webkit's boot.js and the header's `audio` toggle agree before paint. (The old per-page `brief-theme:<pathname>` keys are gone.) **The page view is served from the embedded `shell.html`, not the workdir**: there is no on-disk template. `serve` and `mcp` no longer seed `~/.config/present/template.html` — the file and the `render.Render`/`EnsureTemplate` layer have been removed. A shell or `app.js` change ships by rebuild + `t-man restart present`.
- **Images are files beside the pages.** An `image` block's `src` reaches the renderer as an http(s) URL or as `/img/<sha256 hex>.<png|jpg|gif|webp>`; `render.validateImage` refuses anything else and an empty `alt`. The MCP tools do the conversion in `mcpserver/images.go`. `imageIngest` reads an absolute or `~` path (a relative one is refused), caps it at `present.MaxImageBytes`, and sniffs the type with `http.DetectContentType`: png, jpeg, gif, or webp pass, and svg is refused because it can carry script. It names the file by its hash and rewrites `src`, then compiles both Docs, and only then writes the file into `<workdir>/images/`. So a refused Doc leaves no file, and a stored page never names a missing one. `present rerender` needs nothing: the sources carry the served paths. The markdown import never makes an image block (an image there is its alt text), and no CLI command takes a Doc.
- **Stored images are served, swept, and never shared.** `present serve` answers `GET /img/{name}` from `<workdir>/images/` with the immutable cache header the assets get, and 404 for any name the store could not have written. `serve` sweeps the images no page references at startup and after every `DELETE /p/{id}`, reading `content.html`, `deck.html`, `doc.json`, and `deck.json`; files younger than a minute are skipped, and `images.Store.Write` touches an existing file instead of rewriting it, so a page the MCP is about to write keeps its image. The renditions are written through a temp file and a rename, so the sweep never reads a half-written one. A shared instance has no store, so its tools refuse an image src that is not a URL. `sharedclient.Share` refuses a page whose renditions reference `/img/` with `ErrLocalImages` rather than push a copy with broken images.
- **Codesign for MCP.** macOS kills adhoc-signed binaries with stale provenance xattrs; `make install` re-signs.
- **`present_share` reaches the shared instance only over HTTPS from the MCP sandbox.** The consuming repo registers `present mcp` through the seatbelt wrapper (`recipes/present/present.sb`), which allows outbound `:443` plus DNS and nothing else, and the wrapper passes `PRESENT_SHARED_URL`/`PRESENT_AUTHOR_KEY` through from the ralph-managed secrets file. A plain-http shared URL (a kind port-forward, say) fails there with a connection error. The Share button (served by `present serve`, launched through `present-serve.sh` with the same two vars) and `present share` run outside the sandbox and take any URL.
- **`SetShared` never bumps the version.** Sharing writes only the `shared` record in `meta.json`, so `/p/{id}/version` stays put and the open tab does not reload. The page view learns about a share from the `share` block in `GET /api/p/{id}` on load and from the `POST /p/{id}/share` response it just made.
- **Bearer and host reach tool handlers through `req.Extra.Header`.** The go-sdk streamable HTTP transport copies the HTTP request headers onto every `CallToolRequest`, stateless mode included, so one server instance serves all replicas and nothing is threaded through context. `header(req)` in `tools.go` is nil-safe because stdio and tests pass no request.
- **Go promotes `Host` out of `r.Header`.** A tool handler only sees the header map, so `baseurl.Middleware` copies `r.Host`/`r.TLS` into `X-Forwarded-Host`/`X-Forwarded-Proto` when the ingress did not set them; `baseurl.FromHeader` then works for HTTP handlers and tools alike.
- **Expiry lives in a store decorator.** `store.WithoutExpired` turns an expired page into `ErrNotFound` on every call, so shared handlers carry no expiry branches. The sweeper (k8s store) must use the raw store, or it can never see what it should delete. `runServe` starts the page cache and the sweeper on the raw `*k8sstore.Store` before wrapping.
- **Live reload streams where it can and polls where it must.** `GET /p/{id}/events` exists only when `server.Options.Watcher` is set, which `runServe` does for the k8s store alone: on the filesystem store the MCP process writes the pages and serve never hears of it. The stream sends `version` on connect, on each feed signal, and every `DefaultHeartbeat` (25 s), plus a final `gone` for a deleted or expired page. A feed signal means "re-read", because client-go runs handlers asynchronously after updating its cache, so a watcher can get a late signal for an earlier event. `watchVersion` in `app.js` falls back to `Webkit.poll` when the stream is refused (readyState CLOSED, which a 404 or a non-event-stream response gives) or silent for 60 s (a buffering proxy). It drops the stream while the tab is hidden and marks itself leaving before `location.reload()` and on `pagehide`, because the browser's teardown of a stream fires `onerror` and would otherwise start a poll.
- **Event streams end at shutdown start.** `serveUntilDone` registers `Server.CloseStreams` with `http.Server.RegisterOnShutdown`; without it every open stream holds the graceful drain for its full 10 s. Browsers reconnect after the `retry: 2000` hint, onto a replica that is still serving.
- **The version poll reads the page cache.** `GET /p/{id}/version` calls `GetMeta`, which on the k8s store answers from the informer cache once it has synced, so it can trail a write made through another replica by the watch delay. A test that reads the version straight after a write polls for it (`versionWithin` in `internal/e2e`). Full reads, writes, and `WithoutExpired`'s liveness check all use `Get`, a direct API read, so a lagging cache never refuses a write. The cache stores a slimmed `cachedPage` (transform `slimPage`) rather than the object, so it holds no page bodies. Without `list` the cache never syncs and both readers stay on the API server. With `list` but no `watch` it syncs and then refreshes only when client-go relists after each failed watch, up to 30 seconds apart.
- **The sweeper deletes under a resourceVersion precondition.** It judges expiry from the cache, so a page could change after the cache saw it. The precondition makes the API server refuse that delete (Conflict), and the page waits for the next sweep. The fake tracker keeps resourceVersions beside objects, not in them, so a test that needs one stamps it with `Tracker().Update`.
- **k8s store tests.** `go test ./...` runs the conformance suite on `dynamic/fake` (`NewSimpleDynamicClientWithCustomListKinds` with `GVR → PageList`, or List fails). The same suite runs against a real cluster only with `PRESENT_KIND_TEST=1` (kubeconfig's current context, a throwaway namespace, the CRD installed and never removed). `PRESENT_E2E_URL` gates the other half: `internal/e2e` skips unless it names a shared instance to drive over HTTP. `scripts/kind-e2e.sh` sets both, with the URL pointing at its own port-forward. The e2e test also reads the page's event stream (version on connect, the new version after a replace, gone after the delete). So it needs a shared instance with a change feed. The page cache has its own suite, `runCacheConformance`, run on the fake client and, with `PRESENT_KIND_TEST=1`, against the cluster. client-go is pinned to the cluster's minor (v0.36 for kind 1.36); keep them within one minor of each other.
- **`--store k8s` needs `--shared`.** Local present writes the workdir from the MCP process; a cluster store behind a local serve would leave the MCP writing files nobody serves.
- **`sharedCreateInput` embeds `createInput`.** The schema generator inlines the embedded struct, so the shared tool exposes every create field plus `ephemeral`; keep it embedded rather than copying the field list.
- **Version probe.** `GET /version` and `present version -o json` both return the shared four-key build metadata object (`version`, `commit`, `tag`, `build_time`, every key present and `""` when unknown) from `github.com/mad01/thismoon/buildinfo`, the cross-tool convention `ralph outdated` uses. Plain `present version` stays a bare token — status parses it as one. The recipe bakes this sha into the t-man service env so a new build reloads the running `serve` agent automatically. Not to be confused with `GET /p/{id}/version`, the per-page revision counter `app.js` polls for live reload.

## See also

- Recipe: `recipes/present/recipe.toml` (+ `recipes/present/CLAUDE.md`)
- Skill: repo-root `skills/present/SKILL.md`
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json`
