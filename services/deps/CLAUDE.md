# deps, supply-chain dependency scanner (CLI + MCP + web service)

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, over one
JSON store. `deps` discovers every external dependency across the catalog-
registered repos (Go modules, npm, PyPI, Swift, GitHub Actions) and the npm
and PyPI trees of the tools mise installs, and checks each version against the
OSV.dev advisory database and against the registry that publishes it. It
surfaces findings at `http://deps.this/` and fires a macOS notification when a
package is flagged. **The MCP tools (driven by Claude) and the web page are
the primary surfaces**; the `deps` CLI mirrors them.

## Module layout

```
deps/
  cmd/deps/            - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, scan, check (+--repo), resolve, notify, mcp, config, version
    config/            - discovery config (exclude_repos / exclude_paths / [mise] lock, installs), ~/.config/deps/config.toml
    registry/          - reads the catalog registry.yaml → repo roots to scan
    discover/          - Ecosystem interface + Go (`go list -m -json all`) + npm (package-lock) + Python (requirements.txt exact pins) + Swift (Package.resolved) + browser JavaScript as npm (jsDelivr URLs in *.sh, vendored files by hash) + GitHub Actions (workflow `uses:` refs, tags resolved to commits); shared walker skips worktrees/nested checkouts; Mise reads the installed mise npm/pipx trees
    jsdelivr/          - jsDelivr data API client (GET /v1/lookup/hash/{sha256}), in-memory cache
    osv/               - OSV.dev client (POST /v1/querybatch + GET /v1/vulns/{id})
    regcheck/          - registry checker: npm, PyPI, Go module proxy (withdrawn, blocked, yanked, deprecated, retracted)
    store/             - Dependency/Advisory/Flag model + atomic JSON store; notified + resolved sets
    scanner/           - Engine: discover → check → persist; CheckRepo (per-repo merge); coalesced Notify
    alert/             - Notifier interface + osascript macOS notification (the
                         events archive is kit/notify, a different concern)
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
  network: OSV and the package registries for the check, jsDelivr when
  discovery names a vendored JavaScript file, and github.com for GitHub
  Actions refs.
- **`deps mcp`** and the **`deps` CLI** hold no state: they are thin HTTP
  clients to the serve API on `localhost:<port>`. If serve is down, they return
  "deps serve not reachable … (t-man status deps)".

Every reader (MCP, CLI, web page, scan loop) therefore sees the same data, with
no file locks.

### Discovery

- Repo set = the catalog registry (`~/.config/catalog/registry.yaml`, resolved
  through catalog's config dir so `XDG_CONFIG_HOME` moves both together),
  trimmed by `exclude_repos` in the deps config. Exclude patterns match every
  trailing run of path segments, so `mad01/*` and `*/archive/*` both work; a
  pattern that does not compile fails the scan rather than matching nothing.
  New catalogued repos auto-enroll.
- Each repo is walked for `go.mod` (→ `go list -m -json all`, resolved graph,
  v-prefix stripped for OSV), `package-lock.json` (lockfile v2/v3), and
  `requirements.txt` (exact `name==version` pins only). Unpinned names and
  ranges have no OSV-checkable version and are skipped; names are PEP 503
  normalized. The walk also covers `Package.resolved` (v2/v3, remote pins with
  an exact version), and branch pins are skipped. The shared walker skips `.git/vendor/node_modules/testdata`, **git worktrees and nested
  checkouts** (a `.git` entry in a subdir), and any `exclude_paths` glob.
  `VendoredJS` alone walks into `vendor`.
- **Browser JavaScript outside any lockfile is reported as npm.** Two
  discoverers add `npm` rows, each `Direct` and `Imported`:
  - `JSDelivrScripts` reads every `*.sh` the walker reaches for
    `cdn.jsdelivr.net/npm/<name>@<version>` URLs. The version is a literal or
    a `$VAR` / `${VAR}` the script always sets to the same literal, so no
    script's variable names are built in. A variable assigned two values, or
    anything that expands, is skipped rather than guessed, and so is a URL on
    a comment line. Only exact versions count: a range, a tag such as
    `latest`, or a variable it can't resolve is skipped. present's
    `scripts/cache-assets.sh` yields ten packages; its xterm script and
    stylesheet share one version and make one row.
  - `VendoredJS` hashes every `.js` file at any depth below a directory named
    `static` whose name ends in `.min.js` or carries a version
    (`cytoscape-3.31.0.min.js`). Its walk enters `vendor`, since
    `static/vendor/` is where such files usually live. It looks each sha256 up at
    `data.jsdelivr.com/v1/lookup/hash/`. First-party files like `app.js` are
    never hashed, so the calls stay at one per vendored file (one today). An
    npm match names the package and version. With no match, the file name
    supplies them and the row carries the `deps-vendored-modified` signal.
    The version also gains `+sha256.<first 12 hex>`, so a resolve lapses when
    the file changes; OSV ignores semver build metadata. A GitHub match is
    published and unchanged, so it takes the file name's npm name and
    jsDelivr's version (a leading `v` dropped) without the signal. An
    unversioned `.min.js` with no match is skipped as a first-party build. A
    first-party bundle with a version in its name (`static/app-1.4.0.js`)
    gets a false `deps-vendored-modified` signal and an OSV check of npm
    `app`; `exclude_paths` on its directory is the way out.
  - The lookup cache lives in memory for the serve process: a match for good,
    a miss for 24h. That is the API's own Cache-Control, and a miss can turn
    into a match because jsDelivr indexes a file only after serving it once.
    It isn't persisted: one call per vendored file per serve start isn't
    worth a second file in the workdir.
  - **A failed lookup is a `DiscoveryError`** (ecosystem `npm`) for that
    repo's vendored files, never a guess. The file is neither known clean nor
    known changed, and a sandbox without the jsDelivr egress has to reach
    doctor, the event and the page, which a signal never does. Lookups aren't
    paced and a failed one isn't cached, so a jsDelivr 429 comes back as the
    same error on every scan until `exclude_paths` trims the candidates.
- **Installed mise tools.** Beside the repos, `discover.Mise` reads the tools
  mise installs with its npm: and pipx: backends, code that runs on the host
  outside any sandbox. `mise.lock` (default `~/.config/mise/mise.lock`) pins
  each tool's version, and only that version's directory is read:
  `<installs>/<tool in kebab case>/<version>`, default installs
  `~/.local/share/mise/installs`. The kebab rule is mise's own (Rust's heck
  crate), so `npm:@googleworkspace/cli` lives in `npm-googleworkspace-cli`. The
  `latest`, major and minor alias links are never followed. npm trees come
  from the install's `aube-lock.yaml` (pnpm lockfile v9, `name@version`
  keys); pipx trees from the virtualenv's `*.dist-info` folder names, PEP 503
  normalized. The venv is `<version>/<pkg>/` when uv installed the tool and
  `<version>/venvs/<pkg>/` (plus pip in `shared/`) when pipx did, so both
  layouts are read. Discovery never runs mise, whose shims don't work inside the
  sandbox. Both paths are `[mise]` keys in the deps config. Every mise row sets
  `Imported: true` (see "Adding a producer"), so its vulnerabilities count and
  notify like any compiled-in dep's.
- **mise labels.** Each tool reports under the repo label `mise:<tool>`
  (`mise:npm:yarn`, `mise:pipx:httpie`). The label carries no version, so it
  stays the rescan key and the page's `?repo=` filter across upgrades; the
  tool's own package (marked direct) shows the version. `deps check --repo`
  and `deps_scan_repo` take the whole label, matched exactly; `SaveRepo`
  replaces that tool alone. An unknown label, or a lock that can't be read,
  fails the rescan and leaves the store untouched. The page shows a label
  whole: `basename` in `app.js` leaves `mise:` labels uncut, since
  `mise:npm:@scope/name` would otherwise shorten to `name`.
- **What a mise scan treats as absent vs failed.** No lock or no installs
  directory means no mise, so no tools. A tool the lock pins but this machine
  never installed (no tool directory) is skipped, since one lock covers every
  machine class. A lock that exists but can't be read or parsed is one
  DiscoveryError (ecosystem `mise`, repo = the lock path). A tool whose locked
  version directory, `aube-lock.yaml` or `site-packages` is missing or
  unreadable is a DiscoveryError under its label. A missing version directory
  means the install lags the lock, which `mise install` fixes. An npm version
  installed before mise wrote `aube-lock.yaml` has none, and only `mise
  install -f <tool>` rewrites it. `readSitePackages` reads the version
  directory before globbing, because `filepath.Glob` drops a sandbox
  `operation not permitted` and would report no site-packages instead.
- **GitHub Actions.** Each repo's `.github/workflows/*.yml|yaml` (top level
  only, as GitHub reads them) and every `action.yml|yaml` under
  `.github/actions` are parsed with yaml.v3 for `jobs.<id>.uses`,
  `jobs.<id>.steps[].uses` and a composite action's `runs.steps[].uses`. A
  local `uses: ./path` ref is followed to the `action.yml|yaml` in that
  directory of the repo (`./` is the repo root), and its refs are read the
  same way. For a local ref and an action file under `.github/actions` alike,
  symlinks are resolved first, so the file is checked and keyed at its real
  place in the repo. Each file is read once, so a cycle ends. A file outside
  the repo (a symlink can lead there), under an `exclude_paths` glob, or
  missing (a job may generate it) is skipped without an error. `docker://`
  images are skipped; `owner/repo/path@ref` is package `owner/repo`, as
  written. A ref pinned to a 40-hex SHA needs no lookup. A tag or branch ref
  is resolved to its commit from the action repo's ref list, a plain HTTPS
  GET of
  `https://github.com/<owner>/<repo>.git/info/refs?service=git-upload-pack`.
  It returns what `git ls-remote` prints, peeled `^{}` lines included, with no
  git binary in the sandbox and no API quota. A tag wins over a branch of the
  same name. `discover.Repos` memoizes each ref list for one walk, so a scan
  fetches each action repo once and the next scan sees a moved tag. A tag or
  branch ref carries the `deps-movable-ref` signal, which names the ref as
  pinned. A ref that isn't a full `x.y.z` (`v46`, `main`) takes its label from
  the highest full-version tag on the same commit (`v46.0.5`), so the range
  filter can judge it. A release-line ref takes a tag in its line only: `v45`
  a `v45.x.y` and `v4.2` a `v4.2.x`. A later major's tag on that commit would
  read the pin as past fixes its line may lack. A branch takes any. With no
  such tag, the ref is the label. Every ref is Direct and Imported, one row
  per workflow file.
- **A ref that can't be resolved is a discovery error, not a row.** A missing
  or private action repo (github.com answers 401), an unknown tag, or an
  unreachable host leaves the ref out. The repo's `GitHub Actions` discovery
  error names it, and the repo's other refs stay. An unresolved row would have
  to carry a version without the commit, so an outage would change its key:
  the finding would re-notify, and a resolved one would resurface. Leaving it
  out keeps the keys stable, and the existing discovery-error surfaces show it
  as unchecked.
- Transitive deps are scanned and flagged (real supply-chain risk); the web/CLI
  mark direct vs transitive. The refs inside a third-party action (a remote
  composite action's own `uses:`) are not followed.
- **Discovery errors are saved, never only logged.** A discoverer that fails
  for one repo and ecosystem (a `go list` with no `go` on PATH, an unreadable
  lockfile) becomes a `store.DiscoveryError{Repo, Ecosystem, Error}`; the walk
  goes on with the rest. Serve persists them with the scan and returns them as
  `discovery_errors` from `/api/deps`, `/api/flagged`, `/api/scan` and
  `/api/check`, and the MCP scan/check tools pass them through. The
  scan-complete event goes to `warn` with the count. The doctor
  `discovery-clean` check reads them from the store, and the page shows a red
  callout in place of the all-clear text. A failed `go list` appends its first
  stderr line to the error. Packages a discoverer returns beside its error are
  kept, since it read them.
- **serve needs `go` on its PATH.** The t-man agent runs
  `recipes/deps/deps-serve.sh`, which resolves the real mise `go` with
  `mise which go` outside the sandbox and execs `deps serve` through srt-run
  with that directory first on PATH. The launchd PATH alone has no `go`, and
  the mise shim cannot run inside the sandbox. Doctor deliberately reads the
  saved errors instead of probing for `go` itself: the CLI runs with the login
  shell's PATH, so its own lookup would pass while serve fails.
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
- querybatch pages a result past OSV's limits: deps repeats only the paged
  queries, each with its `next_page_token`, for at most 20 requests a batch.
- **One unreadable record never costs the scan.** A failed `GET
  /v1/vulns/{id}` is logged and the finding is kept with its id and kind but
  no summary, severity or fix; the next check fills them in. Only a failed
  querybatch, a batch still paging after 20 requests, or a cancelled request
  fails the check.
- The finding's kind comes from the id alone (`store.KindForID`): `MAL-` is
  `malicious`, anything else `vuln`. OSV serves GitHub's malware advisories as
  MAL- records too, so the prefix covers both.
- One API; ecosystem strings are OSV's own (`Go`, `npm`, `PyPI`, `SwiftURL`,
  `GitHub Actions`); no mapping. SwiftURL names are the repo URL without scheme
  or `.git` (`github.com/vapor/vapor`), derived from each Package.resolved pin
  location.
- **GitHub Actions are checked by commit.** OSV has no version comparator for
  the ecosystem (google/osv.dev#2308), so a name and version query finds
  nothing. A query with `osv.Query.Commit` set becomes two querybatch
  lookups. The commit alone (OSV rejects a commit beside a version) matches
  the git ranges of records such as CVE-2025-30066. The package name alone
  returns every advisory on the action. Check keeps a name hit only when the
  label falls in one of its `ECOSYSTEM` or `SEMVER` ranges. It fails open: a
  label that isn't a full `x.y.z` (`v4`, `main`), a record that didn't load,
  or a bound it can't compare keeps the hit. At one version, in any record
  order, a `fixed` sorts first, then an `introduced`, then a `last_affected`.
  So a range re-introduced at its own fix stays open, and an `introduced` and
  a `fixed` at one version fail open and keep the hit. An id found by both
  lookups is listed once. A CVE record and its GitHub Security Advisory
  (GHSA) alias stay two findings, as GO- and GHSA- pairs already do for Go.
- **A SHA pin's `# vX.Y.Z` comment is trusted as its version label**, and it
  alone drives the range filter. A comment newer than the pinned code would
  drop advisories whose range holds the real version. Dependabot and Renovate
  keep the comment in sync with the SHA, so this is an accepted risk.
- **OSV matches GitHub Actions names case-sensitively; GitHub doesn't.**
  `azure/setup-kubectl`, as its own README spells it, finds nothing, while
  `Azure/setup-kubectl` finds GHSA-p756-rfxh-x63h. Most of OSV's Actions
  ranges are `ECOSYSTEM` only, so the name lookup is the main detector. So
  Check reads OSV's GitHub Actions export once per call
  (`https://osv-vulnerabilities.storage.googleapis.com/GitHub%20Actions/all.zip`,
  about 100 KB, one JSON record per advisory). It asks for each action as
  written and in every other spelling OSV files it under that matches
  ignoring case, and matches records the same way. The written name stays
  because the export can lag an advisory filed under it. The row and key
  keep the name as written. When the export can't be read, the names go out
  as written and Check returns `*osv.ActionNamesError` beside complete
  results. `scanner.Check` turns that into a `GitHub Actions` discovery error
  for each repo with an Action, since a log line alone would let the scan
  read as clean.
- **serve runs under srt** (anthropics/sandbox-runtime), with the access
  declared as `spec.access` in `service-info.yaml`. It reads the repos under
  `~/code/src` minus secret-shaped files, the module cache,
  `~/.config/mise/mise.lock` and the whole mise installs directory (the Go
  toolchain included). Egress goes to `api.osv.dev`,
  `osv-vulnerabilities.storage.googleapis.com`, `proxy.golang.org`,
  `registry.npmjs.org`, `pypi.org`, `data.jsdelivr.com` and `github.com`. The
  installs grant is the plain
  directory, not `npm-**` / `pipx-**`: a tool declared by its short name
  (`cookiecutter`, backend `pipx:cookiecutter`) installs to
  `installs/cookiecutter`. A plain path grants its subtree; srt drops a
  trailing `/**` from an allow, which leaves only the directory readable.
  `go list` gets a private build cache in the workdir (`GOCACHE`, set by the
  recipe) and `GOFLAGS=-buildvcs=false`, so it never writes the shared build
  cache or runs git.

### Registry check

A registry can pull or flag a version before any advisory exists, or without
one. After OSV, every full and per-repo check asks the registry that publishes
each version (`internal/regcheck`):

| Rule id | Kind | The registry says |
|---------|------|-------------------|
| `deps-npm-withdrawn` | malicious | the package document's `time` lists the version but `versions` does not: unpublished, or replaced by a security holding package (chalk 5.6.1) |
| `deps-npm-deprecated` | signal | the version document carries a non-empty `deprecated` |
| `deps-pypi-withdrawn` | malicious | `/pypi/<name>/<version>/json` is 404 while the project's Simple API page exists (ultralytics 8.3.41) |
| `deps-pypi-quarantined` | malicious | PEP 792 `project-status` is `quarantined` |
| `deps-pypi-yanked` | signal | the release has `yanked: true` |
| `deps-pypi-archived`, `deps-pypi-deprecated` | signal | PEP 792 `project-status` is `archived` or `deprecated` |
| `deps-go-blocked` | malicious | `@latest` answers 403 with a body that starts `SECURITY ERROR` (github.com/boltdb-go/bolt) |
| `deps-go-retracted` | signal | a `retract` directive in the go.mod at `@latest` covers the version |

- **Only positive evidence counts.** A package or version the registry has no
  record of (a private package, a module the proxy lacks) gets no finding.
  PyPI keeps no trace of a project it deleted outright, so OSV's MAL- records
  cover that case. A PyPI version with a local label (`2.1.0+cu118`) never
  counts as withdrawn: PyPI does not serve those. A 403 from the Go proxy
  without `SECURITY ERROR` (the sandbox refusing a host, say) is an
  unanswered lookup, never a blocked module.
- **Calls.** One lookup per unique package per check, cached for that check:
  npm takes one request per version, plus the package document when a version
  is gone. PyPI takes the Simple API page plus one request per version. Go
  takes `@latest` plus the `.mod` at that version. At most 8 packages run at
  once, each request has 10s, and the whole pass has 45s. A module matching
  `go env GONOPROXY` (GOPRIVATE by default) is never sent to the proxy, the
  same rule go follows.
- **Graph-only Go modules are checked.** A blocked module is malicious, and a
  malicious finding bypasses the "Not compiled in" gate. Checking them took a
  full pass from 266 to 638 requests on the 648-dependency inventory
  (2026-10-28).
- **No background scan id.** That full pass took 2 to 6 seconds, so a full
  check (about 20 to 30 seconds with discovery) stays well inside the CLI's
  2-minute request timeout. The 45-second pass budget bounds a registry that
  hangs.

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
- **Malicious findings get their own banner**, fired before the summary and
  never folded into it. It names each package version and its repos, and says
  to check whether it was installed and rotate the credentials it could
  reach. Each banner retries on its own if delivery fails.
- **A manual check notifies too.** `POST /api/check`, full, per repo or per
  mise label (the page's rescan buttons, `deps_check`, `deps check`), fires
  the banners for anything new once the check succeeds. A full check resets
  `ScannedAt`, so the loop skips its next cycle; without this, checking by
  hand every day would mean no banner ever fires. `Engine.Notify` is
  serialized and the notified set keeps the loop from firing the same banner
  again.

## Data model & storage

```go
Dependency{ Ecosystem (Go|npm|PyPI|SwiftURL|GitHub Actions), Name, Version, Repo (root or mise label), ManifestPath, Direct, Imported, RegistryUnknown, Advisories[] }
Advisory{ ID, Kind (vuln|malicious|signal), Summary, Severity, FixedVersion }  // one finding on that exact version
DiscoveryError{ Repo, Ecosystem, Error }  // a repo+ecosystem the scan could not read
```

- **Scan store** holds the last scan's deps and discovery errors plus two
  string sets keyed by
  `FlagKey = ecosystem:name@version:advisoryID`: `notified` (already alerted) and
  `resolved` (user-acknowledged). Because the key includes the version, an
  acknowledged advisory **auto-resurfaces** once the package is bumped (the key
  changes); and a steady-state advisory neither re-notifies nor re-appears.
- **ScannedAt** is the last *full* scan time. A per-repo rescan
  (`SaveRepo`) deliberately does **not** reset it, so the daily catch-up still
  fires; it replaces only that repo's deps and discovery errors.
- **RegistryUnknown** marks a dep whose registry lookup got no answer in the
  check that saved it. It travels with the row, so `SaveRepo` merges it like
  everything else (see "Adding a producer" for what it keeps).

### Finding kinds

`store.Kind` is a closed set. The kind alone decides how a finding surfaces:

| Kind | Set by | Banner | Counts as active | "Not compiled in" gate | Page |
|------|--------|--------|------------------|------------------------|------|
| `vuln` | an OSV id other than MAL- | coalesced summary | yes, until resolved | applies | Active findings, by severity |
| `malicious` | an OSV MAL- id, or a producer's own rule (a version the registry withdrew, quarantined or blocks) | its own | yes, until resolved | bypassed (install scripts may have run) | first in Active findings, red badge; Resolve asks for confirmation |
| `signal` | a deps rule, such as `deps-movable-ref` on a tag-pinned Action, or a yanked, deprecated, archived or retracted version | never | never, nor in `flagged_count` | not used | Warning signs section |

- The banner and count rule is one function, `store.Kind.Alerts(imported)`.
  `PendingFlags` and `api.Dependency.Active` call it; `app.js` mirrors it as
  `alerts()`. `api.Flagged` lists deps with an unresolved malicious finding
  first, so `/api/flagged` and the MCP tools lead with them.
- An empty kind is derived from the id with `store.KindForID`, on load
  (`readScan`) and on `Save`/`SaveRepo`. A scan.json written before kinds
  existed loads a MAL- finding as `malicious` under its old `FlagKey`, and
  everything else as `vuln`.

### Adding a producer

A producer builds `store.Advisory` values with a kind and an id. A warning
sign is `store.Advisory{ID: "deps-movable-ref", Kind: store.KindSignal,
Summary: "…"}`. Follow-up producers build on these rules:

- **Attach in one of two places.** A discoverer sets
  `discover.Package.Advisories` during discovery, or a `scanner.Checker`
  returns the finding for its query index. `scanner.Check` adds the checker's
  findings to the discovered ones; a checker finding whose id is already
  there replaces it. The full path (`Engine.Check`, then `Save`) and the
  per-repo path (`Engine.CheckRepo`, then `SaveRepo`) both run it. A finding
  attached in `Engine.Check` alone would vanish on a per-repo rescan.
- **Ids.** An OSV id is used as-is. Any other id is the producer's rule id,
  whatever its kind: give it a `deps-` prefix and no `:`, since `parseKey`
  splits the key on its last colon.
- **Kinds.** Pick one of the three and never add a kind. `malicious` isn't
  OSV's alone: a producer may set it under its own rule id.
- **New dependency rows set `Imported: true`**, unless their ecosystem
  computes reachability (only Go does today). A row left false is gated as
  "Not compiled in": its vulnerabilities never count and never notify.
- **The version in the key must track the pin.** `FlagKey` embeds the
  version, and a resolved or notified finding keeps that state while its key
  holds. So the version must change whenever the pinned thing does. A GitHub
  Action's version is `<label>+<commit>` (`v45.0.7+a284dc18…`, the full
  40-hex SHA): the label is the tag or branch, or a SHA pin's `# vN` comment,
  and a SHA pin with no comment is the commit alone. A tag or branch that
  isn't a full `x.y.z` uses the release tag on its commit. A tag repointed to
  other code, as in the tj-actions attack, gets a new key. It notifies again,
  and a resolve doesn't carry over. `discover.SplitActionVersion` is the one
  parser.
- **A version never contains `@`.** `parseKey` splits name and version on
  the last `@`, the same way an id never contains `:`. Don't join a tag and
  its commit with `@`: a GitHub Action joins them with `+`, and a label with
  `@` or `:` in it is dropped.
- **The registry checker composes with OSV.** `Engine.Checker` (OSV) runs
  first, and its error fails the check. `Engine.Registry` (a
  `scanner.Registry`, in serve a `*regcheck.Client`) then runs over the same
  deps through `CheckRegistry`. It has no error of its own: it returns one
  `regcheck.Result` per dependency index, and the findings merge in with
  `addFindings`. An answer replaces the registry findings the dep already
  carried (`regcheck.IsRule` tells them apart from OSV's and a discoverer's,
  which stay).
- **"Unknown" belongs to registry lookups only.** A lookup that got no answer
  (a network error, a timeout, an unexpected status, a body that does not
  decode) sets `Result.Err`. The dep is saved with `RegistryUnknown: true`
  and keeps the registry findings the stored inventory held for the same
  version, from any repo. So an outage neither drops a finding nor lists a
  resolved one under "Resolved · updated past". `registry_unknown` counts
  these deps on `/api/deps`, `/api/flagged` and `/api/check`, in the MCP
  tools and `deps check`, in a page callout, and in the scan-complete event
  (at warn). Any other finding that drops out of a scan still reads as gone,
  and an OSV failure still fails the whole check.
- **A partial read keeps what it read.** A discoverer may return packages
  beside its error; `discover.Repos` keeps them and records the error.
- A new producer leaves the key format, the notifier, the API and the page
  as they are. `osv.Query` may grow fields for a new query shape. The
  commit reaches OSV through the version, not a new stored field:
  `scanner.Check` builds each query with `query`, which splits a GitHub
  Action's version into `osv.Query.Version` (the label) and `Commit`. So
  `store.Dependency`, scan.json and the API keep their shape.

## Build / install / test

```bash
make build    # ./deps binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/deps + adhoc codesign
make test     # go test ./... (hermetic: t.TempDir, canned go-list/OSV/jsDelivr/lock fixtures, fake notifier; no real network/go/osascript)
```

## HTTP API

- `GET  /`                       : webkit web page: rescan-all, per-repo rescan, per-finding resolve, acknowledged section
- `GET  /api/deps`              : full inventory (enriched: advisory `key` + `resolved`)
- `GET  /api/flagged`          : flagged deps only
- `POST /api/scan`             : discover only (no OSV), persist, return per-ecosystem counts
- `POST /api/check[?repo=]`    : discover + OSV and registry check (full, or one repo merged), notify, return flagged
- `/api/deps`, `/api/flagged` and `/api/check` also return `discovery_errors` and `registry_unknown`
- `POST /api/resolve`          : body `{keys:[…]}`; acknowledge advisories
- `POST /api/notify`           : fire pending notifications now
- `GET  /version`              : the four-key build metadata object (`version`, `commit`, `tag`, `build_time`) · `GET /webkit/`

## Commands

CLI surface beyond `serve`/`mcp`: thin HTTP clients to `deps serve`, one call
per HTTP endpoint above:

```bash
deps scan                    # POST /api/scan - discover only, print per-ecosystem counts
deps check [--repo <repo>]   # POST /api/check[?repo=] - discover + OSV + registries, print flagged; <repo> may be a mise label
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
- `deps_check`: discover + OSV + registries; return flagged, malicious first (each advisory has `key`, `kind`, `resolved`), plus `registry_unknown`
- `deps_list_flagged`: flagged from the last check, no re-scan
- `deps_scan_repo(repo)`: rescan one repo (path or basename) or mise tool
  (whole label) and merge
- `deps_resolve(keys)`: acknowledge advisories by key; the description tells
  the agent to get the user's confirmation before resolving a malicious one
- `deps_doctor`: run the same checks as `deps doctor` (service-reachable,
  store-readable, version-skew, discovery-clean) and return the report as
  JSON — for a client that can call a tool but has no shell

## Shared UI: webkit

The chrome (header, theme toggle, font/size/fixation controls, toast host) comes
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
<wk-header brand="deps" title="Dependencies"><a data-nav href="/webkit/themes">Themes</a></wk-header>
```

The Themes nav link opens the palette picker webkit serves at `/webkit/themes`,
since the browser stores the choice per origin. `webkit.js` injects the full
control set (font · fixation · size ± · reload · theme). Don't add those
controls manually.

### Per-repo changes

- Header and shell markup live in `internal/server/shell.html`, a static
  chrome-only shell embedded via `//go:embed`. The page body renders
  client-side: `/app.js` (also embedded) fetches `GET /api/deps` and builds the
  DOM in the browser with `Webkit.el`/`Webkit.escapeHtml`/`Webkit.poll`
  (`Webkit.poll('/api/deps', render, 60000, …)` drives the 60s live refresh).
- Components used: `<wk-header>`, `<wk-toast-host>` + `document.createElement('wk-toast')`
  for transient toasts, `<wk-page-header>` (+ `<wk-title>`/`<wk-subtitle>`),
  `<wk-card>`, `<wk-badge>`, `<wk-button>`, and the `<wk-modal>` in
  `shell.html` that confirms resolving a malicious finding.
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

- **Wave 0 builder.** Builds before the consuming repo's agent-sync recipe (wave 1) registers the MCP.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `deps docs`): serve-must-be-running, store layout, phantom
  "Not compiled in" advisories, version-scoped resolve resurfacing,
  version-skew checks. Keep those facts there, not here.
- **Personal Mac only**: recipe, MCP entry, and d-man route are all
  `profiles = ["personal"]`.
- **Codesign for the binary.** `make install` strips xattrs and re-signs.
- **Version probe.** `GET /version` and `deps version -o json` both return the
  shared four-key build metadata object (`version`, `commit`, `tag`,
  `build_time`, every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, the cross-tool convention `ralph` and
  `status` use to probe the build a sibling tool is running. Plain
  `deps version` stays a bare token.

## See also

- Recipe: `recipes/deps/recipe.toml` (+ `recipes/deps/CLAUDE.md`)
- Route: the consuming repo's `recipes/d-man/routes.toml` overlay (`deps` → 7429; docs/adr/0006)
- Config: `recipes/deps/config.toml` → `~/.config/deps/config.toml`
- MCP registration: the consuming repo's agent-sync `servers.json` (worked example: `examples/dotfiles/recipes/agent-config/`)
- Repo list source of truth: `recipes/catalog/registry.yaml`
