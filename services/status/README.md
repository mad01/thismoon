# status

A Go CLI that serves a status dashboard for all t-man-managed local services
at `http://status.this/` (port 7426).

## How it works

It discovers t-man services by scanning launchd plist directories, probes each
one once a minute, and draws a 30-day uptime bar strip per service. No
incident tracking, just running state and history.

It also catches stale binaries: for each web service it compares the running
process's `/version` sha with the sha the binary on disk reports (`<binary>
version`). When they disagree for two consecutive cycles the service shows
"Stale binary" on the dashboard, fires a macOS banner, and records a `warn`
event — the "ralph reported ok but the old binary kept running" failure made
visible.

Each service card also shows the release tag and build time of the running
build, when the service reports them on `/version`. Services that only serve
a bare version sha show just the sha.

## Install

```bash
make install   # builds and installs ~/code/bin/status (adhoc codesigned on macOS)
```

Or via ralph, since it ships from the thismoon monorepo:

```bash
ralph up   # builds status and registers the t-man agent
```

## Usage

`status serve` runs as a launchd user agent via t-man:

```bash
status serve --port 7426
```

```bash
t-man status status      # check the agent
t-man restart status     # restart after a binary rebuild
t-man logs status        # stdout logs
t-man logs status --stderr
```

The dashboard refreshes itself every 60 seconds; visit `http://status.this/`
or `http://localhost:7426/` (on hosts without d-man).

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `7426` | Listen port |
| `--workdir` | `~/.local/share/status` | History store location |
| `--routes` | `~/.config/d-man/routes.toml` | d-man routes file for .this links |

## Endpoints

| Path | Description |
|------|-------------|
| `GET /` | Dashboard (no-store, auto-refreshes every 60s) |
| `GET /api/status` | Full snapshot as JSON |
| `GET /healthz` | 204 |
| `GET /version` | Build metadata: `version`, `commit`, `tag`, `build_time` |
| `GET /webkit/` | Shared chrome from the in-module `webkit` package |

## Where things live

Day-bucketed uptime counts live in `~/.local/share/status/history.json`.
status writes the store atomically each probe cycle and prunes it at 90
days; the dashboard shows the most recent 30 days.

Bar colors: green (>= 99.5%), amber (>= 95%), red (below 95%), gray (no data
recorded that day; not downtime, just no checks).

Removing the t-man agent doesn't delete history.

## Develop

```bash
make test           # go test ./...
make build          # ./status
```

## Docs

- [architecture](architecture.md): internal structure and data flow
- [operating](operating.md): runtime behavior, failure modes, first moves
- [why](why.md): why this component exists
- [config](config.md): configuration reference
