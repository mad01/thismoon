# deps, supply-chain dependency scanner (CLI + MCP + web service)

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, over one
JSON store. `deps` discovers every external dependency across the catalog-
registered repos (Go modules, npm), checks each version against the OSV.dev
advisory database, surfaces findings at `http://deps.this/`, and fires a macOS
notification when a package is flagged. **The MCP tools (driven by Claude) and
the web page are the primary surfaces**; the `deps` CLI mirrors them.

## Module layout

```
deps/
  cmd/deps/            - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, scan, check (+--repo), resolve, notify, mcp, config, version
    config/            - discovery config (exclude_repos / exclude_paths), ~/.config/deps/config.toml
    registry/          - reads the catalog registry.yaml → repo roots to scan
    discover/          - Ecosystem interface + Go (`go list -m -json all`) + npm (package-lock) + Python (requirements.txt exact pins) + Swift (Package.resolved); shared walker skips worktrees/nested checkouts
    osv/               - OSV.dev client (POST /v1/querybatch + GET /v1/vulns/{id})
    store/             - Dependency/Advisory/Flag model + atomic JSON store; notified + resolved sets
    scanner/           - Engine: discover → check → persist; CheckRepo (per-repo merge); coalesced Notify
    notify/            - Notifier interface + osascript macOS notification
    api/               - wire DTOs shared by server+client (advisory key + resolved status)
    server/            - HTTP API + webkit web page (embedded index.html)
    client/            - HTTP client for serve (used by CLI + MCP)
    mcpserver/         - MCP tools (thin client over the API)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

Like `reminder`, **`deps serve` is the only writer**:

- **`deps serve`** owns the store (`~/.local/share/deps/scan.json`), runs the
  HTTP server (web + JSON API + `/version` + `/webkit/`), the daily catch-up
  scan loop, and fires notifications. It is the only process that reaches the
  network (OSV).
- **`deps mcp`** and the **`deps` CLI** hold no state: they are thin HTTP
  clients to the serve API on `localhost:<port>`. If serve is down, they return
  "deps serve not reachable … (t-man status deps)".

Every reader (MCP, CLI, web page, scan loop) therefore sees the same data, with
no file locks.

### Discovery

- Repo set = the catalog registry (`~/.config/catalog/registry.yaml`), trimmed by
  `exclude_repos` in the deps config. New catalogued repos auto-enroll.
- Each repo is walked for `go.mod` (→ `go list -m -json all`, resolved graph,
  v-prefix stripped for OSV), `package-lock.json` (lockfile v2/v3), and
  `requirements.txt` (exact `name==version` pins only — unpinned names and
  ranges have no OSV-checkable version and are skipped; names are PEP 503
  normalized), and `Package.resolved` (v2/v3, remote pins with an exact
  version — branch pins are skipped). The shared walker skips `.git/vendor/node_modules/testdata`, **git worktrees and nested
  checkouts** (a `.git` entry in a subdir), and any `exclude_paths` glob.
- Transitive deps are scanned and flagged (real supply-chain risk); the web/CLI
  mark direct vs transitive.
- **Import-graph reachability (Go).** `go list -m -json all` is the full *module
  graph*, including modules no binary imports, so OSV flags advisories on
  packages that aren't compiled in (false positives). For each Go module dir,
  `go list -deps -json ./...` gives the *import graph*; a dep is tagged
  `Imported` only if it contributes a compiled package (matched on original or
  replacement path). A flagged-but-not-imported dep is **graph-only**: it doesn't
  count as active, never notifies, and the web page shows it in a separate dimmed
  "Not compiled in" section. Reachability is module-level (not function-level
  like govulncheck) and resolves for the current GOOS/GOARCH only. It **fails
  open**: if `go list -deps` errors or finds nothing, every dep stays
  `Imported=true` so a real advisory is never hidden. npm/PyPI/Swift don't compute
  reachability at all, so they're always `Imported=true`.

### OSV check

- `POST /v1/querybatch` (≤1000/batch) to find advisory ids, then `GET
  /v1/vulns/{id}` once per unique id for summary/severity/fixed-version.
- One API; ecosystem strings are OSV's own (`Go`, `npm`, `PyPI`, `SwiftURL`); no
  mapping. SwiftURL names are the repo URL without scheme or `.git`
  (`github.com/vapor/vapor`), derived from each Package.resolved pin location.
- First-party code calling a public API → **no seatbelt** (like reminder/status).

### Scan loop, notifications, offline

- `serve` runs a heartbeat every 15m and a **full check whenever the last scan is
  older than `--interval` (default 24h)**. Checking staleness (not counting
  ticks) means a wake-from-sleep triggers the missed daily scan promptly.
- On failure (typically offline → OSV unreachable) the loop **backs off
  exponentially** up to 6h, resetting on the next success; no hammering an API
  we can't reach.
- Notifications are **coalesced**: one banner summarizing all newly-flagged
  advisories (a fresh scan can surface dozens), never one-per-advisory.
  Acknowledged (resolved) advisories don't notify.

## Data model & storage

```go
Dependency{ Ecosystem (Go|npm|PyPI|SwiftURL), Name, Version, Repo, ManifestPath, Direct, Advisories[] }
Advisory{ ID, Summary, Severity, FixedVersion }   // OSV advisory for that exact version
```

- **Scan store** holds the last scan's deps plus two string sets keyed by
  `FlagKey = ecosystem:name@version:advisoryID`: `notified` (already alerted) and
  `resolved` (user-acknowledged). Because the key includes the version, an
  acknowledged advisory **auto-resurfaces** once the package is bumped (the key
  changes); and a steady-state advisory neither re-notifies nor re-appears.
- **ScannedAt** is the last *full* scan time. A per-repo rescan
  (`SaveRepo`) deliberately does **not** reset it, so the daily catch-up still
  fires.

## Build / install / test

```bash
make build    # ./deps binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/deps + adhoc codesign
make test     # go test ./... (hermetic: t.TempDir, canned go-list/OSV/lock fixtures, fake notifier; no real network/go/osascript)
```

## HTTP API

- `GET  /`                       : webkit web page: rescan-all, per-repo rescan, per-finding resolve, acknowledged section
- `GET  /api/deps`              : full inventory (enriched: advisory `key` + `resolved`)
- `GET  /api/flagged`          : flagged deps only
- `POST /api/scan`             : discover only (no OSV), persist, return per-ecosystem counts
- `POST /api/check[?repo=]`    : discover + OSV check (full, or one repo merged), return flagged
- `POST /api/resolve`          : body `{keys:[…]}`; acknowledge advisories
- `POST /api/notify`           : fire pending notifications now
- `GET  /version`              : the four-key build metadata object (`version`, `commit`, `tag`, `build_time`) · `GET /webkit/`

## Commands

CLI surface beyond `serve`/`mcp`: thin HTTP clients to `deps serve`, one call
per HTTP endpoint above:

```bash
deps scan                    # POST /api/scan - discover only, print per-ecosystem counts
deps check [--repo <repo>]   # POST /api/check[?repo=] - discover + OSV, print flagged
deps resolve <key>...        # POST /api/resolve - acknowledge advisories by key
deps notify                  # POST /api/notify - fire pending notifications now
deps version [-o json]       # bare git sha; -o json prints the full build metadata object
```

`deps config` is the exception: it reads the local discovery config directly (no
serve needed), printing the resolved path with its status (`loaded` / `missing,
defaults in use` / `parse error: <err>`) and the effective exclude lists as TOML.
The annotated key reference lives in `deps config --help`.

## MCP tools

- `deps_scan`: discover all deps (no advisory check); per-ecosystem counts
- `deps_check`: discover + OSV; return flagged (each advisory has `key`, `resolved`)
- `deps_list_flagged`: flagged from the last check, no re-scan
- `deps_scan_repo(repo)`: rescan one repo (path or basename) and merge
- `deps_resolve(keys)`: acknowledge advisories by key

## Shared UI: webkit

The chrome (header, theme toggle, font/size/bionic controls, toast host) comes
from the in-module package **`github.com/mad01/thismoon/webkit`**, which embeds
compiled TypeScript/CSS web components. Don't re-add palette, topbar, or theme
CSS locally; those live in webkit only. There is no pin or bump step: the
binary compiles against the webkit committed alongside it.

### How webkit is mounted

```go
import "github.com/mad01/thismoon/webkit"
webkit.Mount(mux) // mux.Handle("GET /webkit/", webkit.Handler()) - serves webkit.css/.js + /webkit/boot.js + /webkit/version
```

`internal/server/server.go`'s `Handler()` calls `webkit.Mount(mux)` alongside
the deps routes. `handleIndex` writes the embedded `shell.html` with
`webkit.NoCacheHTML(w)` so a webkit or shell change is picked up on next load.

The shell loads the FOUC guard (served by webkit at `/webkit/boot.js`) and the
webkit assets:

```html
<head>
  <script src="/webkit/boot.js"></script>
  <link href="/webkit/webkit.css" rel="stylesheet">
</head>
```

The boot script is blocking (no `defer`/`async`) on purpose: it must set
`data-theme` before first paint to avoid a flash of the wrong theme.

### Header markup (deps)

```html
<wk-header brand="deps" title="Dependencies"></wk-header>
```

`webkit.js` injects the full control set (font · bionic · size ± · reload ·
theme). Don't add those controls manually.

### Per-repo changes

- Header and shell markup live in `internal/server/shell.html`, a static
  chrome-only shell embedded via `//go:embed`. The page body renders
  client-side: `/app.js` (also embedded) fetches `GET /api/deps` and builds the
  DOM in the browser with `Webkit.el`/`Webkit.escapeHtml`/`Webkit.poll`
  (`Webkit.poll('/api/deps', render, 60000, …)` drives the 60s live refresh).
- Components used: `<wk-header>`, `<wk-toast-host>` + `document.createElement('wk-toast')`
  for transient toasts, `<wk-page-header>` (+ `<wk-title>`/`<wk-subtitle>`),
  `<wk-card>`, `<wk-badge>`, `<wk-button>`.
- Only deps-specific layout stays local in `shell.html`'s inline `<style>`:
  the toolbar, repo chips, the inventory search row, the flagged/acknowledged
  card internals (`.flag-*`, `.adv-*`, `.repo-chip`, `.inv-*`, `.ack-*`).
  Palette, header chrome, page-header, card, and badge visuals all come from
  `/webkit/webkit.css`.
- deps has no theme-reactive canvas (no graph or chart), so unlike `present` it
  doesn't listen for the `wk-themechange` document event.

### Version check

`GET /webkit/version` → `{"module":"github.com/mad01/thismoon/webkit","version":"<asset hash>"}`:
confirms which embedded webkit assets the running deps server serves.

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe (wave 1) registers the MCP.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `deps docs`): serve-must-be-running, store layout, phantom
  "Not compiled in" advisories, version-scoped resolve resurfacing,
  version-skew checks. Keep those facts there, not here.
- **Personal Mac only**: recipe, MCP entry, and d-man route are all
  `profiles = ["personal"]`.
- **Codesign for the binary.** `make install` strips xattrs and re-signs.

## See also

- Recipe: `recipes/deps/recipe.toml` (+ `recipes/deps/CLAUDE.md`)
- Route: the consuming repo's `recipes/d-man/routes.toml` overlay (`deps` → 7429; docs/adr/0006)
- Config: `recipes/deps/config.toml` → `~/.config/deps/config.toml`
- MCP registration: the consuming repo's `recipes/claude-mcp/servers.json`
- Repo list source of truth: `recipes/catalog/registry.yaml`
