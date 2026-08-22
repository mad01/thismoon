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
and log the resolved values to stderr at startup. Machines usually also route
http://present.this to serve via the local domain front door; if the
localhost port answers but the .this host does not, the router is the
problem, not this service.

## where state lives

Pages live under {{.StorePath}}/pages/<id>/: meta.json, content.html (the
rendered HTML fragment), doc.json (canonical Doc source when the page was
authored as Doc JSON), graph.js plus graph.json for graphs, refs.json for
references. doc.json and graph.json are the editable sources: present_source
returns them, present_update accepts them back, and `present rerender`
re-renders content.html from them after a renderer change. Pages created from
raw HTML have no doc.json and can only be edited as HTML.

## failure modes

Connection refused, or a tool-returned URL that does not load: serve is not
running. t-man typically supervises it: `t-man list`, then `t-man restart
present`. For a quick test without t-man, `present serve` in a spare terminal
also works.

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
present_create; it is not derived from the title. present_list returns every
page's id and URL.

## version skew

`present version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart present`) and compare again. Do not
confuse this with `GET /p/{id}/version`, the per-page counter open tabs poll
for live reload.

## first moves

1. `curl -s {{.BaseURL}}/version` (a JSON response means serve is up)
2. If unreachable: `t-man list`, then `t-man restart present`
3. Compare `present version -o json` with the `/version` endpoint for skew
4. `present_list`, to confirm the store loads and to get real page ids
5. On a wrong-looking page: `present_source`, check the Doc JSON against the
   content rules above, then `present_update`
