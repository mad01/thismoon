# status

A Go CLI that serves a status dashboard for all t-man-managed local services
at `http://status.this/` (port 7426).

It discovers t-man services by scanning launchd plist directories, probes each
one once a minute, and draws a 30-day uptime bar strip per service. No
incident tracking — just running state and history.

## Install

```bash
make install   # builds and installs ~/code/bin/status (adhoc codesigned on macOS)
```

Or via ralph — it ships from the thismoon monorepo:

```bash
ralph up   # builds status and registers the t-man agent
```

## Run

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
| `GET /version` | `{"version":"<sha>"}` build sha |
| `GET /webkit/` | Shared chrome from the in-module `webkit` package |

## History store

Day-bucketed uptime counts live in `~/.local/share/status/history.json`.
The store is written atomically each probe cycle and pruned at 90 days; the
dashboard shows the most recent 30 days.

Bar colors: green (≥ 99.5%), amber (≥ 95%), red (below 95%), gray (no data
recorded that day — not downtime, just no checks).

Removing the t-man agent does not delete history.

## Develop

```bash
make test           # go test ./...
make build          # ./status
```
