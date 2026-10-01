# t-man, declarative launchd agent/daemon manager

Agent-facing working context for t-man. Read this before changing code or
reasoning about the services that run on top of it.

t-man is a Go CLI that manages macOS launchd services declaratively. You
describe a service once with `t-man add`; t-man writes the plist, loads it
through `launchctl`, and on the next `add` only touches launchd if the
configuration actually changed. Change detection is a SHA256 hash of the
service definition stored inside the plist, so there is no separate state file.

It manages two kinds of services:

- **Agents** (default): per-user LaunchAgents in `~/Library/LaunchAgents`,
  no root needed.
- **Daemons** (`--daemon`): system LaunchDaemons in `/Library/LaunchDaemons`,
  require `sudo`.

t-man is CLI-compatible with [serviceman](https://github.com/therootcompany/serviceman)
and adopts serviceman plists for migration.

## Module layout

```
cmd/t-man/main.go            entry point → cli.Execute()
internal/cli/                cobra commands (root, add, list, top, remove, control, logs, version — build metadata from the shared buildinfo package)
internal/service/            Definition struct, Hash(), Manager interface; schedule parsing (schedule.go) and next-run math (nextrun.go)
internal/platform/launchd/   plist generation, launchctl wrapper, the launchd Manager
internal/procstat/           PID → RSS/CPU%/uptime via one ps call (launchd-agnostic)
internal/reconcile/          read-compare-apply reconciler + state comparison; emits events via the shared kit/notify
Makefile                     part of module github.com/mad01/thismoon (no own go.mod)
```

## How it works

### How change detection works

`internal/reconcile/reconciler.go` runs read-compare-apply on every `add`:

1. **Read** the existing plist from disk and reconstruct a `service.Definition`.
2. **Compare** `current.Hash()` against `desired.Hash()`. The hash is the
   SHA256 of the definition marshalled to JSON (`internal/service/definition.go`),
   so every field participates: command, args, env, workdir, log paths,
   sandbox profile digest, extra logs, schedule.
3. **Apply** only on a mismatch: write the new plist to a temp file in the
   same directory, `launchctl unload` the old one, atomically rename the temp
   file into place, `launchctl load -w`. If the reload fails, restore the old
   plist content and reload that.

The stored hash lives in the plist under `TManMetadata.Hash`. A marker comment
`<!-- Managed by t-man - DO NOT EDIT MANUALLY -->` plus `TManMetadata.ManagedBy`
let `list`/`status` filter to t-man-managed plists.

### How t-man calls launchctl

`internal/platform/launchd/launchctl.go` shells out to the legacy launchctl
verbs only: `load -w`, `unload`, `start`, `stop`, `list`, and
`print gui/<uid>/<label>` for status (falling back to `list`). It doesn't use
`bootstrap`/`bootout`/`kickstart`.

### The `--port` convention (consumers, not t-man)

t-man has no `--port` flag of its own. When a service command takes
`--port N` as one of *its* arguments, that argument lands in the plist's
`ProgramArguments`. The `status` service in this repo reads each
plist's `ProgramArguments`, and if it finds `--port`, it HTTP-probes the
service on that port; otherwise it falls back to a launchctl PID check. So
"give a service a `--port`" is a convention the surrounding tooling relies on,
not something t-man parses or enforces.

## Build / install / test

```bash
make build          # → ./t-man  (NOT ./bin/t-man; the binary lands in this directory)
make install        # build + copy to ~/code/bin + xattr/codesign on Darwin
make test           # go test ./... -timeout 30s
make lint           # golangci-lint run ./...
make fmt            # golines (100 cols, gofumpt base)
```

`make build` injects the build metadata into
`github.com/mad01/thismoon/buildinfo` via `-ldflags` from `../../buildinfo.mk`:
the short sha, the full commit, the newest `t-man/v*` tag, and the build time.
`t-man version` prints the bare version token; `t-man version -o json` prints
the four-key object. The same token is what `GetVersion()` stamps into a
managed plist's `TManMetadata.Version`.

`make install` builds and copies the binary to `~/code/bin/t-man`, signing
with the "mad01 Local Signing" identity (ad-hoc fallback). On consuming
machines ralph builds it via `recipes/t-man/` from the sources cache.

## Commands

| Command | Aliases | What it does |
|---------|---------|--------------|
| `add --name N -- CMD [args]` | | Create or update a service (idempotent) |
| `list` | `ls` | List managed services (NAME / STATUS / COMMAND); `--resources` adds PID / RSS / CPU% / UPTIME; NEXT RUN / LAST RUN / EXIT columns appear when any listed job is scheduled |
| `top` | | Live resource view of managed services, sorted by RSS; `--interval` (default 2s), Ctrl-C quits |
| `remove N` | `rm`, `delete` | Unload and delete a service |
| `start N` / `stop N` | | Control a running service (launchctl start/stop) |
| `restart N` | | Re-register the service: unload + load, not stop/start — this is what clears the launchd "spawn scheduled" EX_CONFIG wedge after a binary replacement |
| `run N` | | Fire a scheduled job once, now (`launchctl start`); refuses a long-lived service and points to `start` |
| `status N` | | Detailed info for one service; a scheduled job also gets its schedule, next run, last run, and last exit code |
| `logs N` | | Tail stdout + stderr (and named extra logs) |
| `logs sandbox [N]` | | Collect `sandbox`/`sandbox-*` extra logs across services |
| `docs` | | Print the embedded operating doc (runtime debugging for supervised services) |
| `version [-o json]` | | Print the build SHA, or the full build metadata object |

Global persistent flags (all commands): `--agent` (default true), `--daemon`
(requires root), `--dryrun`. `--agent` and `--daemon` are one choice spelled
two ways — `internal/cli/root.go` collapses them into `daemonMode` in
`PersistentPreRunE`, so `--agent=false` means daemon and a pair asserting the
same value (`--agent --daemon`) is an error.

`add` flags: `--name` (required), `--desc`, `--workdir`, `--env KEY=VALUE`
(repeatable), `--path` (colon-separated PATH additions), `--logs DIR`,
`--sandbox-profile PATH.sb`, `--extra-log NAME=PATH` (repeatable), and the
schedule trio `--schedule [days@]HH:MM[,...]` (days: a weekday, a range
such as `mon-fri` or the wrapping `fri-mon`, or `weekdays`, `weekend`,
`daily`), `--calendar minute=0,hour=7[,day=1,weekday=1,month=1]` (repeatable;
any field may be a range such as `hour=9-17`, expanded as a cross product
capped at 200 entries), `--every DURATION`.
Without a schedule the plist gets `RunAtLoad` and `KeepAlive` true (a
long-lived service); with one it gets both false and a
`StartCalendarInterval` array or a `StartInterval` (a scheduled job).
`--schedule` and `--calendar` both add calendar entries and combine;
`--every` excludes them. All three parse in `internal/service/schedule.go`,
the one place any future config reader should call too. Sets and ranges
expand to one entry per day (launchd takes one Weekday per dict).
`service.NormalizeCalendar` sorts, dedupes, and writes Sunday as 0, so the
same schedule spelled two ways hashes the same. `apply` in
`internal/cli/schedule.go` normalizes the union of both flags. Display
folding (`mon-fri 07:30` for five entries) lives in
`internal/service/schedulefmt.go` and never feeds the hash.

## Gotchas

- **Runtime debugging lives in `operating.md`** (embedded in the binary,
  printed by `t-man docs`). It covers where supervised services' logs land,
  the crash-loop first moves, the stale-binary-after-rebuild check, and why
  a service can be missing from `list`. Keep those facts there, not here.
- **No declarative manifest.** t-man builds a `Definition` from `add` flags;
  there is no YAML/JSON service file it reads at runtime. `service-info.yaml`
  in this directory is catalog metadata, not service config.
- **`make build` writes to this directory** (`tools/t-man/t-man`), not `bin/`.
  `.gitignore` excludes `/t-man`.
- **Editing a sandbox `.sb` profile changes the hash.** The profile's content
  digest feeds `Definition.Hash()`, so the next `t-man add` re-renders the
  plist and bounces the service. An unchanged re-add stays a no-op.
- **A scheduled job's idle state is `scheduled`, not down.** The only place
  t-man decides a service is down is the status mapping in
  `internal/platform/launchd/schedule.go` (`runStateFor`). It rewrites
  launchd's stopped/error/waiting to `scheduled` when the definition has a
  schedule and no live PID. There is no restart loop in t-man to gate:
  KeepAlive is launchd's, and `Validate()` refuses it on a scheduled job.
- **Last run is a proxy.** launchd keeps no run timestamp, so `LAST RUN` is
  the newest mtime of the job's stdout/stderr files
  (`Definition.LastLogWrite`); a run that writes nothing leaves it
  unchanged. Last exit code comes from `launchctl print` (`last exit code =`)
  or the `launchctl list` status column. Next run for an interval job is
  unknowable without launchd's load time, so it prints the cadence.
- **launchd runs one missed calendar slot after wake** and coalesces several
  into one run. Jobs must be idempotent; the README says so to users.
- **Daemon mode needs root for every command**, not just `add`: `checkSudo()`
  guards `add`, `remove`, `list`, `logs`, and the control commands when
  `--daemon` is set.
- **t-man only manages plists it recognises**: its own (via `TManMetadata`)
  or serviceman's (via the `Generated for serviceman` marker). It ignores
  unrelated plists in the LaunchAgents/LaunchDaemons directories.
- **Version probe convention.** `t-man version -o json` returns the shared
  four-key build metadata object (`version`, `commit`, `tag`, `build_time`,
  every key present and `""` when unknown) from
  `github.com/mad01/thismoon/buildinfo`, the same shape services serve from
  `GET /version`. Plain `t-man version` stays a bare token — status parses it
  as one. t-man is CLI-only, so it has no `/version` endpoint.

## See also

- Recipe: `recipes/t-man/recipe.toml` — package build/install only (wave 0, no service block: t-man is the tool other recipes register services *with*, not a service itself). t-man is a platform foundation (docs/adr/0006): every service recipe in this repo hard-depends on `packages.t_man`.
- `README.md`: user-facing reference and quick start.
- `docs/architecture.md`: how t-man wraps launchd; the reconcile loop.
- `docs/agents-and-daemons.md`: agent vs daemon, the one-time daemon setup,
  the `--port` convention.
- `docs/troubleshooting.md`: a service that won't stay up; debugging with
  launchctl directly.
- `docs/working-on-t-man.md`: build, run, test, and debug from source.
