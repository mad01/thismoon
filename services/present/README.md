# present

A small CLI + MCP server for serving single-page HTML briefing pages over localhost, and the same binary run as a shared instance others reach by link. A page can also carry a slide deck beside its brief, or instead of it, for the same material talked through in a room.

Pages are created, read, updated, and listed through the MCP tools; deleting one is a button in the web index rather than a tool. Each page keeps its rendered HTML body next to the Doc JSON it was rendered from. The browser does the assembly: `present serve` returns a chrome-only shell, fetches the page as JSON, and builds the DOM, so there is no template on disk to edit. An open tab reloads itself after an update: a shared instance on the cluster store pushes the new version over server-sent events, and everywhere else the tab polls for it.

## Install

```bash
make install   # builds and installs ~/code/bin/present (adhoc codesigned on macOS)
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

A slide or a section can use four more blocks beside the ones a brief has. `columns` holds two or three equal columns of blocks and drops to one column on a narrow window. `stat` is a large figure over a label. `quote` carries its attribution. `details` is a collapsible block, closed until opened, which read-aloud opens as it reads it. Callouts come in `info`, `warn`, `ok`, and `error`.

Keys: Right, Space, or PageDown for the next slide; Left, PageUp, or Backspace for the previous one; Home and End for the first and last. F or P starts presenting: the header and the bar go away, one slide fills the window at a larger size, and the browser is asked for fullscreen. Escape ends it, as does leaving fullscreen through the browser, and so does F or P pressed in fullscreen. Pressed while presenting without fullscreen, which is where a reload leaves you, F or P asks for fullscreen again. A reload, the one an update triggers included, comes back on the same slide in the same mode. The tab remembers where it was, and a fresh tab starts at the title slide.

Read-aloud works on a deck the way it does on a brief: the bar sits on the title slide and every slide carries its own play control.

Sharing a page that has a deck to a shared instance built before decks keeps the copy and its link but reports that the deck was dropped. Upgrade the instance and share again.

The deck can be driven from outside the tab too. `present deck <id> start|stop|next|prev|goto <n>` and the `present_deck` tool write the page's last command into its directory, and every open tab of the deck follows within a second. Tabs opened later ignore commands sent before they loaded. This works on the local instance only; a shared instance serves decks but takes no remote commands.

Sharing carries the deck: a shared copy has the same renditions as the local page, opens on its brief or its deck the same way, and answers the same keys.

### Shared mode

`present serve --shared --bind 0.0.0.0` runs the same binary as a network-facing shared instance, meant for a few replicas in Kubernetes behind one hostname. There is no index and no listing: a page is reachable only by the 32-hex id present minted for it. Every write carries an author key as a bearer token (the server keeps only its hash, and only that key can update or delete the page). Reads need nothing, and a page created as ephemeral disappears 30 days after its last write. The root serves a how-to page, `POST /api/pages` and `PUT /api/p/<id>` take pushed pages, and the present tools are served over HTTP at `/mcp` (`present_create` with an `ephemeral` flag, `present_read`, `present_source`, `present_update`, `present_doctor`). Page URLs follow the `X-Forwarded-Host` and `X-Forwarded-Proto` headers the ingress sets.

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

### Importing a markdown file

The index page (`GET /`) has an "Import markdown" button, and a `.md`, `.markdown`, or `.txt` file dropped anywhere on that page does the same. The browser reads it and posts it to `POST /api/import`, the server converts it to a page, and you land on the new page. Headings become the title and sections, and paragraphs, lists, code blocks, tables, and blockquotes map to their present blocks. Raw HTML, horizontal rules, and footnotes are dropped, and nested lists are flattened. The first fenced `mermaid` flowchart becomes the page's graph (later ones, and other mermaid diagram types, stay code blocks).

A file is refused when the page it renders to (the HTML plus the Doc source) would pass 1 MiB. That is the cap a shared instance enforces, so an imported page can always be shared. It works out to about 250 to 500 KiB of markdown, less for markup-heavy files.

The same endpoint works from a shell on the local instance:

```bash
jq -Rs '{name: "notes.md", markdown: .}' notes.md \
  | curl -sS -X POST http://localhost:7423/api/import -H 'Content-Type: application/json' -d @-
```

The import exists in local mode only; a shared instance takes pages through the MCP tools or `POST /api/pages` with an author key.

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
| `present_source(id)` | Get the editable source (Doc JSON, deck Doc JSON, graph JSON) in the format `present_update` accepts; use to mutate a page from a new session |
| `present_update(id, title?, content?, deck?, graph?, references?)` | Patch a page (omitted fields unchanged; `deck: ""` removes the deck); bumps version → open tabs auto-reload |
| `present_list()` | List all pages, newest first; `has_doc` marks pages with an editable Doc source, `has_brief`/`has_deck` say which renditions exist |
| `present_open(id, deck?)` | Open a page in the browser (call once per page); `deck: true` opens the slide deck |
| `present_deck(id, action, slide?)` | Drive the open deck: `start`, `stop`, `next`, `prev`, `goto`. Local instance only |
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
```

`present rerender [id...]` re-renders pages, decks included, from their stored
sources to pick up renderer/template changes (e.g. after a webkit bump); pages
without sources get a deterministic legacy-HTML upgrade instead. A local
`present serve` runs the same sweep in the background at startup, so after an
upgrade the restart alone brings every page current.

## Develop

```bash
make test    # go test ./...
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

MCP + sandbox. The MCP server runs inside a seatbelt profile
(`recipes/present/present.sb`): outbound HTTPS and DNS only (for
`present_share`) plus the local events port, `$HOME` reads default-denied
except `~/code/bin` and `~/.config/present`, writes confined to
`~/.config/present` and temp. The wrapper reads `PRESENT_SHARED_URL` and
`PRESENT_AUTHOR_KEY` from the ralph-managed secrets file, so `present_share`
works with an `https://` shared URL and fails with a connection error on a
plain-http one. The Share button and `present share` run outside the
sandbox and take any URL. If any other MCP tool fails, check sandbox
denials:

```bash
t-man logs sandbox
```

See `recipes/speak/CLAUDE.md` → "Triaging a denial" for the triage steps.

Codesign. macOS kills adhoc-signed binaries with stale provenance xattrs.
After a manual copy: `make resign BIN=~/code/bin/present`. `make install`
handles this automatically.

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
