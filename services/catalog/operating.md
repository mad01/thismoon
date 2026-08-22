# operating catalog

catalog is a self-hosted systems catalog. It reads service-info.yaml files
from the repos listed in a registry, builds an in-memory index of Systems and
Components, and serves a localhost web UI plus a CLI to browse, search, and
validate them. It never owns the data: entity files stay in their source
repos, and nothing is cached to disk between runs.

## how it runs

`catalog web` is a single process serving the UI and JSON API on loopback
(default {{.BaseURL}}), typically supervised as the `catalog-web` agent under
t-man. It scans the repos at startup, holds the result in memory, and rescans
on demand: the header's Refresh button (or POST /api/refresh) swaps in a
fresh scan while in-flight requests finish against the old one. The CLI
commands (`list`, `validate`, `config`) never talk to that process: each
invocation rescans the repos itself and exits. So the worst staleness is one
Refresh (web) or one re-run (CLI) away.

## where data comes from

Two kinds of plain files, no database:

- The registry, default {{.StorePath}} (every command takes `--registry` to
  point elsewhere): a YAML `sources` list of repo paths to scan. `catalog
  config` prints the resolved path and whether it loads.
- service-info.yaml files inside those repos, found by a recursive scan that
  skips .git, node_modules, vendor, .idea, dist, and build. One file can
  hold several entities separated by `---`.

catalog persists no state of its own. The one write path is the web UI's Add
form, which writes a new service-info.yaml into a registered repo and refuses
to overwrite an existing one.

## failure modes

Web UI unreachable: the web process is down. Run `t-man list` to see whether
the `catalog-web` agent exists, then `t-man restart catalog-web`. For a quick
test without t-man, `catalog web` in a spare terminal serves the same UI.

Web UI up but empty: the process probably started before the registry
existed. A missing registry is not fatal at startup (the server comes up with
an empty catalog instead of crash-looping), so create the registry, then
Refresh.

A component is missing: its service-info.yaml is missing, invalid, or outside
every registered source root. Run `catalog validate <repo-path>` on the repo
that should hold it; a clean result means the files are fine and the repo is
just not in the registry (`catalog config` shows which one is in use). A
source path that does not exist on this machine is skipped silently, so a
not-checked-out repo simply contributes nothing.

One bad file blocks everything: a parse or schema error in any
service-info.yaml aborts the whole scan, failing `list`, `validate`, and the
web Refresh alike. The error names the file; fix or delete it.

Two entities share a name: only `catalog validate` enforces the global
name-uniqueness rule. `list` and the web UI render a colliding catalog
without complaint, so a clean-looking UI proves nothing. `catalog validate`
with no arguments checks the whole registry and names the duplicate.

## version skew

`catalog version -o json` reports the build of the binary on PATH. GET
{{.BaseURL}}/version reports the build the running web process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: `t-man restart catalog-web` and compare again.

## first moves

1. `curl -s {{.BaseURL}}/healthz` (`ok` means the web process is up)
2. If unreachable: `t-man list`, then `t-man restart catalog-web`
3. `catalog config`, to confirm which registry is in use and that it loads
4. `catalog list`, to confirm a scan works end to end
5. `catalog validate`, to surface bad files and name collisions the UI hides
