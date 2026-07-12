# status, status page for t-man-managed services

Go CLI serving a status-page-style dashboard at `http://status.this/` (port
7426). Discovers every t-man-managed launchd job, probes it once a minute,
and draws a 30-day uptime bar strip per service. No incident tracking, just
running state plus history.

## Module layout

```
status/
  cmd/status/          entrypoint (delegates to internal/cli)
  internal/
    cli/               cobra: serve, version (Version via ldflags)
    discover/          plist scan + d-man routes.toml link mapping
    check/             HTTP and launchctl PID probes
    crashloop/         restart-counter tracker behind crash-loop alerts
    history/           day-bucketed JSON uptime store
    notify/            events.this emit + macOS banner (best-effort, per-tool copy)
    server/            Monitor (poller), handlers, embedded shell.html + app.js
  Makefile             part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### Discovery

Every cycle scans `~/Library/LaunchAgents` and `/Library/LaunchDaemons` for
plists with `TManMetadata.ManagedBy == "t-man"`. No service list to
maintain: a new `t-man add` shows up on the next cycle.

### Probing

Check type follows from the plist: a `--port N` in ProgramArguments means an
HTTP probe of `http://127.0.0.1:N/` (up = any response below 500; a 404 from
speak-tts still proves the process serves). No port means `launchctl print
gui/<uid>/<label>` (or `system/<label>` for daemons, which works without
root) and looking for a `pid =` line.

### Links

d-man's `~/.config/d-man/routes.toml` maps backend port to route name, so
each web service's name links to its `.this` page (`--routes` flag to
override).

### Metadata

For up HTTP services, `GET /version` and `GET /webkit/version` every 10
minutes, best-effort; services without the endpoints just show no version.
The same refresh also runs `<binary> version` on the plist's binary to learn
the sha installed on disk.

### Version drift

A running process serving an older sha than its on-disk binary means an
install happened without a restart (the "ralph reports ok, old binary keeps
running" failure). When running and installed shas disagree, the service is
re-probed every cycle (the 10-minute meta cadence is bypassed) and the
mismatch must hold for 2 consecutive cycles before it counts — a mismatch
observed mid-`ralph up` clears instead of alerting. Confirmed drift shows
"Stale binary" (amber) on the dashboard, fires one coalesced macOS banner,
and records a `warn` event with `running`/`installed` tags; recovery records
an `info` event. Services whose binary can't answer `version` (or without a
`/version` endpoint) never count as drifted.

### Crash-loop detection

Every cycle also samples launchd's cumulative `runs` counter (`launchctl
print`). A service that respawns 50+ times (`--restart-threshold`) within 1h
(`--restart-window`) gets one coalesced macOS notification plus an `error`
event on events.this, then a 6h per-service cooldown before re-alerting. A
counter drop (job re-registered via bootout/bootstrap) resets the baseline
instead of going negative. This catches both real crash loops and the BTM
"spawn scheduled" EX_CONFIG wedge that hits KeepAlive agents after their
binary is replaced with a new ad-hoc signature.

## Data model & storage

Day buckets (`{ok, fail}` counts) per service live in
`~/.local/share/status/history.json` (`--workdir`), written atomically each
cycle, pruned at 90 days, 30 shown. Bar colors: >= 99.5% green, >= 95% amber,
below that red; a day without recorded checks renders gray.

## Build / install / test

```bash
make build    # ./status binary
make install  # build + cp to ~/code/bin/status + adhoc codesign
make test     # go test ./...
```

## HTTP API

- `GET /`: the dashboard (no-store; reloads itself every 60s)
- `GET /api/status`: full snapshot as JSON
- `GET /healthz`: 204
- `GET /version`: `{"version":"<sha>"}` (ldflags, same convention as the other services)
- `GET /webkit/`: shared chrome from the in-module `github.com/mad01/thismoon/webkit` package

## Gotchas

- **status monitors itself.** Once registered, its own plist is discovered like
  any other; expect a `status` row on the page.
- **History gaps are visible.** If the agent is stopped for a day, that day
  renders gray (no data), not red; absence of checks isn't downtime.
- **t-man's plist `Version` field is t-man's build sha,** not the service's.
  Service versions come from the HTTP `/version` probe (running) and the
  binary's own `version` command (installed).
- **`t-man stop` won't show as Down.** KeepAlive services relaunch within a
  second, faster than any poll interval. Down means a real failure: crash
  loop, hung process, port not answering, or `t-man remove`.

## See also

- Recipe: `recipes/status/recipe.toml` (+ `recipes/status/CLAUDE.md`)
- Route: dotfiles `recipes/d-man/routes.toml` (`status` → 7426; machine overlay stays in dotfiles)
