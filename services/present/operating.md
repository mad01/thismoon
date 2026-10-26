# operating present

present serves scrollable briefing pages: an agent publishes one as Doc JSON
through the present_* MCP tools. The renderer compiles it to HTML at
authoring time, and an open tab follows later updates live. A page can carry
a slide deck beside its brief, or instead of it, at /p/<id>/deck.

## how it runs

Locally, two processes share one workdir and never talk: `present serve` is
the HTTP server on loopback only (default {{.BaseURL}}). `present mcp` is a
stdio shim that writes page files into the workdir and serves no HTTP, so
present_create succeeds with serve down and its URL stays dead until serve
runs. Both read --workdir/PRESENT_WORKDIR and --port/PRESENT_PORT and log them.

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
`present rerender` re-renders from; a raw-HTML page has neither. A deck adds
deck.html and deck.json the same way, and deck-command.json holds the last
remote command (present_deck, `present deck`) that open deck tabs poll. An
image block whose src was a file on this machine puts a copy under
images/<sha256>.<ext> beside pages/, served at /img/<name>; deleting the last
page that shows it removes the copy. history/<version>.json under a page
keeps its newest 20 versions. A shared instance keeps pages as Page
resources, so `kubectl get pages` is its store, takes image URLs only, and
keeps no history.

## failure modes

Start with `present doctor`. Locally it checks store readable, serve
reachable, version skew, and the shared instance when one is configured (one
GET /api/whoami with the key). A FAIL names it and whether it was unreachable
or refused the key. On a shared instance present_doctor checks only that the
store answers, because the tools run inside the serving process. Store
readable is load-bearing: the tools write it directly, so create and update
work with serve down.

Updates land but the page never changes: the MCP and serve resolved a
different workdir or port; compare what each logged at startup. Literal
garbage tokens mean a backtick code span nested inside **bold** in a text
field, so un-nest it through present_source and present_update. Multi-line
code goes in a t=code block; an unknown block type renders as an HTML comment,
so a typoed t leaves a gap. Read-aloud goes through speak (--speak-url, default
http://speak.this; empty turns it off) and skips code and tables. Play buttons
but no audio bar mean registration in flight, nothing speakable, or speak
refused it and the page reads live. No buttons at all mean speak is down.

A tab that fetches GET /p/<id> about once a second is a cached shell from
before pages sent Cache-Control: no-store; hard-reload it or clear the
browser cache.

A deck tab that ignores `present deck` or present_deck: the MCP and serve
resolved different workdirs (same check as above). Or the tab was opened
after the command was sent (only later commands apply). Or the deck is on a
shared instance, which has no remote control; the keys still work there. A slide
that overflows or scrolls carries too much: the skill's deck rules say a
heading and 3 to 5 short items or two sentences per slide.

## editing part of a page

present_patch_section changes one section without resending the Doc. It
works at the section a table-of-contents anchor (next-steps for "Next
steps") or index:N names, and takes the section as Doc JSON or as markdown
opening with a heading. Serve takes the same patch at
PATCH /api/p/<id>/sections/<anchor> and POST /api/p/<id>/sections. Every
write takes a lock on the page directory, so the MCP, serve, and parallel
workers never lose each other's edits. A 409 says what to do next. With a
version, base_version was stale: read and retry. With indices, the anchor is
shared: use index:N or rename the heading. Alone, the page has no Doc to
patch, because it was stored as raw HTML.

Progress goes to POST /api/p/<id>/progress, or to `present progress <id>
<label> <pct>`, which takes the page lock and works with serve up or down;
hooks use it. It sets the progress block carrying that label, or adds one to
a Progress section. A 409 with sections means two blocks share the label:
pass --section. A report the bar already shows writes nothing and keeps the
version. Every other report is a version, and only 20 are kept, so report on
change rather than on a timer. A bad edit is undone from
history: GET /api/p/<id>/history lists versions, present_source with a
version returns one, and present_update with that source restores it.

## sharing a page

A local present pushes a page to a shared instance when both --shared-url and
--author-key (PRESENT_SHARED_URL, PRESENT_AUTHOR_KEY) are set. With one of
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
--shared-url and --author-key are set. It refuses the local delete with 502
when the instance is unreachable, so the two cannot drift apart. Without
those settings the local page goes, the copy stays, and a log line names the
orphaned URL. On the shared side 401 means no author key arrived and 403 that
the wrong one did. A failed share reads "Share failed: ..." in the modal and
answers 502 from POST /p/<id>/share; the failure is the instance's.

## live terminal

A terminal block in a brief or deck shows a terminal session: a process
the local serve runs in a pseudo-terminal (PTY) for that page and session
name. The session outlives the tab, and a tab that attaches later gets its
recent output replayed. The live terminal is off by default. The switch is the file terminal.enabled in
the workdir (~/.config/present/terminal.enabled on a fleet machine).
`present terminal on` creates it and `off` removes it. Serve checks it
every 2 seconds, so neither needs a restart, and off ends every session.
`present terminal status` prints the switch, `present terminal list` the
sessions serve holds, and `present terminal kill <page> <session>` ends
one. A shared instance never runs a session; its terminal blocks say so.

A block that says "Live terminal is off" while status says on: a block
whose session ended waits for Run, and one that never started asks again
within 3 seconds. Past that, serve may run an older binary without the
terminal routes (see version skew). Or the CLI and serve resolved
different workdirs. A block that shows a reason and
stops: the session could not start, most often a cwd that is relative or
missing. Fix the block with present_update, then press Run. A session that
sits blank for about 10 seconds before its prompt: a shell startup hook is
waiting for a terminal reply the browser never sends. Guard that hook on
PRESENT_TERMINAL, which every session sets to 1. A changed command reaches
a running session only after Kill and Run. A restart of serve, including a
ralph up that rebuilds present, ends every session.

## version skew

`present version -o json` and `GET {{.BaseURL}}/version` return the same four
keys (version, commit, tag, build_time) for the binary on PATH and for the
running process; a shared instance answers the same way. Differing `commit`
values mean an old process is still serving: `t-man restart present`.

## first moves

1. `present doctor`: store, reachability, skew, shared instance in one pass
2. Serve unreachable or skewed: `t-man restart present`, then doctor again
3. Wrong-looking page: `present_list` for its id, `present_source` to check
   it against the content rules above, then `present_update`, or
   `present_patch_section` when one section is wrong; a page an edit broke
   comes back from `present_source` with an earlier `version`
4. Failed share: retry from the Share button or `present share <id>`;
   the MCP sandbox lets present_share reach only hosts on its srt allow-list
5. Terminal block off or stuck: `present terminal status`, then
   `present terminal list`; end a stuck session with
   `present terminal kill <page> <session>` and press Run
