# why present

## The problem

Agent sessions produce long outputs: research summaries, review reports,
plans. Read in a terminal they are disposable; a wall of monospace text is a
poor way to digest structure, relationships, or metrics, and it scrolls away
with the session. What was missing on the machine was a place an agent can
publish to: a durable page, readable in a browser with real typography,
graphs, and charts, that later sessions can keep editing while an open tab
follows along.

## Why its own service

The pages have to outlive the session that wrote them, so something must
store them and keep serving them. Publishing is agent-driven, which makes
the write surface MCP tools rather than an editor or a static-site build,
and the store has to support round-tripping: a later session asks for a
page's source and patches it. A page is also a live localhost origin,
`present.this`, not a file opened from disk, which is what lets it load
shared webkit chrome and fetch speech from speak cross-origin.

## Why this shape

Two cooperating processes share one workdir: `present mcp` writes page files
and never serves HTTP, `present serve` reads and serves them. That split
lets the MCP run inside a no-network sandbox while pages still appear at
`present.this`, and neither process depends on the other to do its half.
Content is authored as structured Doc JSON and compiled to HTML once, at
authoring time; the browser then renders client-side from a static shell
plus JSON APIs (docs/adr/0005), so the Go renderer is never duplicated in
JavaScript and the MCP's `present_read` and the web page show the same
compiled output. Because the canonical sources (`doc.json`, `graph.json`)
are stored beside the rendered fragment, `present rerender` can push a
renderer or webkit change through every existing page, and a version bump
per update drives live reload in open tabs.

## Non-goals

Pages are create/read/update/list; there is deliberately no MCP delete, so
an agent cannot destroy a page (removal is a manual act in the web index).
present serves localhost by default; a shared instance can run on an
internal network, but it stays a drop box for single pages reachable by
unguessable id, with no accounts and no listing, and it never becomes a
multi-page site. It is not a rendering framework either; the chrome and
shared primitives come from webkit, and present keeps only its
briefing-specific pieces local.
