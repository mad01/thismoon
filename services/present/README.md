# present

A small CLI + MCP server for serving single-page HTML briefing pages over localhost, and the same binary run as a shared instance others reach by link. A page can also carry a slide deck beside its brief, or instead of it, for the same material talked through in a room.

Pages are created, read, updated, and listed through the MCP tools; deleting one is a button in the web index rather than a tool. Each page keeps its rendered HTML body next to the Doc JSON it was rendered from. The browser does the assembly: `present serve` returns a chrome-only shell, fetches the page as JSON, and builds the DOM, so there is no template on disk to edit. An open tab reloads itself after an update: a shared instance on the cluster store pushes the new version over server-sent events, and everywhere else the tab polls for it.

## Install

```bash
make install   # builds and installs ~/code/bin/present (codesigned on macOS: the stable local identity, ad-hoc without it)
```

## Usage

Two cooperating processes share a working directory (`~/.config/present` where it already exists, otherwise `~/.local/state/present`) and a port (`7423`):

```bash
present serve              # HTTP server: GET / lists pages, /p/<id> renders one
present mcp                # MCP stdio server exposing present_* tools to Claude Code
```

Typically `present serve` runs as a background launchd agent (via t-man); Claude Code launches `present mcp`.

```bash
present version           # bare git sha the binary was built from
present version -o json   # build metadata: version, commit, tag, build_time - probed by `ralph outdated`
present deck <id> next    # drive the deck open in the browser: start, stop, next, prev, goto <n>
present export <id>       # write the page as one HTML file that opens without present: <id>.html
present terminal on       # let terminal blocks run sessions; off, status, list, kill <page> <session>
```

| Flag | Env | Default |
|------|-----|---------|
| `--workdir` | `PRESENT_WORKDIR` | `~/.config/present` if present, else `~/.local/state/present` |
| `--port` | `PRESENT_PORT` | `7423` |
| `--bind` (serve only) | `PRESENT_BIND` | `127.0.0.1`; shared mode requires it explicitly |
| `--shared` (serve only) | `PRESENT_SHARED` | `false` |
| `--shared-url` | `PRESENT_SHARED_URL` | unset; sharing needs it and `--author-key` |
| `--author-key` | `PRESENT_AUTHOR_KEY` | unset; prefer the env var over the flag |
| `--speak-url` (serve only) | `PRESENT_SPEAK_URL` | `http://speak.this`; empty turns read-aloud off |

The header's theme toggle switches light and dark. Its Themes link opens a picker with eight palette families (webkit's default, Catppuccin, Nord, Solarized, Gruvbox, Rosé Pine, Tokyo Night, One), one choice per mode, plus a follow-the-system option. The browser keeps the choice for this host, and an open page recolours as soon as it changes.

Pages are read aloud through the speak service. On load a page registers its text with speak and shows an audio bar under the summary: how many parts are ready and a button to prepare them all. Each section gets its own play button and state badge. Code blocks and tables are left out. Without speak reachable the page shows no read-aloud controls; with `--speak-url ""` it never asks.

### Slide decks

A page has two possible renditions under one id: the brief, which is the scrollable page above, and a deck, a second Doc whose sections are slides. Either can exist without the other, and nothing is converted between them. An agent writes the deck for the room, shorter than the brief, and the brief stays where the detail lives. `present_create` takes the deck as a `deck` argument beside or instead of `content`, and `present_update` replaces or removes it.

The deck opens at `/p/<id>/deck`. A page with both shows a Slides link in the brief's header and a Brief link in the deck's, and a page with only a deck sends `/p/<id>` there. The title slide comes from the deck's title, summary, meta line, and chips; every section is one slide; the references make the last one. A bar at the bottom holds the arrows, the counter, and a Present button, and the URL's `#3` names the slide, so a link can open the deck on a slide.

Along the bottom edge sits the deck's chrome. The repo logo is in the right corner. One dot per slide sits in the centre, filled in as you go, and each dot is a button that jumps to its slide. Past 24 slides a thin bar takes over from the dots. A deck changes that from its Doc with five optional fields. `logo` takes `"none"` or an image URL, `logo_position` names a corner, and `progress` is `dots`, `bar`, or `none`. A footer line made of `footer` and `presenter` appears only when one of them is set, and the presenter also shows as a byline on the title slide. The strip stays on screen while presenting. The logo is served from the binary at `/logo.png`, so a shared instance needs nothing extra. The header's Audio toggle hides the read-aloud bar, play buttons, and badges. A deck opens with them hidden and a brief with them shown, and each view remembers its own choice.

A section can name a `layout` for its slide: `center`, `statement`, or `section`. It can carry a `tone`, a palette role that colours the slide's accent and bands the section in the brief. It can hold speaker `notes`, which the deck shows in a drawer on the N key, and set `reveal` to show a list one item per Next.

A chart with `steps` walks its series one per Next with a caption under it, and the brief lists the captions. A `diagram` with `steps` walks its boxes and arrows the same way, lighting up the ones each step is about. A slide holding one stat, one quote, or one diagram alone gives it the whole slide. The deck Doc's `transition` picks how slides change: a crossfade by default, `slide` for a nudge in the direction of travel, `none` for a cut. The browser's Reduce Motion setting makes every change a cut.

A slide or a section can use seven more blocks beside the ones a brief has. `columns` holds two or three equal columns of blocks and drops to one column on a narrow window; the page's graph may sit in one of them. `stat` is a large figure over a label. `quote` carries its attribution. `details` is a collapsible block, closed until opened, which read-aloud opens as it reads it. `image` shows a picture with a caption: an image URL, or a file on this machine that the tool copies into the page store and serves at `/img/`. `diagram` draws an architecture picture from boxes, boundaries, and arrows, laid out in the browser, and fills a slide when it stands alone. Callouts come in `info`, `warn`, `ok`, and `error`.

The seventh, `terminal`, shows a live terminal session: a process the local `present serve` runs for the page under a session name. Two slides naming one session show the same process, so a demo started on one slide is still there on a later one. It runs nothing until `present terminal on` has switched the live terminal on, and a shared instance shows a placeholder in its place. The skill's worked example `skills/present/examples/live-terminal-demo.json` is a deck that starts a shell and Claude on two slides and comes back to both after three talk slides. Its brief holds the switch commands.

Keys: Right, Space, or PageDown for the next slide; Left, PageUp, or Backspace for the previous one; Home and End for the first and last. F or P starts presenting: the header and the bar go away, one slide fills the window at a larger size, and the browser is asked for fullscreen. Escape ends it, as does leaving fullscreen through the browser, and so does F or P pressed in fullscreen. Pressed while presenting without fullscreen, which is where a reload leaves you, F or P asks for fullscreen again. A reload, the one an update triggers included, comes back on the same slide in the same mode. The tab remembers where it was, and a fresh tab starts at the title slide.

Read-aloud works on a deck the way it does on a brief: the bar sits on the title slide and every slide carries its own play control.

Sharing a page that has a deck to a shared instance built before decks keeps the copy and its link but reports that the deck was dropped. Upgrade the instance and share again.

The deck can be driven from outside the tab too. `present deck <id> start|stop|next|prev|goto <n>` and the `present_deck` tool write the page's last command into its directory, and every open tab of the deck follows within a second. Tabs opened later ignore commands sent before they loaded. This works on the local instance only; a shared instance serves decks but takes no remote commands.

Sharing carries the deck: a shared copy has the same renditions as the local page, opens on its brief or its deck the same way, and answers the same keys.

### Shared mode

`present serve --shared --bind 0.0.0.0` runs the same binary as a network-facing shared instance, meant for a few replicas in Kubernetes behind one hostname. There is no index and no listing: a page is reachable only by the 32-hex id present minted for it. Every write carries an author key as a bearer token (the server keeps only its hash, and only that key can update or delete the page). Reads need nothing, and a page created as ephemeral disappears 30 days after its last write. The root serves a how-to page, `POST /api/pages` and `PUT /api/p/<id>` take pushed pages, and the present tools are served over HTTP at `/mcp` (`present_create` with an `ephemeral` flag, `present_read`, `present_source`, `present_update`, `present_patch_section`, `present_doctor`). Page URLs follow the `X-Forwarded-Host` and `X-Forwarded-Proto` headers the ingress sets.

### Sharing a page

A local present can push any of its pages to a shared instance, so someone without access to your machine can read it by link. Mint an author key once, point the local processes at the instance, and share:

```bash
present key new                                   # prints a 64-hex author key; the instance stores only its hash
export PRESENT_SHARED_URL=https://present.example.com
export PRESENT_AUTHOR_KEY=<the key you minted>    # prefer the env var: a flag shows up in the process list
present share <id>                                # prints the link
present share <id> --ephemeral                    # the copy expires 30 days after the last share
present unshare <id>                              # removes the copy
```

With both variables set, `present serve` shows a Share button in every page's header (a modal with the expiry checkbox, the link, and a Copy button), and `present mcp` registers a `present_share` tool. The button reads "Shared" once a copy exists. Sharing a page again replaces the copy under the same link. Only your author key can change or remove the copy; if you lose it, mint a new one and share each page again. `present doctor` checks the instance and the key once you have set both variables.

### Exporting a page

When the reader has no access to your machine or a shared instance, export the page as one HTML file and send that:

```bash
present export <id>                  # writes <id>.html in the current directory
present export <id> -o brief.html    # or any path
present export <id> --deck           # the slide deck, as <id>-deck.html
```

The file opens in any browser with no present server. It carries the page's markup, styles, scripts, and local images, so the prose reads offline and with scripts off. Graphs, charts, and diagrams load their libraries from cdn.jsdelivr.net, each pinned by an integrity hash. They need the network when the file opens. Offline they say so, and the rest of the page reads. The header keeps the theme, font, size, and fixation controls. A terminal block shows a note, since it needs the local serve, and a deck shows no logo unless its chrome names one. A prose page comes to about 0.9 MB, half of it the inlined latin fonts. The `present_export` tool writes the same file to `exports/` in the page store and returns its path. [ADR-0029](../../docs/adr/0029-present-standalone-export.md) records why the file inlines our code and loads the libraries from a CDN.

### Importing a markdown file

The index page (`GET /`) has an "Import markdown" button, and a `.md`, `.markdown`, or `.txt` file dropped anywhere on that page does the same. The browser reads it and posts it to `POST /api/import`, the server converts it to a page, and you land on the new page. Headings become the title and sections, and paragraphs, lists, code blocks, tables, and blockquotes map to their present blocks. Raw HTML, horizontal rules, and footnotes are dropped, and nested lists are flattened. The first fenced `mermaid` flowchart becomes the page's graph (later ones, and other mermaid diagram types, stay code blocks).

A file is refused when the page it renders to (the HTML plus the Doc source) would pass 1 MiB. That is the cap a shared instance enforces, so an imported page can always be shared. It works out to about 250 to 500 KiB of markdown, less for markup-heavy files.

The same endpoint works from a shell on the local instance:

```bash
jq -Rs '{name: "notes.md", markdown: .}' notes.md \
  | curl -sS -X POST http://localhost:7423/api/import -H 'Content-Type: application/json' -d @-
```

The import exists in local mode only; a shared instance takes pages through the MCP tools or `POST /api/pages` with an author key.

## HTTP API

Besides the page views, `present serve` takes edits to one part of a page, so a worker can report into a page without holding the whole Doc. Every write takes a lock on the page directory, so parallel writers never lose an edit. Local serve takes these from pages on this machine with `Content-Type: application/json`, the import's rule. A shared instance takes them only with the page author's key as a bearer token.

| Route | Body | Does |
|-------|------|------|
| `PATCH /api/p/{id}/sections/{anchor}` | `{"op", "section"?, "body_format"?, "rendition"?, "base_version"?}` | `replace`, `insert_before`, `insert_after`, or `remove` at the section `{anchor}` names |
| `POST /api/p/{id}/sections` | `{"section", "body_format"?, "rendition"?, "base_version"?}` | Append a section at the end |
| `POST /api/p/{id}/progress` | `{"label", "pct", "note"?, "section"?, "rendition"?}` | Set the progress block carrying `label`, or add one |
| `GET /api/p/{id}/history` | | List the kept versions, newest first |
| `GET /api/p/{id}/history/{version}` | | One kept version, in the shape `present_source` returns |
| `GET /api/p/{id}/diff?since={version}` | | The sections of the brief and the deck that changed, were added, or were removed since a kept version |

`{anchor}` is a section's table-of-contents anchor: the heading lowercased, spaces as dashes, other punctuation dropped, so "Next steps" is `next-steps`. `index:N` (zero-based) names a section by position, for a heading that repeats or has no letters. `section` is one Doc section object, or with `"body_format": "markdown"` a string of markdown that opens with a `#` or `##` heading. `rendition` is `content` (the default, the brief) or `deck`. With `base_version`, the patch applies only while the page is still at that version. The whole Doc is recompiled, so a section the renderer refuses is refused here.

A patch answers `200 {"id", "version", "rendition", "op", "index", "anchor", "url"}`. Refusals come back as `{"error"}` with one of these statuses:

- **409 with `version`**: the page moved past `base_version`; read it again.
- **409 with `indices`**: the anchor names several sections, or the section would repeat another section's anchor. Address one as `index:N` or rename the heading.
- **409 alone**: the rendition has no Doc source, such as a brief stored as raw HTML or a page without a deck.
- **404**: no such page, or no section with that anchor (the error lists the anchors).
- **400**: the patch or its section is malformed.
- **413**: the body is over the route's cap, or on a shared instance the page would grow past its size cap.
- **415**: on a local present, the request is not `Content-Type: application/json`.
- **403**: on a local present, the request came from a page on another site. On a shared instance, the author key is not the page's. A shared instance answers 401 when no author key arrived.

```bash
curl -sS -X POST http://localhost:7423/api/p/<id>/sections \
  -H 'Content-Type: application/json' \
  -d '{"body_format":"markdown","section":"## Results\n\nAll green."}'
```

A progress report finds the progress block whose label matches, in the named `section` or anywhere in the Doc. It sets that block's `pct` (0 to 100) and its `note`, a short line shown after the label. With no match the block is added to that section, or to a Progress section at the end, created the first time. A label two blocks share answers 409 with their `sections`. The route takes the guards and statuses of the section routes. It answers `200 {"id", "version", "label", "pct", "section", "anchor", "inserted", "url"}`.

A report the block already shows writes nothing and answers with the current version and `"unchanged": true`. Every other report is a new version with its own history record. A hook that reports on a timer with a moving value therefore pushes older versions out of the 20 kept, so report when the value changes. `present progress <id> <label> <pct> [--note ...] [--section ...] [--deck]` makes the same edit against the workdir under the page lock. It works whether serve is running or not, which suits hooks. `--deck` reports into the deck instead of the brief.

A local page keeps its newest 20 versions under `history/`. `present_source` takes a `version` to read one back, and passing that source to `present_update` restores it as a new version. The diff answers `{"content": {"changed", "added", "removed"}, "deck": {...}}`, naming each section by its anchor, or `index:N` when its anchor is empty or shared. The page view uses it to mark the sections that changed since this browser last opened the page. A version older than the kept window is 404. A shared instance keeps no history, so these routes are missing there.

## MCP

On a standalone install, register it once:

```bash
claude mcp add --scope user present -- present mcp
```

On a ralph-managed machine, skip the manual command. MCP registration is machine-private wiring that ships from the consuming repo's companion recipe ([docs/adr/0006](../../docs/adr/0006-recipe-layering-and-platform-deps.md)).

The tools write the page store directly, so they work with `present serve` down; only the `url` a tool returns needs the server running to actually open in a browser.

| Tool | Purpose |
|------|---------|
| `present_create(title, content?, deck?, graph?, references?)` | Create a page; at least one of `content` and `deck`; returns `{id, url, has_deck, deck_url?, version}` |
| `present_read(id)` | Read a page's rendered title/content/deck/graph/version/urls |
| `present_source(id, version?)` | Get the editable source (Doc JSON, deck Doc JSON, graph JSON) in the format `present_update` accepts; use to mutate a page from a new session. `version` reads a kept earlier version (local pages keep 20) |
| `present_update(id, title?, content?, deck?, graph?, references?)` | Patch a page (omitted fields unchanged; `deck: ""` removes the deck); bumps version → open tabs auto-reload |
| `present_patch_section(id, op, anchor?, section?, body_format?, rendition?, base_version?)` | Change one section without resending the Doc: `replace`, `insert_before`, `insert_after`, or `remove` at an anchor, or `append`; the section as Doc JSON or markdown |
| `present_list()` | List all pages, newest first; `has_doc` marks pages with an editable Doc source, `has_brief`/`has_deck` say which renditions exist |
| `present_open(id, deck?)` | Open a page in the browser (call once per page); `deck: true` opens the slide deck |
| `present_deck(id, action, slide?)` | Drive the open deck: `start`, `stop`, `next`, `prev`, `goto`. Local instance only |
| `present_export(id, deck?)` | Write the page (or its deck) as one standalone HTML file under `exports/` in the page store; returns `{path, bytes, rendition, libraries, notes}`. Local instance only |
| `present_share(id, ephemeral?)` | Push a page to the configured shared instance; returns `{url, ephemeral, expires_at, shared_at}`. Registered only when `--shared-url` and `--author-key` are set |
| `present_doctor()` | Run the `present doctor` checks and return the report |

Confirm the registration with `claude mcp list`, and run `present doctor` for a full check of the store, the server, and version skew.

A shared instance is registered as an HTTP server instead, with the author key as a bearer token. Claude Code:

```bash
claude mcp add --transport http present-shared https://present.example.com/mcp \
  --header "Authorization: Bearer <your author key>"
```

Codex, in `config.toml`:

```toml
[mcp_servers.present-shared]
url = "https://present.example.com/mcp"
bearer_token_env_var = "PRESENT_AUTHOR_KEY"
```

The instance's root page shows both forms filled in with its own hostname.

### Deploying a shared instance

`services/present/Dockerfile` builds `ghcr.io/mad01/present` (linux/amd64 and arm64; releases push `X.Y.Z` and `latest`, main pushes `main` and `sha-<short>`). `deploy/base` is a kustomize base: the Page custom resource definition (CRD), the role-based access control (RBAC) objects the pods run under, a two-replica Deployment, a Service, and a PodDisruptionBudget. `deploy/overlays/ingress` and `deploy/overlays/istio` show how to expose it, with the hostname in a single ConfigMap value your own overlay replaces. [deploy/README.md](deploy/README.md) is the operator guide for a real cluster; [docs/RELEASING.md](../../docs/RELEASING.md) covers the image tags and how to verify the cosign signature. To try it against a kind cluster:

```bash
make -C services/present kind-test   # build, load, apply deploy/overlays/kind, run the cluster tests
```

## Where things live

```
~/.config/present/
  pages/<id>/
    meta.json            # id, title, version, has_brief, has_deck, timestamps
    content.html         # rendered body fragment of the brief
    doc.json             # canonical Doc source (when created from Doc JSON)
    deck.html            # rendered fragment of the slide deck (when the page has one)
    deck.json            # canonical deck Doc source
    deck-command.json    # the last 32 remote commands for open deck tabs
    graph.js             # optional cytoscape init script
    graph.json           # canonical graph source (when created from graph JSON)
    history/<version>.json  # the newest 20 versions, what present_source returns for each
  images/<sha256>.<ext>  # image files brought in from this machine, served at /img/<name>
  exports/<id>.html      # files present_export wrote (<id>-deck.html for a deck)
  terminal.enabled       # the live terminal switch, present only while `present terminal on` holds
```

`present rerender [id...]` re-renders pages, decks included, from their stored
sources to pick up renderer/template changes (e.g. after a webkit bump); pages
without sources get a deterministic legacy-HTML upgrade instead. A local
`present serve` runs the same sweep in the background at startup, so after an
upgrade the restart alone brings every page current.

## Develop

```bash
make test    # go test ./..., then the viz.js node tests (skipped without node)
make build   # ./present
make tidy    # go mod tidy
```

Chrome. Pages render client-side: `present serve` serves an embedded
chrome-only shell and the browser fetches each page as JSON and builds the DOM.
There is no on-disk template to edit.

Chrome (header, theme toggle, font/size/fixation controls) comes from the
in-module `webkit` package (served at `GET /webkit/`). Don't re-add those
controls locally. A webkit change ships at the next build, and the restarted
`present serve` re-renders all pages through the updated renderer on its own.

MCP + sandbox. The MCP server runs under anthropics/sandbox-runtime (srt),
launched by `recipes/present/present-mcp-sandbox.sh` through `srt-run` with
`~/.config/srt/present.json`, a settings file the consuming repo provides.
Egress is a hostname allow-list: the local events port, plus the shared
instance's host for `present_share`. `$HOME` reads are denied except
`~/code/bin` and the page store, and writes are confined to the page store
and srt's temp. The wrapper reads `PRESENT_SHARED_URL` and
`PRESENT_AUTHOR_KEY` from the ralph-managed secrets file, so `present_share`
works when the shared URL's host is on the allow-list and fails with a
connection error otherwise. The Share button and `present share` run outside
the sandbox and take any URL. If any other MCP tool fails, check sandbox
denials:

```bash
t-man logs sandbox
```

See `recipes/speak/CLAUDE.md` → "Triaging a denial" for the triage steps.

Codesign. `make install` signs with the stable local identity (`mad01 Local
Signing`, docs/adr/0004) and falls back to ad-hoc when the cert is absent.
macOS kills a binary whose provenance xattrs went stale, so after a manual
copy run `make resign BIN=~/code/bin/present`.

Debugging. Both processes log their resolved `workdir=… port=…` at
startup. If updates don't appear, compare those values first:

```bash
t-man logs present             # stdout
t-man logs present --stderr    # one line per request: method path -> status bytes (dur)
```

If the MCP writes succeed but pages don't render, the HTTP server is likely
not running: `t-man status present`.

See [CLAUDE.md](CLAUDE.md) for architecture and debugging details.

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
