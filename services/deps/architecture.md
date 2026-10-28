# deps architecture

## Overview

deps is the supply-chain scanner service: it discovers every external
dependency across the catalog-registered repos and the tools mise installs,
and checks each pinned version against the OSV.dev advisory database and the
registry that publishes it. At
runtime `deps serve --port 7429`
(loopback only, fronted by d-man as `http://deps.this/`, run as a t-man agent)
is the single long-running process. It owns the scan store, runs the scan
loop, serves the web page and JSON API, and is the only process that reaches
the network (OSV, and github.com to resolve GitHub Actions refs). The `deps`
CLI and `deps mcp` hold no state; both are thin HTTP clients to the serve API
and report "serve not reachable" when it is down.

## Structure

```
cmd/deps/            entrypoint, delegates to internal/cli
internal/
  cli/               Cobra commands: serve, scan, check, resolve, notify, mcp, version
  config/            ~/.config/deps/config.toml (exclude_repos, exclude_paths,
                     [mise] lock and installs paths)
  registry/          reads the catalog registry.yaml into the repo set to scan
  discover/          Ecosystem interface + Go, npm, Python, Swift, GitHub
                     Actions discoverers, the browser JavaScript discoverers
                     that report npm, and the shared walker that skips nested
                     checkouts; Mise reads the installed mise npm and pipx trees
  jsdelivr/          jsDelivr data API client (file lookup by sha256), cached
  osv/               OSV.dev client (querybatch, then per-id vuln fetch)
  regcheck/          registry checker: npm, PyPI, the Go module proxy
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
`requirements.txt` pins, `Package.resolved`, and the `uses:` refs in
`.github/workflows`, `.github/actions`, and any local action a `uses: ./path`
ref runs. A GitHub Action pinned to a tag or branch is resolved to its commit
from the action repo's ref list on github.com, fetched once per scan. It
carries a movable-ref signal. Its version is the label and the commit
(`v45.0.7+a284dc18…`), so a repointed tag reads as a new version. A ref like
`v4` takes its label from the full release tag on the same commit. A ref
that can't be resolved is left out and reported as a discovery error beside
the refs that were read. For Go it also computes
import-graph reachability with `go list -deps -json ./...`, tagging deps that
contribute no compiled Go package as graph-only; this fails open so a real
advisory is never hidden. Browser JavaScript outside any lockfile becomes npm
rows too. Shell scripts are read for `cdn.jsdelivr.net/npm/<name>@<version>`
URLs, with a `$VAR` version resolved only when the script always assigns it
the same literal, and comment lines ignored. Library files committed at any
depth below a `static` directory, `static/vendor/` included (named with a
version or ending in `.min.js`), are hashed and looked up on
`data.jsdelivr.com`. A match names the npm package and version. A file that
matches nothing takes both from its file name and carries the
`deps-vendored-modified` signal. Lookups are cached in memory, a match for
the life of the process and a miss for a day. A discoverer that fails for one
repo and ecosystem becomes a `DiscoveryError`, and the walk goes on with the
rest. Typical causes are `go list` without a `go` on PATH, or a jsDelivr
lookup that got no answer.

Beside the repos, discovery reads the tools mise installs with its npm: and
pipx: backends. `mise.lock` pins each tool's version, and that version's
install directory holds an `aube-lock.yaml` (npm) or a virtualenv's
`*.dist-info` folders (pipx). It reads those files and never runs mise. Each
tool reports under the repo label `mise:<tool>`, which a per-repo rescan takes
as one unit. Every mise row counts as compiled in, since reachability is
computed for Go alone. An unreadable lock is one `DiscoveryError` for every
tool, and an unreadable tree is one for its tool.

The check step posts versions to OSV's `/v1/querybatch` (up to 1000 per
batch, following `next_page_token` for paged results) and fetches each unique
advisory id once. A GitHub Action is looked up by its commit and by its
name, as written and in each other spelling OSV files it under. OSV matches
names case-sensitively, so the check first reads OSV's export of Actions
advisories from its data bucket. A failed read is a discovery error. A name hit stays
only when the version label falls in its range or can't be compared. A record
that fails to load keeps its finding with the id alone, so one bad fetch never
aborts the scan. The registry check then asks npm, PyPI or the Go module
proxy about each package once, with at most 8 at a time and 45 seconds for
the whole pass. A version the registry withdrew, quarantined or blocks is a
`malicious` finding, and a yanked, deprecated, archived or retracted one is a
`signal`. A failed registry lookup never fails the check: the dependency is
marked `registry_unknown` and keeps the registry findings it had, so an
outage reads as neither clean nor fixed. Each finding has a kind: `malicious`
for an OSV MAL- id or a registry block, `vuln` for any other advisory, and
`signal` for a warning sign a deps rule raises. The checkers' findings are
added to any a discoverer attached, never in place of them. `scanner.Engine`
persists the result and the discovery errors through the store. It then
fires a separate banner for newly flagged malicious packages,
then one coalesced notification covering the other newly flagged advisories.
Signals never notify. Last it emits a scan-complete event to the events
service, at warn level when anything is flagged, discovery failed, or a
registry lookup got no answer.

Scheduling checks staleness rather than counting ticks: serve heartbeats every
15 minutes and runs a full check whenever the last scan is older than
`--interval` (default 24h). So a wake from sleep catches up promptly. Failures
back off exponentially up to 6 hours. A manual check (`POST /api/check`, full
or per repo) notifies the same way, since a full one resets the scan time the
loop goes by. The notified set stops either path from firing a banner twice.
Resolution flows the other way: a
`POST /api/resolve` (from the web page, CLI, or `deps_resolve`) adds advisory
keys to the resolved set. Because the key embeds the exact version, an
acknowledged advisory resurfaces when the package is bumped. The web page asks
for confirmation before it resolves a malicious finding.

## Storage

One file: `~/.local/share/deps/scan.json`, a single JSON document written
atomically (temp file, then rename) and only ever by serve. It holds the last
scan's dependencies (ecosystem, name, version, repo, manifest path, direct
flag, `registry_unknown` flag, advisories with their kind), the `discovery_errors` (repo, ecosystem, error) the scan
hit, plus two string sets keyed by
`ecosystem:name@version:advisoryID`: `notified` (already alerted) and
`resolved` (user-acknowledged). An advisory with no kind, as in a file
written before kinds existed, takes its kind from its id: a MAL- id reads as
`malicious`, any other as `vuln`. `ScannedAt` records the last full scan; a
per-repo rescan merges, replacing only that repo's deps and discovery
errors, without resetting it, so the daily catch-up still fires. Config lives at `~/.config/deps/config.toml`.

## Interfaces

HTTP: `GET /` (web page), `GET /api/deps`, `GET /api/flagged`,
`POST /api/scan` (discover only), `POST /api/check[?repo=]`,
`POST /api/resolve`, `POST /api/notify`, `GET /version`, `GET /webkit/`. CLI:
`deps serve`, `scan`, `check [--repo]`, `resolve <key>...`, `notify`, `mcp`,
`version`, each a single call to the matching endpoint. `--repo` and
`deps_scan_repo` take a repo path or basename, or a mise label. MCP tools:
`deps_scan`, `deps_check`, `deps_list_flagged`, `deps_scan_repo`,
`deps_resolve`, and `deps_doctor`. The scan and check responses carry
`discovery_errors` alongside the findings, and the check and flagged ones
carry `registry_unknown`. Config surfaces: the deps config file above
(which also locates mise.lock and the mise installs directory), the catalog
registry as the repo source of truth, and the `DEPS_PORT` / `DEPS_BASE_URL`
environment variables mirrored by `--port` / `--base-url`.
