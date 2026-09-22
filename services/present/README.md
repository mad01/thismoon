# present

A small CLI + MCP server for serving single-page HTML briefing pages over localhost, and the same binary run as a shared instance others reach by link.

Pages are created, read, updated, and listed through the MCP tools; deleting one is a button in the web index rather than a tool. Each page keeps its rendered HTML body next to the Doc JSON it was rendered from. The browser does the assembly: `present serve` returns a chrome-only shell, fetches the page as JSON, and builds the DOM, so there is no template on disk to edit. An open tab polls the page's version and reloads itself after an update.

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
```

| Flag | Env | Default |
|------|-----|---------|
| `--workdir` | `PRESENT_WORKDIR` | `~/.config/present` if present, else `~/.local/state/present` |
| `--port` | `PRESENT_PORT` | `7423` |
| `--bind` (serve only) | `PRESENT_BIND` | `127.0.0.1`; shared mode requires it explicitly |
| `--shared` (serve only) | `PRESENT_SHARED` | `false` |
| `--shared-url` | `PRESENT_SHARED_URL` | unset; sharing needs it and `--author-key` |
| `--author-key` | `PRESENT_AUTHOR_KEY` | unset; prefer the env var over the flag |

### Shared mode

`present serve --shared --bind 0.0.0.0` runs the same binary as a network-facing shared instance, meant for a few replicas in Kubernetes behind one hostname. There is no index and no listing: a page is reachable only by the 32-hex id present minted for it. Every write carries an author key as a bearer token (the server keeps only its hash, and only that key can update or delete the page), reads need nothing, and a page created as ephemeral disappears 30 days after its last write. The root serves a how-to page, `POST /api/pages` and `PUT /api/p/<id>` take pushed pages, and the present tools are served over HTTP at `/mcp` (`present_create` with an `ephemeral` flag, `present_read`, `present_source`, `present_update`, `present_doctor`). Page URLs follow the `X-Forwarded-Host` and `X-Forwarded-Proto` headers the ingress sets.

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

With both variables set, `present serve` shows a Share button in every page's header (a modal with the expiry checkbox, the link, and a Copy button; the button reads "Shared" once a copy exists) and `present mcp` registers a `present_share` tool. Sharing a page again replaces the copy under the same link. Only your author key can change or remove the copy; if you lose it, mint a new one and share each page again. `present doctor` checks the instance and the key once you have set both variables.

## MCP

On a standalone install, register it once:

```bash
claude mcp add --scope user present -- present mcp
```

On a ralph-managed machine, skip the manual command. MCP registration is machine-private wiring that ships from the consuming repo's companion recipe ([docs/adr/0006](../../docs/adr/0006-recipe-layering-and-platform-deps.md)).

The tools write the page store directly, so they work with `present serve` down; only the `url` a tool returns needs the server running to actually open in a browser.

| Tool | Purpose |
|------|---------|
| `present_create(title, content, graph?)` | Create a page; returns `{id, url, version, server_running}` |
| `present_read(id)` | Read a page's rendered title/content/graph/version/url |
| `present_source(id)` | Get the editable source (Doc JSON + graph JSON) in the format `present_update` accepts; use to mutate a page from a new session |
| `present_update(id, title?, content?, graph?)` | Patch a page (omitted fields unchanged); bumps version → open tabs auto-reload |
| `present_list()` | List all pages, newest first; `has_doc` marks pages with an editable Doc source |
| `present_open(id)` | Open a page in the browser (call once per page) |
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

Chrome. Pages render client-side: `present serve` serves an embedded
chrome-only shell and the browser fetches each page as JSON and builds the DOM.
There is no on-disk template to edit.

Chrome (header, theme toggle, font/size/fixation controls) comes from the
in-module `webkit` package (served at `GET /webkit/`). Don't re-add those
controls locally. A webkit change ships at the next build; run
`present rerender` afterwards to re-render all pages through the updated
renderer.

MCP + sandbox. The MCP server runs inside a seatbelt profile
(`recipes/present/present.sb`): outbound HTTPS and DNS only (for
`present_share`) plus the local events port, `$HOME` reads default-denied
except `~/code/bin` and `~/.config/present`, writes confined to
`~/.config/present` and temp. The wrapper reads `PRESENT_SHARED_URL` and
`PRESENT_AUTHOR_KEY` from the ralph-managed secrets file, so `present_share`
works with an `https://` shared URL and fails with a connection error on a
plain-http one; the Share button and `present share` run outside the
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
