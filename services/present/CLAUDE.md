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
                         sharedclient.Client (nil, with a stderr warning, when only one is set); terminal.go adds
                         present terminal on|off|status|list|kill, and serve.go's startTerminals builds the terminal
                         manager in local mode only
    terminal/          - the live terminal's sessions: Manager (Attach, Kill, KillAll, List, Close) over named PTY sessions
                         keyed by page id and session name (terminal.go, session.go), the replay ring buffer (ring.go), a
                         subscriber's output queue (subscription.go), the switch file and its watcher (switch.go), and the
                         process-group listing a kill walks (procs_darwin.go); see *Live terminal*
    store/             - Store interface (create/read/edit/update/list + GetMeta + sources + SetShared + Ping) with FS, the filesystem
                         implementation over pages/<id>/ (store.go, doc.go, id.go); edit.go holds the pure edit model (Snapshot,
                         EditFunc, ApplyEdit, PatchEdit, RenewEphemeral), lock.go the page lock, history.go the version history
                         (Historian, implemented by FS alone over history/<version>.json); deck.go adds the deck remote control
                         (DeckController, implemented by FS alone over deck-command.json); WithoutExpired wraps any
                         Store to hide expired pages; Delete is web-index-only, not exposed via MCP
    store/storetest/   - the Edit contract as a suite every store runs against itself (FS raw and wrapped, the cluster
                         store on its fake client and on kind), including the two-writer no-lost-update test
    docedit/           - Doc edits as store edits: a section patch (sections.go; Parse checks a Request from the route or the
                         tool, SectionPatch.Edit applies one op at an anchor and recompiles the whole Doc), a progress report
                         (progress.go; ParseProgress, Progress.Edit upserts the progress block carrying a label), and the
                         section diff between two versions (diff.go; DiffSections); typed errors (VersionConflictError,
                         AnchorConflictError, LabelConflictError, ErrAnchorNotFound, ErrNoDoc, ErrInvalid) that the routes
                         map to 409/404/400
    store/k8sstore/    - Store over Page custom resources for the shared instance: dynamic client + unstructured, no codegen; crd.yaml embedded and copied to deploy/base/crd.yaml (a test enforces byte equality); the kustomize base installs it, and the test-only installCRD in crd_install_test.go puts it in a cluster for the kind suite; cache.go is the metadata-only page cache (an informer built on `cache.NewSharedIndexInformerWithOptions`, not `dynamicinformer`, which imports all of k8s.io/api) that RunCache starts and GetMeta and the sweeper read once it has synced; feed.go fans the informer's events out to WatchPage subscribers, the server's change feed for event streams; SweepExpired/RunSweeper purge expired ephemeral pages; RESTConfig (in-cluster, else kubeconfig) and ResolveNamespace
    author/            - author keys for the shared instance: NewKey, Hash, FromHeader (bearer), Check (ErrMissing→401, ErrMismatch→403)
    baseurl/           - public base URL from X-Forwarded-Proto/Host: Middleware fills them from the request, FromHeader derives scheme://host
    sharedclient/      - client a local present uses against a shared instance: Create/Replace/Delete/WhoAmI over its JSON API with the
                         author key as a bearer token; Share pushes a page bundle (title, content, graph, references, doc, graph_source,
                         ephemeral) and records the result via store.SetShared, Unshare deletes the copy and clears the record
    render/            - Doc-to-HTML renderer (doc.go), Graph-to-JS renderer (graph.go), legacy raw-HTML upgrade (upgrade.go); compile.go
                         holds Compile (Doc JSON in), CompileDoc (parsed Doc in), and CompileGraph (parsed GraphInput in), the one
                         parse → render → canonical JSON step the MCP tools, present rerender, and the import all call
    mdimport/          - markdown file → title + Doc + optional graph (goldmark, GFM): Convert(name, src); ConvertSection(src) maps
                         one section's markdown for a section patch; pure, no HTTP; the mapping and its losses are under
                         *Importing markdown* below
    mermaid/           - Mermaid flowchart source → render.GraphInput: Convert(src); pure; the import calls it for a fenced mermaid
                         block, anything it cannot parse (a sequence diagram, unsupported syntax) stays a code block
    server/            - HTTP handlers per Mode: local = GET / (static index shell), /api/pages (page list as JSON), /index.js, POST /api/import (import.go; markdown to page), GET /img/{name} (images.go; a stored image, immutable), POST /p/{id}/share (share.go; only when a shared instance is configured), GET /api/terminal and GET (a WebSocket upgrade) and DELETE /api/p/{id}/terminal/{session} (terminal.go; only when serve passes a terminal manager, which a shared instance never does); shared = GET / (how-to shell), POST /api/pages, PUT /api/p/{id}, GET /api/whoami, /mcp; both = /p/{id} (static shell.html; a deck-only page redirects to its deck), /p/{id}/deck (deck.go; the same shell, redirects to the brief when the page has no deck), /api/p/{id} (page as JSON, plus a share block in local mode), /app.js, /viz.js (the pure step helpers, window.PresentViz, loaded before app.js and tested under node in internal/server/test), /logo.png (logo.go; the embedded repo logo the deck chrome shows, served no-cache with a content ETag; a test keeps it byte-identical to docs/assets/logo.png), /p/{id}/version, DELETE /p/{id} (author-checked when shared), PATCH /api/p/{id}/sections/{anchor} and POST /api/p/{id}/sections (sections.go; a section patch, with the import's guard locally and author-checked when shared), POST /api/p/{id}/progress (progress.go; same guards), GET /api/p/{id}/history, /api/p/{id}/history/{version}, and /api/p/{id}/diff?since= (history.go) whenever the store is a Historian, which only the raw filesystem store is, GET /p/{id}/events (events.go) whenever Options.Watcher is set, which serve does for the k8s store, and GET /p/{id}/deck/command (deck.go) whenever the store is a DeckController, which only the raw filesystem store is; embeds index_shell.html, shared_index_shell.html, shell.html, index.js, app.js, viz.js
    mcpserver/         - MCP wiring + present_* tools; Mode picks the tool set and whether the bearer/forwarded headers on req.Extra.Header are read; present_share is registered only when Config.Sharer is set, present_deck only when the store is a DeckController; images.go turns an image block's local file into a stored image before the Doc compiles (local mode only)
    export/            - a page as one standalone HTML file (docs/adr/0029): export.go picks the rendition, inlines the
                         page's /img/ files as data URIs, and writes through a temp file; body.go parses and serializes
                         the body again (wellFormed) so it closes everything it opens; libraries.go is the library
                         table (cached file, npm path on jsDelivr), needs() reads which ones the body and graph script
                         use, and cdnScripts hashes the cached files for the integrity attributes; fonts.go inlines the
                         latin faces of the cached fonts.css; notices.go holds the font copyright lines and Prism's
                         licence for the head comment; document.go and document.html assemble the file around
                         server.StandaloneAssets (standalone.go: the shell's stylesheet and fixation targets, viz.js,
                         app.js) and webkit.FS()
    images/            - the local image store under <workdir>/images/: content-addressed names (sha256 hex plus png, jpg, gif, or webp), ValidName/NameOf/Referenced for the /img/ path rule the renderer, the MCP tools, the server, and the share guard all use, Sniff for the content-type check, Sweep for the delete-time cleanup
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

- **`present mcp`** (stdio MCP server): tools write/read pages under the workdir. Never serves HTTP.
- **Decks.** A page id carries up to two renditions: the brief (`content` → `content.html` + `doc.json`) and the deck (`deck` → `deck.html` + `deck.json`), each a Doc authored on its own, never converted from the other (docs/adr/0019). At least one must exist; `present_create` refuses neither and `present_update` refuses stripping the last one. Both share the title, the one graph, the references, and the share record. `GET /p/{id}` shows the brief, `GET /p/{id}/deck` the deck, and a deck-only page redirects from the first to the second. The deck view splits `deck.html` client-side: the nodes before the first section make the title slide, every `wk-section` is one slide, `wk-toc` is dropped, references are the last slide. The deck Doc's chrome fields (`logo`, `logo_position`, `progress`, `presenter`, `footer`) travel as a `script.deck-chrome` JSON island at the head of `deck.html`, emitted only when one is set; `app.js` pulls it out of the hero and builds a bottom strip (footer line left in `--text-3`, dots or bar centre, logo right; a bottom-left logo swaps sides with the footer, a top corner logo is its own fixed element). Defaults with no island: logo shown from `/logo.png`, one dot per slide (a bar above 24), no footer line. `show()` updates the marker; a dot click jumps. The strip stays visible while presenting at a z-index above the fixed slide, whose bottom padding becomes `max(5vh, 3.25rem)` through `html.has-deck-strip` (not named after the strip's own class, or the strip rules would hit the html element). `decorateSlide` copies `data-layout`, `data-tone` (with `--slide-accent`), and `data-reveal` from the `wk-section` to the slide, adds `solo-stat`/`solo-quote` when the section's only block is one, reads the notes template, and builds the slide's steps: on a reveal slide each `li` of a top-level list and every other top-level block is one `.fragment`, and a chart with `steps` (`data-steps`) adds its own steps right after the fragment holding it, on a slide without reveal too (`PresentViz.stepPlan` in `viz.js` orders them; `applySteps` writes `data-step-at` on the block, calls its `_presentStep` hook when built, and marks the current caption `li.current`); `show(i, stepsMode)` wraps its body in `apply()` and runs it through `document.startViewTransition` (the active slide carries `view-transition-name: deck-slide`; `deck-next`/`deck-prev` on html pick the 24 px keyframes for `transition: slide`; `deck-vt` turns the fallback `briefEnter` off) unless the transition is none, Reduce Motion is on, the API is missing, or it is the first showing; `next`/`previous` take the slide's steps (reveal fragments and a stepped chart's steps) before moving, and the remote next/prev go through them. Charts on a slide build on its first showing (`buildChart(b, {duration: 400})`), a theme change rebuilds only built ones, and the cytoscape shim adds `ready` to the container on the first `layoutstop` so the shell fades it in. The notes drawer (`.deck-notes-drawer`, N key, a bar button when any slide has notes) sits outside every slide; the renderer's inert `template.deck-notes` keeps its own class. The deck view is served from `deckShell`, the page shell with the audio toggle's per-view key and off default on `<html>`, so a deck opens without the read-aloud controls while the brief shows them. Keys: Right, Space, or PageDown next; Left, PageUp, or Backspace previous; Home and End. F or P start presenting (chrome hidden, one slide filling the window, browser fullscreen requested best-effort). They ask for fullscreen again when pressed while presenting without it, and end presenting when pressed in fullscreen. Escape always ends it. A `#<n>` hash names the slide, and the tab keeps its slide and presenting mode in `sessionStorage` (`present-deck:<id>`) so the reload an update triggers resumes where it was. The browser-fullscreen request is refused without a gesture, so a resumed presentation fills the window until the reader presses F, which asks again with the gesture the request needs. Remote control (`present_deck`, `present deck`) writes `deck-command.json`, which keeps the last 32 commands. Every open deck tab polls `GET /p/{id}/deck/command?after=<seq>` once a second and applies what came after the sequence number it last ran. A fresh tab runs only a command sent within the last ten seconds, so present_open followed by present_deck start works. Every tab remembers that number with the slide. Remote control exists on the filesystem store only, so a shared instance serves decks without it.
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
    history/<version>.json # the newest 20 versions (store.Revision: title, references, sources, and the HTML or script of a field without a source)
  images/<sha256>.<ext>    # image files the image blocks of any page brought in from this machine (png, jpg, gif, webp), served at /img/<name>
  exports/<id>.html        # files present_export wrote (<id>-deck.html for a deck); the CLI writes to the cwd instead
  terminal.enabled         # the live terminal switch: an empty file, present only while `present terminal on` holds
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

## Sandbox access

`spec.access` in `service-info.yaml` declares what `present serve` may reach
under srt. `make sandbox-settings` renders it to
`recipes/present/present-serve.srt.json`, which CI diffs, so change the
manifest and never the JSON. Authoring guide: `docs/SANDBOXING.md`,
"Declaring access".

## Build / install / test

```bash
make build    # ./present binary
make install  # build + cp to ~/code/bin/present + codesign (stable local identity, ad-hoc fallback)
make test     # go test ./... then node --test on internal/server/test (skipped without node)
make tidy     # go mod tidy
make resign   # re-apply the signature (macOS)
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

`present progress <id> <label> <pct> [--note <text>] [--section <anchor>]
[--deck]` sets the progress block carrying `<label>` to `<pct>` (0 to 100). With
no such block it adds one to `--section`, else to a Progress section at the
end, created the first time. It runs `docedit.Progress` as a store edit against
the workdir under the page lock, so it works with serve up or down and beside
the MCP server and other reporters. It prints `<id>: <label> <pct>% in
<anchor>, version <n>`, with ` (unchanged)` after a report the block already
showed: that edit returns `store.ErrUnchanged`, writes nothing, and keeps the
version. Every other report is a version with a history record, so a hook
ticking a moving value uses up the 20 kept versions. Hooks use it.
`POST /api/p/{id}/progress` is the same edit over HTTP. The block's
`note` renders after the label in the existing `wk-progress-label` element,
only when set, so the golden fixtures are unchanged. A label two blocks carry
is refused with their section indices (409 with `sections` over HTTP).

`present share <id> [--ephemeral]` pushes a page to the shared instance named
by `--shared-url` and prints its link (the expiry goes to stderr when
ephemeral); `present unshare <id>` removes the copy; `present key new` prints a
fresh 64-hex author key. share and unshare need both `--shared-url` and
`--author-key`; key new needs nothing.

`present export <id> [-o path] [--deck]` writes the page as one HTML file
that opens with no server (`internal/export`, docs/adr/0029). The default is
`<id>.html` in the cwd, or `<id>-deck.html` for a deck. When `-o` names an
existing directory, or ends in a separator, that name goes inside it. A deck-only page
exports its deck. It prints `<id>: <rendition> <path>, <n> bytes, <n> libraries from
the CDN` and any notes on stderr. It needs only the workdir and its asset
cache. A page whose blocks need a library the cache lacks is refused. A
cache without fonts.css exports with system fonts and a note.

`present terminal on|off|status` creates, removes, or reports the switch file
`<workdir>/terminal.enabled`, so they work with serve up or down and need no
restart; `on` prints a note that only the local serve reads it.
`present terminal list` asks the local serve on `--port` for
`GET /api/terminal` and prints the switch and one row per session (page,
session, pid, running or exited with its code, attached tabs, start time,
command). `present terminal kill <page> <session>` sends
`DELETE /api/p/{id}/terminal/{session}` and waits out the kill grace. See
*Live terminal*.

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

## Live terminal

A `terminal` Doc block attaches the browser to a terminal session the local `present serve` owns. The session is a process in a pseudo-terminal (PTY), keyed by page id and session name, and it outlives the tab showing it (docs/adr/0025). Every page update reloads the tab, so the state has to live in serve. `internal/terminal` holds the sessions, `internal/server/terminal.go` the routes, and `internal/cli/terminal.go` the command.

- **The switch is a file.** Off by default. `<workdir>/terminal.enabled` (`terminal.SwitchFile`) turns it on by existing. Every terminal request reads it, and `Manager.WatchSwitch` checks it every 2 s (`terminal.SwitchPoll`) and kills every session while it is off, so `present terminal off` ends them within seconds without a restart. A switch serve cannot stat counts as off, and the error is logged once.
- **Local only.** `runServe` calls `startTerminals` only without `--shared`, and `registerTerminal` mounts nothing without a manager, so a shared instance has no terminal route at all. Its shell carries `data-present-mode="shared"` on `<html>`, from which `app.js` shows the static placeholder "Live terminal is not available on a shared instance." and never asks the server. A Doc holding terminal blocks is accepted in both modes, and sharing one is never refused.
- **Routes.** `GET /api/terminal` answers `{enabled, sessions}` (`Manager.List`) whether the switch is on or off, so the browser can choose between the terminal and the off note. `GET /api/p/{id}/terminal/{session}` upgrades to the session's WebSocket. Before upgrading it checks six things in order. The switch comes first (404 "live terminal is off" while off), then the session name against `^[a-z0-9-]{1,32}$` (400) and the page (404). Then come the Origin (403), the `cols` and `rows` query, 1 to 1000 (400), and the stored spec (404 for a session the page does not declare). `DELETE /api/p/{id}/terminal/{session}` kills one session: 204, or 404 when serve holds none. The switch does not gate it, since ending a process is always safe.
- **The spec comes from the stored page, never the socket.** The query carries the size only. `storedTerminalSpec` loads the page's `doc.json` and `deck.json` and takes `cmd`, `cwd`, and `prefill` from the block naming the session (`render.FindTerminalSpec`). Page content reaches the browser through innerHTML with no Content Security Policy. A command chosen by the browser would let anything injected into a page run as the user while the switch is on. The `data-cmd`, `data-cwd`, and `data-prefill` attributes the template writes are for the reader. Only the attach that spawns a session reads the spec; a running session ignores later edits until it is killed.
- **Origins.** `terminalOriginAllowed` passes a request with no Origin (the CLI, curl), loopback, `localhost`, and the `terminalOrigins` patterns: `present.this` plus `server.Options.TerminalOrigins`. It never consults the Host, because a DNS name pointed at 127.0.0.1 would agree with itself. `present.this` is listed explicitly: d-man proxies the upgrade but rewrites the Host to `127.0.0.1:7423`, so websocket.Accept's own same-host rule never covers that path.
- **Frames.** Binary frames carry raw PTY bytes both ways; one input frame is at most 1 MiB. Text frames are JSON control. The client sends `{"t":"resize","cols","rows"}` (held to the query's bounds) and `{"t":"kill"}`. Serve sends `{"t":"hello","replayed":N}`, then the replay as one binary frame, then the live output, and `{"t":"exit","code":N}` once the process has ended.
- **Close codes.** 1000: the process exited, after the exit frame. 1013: the tab fell 4 MiB behind and was dropped; the client reconnects and the replay fills the gap. 1008: the spawn was refused (`ErrInvalidSpec`, a relative or missing `cwd`, say), with the reason in the close frame; the client stops and shows it. 1001: serve is shutting down; the client waits for Run. 1011, and any drop without a close frame: the client reconnects, 0.5 s doubling to 8 s, ten tries.
- **What a session runs.** `zsh -l -i`, or `zsh -l -i -c <cmd>`. It is interactive as well as login because the launchd agent's PATH has neither the mise shims nor `~/code/bin`, and `~/.zshrc`, which adds them, is read by interactive shells only. An empty `cwd` is the home directory, a leading `~` expands, and anything else must be an absolute directory. The environment is serve's without its `TERM`, plus `TERM=xterm-256color` and `PRESENT_TERMINAL=1` (`terminal.MarkerVar`), so a startup file can tell it runs under present. The prefill is typed once, with no newline, after the output has gone quiet for 250 ms, and at most 5 s after the start.
- **Replay.** Each session keeps its last 1 MiB of output (`DefaultRingBytes`). A new subscriber gets it under the lock `publish` holds, so replay and live output never gap or repeat. Joining a running session also sends SIGWINCH to its foreground job, so a TUI draws its whole screen again after a replay that may start inside an escape sequence. Two tabs or blocks on one session each get their own socket, and the last resize wins.
- **How a session ends.** A session no tab has watched for 2 h (`DefaultIdleTimeout`) is killed. Kill sends SIGHUP to every process group in the session the shell leads, and a second SIGHUP 100 ms later. Whatever is left after the 2 s grace (`DefaultKillGrace`) gets SIGKILL. Background jobs and `nohup` die with it; a process that called setsid does not. A process that exits on its own stays listed with its exit code until it is killed, idles out, or serve stops, so a reattach replays its end. Run on an ended block deletes the session first and starts it again.
- **Doc rules.** `render.validateTerminal` refuses a missing or malformed session name and `rows` outside 8 to 60 (0 leaves the height to the browser). It also refuses a `prefill` that holds a control character or runs past 512 characters once trimmed. `validateBlocks` lets a terminal sit in `columns`, never in `details`. `ValidateTerminalSessions` refuses two blocks that name one session with a different `cmd`, `cwd`, or `prefill`; `rows` may differ. `validateDoc` runs it on one Doc and `checkRenditions` in `mcpserver` on the brief and the deck together, since serve starts a session once from one of them. On a slide the terminal is the one card a deck keeps.
- **`rows` is a shared wire key.** It holds a table's cells and a terminal's height, told apart by `t` in `Block.UnmarshalJSON`/`MarshalJSON` the way `cols` is; the Go field is `TerminalRows`. `TestCompileDocGoldenStage1JSON` pins `render/testdata/golden-stage1-doc.json`, captured before the split, so a table's rows inside a column keep their canonical JSON.
- **Browser.** xterm.js 6.0.0 and its fit addon 0.11.0 are vendored by `scripts/cache-assets.sh` with sha256 pins (docs/adr/0022, `THIRD-PARTY.md`). Without them the block says so and only Kill works. `upgradeTerminal` in `app.js` attaches lazily: when the block scrolls into view in the brief, on its slide's (or fragment's) first showing through `terminalShow`, or on Run. Each attach asks `/api/terminal` first, and a 404 there (an older binary) counts as off. A block that found the switch off asks again every 3 s (`terminalPollOff`, paused while the tab is hidden or the block is off screen) and attaches by itself once it is on. A block whose session has ended waits for Run instead. A session that ends asks the switch once, and shows the off note when `present terminal off` was the cause. A slide whose only block is the terminal gets `solo-terminal`, and a block naming no `rows` fills it. Kill is held per tab in `sessionStorage` (`present-terminal-held:<id>:<session>`) until Run, so the reload an update triggers does not start the command again.

## MCP tools

- `present_create(title, content?, deck?, graph?, references?)` → `{id, url, has_deck, deck_url?, version}`; at least one of `content` and `deck` is required, and `url` is the deck URL when the page has no content
  - `content`: Doc JSON object `{summary?, meta?, chips?, sections:[{h, blocks}]}` or legacy HTML string (auto-detected)
  - `deck`: a second Doc JSON object with the same schema, Doc JSON only (a non-object is refused); every section is one slide, summary/meta/chips make the title slide, references the last one. The tool description carries the shape and the rules that keep a deck a deck (the heading is the claim, 3 to 5 short items or two sentences per slide, one highlight, one visual alone per slide, 5 to 12 slides, no filled boxes). The skill has the full list, the building-a-deck recipe, and the worked examples under `skills/present/examples/` (MAD-387). The deck Doc may also set the chrome fields `logo`, `logo_position`, `progress`, `presenter`, `footer` (all optional; `render.validateChrome` refuses an unknown position or progress value and a logo that is neither `none` nor an http(s) URL)
  - Section fields beside `h` and `blocks`: `tone` (a role from `webkit.Roles()`, no aliases; rendered as `data-tone` plus `style="--slide-accent: var(--<role>)"`, banding the section in the brief and colouring the slide's heading rule and accent bar; a slide paints no filled box, MAD-389), and deck-only `layout` (default|center|statement|section, emitted as `data-layout` when not default), `notes` (an inert `<template class="deck-notes">` at the end of the section, outside both walkers by construction), `reveal` (`data-reveal`). Deck-level `transition` (fade|slide|none) rides in the chrome island only when set and not fade. `render.validateSection` refuses an unknown layout or tone; a Doc without any of them renders byte-identically (`render/testdata/golden-stage1.html`). `render/testdata/sample-deck.json` exercises all of it and `TestSampleDeckRenders` keeps it compiling
  - Doc blocks: `p`, `h3`, `callout` (`sev` info|warn|ok|error), `table`, `kv`, `list`, `panel`, `progress` (`pct`, `label?`, `note?`; `validateFieldHomes` refuses `note` on any other block), `graph`, `diagram` (`nodes`, `edges`, `groups`, `direction`, `caption`, `steps` with `focus`; `render.validateDiagram` refuses unknown kinds, tones, and directions, an id that is not a plain token or is reused, labels past their caps, an edge to a group, to itself, or repeated, a weight without flow, an empty group, groups nested past two, more than 24 nodes or 12 groups, a frame, a step out of range, a focus naming nothing or something not yet shown, and an element stepped before what holds it; `validateFieldHomes` refuses its fields on any other block; many per page, outside the one-graph rule), `chart` (`steps: [{caption}]` and a series `step` for a chart a deck walks one step per Next; `render.validateChart` refuses an unknown kind, a step without a caption, a series step out of range, steps on a sparkline, a series step on a doughnut, a sankey, or a ribbon, `order` on any kind but ribbon, and a stepped chart inside `details`; `ribbon` adds `order` (rank or given); `frame: true` keeps a card around any chart, see *Visual blocks* under *Gotchas*), `code`, `html`, `image` (`src`, `alt`, `caption?`, `frame?`; see *Images* under *Gotchas*), `terminal` (`session`, `cmd?`, `cwd?`, `prefill?`, `rows?`; see *Live terminal*), plus the layout blocks `columns` (`cols`: 2 or 3 arrays of blocks), `stat` (`value`, `label`, `sub?`), `quote` (`text`, `cite?`), and `details` (`summary`, `blocks`). A `columns` or `details` block holds any block but `columns` and `details`. The page's one `graph` may sit in a column but not in `details`; the deck splitter and `initGraphAndFlow` find `#cy-graph` wherever it is, and `validateDoc` refuses a Doc that places it twice. `render.validateBlocks` refuses the rest and `renderBlockAt` carries a depth guard. `cols` is one wire key for a table's header strings and a columns block's arrays, told apart by `t` in `Block.UnmarshalJSON`/`MarshalJSON`, so a stored table's canonical JSON never moves. A golden fixture (`render/testdata/golden-brief.html`) pins that a Doc without the new fields renders byte-identically
  - `graph`: Graph JSON object `{nodes, edges, layout?}` or legacy JS string (auto-detected); one per page, shared by both renditions
  - `references`: `[{title, url}]`
- `present_read(id)` → full page with rendered HTML content and `deck` (includes references, `has_deck`, `deck_url`)
- `present_source(id, version?)` → editable source: `{content_format: "doc"|"html", content, deck?, graph_format: "json"|"js"|"", graph, references, deck_url?}`: outputs round-trip directly into `present_update` (it auto-detects both formats; `deck` is the canonical deck Doc JSON, empty when the page has none). Use this to mutate a page from a new/restored session. `version`, when it is not the current one, answers from the page's history (`store.Historian`; the newest 20 versions); passing that source to present_update restores it as a new version. A shared instance serves a wrapped store with no history and refuses it (`errNoHistory`)
- `present_update(id, title?, content?, deck?, graph?, references?)` → patch (omitted fields unchanged; empty graph clears it; empty deck removes the deck and its source; empty array clears refs); bumps version. Doc/graph JSON input refreshes `doc.json`/`deck.json`/`graph.json`; raw HTML/JS input deletes the now-stale source file. An update that would leave neither content nor deck is refused before anything is written
- `present_patch_section(id, op, anchor?, section?, body_format?, rendition?, base_version?)` → `{id, version, rendition, op, index, anchor, url}`: one section of the brief (`rendition: content`, the default) or the deck, without resending the Doc. Registered in both modes; on a shared instance it is author-checked and restarts an ephemeral page's expiry like present_update
  - `op`: `replace`, `insert_before`, `insert_after`, or `remove` at `anchor`, or `append` with no anchor
  - `anchor`: the section's TOC anchor, `render.SectionAnchor` of its heading (a heading or `#anchor` is normalized to it), or `index:N`, zero-based. A heading with an empty anchor (no ASCII letter or digit) never clashes and is addressed by index. Section ids stay deprecated (MAD-377): anchors come from headings only
  - `section`: one Doc section as a JSON string, or markdown opening with a `#`/`##` heading when `body_format: markdown` (`mdimport.ConvertSection`). A second `#`/`##` is refused, and a mermaid fence stays code
  - The section's local image files are ingested like present_update's. They are written only once the patch passes a dry run of its edit over the page as the tool read it. So a stale base_version, an anchor problem, or a Doc the renderer refuses writes no file. A write landing between the dry run and the edit can still refuse the patch after its files are written, and those wait for the next sweep. The edit then recompiles the whole Doc under the page title, so the renderer's rules judge the result, one graph per page included
  - Refusals: a stale `base_version` (`VersionConflictError`, naming the current version) and an anchor several sections share (`AnchorConflictError`, with their indices). A heading whose non-empty anchor another section holds is refused the same way, and a rendition with no Doc source is `ErrNoDoc`
- `present_list()` → all pages (metadata only, no content), newest first; `has_doc` flags source-editable pages, `has_brief`/`has_deck` say which renditions exist, `deck_url` is set when there is a deck
- `present_open(id, deck?)` → macOS `open` (call once per page; updates auto-reload); `deck: true` opens `deck_url` and errors when the page has no deck
- `present_deck(id, action, slide?)` → `{seq, action, slide?, deck_url}`: remote control for the deck open in the browser: `start`/`stop` presenting, `next`, `prev`, `goto` a 1-based `slide`. Registered in local mode only, and only when the store is the raw filesystem store (the one that relays commands); every open tab of the deck follows within a second
- `present_export(id, deck?)` → `{path, bytes, rendition, libraries?, notes?}`: the same file `present export` writes, under `<workdir>/exports/`, the one place the MCP sandbox may write. Registered in local mode only, and only with a workdir. Annotations: not read-only, idempotent (a second export overwrites the file)
- `present_share(id, ephemeral?)` → `{url, ephemeral, expires_at?, shared_at}`: push the page to the configured shared instance; registered only when `--shared-url` and `--author-key` are set. Sharing again replaces the copy under the same link; `ephemeral` makes it expire 30 days after the last share
- `present_doctor()` → the `kit/doctor` report: store readable, serve reachable, no version skew, shared instance answering `GET /api/whoami` (skipped without one); for a client that can call a tool but has no shell

On a shared instance the set is `present_create` (plus an `ephemeral` bool; the bearer becomes the page's author), `present_read`, `present_source`, `present_update` and `present_patch_section` (author-checked; reset an ephemeral page's expiry), and `present_doctor` (store ping only). No `present_list`, no `present_open`, no `present_deck` (the cluster store relays no commands; decks there are keyboard-driven). The deck fields on create, read, source, and update are the same as locally. URLs come from the request's forwarded headers unless `--base-url` overrides them.

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
- **Visual blocks sit on the surface, not in a card.** A chart has no
  card unless its Doc sets `frame: true`, and an image keeps its border
  unless `frame: false`. `frameAttr` writes `data-frame="true"` or
  `"false"` as given, from a `*bool`, so an unset field emits nothing and
  the golden fixtures hold. The shell names the colour behind content
  `--surface`: `--bg` on the page and on every slide (a toned or
  section slide colours its accent, never its background), or
  `--card-bg` inside `wk-panel`, `details`, and a framed chart. SVG reads
  it through `var()`. Chart.js and Cytoscape need a literal, so
  `surfaceColor(el)` in app.js reads `--surface` through a probe child
  with its transition off (webkit eases backgrounds over 300 ms and fires
  `wk-themechange` in the same tick, so a painted background would still
  be the old theme's) and normalises it through a one-pixel canvas
  (`colorLiteral`), since a computed colour can come back as
  `color(srgb …)`, which neither library parses. The gaps between stacked
  bars and doughnut slices and the graph's edge label backing follow the
  page or the card that way, in the graph's fullscreen popup too.
  This default is CSS only, so every stored chart changed look when it
  shipped (MAD-382, 2026-10-07).
- **Metric charts are present-local, not webkit.** The `t=chart` Doc block
  (`kind` from `render.ChartKinds`: `bar`/`line`/`area`/`sparkline`/`stacked-bar`/
  `horizontal-bar`/`doughnut`/`scatter`/`sankey`/`ribbon`; `validateChart` refuses any
  other, and tests pin the list to the MCP schema text and the skill) renders
  a `<div class="present-chart">` with a JSON spec in a
  `<script type="application/json">` island.
  `chartSpec` returns `template.JS` so html/template emits it verbatim instead
  of re-encoding it inside the script context. The `initPresentCharts`
  bootstrap in `app.js` reads each spec and builds a Chart.js instance,
  mirroring how the Cytoscape graph works: a vendored lib in `assets/js/`
  (`chart-4.4.6.umd.js`, fetched by `scripts/cache-assets.sh`) plus a
  theme-aware color function. `sankey` also needs the vendored
  `chartjs-chart-sankey-0.15.3.min.js` (same script, loaded by `shell.html`
  right after Chart.js); when that asset is missing the block shows a note
  instead of a chart. `ribbon` is the one kind D3 draws instead of Chart.js.
  `buildRibbon` in `app.js` lays it out with `PresentViz.ribbonLayout` in
  `viz.js` (node-tested: periods in order of first appearance, one
  bottom-aligned column per period, segments ordered by rank or as given,
  a ribbon per category between adjacent columns). It draws SVG into the
  canvas wrapper with `var(--series-N)` fills, so the theme rebuild redraws
  it in place with no colour to resolve. The template gives the block
  `is-ribbon` (no rule keys on it today). `validateRibbon` refuses an unknown
  `order`, a fifth series (`ribbonMaxSeries`, the palette's four series
  roles), a series without a name or with a name another series has, a
  point without an x or with a negative y, a period repeated in a series,
  a column total that overflows, series that disagree on the period order
  (periods run in order of first appearance; `ribbonPeriods`, pinned
  against the JS layout by `testdata/ribbon-order.json`), and a step count
  that differs from the period count. A ribbon steps by period (its
  `_presentStep` shows the first n columns, step 0 is the empty axis),
  never by series. The graph's layout engines (dagre, the ELK
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
  (`stepPlan`, `chartStepVisibility`, `ribbonLayout`, and the diagram's
  `wrapLines`, `diagramBox`, `diagramElk`, `diagramGraph`, `diagramText`,
  `elkPath`, `routeLabelPoint`, `diagramShown`, `diagramFocus`,
  `flowSpeed`), tested under node. `d3-7.9.0.min.js` is vendored the same way as Chart.js
  (docs/adr/0022, `THIRD-PARTY.md`); the ribbon chart and the diagram
  block read it.
- **Diagram blocks are laid out in the browser.** A `t=diagram` block
  renders `<div class="present-diagram">` with a `script.diagram-spec`
  JSON island (`diagramSpec`: direction, groups, nodes, edges, steps with
  focus), the caption as fixated prose, and the step captions as the same
  `ol.present-steps` a chart carries. `buildDiagram` in `app.js` measures
  the labels with a canvas and builds the ELK graph through
  `PresentViz.diagramGraph` (which wraps `diagramElk`: layered,
  `INCLUDE_CHILDREN`, orthogonal routes, root coordinates, a layer gap
  sized from the widest edge label, left to right only, set on the root
  and on every group since ELK spaces siblings by their parent's
  options). It runs the
  vendored elkjs once per spec and draws SVG with D3: groups, boxes by
  kind, routed edges, labels placed by
  `routeLabelPoint` on a halo in `--surface`, and flow dots whose speed
  follows `flowSpeed`. Fills are `var(--tone-<t>-bg)` and friends, so a
  theme change redraws nothing. The svg scales to its wrapper through
  the viewBox; a ResizeObserver only refits, never redraws. Steps toggle
  `off`, `focus`, and `dim` classes through `_presentStep` on the MAD-385
  contract, and Reduce Motion or a cutting deck swap without transitions
  (`cutStep`, shared with the ribbon). Flow dots run through
  `visibleLoop`, which the graph's edge flow shares. It needs
  `window.ELK` and `d3`; without one it shows a note. Known limit: the
  header's font control changes the family after the layout, so the
  boxes keep their measured width until a reload. Every tone
  list has three consumers now: `graphTones` in graph.go, `GRAPH_TONES`
  in app.js, and the diagram's CSS, which names the same six roles.
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

- **Wave 0 builder.** Builds before the consuming repo's agent-sync recipe (`agent-config/` in `examples/dotfiles`, wave 1) registers it.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `present docs`): the two-process split, shared-workdir/port
  mismatch, content rules that corrupt pages, failure modes, version-skew
  checks. Keep those facts there, not here. Dev-side notes that stay: the MCP
  is pinned via `env` in the consuming repo's agent-sync `servers.json`
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
- **An exported page runs app.js with no server.** `internal/export` writes the body into `<main id="root"><div class="brief">` (plus a static copy of the references block), the page data minus the bodies as `script#present-export`, and `data-present-mode="export"` on `<html>`. `app.js` then takes `renderExport` instead of the fetch. The brief keeps that body, and `enhanceBrief` (split out of `render`) adds the graph, charts, diagrams, and terminals to it. The deck is split from the same body. One flag, `standalone`, keeps every control that needs the server out of an export. Read-aloud and its player, share, the brief and deck links, change marks, the version poll, the deck remote, and terminal sessions all check it. A new server-backed control must check it too (`TestExportNamesNoServer`, and the read-aloud case in `app-export.test.mjs`). The deck also drops its default logo. A terminal block shows "not available in an exported page". `needsAsset` words a missing-library note for either mode, and `availableEngines` drops the ELK engines from the graph's engine cycle when ELK did not load. An inline script before webkit.js answers root-relative `fetch` calls with a 404, because webkit polls `/webkit/version` and an export has no server. The library table in `internal/export/libraries.go` must name the same URL and file `scripts/cache-assets.sh` fetches, and the script must pin a sha256 for each (`TestLibrariesMatchTheAssetCache`); `TestIntegrityOfTheRealAssetCache` checks the cache on the machine against the pins when there is one. Every library, Chart.js included, comes from an npm file, never a `.min.js` jsDelivr generates, because the export pins its hash. The guard and webkit.js come right after the page data, then the library tags, then viz.js and app.js, so the prose and the header controls are ready while jsDelivr loads, hangs, or fails. The body goes through `wellFormed` (x/net/html ParseFragment and Render, div context, scripting off) before it is placed: innerHTML contains a broken html block on a served page, a file does not. `contain` also gives every body script but a JSON island the type `text/plain`, since innerHTML never runs one on a served page, turns plaintext into pre, writes a `<!--` inside a script with its `<` as a JSON-safe escape, and escapes noscript text. The head comment carries the four font copyright lines, the OFL link, and Prism's MIT licence (`notices.go`, kept in step with `THIRD-PARTY.md` by a test). `internal/server/test/app-export.test.mjs` runs app.js's export branch against a fake DOM.
- **Images are files beside the pages.** An `image` block's `src` reaches the renderer as an http(s) URL or as `/img/<sha256 hex>.<png|jpg|gif|webp>`; `render.validateImage` refuses anything else and an empty `alt`. The MCP tools do the conversion in `mcpserver/images.go`. `imageIngest` reads an absolute or `~` path (a relative one is refused), caps it at `present.MaxImageBytes`, and sniffs the type with `http.DetectContentType`: png, jpeg, gif, or webp pass, and svg is refused because it can carry script. It names the file by its hash and rewrites `src`, then compiles both Docs, and only then writes the file into `<workdir>/images/`. So a Doc the renderer refuses leaves no file, and a stored page never names a missing one. A write the store edit refuses after the files landed, such as one racing a delete or a concurrent writer, leaves those files until the next sweep. `present rerender` needs nothing: the sources carry the served paths. The markdown import never makes an image block (an image there is its alt text), and no CLI command takes a Doc.
- **Stored images are served, swept, and never shared.** `present serve` answers `GET /img/{name}` from `<workdir>/images/` with the immutable cache header the assets get, and 404 for any name the store could not have written. `serve` sweeps the images no page references at startup and after every `DELETE /p/{id}`, reading `content.html`, `deck.html`, `doc.json`, and `deck.json`; files younger than a minute are skipped, and `images.Store.Write` touches an existing file instead of rewriting it, so a page the MCP is about to write keeps its image. The renditions are written through a temp file and a rename, so the sweep never reads a half-written one. A shared instance has no store, so its tools refuse an image src that is not a URL. `sharedclient.Share` refuses a page whose renditions reference `/img/` with `ErrLocalImages` rather than push a copy with broken images.
- **Every page write is an Edit.** `Store.Edit(ctx, id, fn)` runs a pure `EditFunc` over a `Snapshot`: the page with its bodies, plus its sources. Whatever fn returns becomes the next version. Update is an Edit running `PatchEdit`. An EditFunc returns `store.ErrUnchanged` to write nothing and spend no version; `present rerender` and progress reports do when their output matches what is stored. The MCP update, the shared replace, and `present rerender` each run one Edit, so a page and its sources change in one write. The store keeps only the editable fields fn returns (title, content, deck, graph, references, ephemeral, expiry). It owns the id, version, timestamps, author, and share record.
- **The page lock and the commit order.** FS takes an exclusive flock on the page directory for every write: Edit, Update, Delete, and SetShared. Sources have no write method of their own; they change only through Edit. The MCP server, serve, and the CLI are separate processes over one workdir, so a mutex would not do. An edit writes the sources, then the renditions, then `meta.json` last, each through a temp file and a rename. A process crash therefore never leaves a new version over old sources. Nothing calls fsync, so a power loss or kernel panic carries no such promise. The lock dies with its process. The cluster store runs fn inside its resourceVersion retry instead, so fn may run more than once.
- **A writer that waited checks the page again.** Once the lock is granted, a page whose `meta.json` is gone is `ErrNotFound`. So is a page whose directory was replaced while the writer waited. Either way the write never recreates the page. The workdir must be on a local filesystem. On a network mount flock is either unsupported, so every write fails, or local to one host, so writers on two hosts lose updates.
- **Wrappers must define Edit and Update.** The size cap and the expiry wrapper embed the unexported `forwardable` interface, not `Store`. A wrapper missing Edit or Update then fails to compile, so no write reaches the wrapped store by promotion. The conformance fake adds an update reactor that enforces resourceVersion. The stock tracker is last-write-wins and would hide a lost update.
- **History belongs to the filesystem store.** Every FS write records `history/<version>.json` before `meta.json` moves, then prunes all but the newest 20. A crash between the two leaves a record for a version that never committed, and the next edit, writing the same number, overwrites it. Create records version 1. A page last written before history existed lists nothing until its next write; the diff then reads the live sources for the current side. The image sweep ignores history, so an old revision can name a `/img/` file the sweep has removed.
- **The brief marks what changed since the last visit.** On load the brief calls `markChangedSections` in `app.js`, which keeps the last version seen in `localStorage` (`present-seen:<id>`). It asks `/diff?since=` and adds `section-changed` to the changed and added sections. `PresentViz.changedSectionTargets` (node-tested) maps `index:N` names to positions, and the "updated" chip is CSS in `shell.html`. A first visit only records the version. A shared instance answers 404, so nothing is marked there, and the deck view never marks.
- **A shell startup file can stall a terminal session for 10 s.** Some hooks in `~/.zshrc`, a theme hook say, write an escape sequence and wait for the terminal's reply. xterm.js never answers, so the session sits blank until the hook times out (10 s for a kitty hook that queries the terminal). Guard such hooks on `PRESENT_TERMINAL`, which every session sets to 1.
- **Escape in a presenting deck is Chromium-only.** In browser fullscreen the browser takes Escape and leaves fullscreen, which ends presenting. A deck holding a terminal asks for the Keyboard Lock API on Escape when it enters fullscreen, so a focused terminal keeps the key, and holding it still leaves. Only Chromium has the API, and only in a secure context (https, or `localhost`): `http://present.this` gets none, and Safari and Firefox never do. There Escape leaves fullscreen as it would without a terminal.
- **`ralph up` kills terminal sessions.** A rebuild of present restarts the serve agent, and `Manager.Close` kills every session as serve stops. Accepted for the first version; don't run an apply during a talk.
- **Codesign for MCP.** macOS kills a binary whose provenance xattrs went stale after a manual copy; `make install` re-signs with the stable local identity (docs/adr/0004), ad-hoc when the cert is absent.
- **`present_share` reaches only allow-listed hosts from the MCP sandbox.** The consuming repo registers `present mcp` through `recipes/present/present-mcp-sandbox.sh`, which runs it under srt with `~/.config/srt/present.json`. Egress goes only to localhost (events) and the shared URL's host, which the wrapper derives from `PRESENT_SHARED_URL` and `srt-run` merges into the allow-list at launch; the wrapper also passes `PRESENT_SHARED_URL`/`PRESENT_AUTHOR_KEY` through from the ralph-managed secrets file. Any other host fails there with a connection error. The Share button (served by `present serve`, launched through `present-serve.sh` with the same two vars) and `present share` run outside the sandbox and take any URL.
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
- Skill: repo-root `skills/present/SKILL.md`, the entry point for authoring (block picker, chart and engine pickers, colour rules, deck rules); `skills/present/examples/` holds four complete pages as `present_create` arguments (one of them the live terminal demo), and `skills/present/evals/` the `claude plugin eval` cases that check an agent picks the right blocks
- MCP registration: the consuming repo's agent-sync `servers.json` (worked example: `examples/dotfiles/recipes/agent-config/`)
