# operating present

present serves scrollable briefing pages: an agent publishes a page as
structured Doc JSON through the present_* MCP tools, the renderer compiles it
to HTML once at authoring time, and an open browser tab follows later updates
live. Pages outlive the session that wrote them and stay editable from any
later session via present_source and present_update.

## how it runs

Two processes share one workdir and never talk to each other. `present serve`
is the HTTP server: the page index, the page views, and the JSON APIs, bound
to localhost only (default {{.BaseURL}}). `present mcp` is a stdio MCP shim
that writes page files straight into the workdir and never serves HTTP. The
shim being up says nothing about serve: present_create succeeds with serve
down, and the returned URL is dead until serve runs. Both processes resolve
the workdir and port from --workdir/PRESENT_WORKDIR and --port/PRESENT_PORT
and log the resolved values to stderr at startup. The local domain front door
usually also routes http://present.this to serve; if the localhost port
answers but the .this host does not, blame the router, not this service.

## where state lives

Pages live under the workdir's pages/<id>/ — {{.StorePath}} on a fresh
install, ~/.config/present on one that already had that directory (pages are
never migrated), and whatever --workdir/PRESENT_WORKDIR say when set. Each
page directory holds meta.json, content.html (the
rendered HTML fragment), doc.json (canonical Doc source when the page was
authored as Doc JSON), graph.js plus graph.json for graphs, refs.json for
references. doc.json and graph.json are the editable sources: present_source
returns them, present_update accepts them back, and `present rerender`
re-renders content.html from them after a renderer change. Pages created from
raw HTML have no doc.json and can only be edited as HTML.

## failure modes

Start with `present doctor`: store readable, serve reachable, version skew;
one line per check, FAIL lines naming the cause. Store-readable is the
load-bearing check: the MCP tools write the store directly, so a readable
store means create and update still work with serve down. A service-reachable
FAIL means the page URLs the tools return are dead, not that the tools are
broken. t-man typically supervises serve: `t-man list`, then `t-man restart
present`; without t-man, `present serve` in a spare terminal works too.

Updates land but the page never changes: the MCP and serve resolved a
different workdir or port. Compare the resolved workdir and port each process
logged at startup; they must match.

Page shows literal garbage tokens: a backtick code span was nested inside
**bold** in a text field (paragraph, list item, kv value, table cell). The
renderer cannot nest the two and leaves a placeholder token in the page. Fetch
the source with present_source, un-nest the span, push it back with
present_update. Related content rules: multi-line code goes in a t=code block,
never a p with backticks; an unknown block type renders as an HTML comment, so
a typoed t leaves a silent gap; unicode arrows and math symbols in text are
rewritten to words for speech (an arrow renders as "to").

Read-aloud skips content: the reader deliberately skips code blocks and
tables, which read terribly as speech. Anything that must be heard belongs in
paragraphs, lists, callouts, or kv rows. Inline code in prose is fine and is
read aloud.

Page not found: the id is the directory name under pages/, minted by
present_create, not derived from the title. present_list returns every id and
URL.

## shared mode

`present serve --shared` is the same binary as a network-facing instance,
usually several replicas in Kubernetes behind one hostname. It serves no
index: the root is a how-to page, there is no listing endpoint and no
present_list, and a page is reachable only by the id present minted for it
(32 hex characters, a capability). Every write needs the caller's author key
as a bearer token; the server stores only its SHA-256 as the page's author,
and present_update, PUT /api/p/<id>, and DELETE /p/<id> refuse any other key
(401 without one, 403 with the wrong one). Reads need nothing. A page
created with ephemeral set expires 30 days after its last write and then
reads as 404 until the sweeper deletes it; other pages stay until their
author deletes them. The present_* tools are served over HTTP at /mcp with
the reduced set (present_create, present_read, present_source,
present_update, present_doctor); present_doctor there checks only that the
store answers, because the tools run inside the serving process. Page URLs
come from the X-Forwarded-Host and X-Forwarded-Proto headers the ingress
sets, or from --base-url when given, so a URL naming the wrong host means
the proxy in front isn't forwarding them. Shared mode refuses to start
without an explicit --bind; local mode refuses any bind but loopback.

## version skew

`present version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart present`) and compare again; `present
doctor` runs this comparison as its version-skew check. Do not confuse it
with `GET /p/{id}/version`, the per-page counter open tabs poll for reload.

## first moves

1. `present doctor`: store readable, serve reachable, version skew in one pass
2. If serve is unreachable: `t-man restart present` (dead page links only)
3. On version skew: `t-man restart present`, then `present doctor` again
4. `present_list`, to confirm the store loads and to get real page ids
5. On a wrong-looking page: `present_source`, check the Doc JSON against the
   content rules above, then `present_update`
