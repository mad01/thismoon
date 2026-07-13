# status architecture

## Overview

status is one Go process, `status serve`, run as a launchd agent under t-man.
It listens on 127.0.0.1:7426 and is reached as `http://status.this/` through
d-man. Inside it, two halves share a snapshot: a poller (`Monitor`) that
discovers, probes, and records every t-man-managed service once a minute, and
HTTP handlers that serve the latest snapshot to the dashboard. The boundary
is observation only; starting, restarting, and removing services stays with
t-man, and status never probes anything that is not a local t-man-managed
launchd job.

## Structure

```
cmd/status/          entrypoint, delegates to internal/cli
internal/cli/        Cobra commands: serve, version; flag and env defaults
internal/discover/   launchd plist scan + d-man routes.toml link mapping
internal/check/      HTTP and launchctl PID probes, on-disk version lookup
internal/crashloop/  restart-counter tracker (window, threshold, cooldown)
internal/history/    day-bucketed JSON uptime store
internal/notify/     events emit + macOS banner, both best-effort
internal/server/     Monitor poll loop, handlers, embedded shell.html + app.js
```

`internal/server` embeds a chrome-only `shell.html` and `app.js` and mounts
the shared webkit chrome with `webkit.Mount(mux)`; `app.js` builds the dashboard body in the browser from
`GET /api/status`, the client-side rendering model of docs/adr/0005.

## Data flow

The probe cycle is `Monitor.cycle`, driven by a ticker at `--interval`
(default one minute). Each cycle: `discover.Scan` reads the launchd plist
directories for `TManMetadata.ManagedBy == "t-man"`, so a new `t-man add`
appears with no list to maintain. Every discovered service is probed
concurrently by `check.Service` (an HTTP GET against `127.0.0.1:<port>` when
the plist has a `--port`, otherwise a `launchctl print` PID check) while
`check.Runs` samples launchd's cumulative respawn counter. Outcomes are
recorded into the history store, pruned at 90 days, and saved.

Still inside the cycle, `refreshMeta` fetches `/version` and
`/webkit/version` from up HTTP services and runs the installed executable's
own `version` command, normally every 10 minutes but every cycle while the
two shas disagree. `countDrift` requires the mismatch to hold for two
consecutive cycles before `Drift` flags, so a mismatch seen mid-install
clears instead of alerting. The cycle then builds a sorted `Snapshot` (routed
web services first, then other HTTP services, then PID-checked jobs) and
swaps it in under the lock. The cycle closes by comparing against the
previous snapshot: up/down flips and drift flips each emit an event through
`internal/notify`, drift and crash-loop alerts also post one coalesced macOS
banner, and the respawn counters feed `crashloop.Tracker`, which fires at 50
restarts within an hour and then holds a 6-hour per-service cooldown.

Requests are the short half: `GET /` serves the shell, `app.js` fetches
`GET /api/status`, and the handler encodes `Monitor.Snapshot()` as JSON.

## Storage

One file: `history.json` under `--workdir` (default `~/.local/share/status`),
a JSON map of service label to date to `{ok, fail}` counts. Each cycle
writes it atomically and prunes it at 90 days; the dashboard renders the
most recent 30, with days lacking data drawn gray rather than red. In-memory
state (current snapshot, version metadata, drift and crash-loop counters)
does not survive a restart, only the day buckets do.

## Interfaces

Web: `GET /` (dashboard shell, no-store), `GET /app.js`, `GET /api/status`
(full snapshot as JSON), `GET /healthz` (204), `GET /version`
(ldflags-injected sha), and `/webkit/*`.

CLI: `status serve` with `--port` (7426, env `STATUS_PORT`), `--interval`,
`--workdir` (env `STATUS_WORKDIR`), `--routes`, `--restart-window`, and
`--restart-threshold`; plus `status version [-o json]`.

There is no config file; flags and environment variables are the whole
surface. status reads two external files it does not own: the launchd plist
directories for discovery, and d-man's `~/.config/d-man/routes.toml`
(overridable via `--routes`) to link each web service to its `.this` page.
