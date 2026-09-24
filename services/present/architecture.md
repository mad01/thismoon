# present architecture

## Overview

present runs as two cooperating processes built from one component: `present
serve`, a t-man launchd agent serving HTTP on port 7423 behind
`http://present.this/`, and `present mcp`, a stdio MCP server that Claude Code
launches per session. The MCP writes page files and never serves HTTP; serve
reads and serves them. They share one workdir (`~/.config/present` where it
already exists, otherwise `~/.local/state/present`), which is the entire
coupling between them: neither calls the other, and the MCP can run inside
a no-network sandbox. Local mode is loopback-only; shared mode, below, is
the exception.

The same binary also runs as a shared instance: `present serve --shared`
is one process that serves pages by id alone (no index, no listing), takes
writes only from callers presenting an author key, and mounts the MCP
tools over streamable HTTP at `/mcp`. Every request carries what the
handler needs (the key, the forwarded host), and the transport is
stateless, so any number of replicas can sit behind one hostname without
sticky sessions. The store behind either mode is the `store.Store`
interface; local present uses the filesystem implementation.

A local present can push its pages to such an instance. With `--shared-url`
and `--author-key` set, `present serve` shows a Share button on every page,
`present share <id>` does the same from a shell, and the local MCP registers
`present_share`; all three go through `internal/sharedclient`, the one place
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
                     pages/<id>/; id generation; expiry wrapper
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
internal/mdimport/   markdown file to title + Doc (goldmark), pure
internal/server/     HTTP handlers per mode; embeds shell.html, index_shell.html,
                     shared_index_shell.html, app.js, index.js
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
auto-detected) and an optional Graph JSON; `internal/render` compiles them
to HTML and a Cytoscape init script at authoring time (`Compile` wraps
`RenderDoc` and hands back the canonical Doc JSON too; `RenderGraph` does the
graph), and `internal/store` writes the results under `pages/<id>/` with a
version bump. `present_source` returns the stored `doc.json`/`graph.json` so
a later session can round-trip them back through `present_update`. `present
rerender` pushes a renderer or webkit change through existing pages by
re-rendering from those sources (pages without sources get a deterministic
legacy-HTML upgrade).

The exception is the markdown import, a local-mode route the index offers as
a button and a drop target. The browser reads the file and posts it to
`POST /api/import`; `internal/mdimport` maps the markdown onto a title and
a Doc, the same `Compile` step renders it, and the store gets exactly what
an MCP create would have written, `doc.json` included, so the page is
editable through `present_source` afterwards. Local serve authenticates
nothing, so the handler narrows who can reach it in three ways: it requires
`Content-Type: application/json`, which a cross-origin form or plain fetch
cannot send without a CORS preflight the server never answers; it refuses a
request the browser marks `Sec-Fetch-Site: cross-site`; and when an
`Origin` header is present it accepts only http(s) on localhost, a loopback
address, or a `.this` host, never comparing against the request's own Host,
because a DNS name pointed at 127.0.0.1 would agree with itself. It also
applies the shared instance's size cap so an imported page can always be
shared later.

The read path renders client-side (docs/adr/0005). `GET /p/{id}` serves the
embedded chrome-only `shell.html`; the browser loads `app.js`, fetches the
page as JSON from `GET /api/p/{id}`, mounts the compiled `content` fragment,
runs the graph script, and initializes the Cytoscape graph, metric charts,
references, and read-aloud. It then watches the page's version and reloads
when it moves, so an update reaches every open tab. The index works the same
way: `GET /` serves `index_shell.html` and `index.js` builds the list from
`GET /api/pages`. There is no server-side template layer.

Read-aloud is the page view's one dependency outside present: the speak
service named by `--speak-url` (default `http://speak.this`), which the page
JSON carries as `speak_url`. With a URL, `app.js` places webkit's
`<wk-read-aloud>` element in prepared mode right under the summary, and the
browser talks to speak directly: it registers the page's readable blocks
with `POST /read`, speak answers with a part key per block, and the element
shows how many parts are ready, prepares them on request, and downloads the
page's or one section's audio once every part exists. speak keeps that
registry in memory, so after a restart the element registers the page again.
When speak refuses the registration the page reads live, one request per
part, starting with a single sentence and ramping up to several, with no
status bar; when speak is unreachable the page has no read-aloud at all;
with the URL empty, which the shared manifest sets, the element is never
created. Code blocks and tables are never read. present
itself never calls speak.

Where the server has a change feed, the tab watches over server-sent events:
`GET /p/{id}/events` sends the version on connect, again whenever the page
moves, a heartbeat every 25 seconds, and a final `gone` for a deleted or
expired page. Only the cluster store has a feed, the informer behind its
page cache, so every replica can serve any tab's stream and an update
written through one replica reaches tabs on the other in well under a
second. Everywhere else, and whenever a stream is refused or goes silent
for a minute behind a buffering proxy, the tab polls `GET /p/{id}/version`
via `Webkit.poll`. A hidden tab closes its stream and reopens it when shown,
and serve ends every stream as shutdown starts, so a rollout moves tabs to
the other replica instead of waiting out the drain.

The share path starts local and ends on a shared instance. The Share button
posts to the local serve's `POST /p/{id}/share`; `present share` and
`present_share` call the same function directly. `sharedclient.Share` loads
the page and its sources from the local store, bundles them (title, content,
graph, references, doc, graph_source, ephemeral), and sends the bundle with
the author key as a bearer token to the shared instance's `POST /api/pages`,
or to `PUT /api/p/{id}` when the page was shared before, so the link stays
the same (when the instance has purged the copy, Share creates it afresh).
The result (shared id, URL, expiry, time of the share) lands in the local page's
`meta.json` through `store.SetShared`, which touches metadata only and never
bumps the version, so the open tab keeps its place. `GET /api/p/{id}` then
reports it in a `share` block that the page view reads to label the button
"Shared" and show the link. `present unshare` sends `DELETE /p/{id}` to the
instance and clears the record.

## Storage

```
~/.config/present/pages/<id>/
  meta.json      id, title, version, has_* flags, timestamps, and the
                 shared record once the page was pushed somewhere
  content.html   rendered HTML body fragment
  doc.json       canonical Doc source (when authored as Doc JSON)
  graph.js       rendered Cytoscape init (when has_graph)
  graph.json     canonical graph source (when authored as JSON)
  refs.json      optional [{title,url},...]
```

Plain files, one directory per page. Raw HTML/JS input deletes the now-stale
source file for that field; nothing else lands on disk.

A shared instance in Kubernetes swaps the filesystem for `--store k8s`: one
`Page` custom resource per page (`present.thismoon.mad01.dev/v1alpha1`,
namespaced), holding the same fields plus the rendered artifacts and the
canonical sources as strings in its spec. Replicas read and write it through
the API server with client-go's dynamic client; a write is a read-modify-
write guarded by resourceVersion and retried on conflict, so two replicas
never clobber each other. `version` is an explicit spec field rather than
`metadata.generation`, because source saves would bump the generation and
make open tabs reload for nothing. Each replica also runs an informer over
the namespace's pages. Its cache keeps metadata only, because a transform
drops content, graph, and references before the informer stores an object,
so its memory stays small however large pages get. Once synced it answers
the version poll every open tab makes, so a poll costs no API request; the
answer can trail a write made through another replica by the watch delay.
Full reads and every write still go to the API server. Ephemeral pages carry
a label the sweeper selects on; each replica sweeps on an interval, judging
expiry from the cache, and deletes each expired page under a resourceVersion
precondition, so a page that changed after the cache saw it survives until
the next sweep. A delete of an already-gone object counts as done, so no
leader is needed. A page is one object, so the store refuses a page over
1 MiB before it reaches the API server. The CRD is embedded in the binary
and a test keeps `deploy/base/crd.yaml` byte-identical to it.

## Deployment

`Dockerfile` builds the shared instance from the repo root: a cross-compiled
static binary on `distroless/static:nonroot` with the page assets (fonts,
Cytoscape, Chart.js) baked into `/var/lib/present/assets`, so a pod needs no
network and a read-only root filesystem. `deploy/base` is the kustomize
base: the CRD, a service account with a Role over `pages` in its namespace,
a two-replica Deployment running `serve --shared --store k8s --bind
0.0.0.0` with `PRESENT_NAMESPACE` from the downward API and
`PRESENT_SPEAK_URL` empty (no speak service runs there), and a ClusterIP
service on 7423. Overlays choose the namespace and the way in:
`overlays/kind` for local and CI testing (image tag `ci`, loaded, never
pulled), `overlays/ingress` (an Ingress with an external-dns hostname
annotation), `overlays/istio` (a VirtualService on an existing Gateway); the
hostname lives in one `present-host` ConfigMap value a private overlay
replaces. `make image` and `make kind-test` build and exercise the whole
thing against a kind cluster, and CI does the same on every PR that
touches present.

## Interfaces

Web: `GET /` (index shell), `GET /api/pages`, `GET /index.js`, `GET /p/{id}`
(page shell), `GET /api/p/{id}` (with a `share` block in local mode),
`GET /app.js`, `GET /p/{id}/version`, `DELETE /p/{id}` (the only delete
surface), `POST /api/import` (local mode; body `{"name", "markdown"}` as
JSON, answers `201 {"id", "url"}`, 415 without the JSON content type, 403
cross-site, 413 when the rendered page would pass the 1 MiB page cap, 400
for a file with nothing to import),
`POST /p/{id}/share` (local mode with a shared instance configured;
body `{"ephemeral": bool}`, answers the share block, 404 for an unknown page,
502 when the shared instance refuses or is unreachable), `GET /webkit/` and
`GET /webkit/version` from the webkit Go package.

CLI: `present serve`, `present mcp`, `present rerender [id...]`,
`present share <id> [--ephemeral]`, `present unshare <id>`,
`present key new`, `present version [-o json]`.

MCP tools: `present_create`, `present_read`, `present_source`,
`present_update`, `present_list`, `present_open` (macOS `open`), and
`present_share` when a shared instance is configured. Deliberately no delete
tool.

Shared mode drops `GET /`'s index for a static how-to page, drops
`GET /api/pages` and `GET /index.js`, and adds `POST /api/pages` (create
from a pushed page bundle), `PUT /api/p/{id}` (replace, author only),
`GET /api/whoami` (echo the caller's author hash), and `/mcp` (the tool
server over streamable HTTP). `DELETE /p/{id}` stays but needs the author's
key. Reads (`/p/{id}`, `/api/p/{id}`, `/p/{id}/version`) need nothing.
The chrome changes with the mode too: both shared shells leave the header's
⌘K site picker out (there are no local `.this` sites to jump to, and webkit
turns the shortcut off with the control), the page shell's back link reads
"← About" because the root is the how-to page rather than an index, and the
Share button stays hidden because the page JSON reports sharing disabled.

Config: `--workdir`/`PRESENT_WORKDIR` and `--port`/`PRESENT_PORT`, resolved
identically by both processes (a leading `~` is expanded in Go); both log
their resolved `workdir=… port=…` at startup. `--shared-url`/
`PRESENT_SHARED_URL` and `--author-key`/`PRESENT_AUTHOR_KEY` are persistent
too, and sharing is on only when both are set. `present serve` alone takes
`--bind`/`PRESENT_BIND`, `--shared`/`PRESENT_SHARED`, and
`--speak-url`/`PRESENT_SPEAK_URL`, where an empty value is the off switch
rather than a fallback to the default.
