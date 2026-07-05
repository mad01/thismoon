# deps — supply-chain dependency scanner (CLI + MCP + web service)

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, over one
JSON store. `deps` discovers every external dependency across the catalog-
registered repos (Go modules, npm), checks each version against the OSV.dev
advisory database, surfaces findings at `http://deps.this/`, and fires a macOS
notification when a package is flagged. **The MCP tools (driven by Claude) and
the web page are the primary surfaces**; the `deps` CLI mirrors them.

## Single-writer architecture (load-bearing)

Like `reminder`, **`deps serve` is the only writer**:

- **`deps serve`** owns the store (`~/.local/share/deps/scan.json`), runs the
  HTTP server (web + JSON API + `/version` + `/webkit/`), the daily catch-up
  scan loop, and fires notifications. It is the only process that reaches the
  network (OSV).
- **`deps mcp`** and the **`deps` CLI** hold no state — they are thin HTTP
  clients to the serve API on `localhost:<port>`. If serve is down, they return
  "deps serve not reachable … (t-man status deps)".

So the MCP, the CLI, the web page, and the scan loop all see the same data, with
no file locks.

## Module layout

```
deps/
  cmd/deps/            — entrypoint (delegates to internal/cli)
  internal/
    cli/               — cobra: root, serve, scan, check (+--repo), resolve, notify, mcp, version
    config/            — discovery config (exclude_repos / exclude_paths), ~/.config/deps/config.toml
    registry/          — reads the catalog registry.yaml → repo roots to scan
    discover/          — Ecosystem interface + Go (`go list -m -json all`) + npm (package-lock) + Python (stub); shared walker skips worktrees/nested checkouts
    osv/               — OSV.dev client (POST /v1/querybatch + GET /v1/vulns/{id})
    store/             — Dependency/Advisory/Flag model + atomic JSON store; notified + resolved sets
    scanner/           — Engine: discover → check → persist; CheckRepo (per-repo merge); coalesced Notify
    notify/            — Notifier interface + osascript macOS notification
    api/               — wire DTOs shared by server+client (advisory key + resolved status)
    server/            — HTTP API + webkit web page (embedded index.html)
    client/            — HTTP client for serve (used by CLI + MCP)
    mcpserver/         — MCP tools (thin client over the API)
  Makefile             — part of module github.com/mad01/thismoon (no own go.mod)
```

## Data model

```go
Dependency{ Ecosystem (Go|npm|PyPI), Name, Version, Repo, ManifestPath, Direct, Advisories[] }
Advisory{ ID, Summary, Severity, FixedVersion }   // OSV advisory for that exact version
```

- **Scan store** holds the last scan's deps plus two string sets keyed by
  `FlagKey = ecosystem:name@version:advisoryID`: `notified` (already alerted) and
  `resolved` (user-acknowledged). Because the key includes the version, an
  acknowledged advisory **auto-resurfaces** once the package is bumped (the key
  changes) — and a steady-state advisory neither re-notifies nor re-appears.
- **ScannedAt** is the last *full* scan time. A per-repo rescan
  (`SaveRepo`) deliberately does **not** reset it, so the daily catch-up still
  fires.

## Discovery

- Repo set = the catalog registry (`~/.config/catalog/registry.yaml`), trimmed by
  `exclude_repos` in the deps config. New catalogued repos auto-enroll.
- Each repo is walked for `go.mod` (→ `go list -m -json all`, resolved graph,
  v-prefix stripped for OSV) and `package-lock.json` (lockfile v2/v3). The shared
  walker skips `.git/vendor/node_modules/testdata`, **git worktrees and nested
  checkouts** (a `.git` entry in a subdir), and any `exclude_paths` glob.
- Transitive deps are scanned and flagged (real supply-chain risk); the web/CLI
  mark direct vs transitive.
- **Import-graph reachability (Go).** `go list -m -json all` is the full *module
  graph* — it includes modules no binary imports, so OSV flags advisories on
  packages that aren't compiled in (false positives). For each Go module dir,
  `go list -deps -json ./...` gives the *import graph*; a dep is tagged
  `Imported` only if it contributes a compiled package (matched on original or
  replacement path). A flagged-but-not-imported dep is **graph-only**: it doesn't
  count as active, never notifies, and the web page shows it in a separate dimmed
  "Not compiled in" section. Reachability is module-level (not function-level
  like govulncheck) and resolves for the current GOOS/GOARCH only. It **fails
  open**: if `go list -deps` errors or finds nothing, every dep stays
  `Imported=true` so a real advisory is never hidden. npm/PyPI don't compute
  reachability — they're always `Imported=true`.

## OSV check

- `POST /v1/querybatch` (≤1000/batch) to find advisory ids, then `GET
  /v1/vulns/{id}` once per unique id for summary/severity/fixed-version.
- One API; ecosystem strings are OSV's own (`Go`, `npm`, `PyPI`) — no mapping.
- First-party code calling a public API → **no seatbelt** (like reminder/status).

## Scan loop, notifications, offline

- `serve` runs a heartbeat every 15m and a **full check whenever the last scan is
  older than `--interval` (default 24h)**. Checking staleness (not counting
  ticks) means a wake-from-sleep triggers the missed daily scan promptly.
- On failure (typically offline → OSV unreachable) the loop **backs off
  exponentially** up to 6h, resetting on the next success — no hammering an API
  we can't reach.
- Notifications are **coalesced**: one banner summarizing all newly-flagged
  advisories (a fresh scan can surface dozens), never one-per-advisory.
  Acknowledged (resolved) advisories don't notify.

## HTTP API (owned by serve)

- `GET  /`                       — webkit web page: rescan-all, per-repo rescan, per-finding resolve, acknowledged section
- `GET  /api/deps`              — full inventory (enriched: advisory `key` + `resolved`)
- `GET  /api/flagged`          — flagged deps only
- `POST /api/scan`             — discover only (no OSV), persist, return per-ecosystem counts
- `POST /api/check[?repo=]`    — discover + OSV check (full, or one repo merged), return flagged
- `POST /api/resolve`          — body `{keys:[…]}`; acknowledge advisories
- `POST /api/notify`           — fire pending notifications now
- `GET  /version` · `GET /webkit/`

## MCP tools (thin client over the API)

- `deps_scan` — discover all deps (no advisory check); per-ecosystem counts
- `deps_check` — discover + OSV; return flagged (each advisory has `key`, `resolved`)
- `deps_list_flagged` — flagged from the last check, no re-scan
- `deps_scan_repo(repo)` — rescan one repo (path or basename) and merge
- `deps_resolve(keys)` — acknowledge advisories by key

## Build / install / test

```bash
make build    # ./deps binary (Version via ldflags)
make install  # build + cp to ~/code/bin/deps + adhoc codesign
make test     # go test ./... (hermetic: t.TempDir, canned go-list/OSV/lock fixtures, fake notifier; no real network/go/osascript)
```

## Gotchas

- **Wave 0 builder.** Builds before `recipes/claude-mcp` (wave 1) registers the MCP.
- **serve must be running for the MCP/CLI to work** — it owns the store and is
  the only one that reaches OSV. `t-man status deps` / `t-man restart deps`.
- **Personal Mac only** — recipe, MCP entry, and d-man route are all
  `profiles = ["personal"]`.
- **Resolve is version-scoped** — acknowledging an advisory hides it until the
  package version changes; it intentionally resolves the same advisory across
  every repo that pins that exact version.
- **Codesign for the binary.** `make install` strips xattrs and re-signs.

## See also

- Recipe: `recipes/deps/recipe.toml` (+ `recipes/deps/CLAUDE.md`)
- Route: `recipes/d-man/routes.toml` (`deps` → 7429)
- Config: `recipes/deps/config.toml` → `~/.config/deps/config.toml`
- MCP registration: `recipes/claude-mcp/servers.json`
- Repo list source of truth: `recipes/catalog/registry.yaml`
