# operating deps

deps is a supply-chain scanner. It discovers every external dependency across
the catalog-registered repos (Go modules, npm lockfiles, exact Python pins,
Swift resolved files) and the npm and PyPI trees of the tools mise installs
with its npm: and pipx: backends. It checks each pinned version against the
OSV.dev advisory database and fires one coalesced macOS notification when
packages are newly flagged, plus a separate one for a malicious package.
Findings live on a local web page (default {{.BaseURL}}).

## how it runs

A single `deps serve` process is the only writer. It owns the scan store, runs
the daily scan loop, serves the web page and JSON API on localhost, and is the
only process that reaches OSV. Everything else is a thin HTTP client over that
API: the CLI commands (`scan`, `check`, `resolve`, `notify`) and the MCP
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
up`), then `deps check --repo <label>`. An `operation not permitted` error
comes from the sandbox. If mise.lock links to a file outside `~/code/src`, add
that target to `~/.config/srt/deps.local.json`. No mise tools in the inventory
and no error means there is no lock at the configured path.

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

An acknowledged advisory came back: resolution is version-scoped by design.
The key embeds the exact version, so bumping the package (or a new advisory
landing on it) resurfaces the finding. Resolve it again if it still does not
apply.

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
