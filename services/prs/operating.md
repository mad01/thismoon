# operating prs

prs is an open-PR dashboard over local state. It discovers every git repo
under the configured directories, polls each repo's GitHub host for open pull
requests through the user's existing gh logins, and caches the result. Only
open, non-draft PRs are tracked: closed, merged, and draft PRs never enter the
cache.

## how it runs

A single `prs serve` process is the only writer. It owns the cache, runs the
background poll loop, and serves the JSON API plus a read-only web page on
localhost (default {{.BaseURL}}). Everything else is a thin HTTP client over
that API: the CLI commands (`list`, `refresh`, `status`) and the MCP server, a
stdio shim spawned as `prs mcp`. The shim being up says nothing about the
service: every tool call it handles is a live HTTP request to serve, and it
fails when serve is down. Machines usually also route http://prs.this to serve
via the local domain front door; if the localhost port answers but the .this
host does not, the router is the problem, not this service.

Polling is staleness-driven: a cycle runs when the last one is older than the
poll interval (default 5m), checked every 30 seconds, so a laptop waking from
sleep refreshes promptly. `POST /api/refresh`, `prs refresh`, and the
`prs_refresh` MCP tool all force one synchronous cycle.

## where state lives

The cache is an append-only JSONL log in the workdir, default {{.StorePath}}
(override with PRS_WORKDIR). One file, `repos.jsonl`: one line per repo state
change, newest record per repo wins on load. Quiet cycles append nothing; the
file compacts itself at startup when superseded lines pile up. It is purely a
cache of upstream GitHub state — deleting it costs one cold poll cycle.

Config is a YAML file, default ~/.config/prs/config.yaml (override with
PRS_CONFIG or --config): `dirs` to scan, `exclude` globs, an optional `hosts`
allowlist, and `poll_interval`. No tokens live there — auth comes from
`gh auth token --hostname <host>` per host, cached in memory.

## failure modes

Start with `prs doctor`: one command runs the reachability, store, version-skew,
config, and gh checks and prints one line per check, FAIL lines naming the
cause. The paragraphs here cover what each failure means and what to do next.

Connection refused, or "prs serve not reachable": serve is not running. t-man
typically supervises it. Run `t-man list` to see whether the prs agent exists,
then `t-man restart prs`. For a quick test without t-man, `prs serve` in a
spare terminal also works.

Empty PR list: usually not an error — check `prs status` first. `repos: 0`
with no dirs configured means the config file is missing or sets no `dirs`.
A specific repo missing means it matched an `exclude` glob, its host is not in
the `hosts` allowlist, its checkout has no origin remote, or its fetch failed
(listed under errors in the same output).

Per-repo fetch errors mentioning "gh auth token": gh has no login for that
host. Run `gh auth login --hostname <host>` as the user serve runs as, then
refresh. A 401 mid-flight re-mints the token automatically; only a missing
login needs a human.

Rate-limit errors: the poll fans one PR-list call per repo plus one reviews
call per open PR. At the default interval this sits far under GitHub's
limits; if a host still rate-limits, raise `poll_interval` in the config.

## version skew

`prs version -o json` reports the build of the binary on PATH. `GET
{{.BaseURL}}/version` reports the build the running serve process came from.
When the `commit` values differ, an old process is still serving after an
upgrade: restart it (`t-man restart prs`) and compare again. `prs doctor` runs
this comparison as its version-skew check.

## first moves

1. `prs doctor`: serve reachable, store readable, version skew, config, gh in one pass
2. If serve is unreachable: `t-man list`, then `t-man restart prs`
3. On version skew: `t-man restart prs`, then `prs doctor` again
4. `prs status`: last poll time, repo counts, per-repo fetch errors, config in effect
5. `prs refresh`: force a cycle and read its repo/PR/error counts directly
