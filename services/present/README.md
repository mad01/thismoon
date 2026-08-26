# present

A small CLI + MCP server for serving single-page HTML briefing pages over localhost.

Presentations are managed as **create / read / update / list**; there is no delete, so pages stick around. Each page stores only its HTML body; a shared **core template** supplies the chrome (styling, controls, scripts) and is injected when the page is served. Editing the template re-renders every page, old and new. An injected live-reload script refreshes any open browser tab after an update, so you open the page once and edits stream in.

## Install

```bash
make install   # builds and installs ~/code/bin/present (adhoc codesigned on macOS)
```

## Usage

Two cooperating processes share a working directory (`~/.config/present` by default) and a port (`7423`):

```bash
present serve              # HTTP server: GET / lists pages, /p/<id> renders one
present mcp                # MCP stdio server exposing present_* tools to Claude Code
```

Typically `present serve` runs as a background launchd agent (via t-man); Claude Code launches `present mcp`.

```bash
present version           # bare git sha the binary was built from
present version -o json   # build metadata: version, commit, tag, build_time - probed by `ralph outdated`
```

| Flag | Env | Default |
|------|-----|---------|
| `--workdir` | `PRESENT_WORKDIR` | `~/.config/present` |
| `--port` | `PRESENT_PORT` | `7423` |

## MCP

| Tool | Purpose |
|------|---------|
| `present_create(title, content, graph?)` | Create a page; returns `{id, url, version, server_running}` |
| `present_read(id)` | Read a page's rendered title/content/graph/version/url |
| `present_source(id)` | Get the editable source (Doc JSON + graph JSON) in the format `present_update` accepts; use to mutate a page from a new session |
| `present_update(id, title?, content?, graph?)` | Patch a page (omitted fields unchanged); bumps version → open tabs auto-reload |
| `present_list()` | List all pages, newest first; `has_doc` marks pages with an editable Doc source |
| `present_open(id)` | Open a page in the browser (call once per page) |

## Where things live

```
~/.config/present/
  pages/<id>/
    meta.json            # id, title, version, timestamps
    content.html         # rendered body fragment
    doc.json             # canonical Doc source (when created from Doc JSON)
    graph.js             # optional cytoscape init script
    graph.json           # canonical graph source (when created from graph JSON)
```

`present rerender [id...]` re-renders pages from their stored sources to pick
up renderer/template changes (e.g. after a webkit bump); pages without sources
get a deterministic legacy-HTML upgrade instead.

## Develop

```bash
make test    # go test ./...
make build   # ./present
make tidy    # go mod tidy
```

**Chrome.** Pages render client-side: `present serve` serves an embedded
chrome-only shell and the browser fetches each page as JSON and builds the DOM.
There is no on-disk template to edit.

Chrome (header, theme toggle, font/size/bionic controls) comes from the
in-module `webkit` package (served at `GET /webkit/`). Do not re-add those
controls locally. A webkit change ships at the next build; run
`present rerender` afterwards to re-render all pages through the updated
renderer.

**MCP + sandbox.** The MCP server runs inside a seatbelt profile
(`recipes/present/present.sb`): no network at all, `$HOME` reads
default-denied except `~/code/bin` and `~/.config/present`. Writes are
confined to `~/.config/present` and temp. The sandbox should not affect
normal page operations; if an MCP tool fails, check sandbox denials:

```bash
t-man logs sandbox
```

See `recipes/speak/CLAUDE.md` → "Triaging a denial" for the triage steps.

**Codesign.** macOS kills adhoc-signed binaries with stale provenance xattrs.
After a manual copy: `make resign BIN=~/code/bin/present`. `make install`
handles this automatically.

**Debugging.** Both processes log their resolved `workdir=… port=…` at
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
