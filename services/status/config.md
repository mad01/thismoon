# status configuration

## Where config lives

status has no config file. The whole surface is persistent command-line
flags on the `status` command, each with an environment-variable fallback for
the two that matter to a launchd agent (port and store location). Precedence
is flag, then environment variable, then built-in default.

The list of services status checks is not configuration at all: every cycle
it scans `~/Library/LaunchAgents` and `/Library/LaunchDaemons` for plists
with `TManMetadata.ManagedBy == "t-man"`. There is no service list to edit —
a new `t-man add` shows up on the next probe cycle, and there is no flag or
env var to point the scan at different directories.

## Flags

These are persistent flags on the root command, so `serve`, `doctor`, `docs`,
and `version` all accept them and resolve them identically. That matters for
`doctor`: it probes the port and reads the workdir, so it has to be told the
same values `serve` was. They were `serve`-only in earlier releases, where
`status --port 9999 doctor` was rejected as an unknown flag and a bare
`doctor` diagnosed the compiled-in port no matter which one serve listened
on.

- `--port` (int, default `7426`, env `STATUS_PORT`): port the HTTP server
  listens on.
- `--workdir` (string, default `~/.local/share/status`, env
  `STATUS_WORKDIR`): directory holding `history.json`, the uptime store.
- `--interval` (duration, default `1m`): how often the poller probes every
  discovered service.
- `--routes` (string, default `~/.config/d-man/routes.toml`): the d-man
  routes file used to map a service's port to its `.this` link on the
  dashboard. The default comes from d-man's own `DefaultRoutesPath`
  constant, so the two cannot drift. No env var; `--routes` is the only
  override.
- `--restart-window` (duration, default `1h`): the window over which
  launchd respawns are counted per service for crash-loop detection.
- `--restart-threshold` (int, default `50`): respawns within
  `--restart-window` that trigger a crash-loop alert.

A leading `~` in `--workdir` or `--routes` is expanded at runtime (launchd
agents don't run through a shell, so nothing else expands it). A `~` that
cannot be expanded — no resolvable home directory, which a stripped launchd
environment produces — is an error at startup. Earlier releases stripped the
`~` and used a path relative to the working directory instead, which quietly
wrote uptime history nobody reads. An unparseable `STATUS_PORT` warns once on
stderr and falls back to the compiled-in default.

To see what a given environment actually resolved to, run `status doctor` —
with the same flags you would give `serve`, now that they are accepted
there.

### Fixed values (not configurable)

`internal/server.Options` carries a few more fields that `status serve` never
wires to a flag; callers only ever see their built-in defaults:

- `MetaInterval` (`10m`): how often the poller re-fetches `/version` and the
  on-disk binary's `version` command for drift detection.
- `HistoryDays` (`30`): days of uptime shown on the dashboard.
- `KeepDays` (`90`): days of uptime kept in `history.json` before pruning.
- `RestartCooldown` (`6h`): minimum gap between crash-loop alerts for the
  same service.
- `AgentsDir` / `DaemonsDir` (`~/Library/LaunchAgents`,
  `/Library/LaunchDaemons`): where the launchd scan looks for plists.

## Environment variables

- `STATUS_PORT`: fallback for `--port`.
- `STATUS_WORKDIR`: fallback for `--workdir`.
- `EVENTS_BASE_URL` (default `http://127.0.0.1:7430`): base URL of the local
  events service that status posts `warn`/`info` events to for stale-binary
  and crash-loop detection. The post is best-effort and fire-and-forget: if
  the events service is down or `EVENTS_BASE_URL` points nowhere, the event
  is dropped silently and status keeps running. Read by the shared
  `kit/notify` package, not exposed as a flag.

## Example

```bash
STATUS_PORT=7426 STATUS_WORKDIR=~/.local/share/status status serve \
  --interval 1m \
  --routes ~/.config/d-man/routes.toml \
  --restart-window 1h \
  --restart-threshold 50
```

Equivalently, with every value left at its default:

```bash
status serve
```
