# operating deps

deps is a supply-chain scanner. It discovers every external dependency across
the catalog-registered repos: Go modules, npm lockfiles, exact Python pins,
Swift resolved files, browser libraries kept outside any lockfile, and GitHub
Actions refs. It also reads the npm and PyPI trees of the tools mise installs
with its npm: and pipx: backends. It checks each pinned version, or for an
action the commit it runs, against the OSV.dev advisory database. It fires
one coalesced macOS notification when packages are newly flagged, plus a
separate one for a malicious package. Findings live on a local web page
(default {{.BaseURL}}).

## how it runs

A single `deps serve` process is the only writer. It owns the scan store, runs
the daily scan loop, serves the web page and JSON API on localhost, and is the
only process that reaches the network: OSV, jsDelivr when discovery names a
vendored JavaScript file, and github.com to resolve the tag or branch an
action is pinned to. Everything else is a thin HTTP client over that API: the
CLI commands (`scan`, `check`, `resolve`, `notify`) and the MCP
server, a stdio shim spawned as `deps mcp`. The shim being up says nothing
about the service: every deps_* tool call is a live HTTP request to serve, and
it fails when serve is down. Machines usually also route http://deps.this to
serve via the local domain front door. If the localhost port answers but the
.this host does not, the router is the problem, not this service.

## where state lives

The store is one JSON file, `scan.json`, in the workdir, default {{.StorePath}}
(override with DEPS_WORKDIR). It holds the last scan's dependencies, the
`discovery_errors` it hit, and two string sets keyed by
`ecosystem:name@version:advisoryID`: `notified` (already alerted) and
`resolved` (acknowledged). serve rewrites the file atomically, so
it is always safe to read directly. The discovery config (exclude lists) is a
separate file, default `~/.config/deps/config.toml` (or under
$XDG_CONFIG_HOME/deps when that is set), and the repo set comes from the
catalog registry. Its `[mise]` keys locate mise.lock and the mise installs
directory; `deps config` prints the paths in use.

## failure modes

Start with `deps doctor`: one command runs the reachability, store,
version-skew, and discovery-clean checks below and prints one line per check,
FAIL lines naming the cause. From an MCP client with no shell, the `deps_doctor` tool runs the same
checks and returns the report as JSON. The paragraphs here cover what each
failure means and what to do next.

A scan cycle failing with a config parse error: a pattern in `exclude_repos` or
`exclude_paths` does not compile. The log line names the key and the pattern.
`deps config` prints the same error beside the resolved path. Exclusions are
never skipped in silence, so this fails the cycle rather than scanning a repo
the config asked to skip.

Connection refused, or "deps serve not reachable": serve is not running. t-man
typically supervises it. Run `t-man list` to see whether the deps agent
exists, then `t-man restart deps`. For a quick test without t-man, `deps
serve` in a spare terminal also works.

A discovery-clean FAIL, or far fewer packages than expected: the last scan
could not read some repo and ecosystem, so none of its dependencies were
checked. serve saves each failure with the scan; `/api/deps`, the scan and
check tools (`discovery_errors`), the scan-complete event (warn level), and a
red callout on the web page all show it. Doctor reads what serve saved, never
its own shell: serve runs under launchd with a narrower PATH. The usual cause
is `exec: "go": executable file not found`, meaning the agent was registered
without the launcher that resolves the mise go. Re-run `ralph up` to
re-register it, then `deps check`. serve runs sandboxed, so two more causes
are the sandbox's: `operation not permitted` for a repo outside `~/code/src`
(add it to `~/.config/srt/deps.local.json`), and `host is not on the allow
list` for a module go would fetch from somewhere other than proxy.golang.org,
such as a GOPRIVATE module. `t-man logs deps --source sandbox` shows the
denials.

A discovery error under a `mise:` label, or with ecosystem `mise`: deps could
not read an installed tool's tree. Each tool reports under its label, such as
`mise:npm:yarn`. Ecosystem `mise` under the lock's path means mise.lock itself
could not be read or parsed, which hides every tool. A `not installed` error
under a label means the install lags the lock: run `mise install` (or `ralph
up`), then `deps check --repo <label>`. An `aube-lock.yaml: no such file`
error means mise installed that npm tool before it wrote aube lockfiles, and
`mise install` skips a version that is already there. Reinstall it with `mise
install -f <tool>` (the label without `mise:`), then rescan. An `operation not
permitted` error comes from the sandbox, which reads mise.lock and the
installs directory only at their default paths. If mise.lock links to a file
outside `~/code/src`, or the `[mise]` keys move the lock or the installs
directory, add that path to `~/.config/srt/deps.local.json`. No mise tools in
the inventory and no error means there is no lock at the configured path.

A discovery error for `npm` that reads `look up <file>: jsdelivr: ...`: deps
hashes each vendored library file (a `.js` at any depth below a `static`
directory, `static/vendor/` included, named with a version or ending in
`.min.js`). It looks the sha256 up on data.jsdelivr.com to learn the npm name
and version. The lookup failed, so none of that repo's vendored files were
checked, rather than read as clean. A timeout or a 5xx from jsDelivr clears on
the next check. `host is not on the allow list` means the sandbox settings
predate the jsDelivr egress: re-run `ralph up`.

The same error with `lookup returned 429` on every check: jsDelivr is
limiting the request rate. deps sends one lookup per candidate file with no
pause between them, and a failed lookup isn't cached, so each scan asks again
and fails again. Add the directories that hold the most candidates to
`exclude_paths`.

A `GitHub Actions` discovery error names each action ref that could not be
resolved to a commit, and that ref went unchecked. `ref list answered 401`
means the action repo is missing or private, `no such tag or branch` means
the ref is gone upstream, and `host is not on the allow list` means the
sandbox lacks github.com. The repo's other refs were still checked. A ref
pinned to a commit SHA never needs github.com. A private action repo or
reusable workflow answers 401 on every scan, so it stays a discovery error.
The workaround is `exclude_paths`, which matches the repo-relative path in
every repo, so `.github` stops the Actions check everywhere; exclude the
narrowest directory that pins it, such as `.github/actions/<name>`.

`OSV's GitHub Actions name list` in that error means the check couldn't read
OSV's export of Actions advisories from its data bucket. Every action was
still checked, by commit and by its name as written, but an advisory OSV
files under another capitalization was missed. The next check retries.
`host is not on the allow list` there means the sandbox settings predate the
data bucket egress: re-run `ralph up`.

A flagged advisory on a package nothing compiles in: Go scans cover the module
graph (`go list -m -json all`), which includes modules no binary imports. So
an advisory can flag a version that is never compiled. deps computes
import-graph reachability (`go list -deps -json ./...`) and marks such deps
not imported: the web page dims them in a separate "Not compiled in" section
and they never notify. Do not chase them with `go get` bumps; the module graph
usually cannot be pruned of them. Acknowledge with `deps_resolve` (or `deps
resolve <key>`) instead. Reachability fails open: when `go list -deps` errors,
every dep counts as imported, so a real advisory is never hidden. npm, PyPI,
and Swift deps always count as imported.

A finding of kind `malicious` (an OSV MAL- record): that version was published
with malicious code. Its install scripts may have run even if nothing compiles
it in, so it skips the "Not compiled in" section, sorts first, and fires its
own banner. Treat it before anything else: check whether it was installed on
this machine, rotate the credentials it could reach, then move the pin off
that version. Resolve it only after that; the web page asks to confirm.

A finding with an id but no summary, severity, or fix: OSV's record fetch for
that id failed during the scan, and the serve log names it. The finding is
kept so it is not lost, with its kind intact; the next check fills it in.

A finding of kind `signal`, in the "Warning signs" section: a deps rule
flagged something that is not an advisory. It never notifies and never counts
toward `flagged_count`, and it resolves by key like any other finding.
`deps-movable-ref` marks a GitHub Action pinned to a tag or branch, which the
action repo can point at other code. Pin the commit SHA, with the version in
a trailing comment, to clear it.

The signal `deps-vendored-modified`: a committed library file's sha256
matches no file jsDelivr has served. It was edited after download, or it never
came from npm as is. Its name and version come from the file name. The
version also carries `+sha256.` and the first 12 hex digits of the hash, so a
new edit brings a resolved signal back. OSV still checks that version. Compare
the file with the published one at
`https://cdn.jsdelivr.net/npm/<name>@<version>/<path>`, then download it again
or resolve the signal if the change is yours. A file just added to npm can
also miss, because jsDelivr indexes a file only after serving it once.
deps asks again after a day. A first-party bundle with a version in its name,
such as `static/app-1.4.0.js`, gets this signal falsely, and OSV checks it as
the unrelated npm package `app`. Add its directory to `exclude_paths`.

An acknowledged advisory came back: resolution is version-scoped by design.
The key embeds the exact version, so bumping the package (or a new advisory
landing on it) resurfaces the finding. A GitHub Action's version is its label
plus the commit it runs (`v4+11d5960a…`), so a tag moved to other code
resurfaces it too. Resolve it again if it still does not apply.

Large scan output: `deps check` and `deps_check` return every flagged dep, and
the full inventory can be big. Query the persisted store file with jq instead
of re-scanning, for example
`jq '.deps[] | select(.advisories != null)' scan.json`, or only the malicious
ones with `jq '.deps[] | select(any(.advisories[]?; .kind == "malicious"))' scan.json`.

## version skew

`deps version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart deps`) and compare again. `deps doctor`
runs this comparison as its version-skew check.

## first moves

1. `deps doctor`: serve reachable, store readable, version skew, and discovery
   errors in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart deps`
3. On version skew: `t-man restart deps`, then `deps doctor` again
4. Before treating a finding as real, check whether it sits in the dimmed
   "Not compiled in" section
5. List the workdir, to confirm `scan.json` exists and its `scanned_at` is
   recent
