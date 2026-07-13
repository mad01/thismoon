# deps architecture

## Overview

deps is the supply-chain scanner service: it discovers every external
dependency across the catalog-registered repos and checks each pinned version
against the OSV.dev advisory database. At runtime `deps serve --port 7429`
(loopback only, fronted by d-man as `http://deps.this/`, run as a t-man agent)
is the single long-running process: it owns the scan store, runs the scan
loop, serves the web page and JSON API, and is the only process that reaches
the network. The `deps` CLI and `deps mcp` hold no state; both are thin HTTP
clients to the serve API and report "serve not reachable" when it is down.

## Structure

```
cmd/deps/            entrypoint, delegates to internal/cli
internal/
  cli/               Cobra commands: serve, scan, check, resolve, notify, mcp, version
  config/            ~/.config/deps/config.toml (exclude_repos, exclude_paths)
  registry/          reads the catalog registry.yaml into the repo set to scan
  discover/          Ecosystem interface + Go, npm, Python, Swift discoverers
                     and the shared walker that skips nested checkouts
  osv/               OSV.dev client (querybatch, then per-id vuln fetch)
  scanner/           Engine: discover, check, persist, coalesced notify
  notify/            Notifier interface, osascript banner, event emission
  store/             Dependency/Advisory/Flag model, atomic JSON store
  api/               wire DTOs shared by server and client
  server/            HTTP API + webkit web page (embedded shell.html + app.js)
  client/            HTTP client used by the CLI and mcpserver
  mcpserver/         MCP stdio tools, thin client over the API
```

`internal/server` mounts the shared chrome with `webkit.Mount(mux)` and serves
a chrome-only `shell.html`; the page body renders client-side from
`GET /api/deps` per docs/adr/0005, with only deps-specific layout kept local.

## Data flow

A scan cycle runs discover, check, persist, notify. Discovery reads the
catalog registry, trims it by `exclude_repos`, and walks each repo for
`go.mod` (resolved with `go list -m -json all`), `package-lock.json`, exact
`requirements.txt` pins, and `Package.resolved`. For Go it also computes
import-graph reachability with `go list -deps -json ./...`, tagging deps that
contribute no compiled Go package as graph-only; this fails open so a real
advisory is never hidden. The check step posts versions to OSV's
`/v1/querybatch` (up to 1000 per batch) and fetches each unique advisory id
once. `scanner.Engine` persists the result through the store, then fires one
coalesced notification covering all newly flagged advisories and emits an
event to the events service.

Scheduling checks staleness rather than counting ticks: serve heartbeats every
15 minutes and runs a full check whenever the last scan is older than
`--interval` (default 24h), so a wake from sleep catches up promptly. Failures
back off exponentially up to 6 hours. Resolution flows the other way: a
`POST /api/resolve` (from the web page, CLI, or `deps_resolve`) adds advisory
keys to the resolved set; because the key embeds the exact version, an
acknowledged advisory resurfaces when the package is bumped.

## Storage

One file: `~/.local/share/deps/scan.json`, a single JSON document written
atomically (temp file, then rename) and only ever by serve. It holds the last
scan's dependencies (ecosystem, name, version, repo, manifest path, direct
flag, advisories) plus two string sets keyed by
`ecosystem:name@version:advisoryID`: `notified` (already alerted) and
`resolved` (user-acknowledged). `ScannedAt` records the last full scan; a
per-repo rescan merges without resetting it, so the daily catch-up still
fires. Config lives at `~/.config/deps/config.toml`.

## Interfaces

HTTP: `GET /` (web page), `GET /api/deps`, `GET /api/flagged`,
`POST /api/scan` (discover only), `POST /api/check[?repo=]`,
`POST /api/resolve`, `POST /api/notify`, `GET /version`, `GET /webkit/`. CLI:
`deps serve`, `scan`, `check [--repo]`, `resolve <key>...`, `notify`, `mcp`,
`version`, each a single call to the matching endpoint. MCP tools:
`deps_scan`, `deps_check`, `deps_list_flagged`, `deps_scan_repo`, and
`deps_resolve`. Config surfaces: the deps config file above, the catalog
registry as the repo source of truth, and the `DEPS_PORT` / `DEPS_BASE_URL`
environment variables mirrored by `--port` / `--base-url`.
