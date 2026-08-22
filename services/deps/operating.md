# operating deps

deps is a supply-chain scanner. It discovers every external dependency across
the catalog-registered repos (Go modules, npm lockfiles, exact Python pins,
Swift resolved files), checks each pinned version against the OSV.dev advisory
database, and fires one coalesced macOS notification when packages are newly
flagged. Findings live on a local web page (default {{.BaseURL}}).

## how it runs

A single `deps serve` process is the only writer. It owns the scan store, runs
the daily scan loop, serves the web page and JSON API on localhost, and is the
only process that reaches OSV. Everything else is a thin HTTP client over that
API: the CLI commands (`scan`, `check`, `resolve`, `notify`) and the MCP
server, a stdio shim spawned as `deps mcp`. The shim being up says nothing
about the service: every deps_* tool call is a live HTTP request to serve, and
it fails when serve is down. Machines usually also route http://deps.this to
serve via the local domain front door; if the localhost port answers but the
.this host does not, the router is the problem, not this service.

## where state lives

The store is one JSON file, `scan.json`, in the workdir, default {{.StorePath}}
(override with DEPS_WORKDIR). It holds the last scan's dependencies plus two
string sets keyed by `ecosystem:name@version:advisoryID`: `notified` (already
alerted) and `resolved` (acknowledged). serve rewrites the file atomically, so
it is always safe to read directly. The discovery config (exclude lists) is a
separate file, default `~/.config/deps/config.toml`, and the repo set comes
from the catalog registry.

## failure modes

Start with `deps doctor`: one command runs the reachability, store, and
version-skew checks below and prints one line per check, FAIL lines naming the
cause. The paragraphs here cover what each failure means and what to do next.

Connection refused, or "deps serve not reachable": serve is not running. t-man
typically supervises it. Run `t-man list` to see whether the deps agent
exists, then `t-man restart deps`. For a quick test without t-man, `deps
serve` in a spare terminal also works.

A flagged advisory on a package nothing compiles in: Go scans cover the module
graph (`go list -m -json all`), which includes modules no binary imports, so
an advisory can flag a version that is never compiled. deps computes
import-graph reachability (`go list -deps -json ./...`) and marks such deps
not imported: the web page dims them in a separate "Not compiled in" section
and they never notify. Do not chase them with `go get` bumps; the module graph
usually cannot be pruned of them. Acknowledge with `deps_resolve` (or `deps
resolve <key>`) instead. Reachability fails open: when `go list -deps` errors,
every dep counts as imported, so a real advisory is never hidden. npm, PyPI,
and Swift deps always count as imported.

An acknowledged advisory came back: resolution is version-scoped by design.
The key embeds the exact version, so bumping the package (or a new advisory
landing on it) resurfaces the finding. Resolve it again if it still does not
apply.

Large scan output: `deps check` and `deps_check` return every flagged dep, and
the full inventory can be big. Query the persisted store file with jq instead
of re-scanning, for example
`jq '.deps[] | select(.advisories != null)' scan.json`.

## version skew

`deps version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart deps`) and compare again. `deps doctor`
runs this comparison as its version-skew check.

## first moves

1. `deps doctor`: serve reachable, store readable, and version skew in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart deps`
3. On version skew: `t-man restart deps`, then `deps doctor` again
4. Before treating a finding as real, check whether it sits in the dimmed
   "Not compiled in" section
5. List the workdir, to confirm `scan.json` exists and its `scanned_at` is
   recent
