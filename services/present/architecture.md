# present architecture

## Overview

present runs as two cooperating processes built from one component. They are
`present serve`, a t-man launchd agent serving HTTP on port 7423 behind
`http://present.this/`, and `present mcp`, a stdio MCP server that Claude Code
launches per session. The MCP writes page files and never serves HTTP; serve
reads and serves them. They share one workdir (`~/.config/present` where it
already exists, otherwise `~/.local/state/present`), which is the entire
coupling between them: neither calls the other, and the MCP can run inside
a no-network sandbox. Local mode is loopback-only; shared mode, below, is
the exception.

The same binary also runs as a shared instance. `present serve --shared`
is one process that serves pages by id alone (no index, no listing) and
takes writes only from callers presenting an author key. It mounts the MCP
tools over streamable HTTP at `/mcp`. Every request carries what the
handler needs (the key, the forwarded host), and the transport is
stateless, so any number of replicas can sit behind one hostname without
sticky sessions. The store behind either mode is the `store.Store`
interface; local present uses the filesystem implementation.

A local present can push its pages to such an instance. With `--shared-url`
and `--author-key` set, `present serve` shows a Share button on every page,
`present share <id>` does the same from a shell, and the local MCP registers
`present_share`. All three go through `internal/sharedclient`, the one place
that knows the wire shape. With only one of the two set, sharing stays off
and the process warns at startup. A shared instance never pushes anywhere;
it is where pages land.

## Structure

```
cmd/present/         entrypoint, delegates to internal/cli
internal/cli/        cobra command tree: serve, mcp, rerender, share, unshare,
                     key, version; share.go also builds the shared-instance
                     client from --shared-url/--author-key (nil when either
                     is missing)
internal/store/      Store interface + FS, the filesystem implementation over
                     pages/<id>/; id generation; expiry wrapper; deck.go, the
                     deck remote control the filesystem store alone provides
internal/store/k8sstore/  Store over Page custom resources (dynamic client),
                     the CRD (embedded, copied to deploy/base), the page
                     cache, the sweeper
internal/author/     author keys: mint, hash, read from a bearer header, check
internal/baseurl/    public base URL from X-Forwarded-* (middleware + derivation)
internal/sharedclient/  client for a shared instance (create, replace, delete,
                     whoami, author key as bearer); Share and Unshare wrap a
                     push with the local store's shared record
internal/render/     Doc-to-HTML (doc.go), Graph-to-JS (graph.go), legacy upgrade;
                     compile.go is the parse, render, canonical-JSON step every
                     write path shares
internal/mdimport/   markdown file to title + Doc + optional graph (goldmark), pure
internal/mermaid/    Mermaid flowchart source to GraphInput, pure; mdimport calls
                     it for a fenced mermaid block
internal/server/     HTTP handlers per mode (deck.go holds the deck shell and
                     command routes); embeds shell.html, index_shell.html,
                     shared_index_shell.html, app.js, viz.js, index.js
internal/mcpserver/  MCP wiring and the present_* tools, one tool set per mode
kit/notify           best-effort event emit to events.this (shared)
```

`internal/server` mounts the in-module webkit Go package with
`webkit.Mount(mux)`, serving the shared chrome (header, theme, font, size,
fixation controls) at `GET /webkit/`. The embedded shells carry only the
`<wk-header>` markup and present-specific styling (hero, chips, graph and
chart chrome); the client renderers build the DOM with webkit's shared
helpers (`Webkit.el`, `Webkit.escapeHtml`, `Webkit.poll`).

## Data flow

The write path is the MCP, with one browser-side exception. `present_create`
and `present_update` accept content as structured Doc JSON (or legacy HTML,
auto-detected) and an optional Graph JSON. `internal/render` compiles them
to HTML and a Cytoscape init script at authoring time (`Compile` wraps
`RenderDoc` and hands back the canonical Doc JSON too; `CompileGraph` does the
same around `RenderGraph`). `internal/store` then writes the results under `pages/<id>/` with a
version bump. `present_source` returns the stored `doc.json`/`graph.json` so
a later session can round-trip them back through `present_update`. `present
rerender` pushes a renderer or webkit change through existing pages by
re-rendering from those sources (pages without sources get a deterministic
legacy-HTML upgrade), and a local `present serve` runs that sweep itself in
the background at startup.

A page can carry a second rendition, the deck (docs/adr/0019). The tools take
it as `deck`, a Doc of its own whose sections are slides, and compile it
through the same `Compile` step to `deck.html` with its source in
`deck.json`. Nothing is derived from the brief, and a page may hold the
brief, the deck, or both, never neither. The two share the id, the title,
the one graph, the references, and the share record. So `present_source`
hands both Docs back, `present rerender` re-renders both, and one share
pushes both (the bundle carries `deck` and `deck_source` beside `content`
and `doc`). The store persists `has_brief` and `has_deck` in the metadata
so listings can say which renditions exist without loading a body.

The exception is the markdown import, a local-mode route the index offers as
a button and a drop target. The browser reads the file and posts it to
`POST /api/import`. Then `internal/mdimport` maps the markdown onto a title
and a Doc, and the first fenced mermaid flowchart onto a graph through
`internal/mermaid`. The same `Compile` steps render them. The store gets
exactly what an MCP create would have written, `doc.json` included, so the
page is editable through `present_source` afterwards.

Local serve authenticates nothing, so the handler narrows who can reach it
in three ways. It requires `Content-Type: application/json`, which a
cross-origin form or plain fetch cannot send without a CORS preflight the
server never answers. It refuses a request the browser marks
`Sec-Fetch-Site: cross-site`. When an `Origin` header is present it
accepts only http(s) on localhost, a loopback address, or a `.this` host. It
never compares against the request's own Host, because a DNS name pointed at
127.0.0.1 would agree with itself. It also applies the shared instance's
size cap so an imported page can always be shared later.

The read path renders client-side (docs/adr/0005). `GET /p/{id}` serves the
embedded chrome-only `shell.html`. The browser loads `app.js`, fetches the
page as JSON from `GET /api/p/{id}`, mounts the compiled `content` fragment,
runs the graph script, and initializes the Cytoscape graph, metric charts,
references, and read-aloud. It then watches the page's version and reloads
when it moves, so an update reaches every open tab. The index works the same
way: `GET /` serves `index_shell.html` and `index.js` builds the list from
`GET /api/pages`. There is no server-side template layer.

The deck view is the same shell and script at `GET /p/{id}/deck`, told apart
by its own URL. `app.js` splits the compiled `deck` fragment into slides in
the browser. The nodes before the first section make the title slide, every
section is one slide, the table of contents is dropped, and the references
close the deck. One slide shows at a time, the URL hash names it, and the
keys move (Right, Space, PageDown; Left, PageUp, Backspace; Home, End). F or
P starts presenting, a class on the document that hides the chrome and lets
the slide fill the window. Browser fullscreen is requested on top when a
key or click allows it. The graph initialises the first time its slide
shows, because Cytoscape sizes itself from a visible container. A deck-only
page redirects `GET /p/{id}` to the deck, and a page without a deck redirects
the deck route to the brief.

The browser builds the deck's chrome (logo, progress marker, footer line)
too. When the deck Doc sets a chrome field, the renderer puts one JSON
script island at the head of the fragment. `app.js` pulls it out before the
title slide is built and makes a strip along the bottom edge from it. The
island is all the store holds for it, so a deck that sets no field renders
as before. The defaults (logo shown, one dot per slide, no footer) come from
the view.

Remote control keeps the two processes as separate as everything else:
`present_deck` and `present deck` write the page's `deck-command.json`
through `store.DeckController`, which only the filesystem store implements.
A visible deck tab polls `GET /p/{id}/deck/command` once a second,
records the sequence number it first sees, and applies every later one
(start, stop, next, prev, goto). The route and the tool exist only when the
store relays commands. So a shared instance, whose store is wrapped or
lives in the cluster, serves decks with the keys alone.

Read-aloud is the page view's one dependency outside present: the speak
service named by `--speak-url` (default `http://speak.this`), which the page
JSON carries as `speak_url`. With a URL, `app.js` places webkit's
`<wk-read-aloud>` element in prepared mode right under the summary. The
browser talks to speak directly: it registers the page's readable blocks
with `POST /read`, and speak answers with a part key per block. The element
shows how many parts are ready and prepares them on request. speak keeps
that registry in memory, so after a restart the element registers the page
again.

When speak refuses the registration the page reads live, one request per
part, starting with a single sentence and ramping up to several, with no
status bar. When speak is unreachable the page has no read-aloud at all.
With the URL empty, which the shared manifest sets, the element is never
created. Code blocks and tables are never read. present itself never calls
speak.

Where the server has a change feed, the tab watches over server-sent events.
`GET /p/{id}/events` sends the version on connect, again whenever the page
moves, a heartbeat every 25 seconds, and a final `gone` for a deleted or
expired page. Only the cluster store has a feed, the informer behind its
page cache. So every replica can serve any tab's stream and an update
written through one replica reaches tabs on the other in well under a
second. Everywhere else, and whenever a stream is refused or goes silent
for a minute behind a buffering proxy, the tab polls `GET /p/{id}/version`
via `Webkit.poll`. A hidden tab closes its stream and reopens it when shown.
serve ends every stream as shutdown starts, so a rollout moves tabs to
the other replica instead of waiting out the drain.

The share path starts local and ends on a shared instance. The Share button
posts to the local serve's `POST /p/{id}/share`; `present share` and
`present_share` call the same function directly. `sharedclient.Share` loads
the page and its sources from the local store and bundles them (title,
content, graph, references, doc, graph_source, ephemeral). It sends the
bundle with the author key as a bearer token to the shared instance's
`POST /api/pages`, or to `PUT /api/p/{id}` when the page was shared before.
So the link stays the same (when the instance has purged the copy, Share
creates it afresh).
The result (shared id, URL, expiry, time of the share) lands in the local page's
`meta.json` through `store.SetShared`. It touches metadata only and never
bumps the version, so the open tab keeps its place. `GET /api/p/{id}` then
reports it in a `share` block that the page view reads to label the button
"Shared" and show the link. `present unshare` sends `DELETE /p/{id}` to the
instance and clears the record.

## Storage

```
~/.config/present/pages/<id>/
  meta.json      id, title, version, has_* flags, timestamps, and the
                 shared record once the page was pushed somewhere
  content.html   rendered HTML body fragment of the brief
  doc.json       canonical Doc source (when authored as Doc JSON)
  deck.html      rendered fragment of the deck (when has_deck)
  deck.json      canonical deck Doc source
  deck-command.json  the last 32 remote commands for open deck tabs
  graph.js       rendered Cytoscape init (when has_graph)
  graph.json     canonical graph source (when authored as JSON)
  refs.json      optional [{title,url},...]
~/.config/present/images/<sha256>.<ext>
                 image files the image blocks brought in from this machine
                 (png, jpg, gif, webp), named by content, served at /img/<name>,
                 removed when no page references them any more
```

Plain files, one directory per page. Raw HTML/JS input deletes the now-stale
source file for that field; nothing else lands on disk.

A shared instance in Kubernetes swaps the filesystem for `--store k8s`: one
`Page` custom resource per page (`present.thismoon.mad01.dev/v1alpha1`,
namespaced). Each resource holds the same fields plus the rendered artifacts
and the canonical sources as strings in its spec. Replicas read and write it
through the API server with client-go's dynamic client. A write is a
read-modify-write guarded by resourceVersion and retried on conflict, so two
replicas never clobber each other. `version` is an explicit spec field
rather than `metadata.generation`, because source saves would bump the
generation and make open tabs reload for nothing.

Each replica also runs an informer over the namespace's pages. Its cache
keeps metadata only, because a transform drops content, graph, and
references before the informer stores an object, so its memory stays small
however large pages get. Once synced it answers the version poll every open
tab makes, so a poll costs no API request. The answer can trail a write made
through another replica by the watch delay. Full reads and every write still
go to the API server.

Ephemeral pages carry a label the sweeper selects on. Each replica sweeps on
an interval, judging expiry from the cache, and deletes each expired page
under a resourceVersion precondition. The precondition means a page that
changed after the cache saw it survives until the next sweep. A delete of an
already-gone object counts as done, so no leader is needed.

A page is one object, so the store refuses a page over 1 MiB before it
reaches the API server. The CRD is embedded in the binary and a test keeps
`deploy/base/crd.yaml` byte-identical to it.

## Deployment

`Dockerfile` builds the shared instance from the repo root: a cross-compiled
static binary on `distroless/static:nonroot` with the page assets (fonts,
Cytoscape, Chart.js, D3) baked into `/var/lib/present/assets`. So a pod
needs no network and a read-only root filesystem. `deploy/base` is the kustomize
base: the CRD and a service account with a Role over `pages` in its
namespace. It also holds a two-replica Deployment running
`serve --shared --store k8s --bind 0.0.0.0` with `PRESENT_NAMESPACE` from
the downward API and `PRESENT_SPEAK_URL` empty (no speak service runs
there), and a ClusterIP service on 7423. Overlays choose the namespace and
the way in. They are `overlays/kind` for local and CI testing (image tag
`ci`, loaded, never pulled), `overlays/ingress` (an Ingress with an
external-dns hostname annotation), and `overlays/istio` (a VirtualService
on an existing Gateway). The hostname lives in one `present-host` ConfigMap
value a private overlay replaces. `make image` and `make kind-test` build
and exercise the whole thing against a kind cluster, and CI does the same
on every PR that touches present.

## Interfaces

Web: the page routes are `GET /` (index shell), `GET /api/pages`,
`GET /index.js`, and `GET /p/{id}` (page shell; 302 to the deck for a
deck-only page). The deck routes are `GET /p/{id}/deck` (the deck shell; a
page without a deck redirects to the brief) and `GET /p/{id}/deck/command`
(the last remote command, local filesystem store only). `GET /api/p/{id}`
answers with a `share` block in local mode and the `deck`, `has_deck`, and
`deck_control` fields. The remaining page routes are `GET /app.js`,
`GET /viz.js` (the deck's pure stepping helpers, loaded before `app.js`),
`GET /logo.png` (the embedded repo logo the deck chrome shows, served
no-cache with a content ETag), `GET /p/{id}/version`, and `DELETE /p/{id}`
(the only delete surface).
`POST /api/import` is local mode only. The body is `{"name", "markdown"}` as
JSON. It answers `201 {"id", "url"}`, 415 without the JSON content type, or
403 cross-site. It answers 413 when the rendered page would pass the 1 MiB
page cap, and 400 for a file with nothing to import. `POST /p/{id}/share`
needs local mode with a shared instance configured. The body is
`{"ephemeral": bool}`. It answers the share block, 404 for an unknown page,
and 502 when the shared instance refuses or is unreachable. `GET /webkit/`
and `GET /webkit/version` come from the webkit Go package.

CLI: `present serve`, `present mcp`, `present rerender [id...]`,
`present deck <id> <start|stop|next|prev|goto> [slide]`,
`present share <id> [--ephemeral]`, `present unshare <id>`,
`present key new`, `present version [-o json]`.

MCP tools: `present_create`, `present_read`, `present_source`,
`present_update`, `present_list`, `present_open` (macOS `open`),
`present_deck` on the local filesystem store, and `present_share` when a
shared instance is configured. Deliberately no delete tool.

Shared mode drops `GET /`'s index for a static how-to page and drops
`GET /api/pages` and `GET /index.js`. It adds `POST /api/pages` (create
from a pushed page bundle), `PUT /api/p/{id}` (replace, author only),
`GET /api/whoami` (echo the caller's author hash), and `/mcp` (the tool
server over streamable HTTP). `DELETE /p/{id}` stays but needs the author's
key. Reads (`/p/{id}`, `/api/p/{id}`, `/p/{id}/version`) need nothing.
The chrome changes with the mode too. Both shared shells leave the header's
⌘K site picker out (there are no local `.this` sites to jump to, and webkit
turns the shortcut off with the control). The page shell's back link reads
"← About" because the root is the how-to page rather than an index. The
Share button stays hidden because the page JSON reports sharing disabled.

Config: `--workdir`/`PRESENT_WORKDIR` and `--port`/`PRESENT_PORT`, resolved
identically by both processes (a leading `~` is expanded in Go); both log
their resolved `workdir=… port=…` at startup. `--shared-url`/
`PRESENT_SHARED_URL` and `--author-key`/`PRESENT_AUTHOR_KEY` are persistent
too, and sharing is on only when both are set. `present serve` alone takes
`--bind`/`PRESENT_BIND`, `--shared`/`PRESENT_SHARED`, and
`--speak-url`/`PRESENT_SPEAK_URL`, where an empty value is the off switch
rather than a fallback to the default.
