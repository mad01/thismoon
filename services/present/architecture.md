# present architecture

## Overview

present runs as two cooperating processes built from one component: `present
serve`, a t-man launchd agent serving HTTP on port 7423 behind
`http://present.this/`, and `present mcp`, a stdio MCP server that Claude Code
launches per session. The MCP writes page files and never serves HTTP; serve
reads and serves them. They share one workdir (`~/.config/present` by
default), which is the entire coupling between them — neither calls the other,
and the MCP can run inside a no-network sandbox. Everything is localhost-only.

## Structure

```
cmd/present/         entrypoint, delegates to internal/cli
internal/cli/        cobra command tree: serve, mcp, rerender, version
internal/store/      filesystem CRUD over pages/<id>/; id generation
internal/render/     Doc-to-HTML (doc.go), Graph-to-JS (graph.go), legacy upgrade
internal/server/     HTTP handlers; embeds shell.html, index_shell.html,
                     app.js, index.js
internal/mcpserver/  MCP wiring and the present_* tools
internal/notify/     best-effort event emit to events.this
```

`internal/server` mounts the in-module webkit Go package with
`webkit.Mount(mux)`, serving the shared chrome (header, theme, font, size,
bionic controls) at `GET /webkit/`. The embedded shells carry only the
`<wk-header>` markup and present-specific styling (hero, chips, graph and
chart chrome); the client renderers build the DOM with webkit's shared
helpers (`Webkit.el`, `Webkit.escapeHtml`, `Webkit.poll`).

## Data flow

The write path is MCP-only. `present_create` and `present_update` accept
content as structured Doc JSON (or legacy HTML, auto-detected) and an optional
Graph JSON; `internal/render` compiles them to HTML and a Cytoscape init
script at authoring time (`RenderDoc`/`RenderGraph`), and `internal/store`
writes the results under `pages/<id>/` with a version bump. `present_source`
returns the stored `doc.json`/`graph.json` so a later session can round-trip
them back through `present_update`. `present rerender` pushes a renderer or
webkit change through existing pages by re-rendering from those sources
(pages without sources get a deterministic legacy-HTML upgrade).

The read path renders client-side (docs/adr/0005). `GET /p/{id}` serves the
embedded chrome-only `shell.html`; the browser loads `app.js`, fetches the
page as JSON from `GET /api/p/{id}`, mounts the compiled `content` fragment,
runs the graph script, and initializes the Cytoscape graph, metric charts,
references, and read-aloud. It then polls `GET /p/{id}/version` via
`Webkit.poll`, so an MCP update to the page reloads any open tab. The index
works the same way: `GET /` serves `index_shell.html` and `index.js` builds
the list from `GET /api/pages`. There is no server-side template layer.

## Storage

```
~/.config/present/pages/<id>/
  meta.json      id, title, version, has_* flags, timestamps
  content.html   rendered HTML body fragment
  doc.json       canonical Doc source (when authored as Doc JSON)
  graph.js       rendered Cytoscape init (when has_graph)
  graph.json     canonical graph source (when authored as JSON)
  refs.json      optional [{title,url},...]
```

Plain files, one directory per page. Raw HTML/JS input deletes the now-stale
source file for that field; nothing else lands on disk.

## Interfaces

Web: `GET /` (index shell), `GET /api/pages`, `GET /index.js`, `GET /p/{id}`
(page shell), `GET /api/p/{id}`, `GET /app.js`, `GET /p/{id}/version`,
`DELETE /p/{id}` (the only delete surface), `GET /webkit/` and
`GET /webkit/version` from the webkit Go package.

CLI: `present serve`, `present mcp`, `present rerender [id...]`,
`present version [-o json]`.

MCP tools: `present_create`, `present_read`, `present_source`,
`present_update`, `present_list`, `present_open` (macOS `open`). Deliberately
no delete tool.

Config: `--workdir`/`PRESENT_WORKDIR` and `--port`/`PRESENT_PORT`, resolved
identically by both processes (a leading `~` is expanded in Go); both log
their resolved `workdir=… port=…` at startup.
