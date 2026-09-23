# operating present

present serves scrollable briefing pages: an agent publishes one as Doc JSON
through the present_* MCP tools, the renderer compiles it to HTML at
authoring time, and an open tab follows later updates live.

## how it runs

Locally, two processes share one workdir and never talk to each other.
`present serve` is the HTTP server, bound to loopback only (default
{{.BaseURL}}); `present mcp` is a stdio shim that writes page files into the
workdir and serves no HTTP, so present_create succeeds with serve down and
the URL it returns stays dead until serve runs. Both read
--workdir/PRESENT_WORKDIR and --port/PRESENT_PORT and log what they resolved.

`present serve --shared` is the same binary run as one network-facing
process, usually several replicas in Kubernetes behind one hostname. It has
no index: a page is reachable only by the 32-hex id present minted for it, a
capability. Reads need nothing, writes need the caller's author key as a
bearer token, and the tools are served at /mcp without present_list and
present_open. Shared mode needs an explicit --bind, local mode refuses any
bind but loopback, and running one on a cluster is `deploy/README.md`.

## where state lives

Local pages live under pages/<id>/ in the workdir: {{.StorePath}} on a fresh
install, ~/.config/present where a pages/ directory already exists (pages are
never migrated). doc.json and graph.json beside the rendered content.html are
the editable sources present_source returns, present_update takes back, and
`present rerender` re-renders from; a raw-HTML page has neither. A shared
instance keeps pages as Page resources, so `kubectl get pages` is its store.

## failure modes

Start with `present doctor`. Locally it checks store readable, serve
reachable, version skew, and the shared instance when one is configured (one
GET /api/whoami with the key; a FAIL names it and whether it was unreachable
or refused the key). On a shared instance present_doctor checks only that the
store answers, because the tools run inside the serving process. Store
readable is load-bearing: the tools write it directly, so create and update
work with serve down.

Updates land but the page never changes: the MCP and serve resolved a
different workdir or port; compare what each logged at startup. Literal
garbage tokens mean a backtick code span nested inside **bold** in a text
field, so un-nest it through present_source and present_update. Multi-line
code goes in a t=code block, an unknown block type renders as an HTML
comment so a typoed t leaves a gap, and read-aloud skips code and tables.

## sharing a page

A local present pushes a page to a shared instance when both --shared-url and
--author-key (PRESENT_SHARED_URL, PRESENT_AUTHOR_KEY) are set; with one of
them it warns at startup and sharing stays off. Three entry points do the same
push: the Share button in the page header, `present share <id> [--ephemeral]`,
and the present_share tool. Each records the copy in the local meta.json, the
source of the button's "Shared" label. `present unshare <id>` clears both.

An ephemeral copy expires 30 days after the last share, and sharing again
restarts those 30 days under the same link; any other copy stays until its
author deletes it. Only the author key can change or remove a copy, and a
lost one is replaced rather than recovered: `present key new`, then share
every page again. On any shared write path, whatever store the instance runs
on, a page over 1 MiB is refused: 413 over HTTP, a tool error over MCP.

Deleting a local page that has a shared copy removes the copy first when
--shared-url and --author-key are set, and refuses the local delete with 502
when the instance is unreachable, so the two cannot drift apart. Without
those settings the local page goes, the copy stays, and a log line names the
orphaned URL. On the shared side 401 means no author key arrived and 403 that
the wrong one did. A failed share reads "Share failed: ..." in the modal and
answers 502 from POST /p/<id>/share; the failure is the instance's.

## version skew

`present version -o json` and `GET {{.BaseURL}}/version` return the same four
keys (version, commit, tag, build_time), for the binary on PATH and for the
running process; a shared instance answers the same way. Differing `commit`
values mean an old process is still serving: `t-man restart present`. Not
`GET /p/{id}/version`, the per-page counter tabs poll for reload (a shared
instance on the cluster store streams it from `GET /p/{id}/events`).

## first moves

1. `present doctor`: store, reachability, skew, shared instance in one pass
2. Serve unreachable or skewed: `t-man restart present`, then doctor again
3. `present_list`, to confirm the store loads and to get real page ids
4. Wrong-looking page: `present_source`, check it against the content rules
   above, then `present_update`
5. Failed share: retry from the Share button or `present share <id>`;
   the MCP sandbox lets present_share reach only an https:// shared URL
