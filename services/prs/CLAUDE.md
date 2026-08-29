# prs, open-PR dashboard over local checkouts

Go CLI, HTTP server with an embedded webkit web UI, and an MCP server, all
over one append-only JSONL cache. prs discovers every git repo under the
configured directories, polls each repo's GitHub host for open pull requests,
and answers "is there anything I need to act on?" in one place. Only open,
non-draft PRs are tracked: closed, merged, and draft PRs never enter the
cache. **The web page at `http://prs.this/` and the MCP tools are the primary
surfaces**; the `prs` CLI mirrors them.

## Module layout

```
prs/
  cmd/prs/             - entrypoint (delegates to internal/cli)
  internal/
    cli/               - cobra: root, serve, mcp, manage (list/refresh/status),
                         docs, doctor, version (build metadata from the shared
                         buildinfo package)
    client/            - HTTP client shared by the CLI and mcpserver (client.go)
    config/            - ~/.config/prs/config.yaml loading + validation
    github/            - token minting via the gh CLI (token.go) and the
                         go-github REST client with review-decision reduction
                         (client.go)
    poller/            - repo discovery (kit/repofind) + the staleness-driven
                         poll loop and one-shot refresh cycle
    store/             - PR/RepoState model, filtering (store.go), JSONL
                         scan/append/compact (jsonl.go)
    server/            - HTTP API + webkit web page (embedded shell.html + app.js)
    mcpserver/         - MCP tools (server.go = MCP server setup, tools.go = 4 tools)
  Makefile             - part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Single-writer architecture (load-bearing)

**`prs serve` is the only writer**: it owns the cache, runs the background
poll loop, and is the only process that talks to GitHub.

- **`prs serve`** loads the config, discovers repos, polls their hosts, and
  serves the HTTP API + web page + `/version` + `/webkit/`.
- **`prs mcp`** holds no state: it is a thin HTTP client to the serve API on
  `localhost:<port>`. If serve is down, tools return "prs serve not
  reachable … (t-man status prs)".
- **`prs` CLI** commands (`list`, `refresh`, `status`) are the same thin
  HTTP client.

### Discovery and polling

Each cycle walks the configured `dirs` with `kit/repofind` (the same
concurrent walker suspenders and belt use, extended with host parsing),
derives `host` + `org/repo` from every checkout's origin remote, and reduces
the result to poll targets: repos with no parseable remote are skipped, the
optional `hosts` allowlist applies, exclude globs apply (config `exclude`
plus a built-in default that hides `mad01/prs-testbed`, with `include` globs
overriding both), and two checkouts of the same remote poll once.

Fetching goes through the official go-github client, one REST list-pulls
call per repo plus one list-reviews call per open PR, fanned over 8 workers.
The reviews reduce to a standing decision: each reviewer's latest APPROVED
or CHANGES_REQUESTED counts, any CHANGES_REQUESTED wins, no standing review
is "". A fetch error keeps the repo's previous PRs and records the error
beside them, so a transient failure never blanks the page.

The loop is staleness-driven (the deps pattern): every 30 seconds it checks
whether the last cycle is older than `poll_interval` (default 5m), so a
laptop waking from sleep refreshes promptly and a failing cycle retries at
the interval, not the heartbeat.

### Multi-host auth (gh-minted tokens)

prs talks to github.com and GitHub Enterprise hosts alike, using whatever
`gh auth login` sessions already exist: a token per host is minted with
`gh auth token --hostname <host>` on first use and cached in memory. No
token is ever written to config or disk. A 401 drops the cached token and
re-mints once; a mint failure (host not logged in) is cached for a minute so
a dead host costs one gh exec per minute, not one per repo. Enterprise hosts
resolve to `https://<host>/api/v3/`.

## Data model & storage

```go
PR{
  Host, Repo,        // github.com, org/name
  Number, Title, URL,
  Author,            // GitHub login
  CreatedAt, UpdatedAt,
  Labels,
  ReviewDecision,    // APPROVED | CHANGES_REQUESTED | "" (no standing review)
}

RepoState{
  Host, Repo,
  PRs,               // the open, non-draft PRs
  FetchedAt,
  Error,             // last fetch error; previous PRs are kept beside it
  Removed,           // tombstone for a repo no longer discovered locally
}
```

Persisted as an append-only JSONL log, `repos.jsonl` under the workdir
(default `~/.local/share/prs`, overridable with `PRS_WORKDIR`): one line per
repo state change, and on load the newest record per repo wins (greater
`fetched_at`, a tie goes to the later line) — the same last-wins semantics
kof and wire use. Two properties keep the file small: a cycle where a repo's
content did not change appends nothing, and startup compacts the log to one
line per repo once superseded lines pile up. A torn final line (an append
interrupted by a crash) is truncated away at startup; a mid-file parse error
is fatal and names the file. The log is purely a cache of upstream GitHub
state — deleting it costs one cold poll cycle.

## Build / install / test

```bash
make build    # ./prs binary (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/prs + adhoc codesign
make test     # go test ./...  (hermetic: t.TempDir + httptest fakes, never touches GitHub)
```

## HTTP API

Owned by `prs serve`:

- `GET  /`                 : webkit-chromed web page, read-only list with filters
- `GET  /app.js`           : the page's client script
- `GET  /api/prs?repo=&author=&review=&sort=` : the filtered PRs plus facets
  (distinct repos/authors, the dropdown options) and the cache status;
  `review` is APPROVED | CHANGES_REQUESTED, `sort` is newest (default) | oldest
- `GET  /api/status`       : cache freshness, per-repo fetch errors, and the
  config in effect (dirs, excludes, hosts, interval)
- `POST /api/refresh`      : one synchronous poll cycle → `{repos, open_prs, errors, polled_at, duration_ms}`
- `GET  /healthz`          : 204
- `GET  /version`          → the four-key build metadata object (`version`, `commit`, `tag`, `build_time`)
- `GET  /webkit/`          : shared chrome (asset drift checks use `GET /webkit/version`)

Empty lists serialize as `[]`, never `null`. Invalid `sort`/`review` values
are a 400 naming the valid set.

## Commands

CLI surface beyond `serve`/`mcp`, wired as thin HTTP clients to `prs serve`
(`internal/client`):

```bash
prs serve --port 7427 --workdir ~/.local/share/prs [--config ~/.config/prs/config.yaml]
prs mcp
prs list [--repo org/name] [--author login] [--review APPROVED|CHANGES_REQUESTED] \
         [--sort newest|oldest] [--json]
prs refresh [--json]        # force one poll cycle, print the summary
prs status [--json]         # cache freshness, errors, config in effect
prs docs                    # print the embedded operating doc
prs doctor                  # serve reachable, store readable, version skew, config, gh
prs version [-o json]
```

`prs version` prints the bare git commit that built the binary, the token
sibling tools also print so ralph and status can probe any of them for the
build they are running; `-o json` prints the full build metadata object.

## MCP tools

Thin client over the API above (`internal/client`), served on stdio by
`prs mcp`:

- `prs_list(repo?, author?, review?, sort?)`: the cached open PRs, filtered;
  the response carries the distinct repos/authors (the valid filter values)
  and the cache status including per-repo fetch errors
- `prs_refresh()`: force one synchronous poll cycle; returns repo/PR/error counts
- `prs_status()`: cache freshness, per-repo errors, and the config in effect
- `prs_doctor()`: run the same checks as `prs doctor` and return the report as
  JSON — for a client that can call a tool but has no shell

Tool responses include `url` (the human-facing `PRS_BASE_URL`, e.g.
`http://prs.this`), while the client itself calls `localhost:<PRS_PORT>`.
Both env vars pin where the MCP looks, the same way kof's do.

## Shared UI: webkit

The web page chrome (`<wk-header>` + theme/font/size controls) and components
(`<wk-card>`, `<wk-badge>`, `<wk-page-header>`, `<wk-title>`, `<wk-callout>`)
come from the in-module package **`github.com/mad01/thismoon/webkit`**,
mounted at `GET /webkit/` via `webkit.Mount(mux)` and loaded by
`internal/server/shell.html` (which pulls the FOUC guard from
`/webkit/boot.js`). Don't re-add palette/topbar/theme CSS locally; it lives
in webkit only.

prs's web page is **read-only**: it lists the open PRs with a repo filter, an
author filter, a newest/oldest sort toggle, and a reset button, each PR
linking out to GitHub. Acting on a PR happens on GitHub. prs is a webkit
consumer like the other services in this repo: no pin or bump step, so a
webkit change ships at the next build.

```bash
make install && t-man restart prs
```

## Live testing against real PRs

`github.com/mad01/prs-testbed` (private) holds one PR per state — open,
draft, closed, merged — so the poller can be validated against the real API:
exactly the open PR may appear. The repo is **excluded by default** via a
built-in glob; a test config opts back in with:

```yaml
include:
  - mad01/prs-testbed
```

Single-account limitation: GitHub forbids reviewing your own PR, so the
APPROVED / CHANGES_REQUESTED reduction cannot be exercised there; it is
covered by unit tests against a fake API (`internal/github/client_test.go`).

## Gotchas

- **Wave 0 builder.** Builds before the consuming repo's `claude-mcp` recipe
  (wave 1) registers the MCP.
- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `prs docs`): serve-must-be-running, store layout, failure
  modes, version-skew checks. Keep those facts there, not here.
- **gh is the only credential path.** serve execs `gh auth token` — the gh
  binary must be reachable from serve's environment (PATH or the Homebrew
  locations). A host without a gh login shows up as per-repo errors naming
  `gh auth login --hostname <host>`.
- **No tokens in config.** The YAML holds dirs/excludes/hosts/interval only;
  auth state lives in gh's own keychain.
- **Codesign for the binary.** `make install` strips xattrs and re-signs
  (macOS kills adhoc-signed binaries with drifted provenance).
- **Version probe convention.** `GET /version` and `prs version -o json`
  both return the shared four-key build metadata object from
  `github.com/mad01/thismoon/buildinfo`. Plain `prs version` stays a bare
  token — status parses it as one.

## See also

- Recipe: `recipes/prs/recipe.toml` (+ `recipes/prs/CLAUDE.md`)
- Human docs: `README.md`
- The `prs.this` d-man route, the `claude-mcp` `servers.json` MCP
  registration, and the machine's real `~/.config/prs/config.yaml` live in
  the consuming repo's private overlay (docs/adr/0006), not here.
