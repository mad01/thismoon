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
internal/cli/                cobra commands (root, add, list, remove, control, logs, version — build metadata from the shared buildinfo package)
internal/service/            Definition struct, Hash(), Manager interface
internal/platform/launchd/   plist generation, launchctl wrapper, the launchd Manager
internal/reconcile/          read-compare-apply reconciler + state comparison
```

Package: `github.com/mad01/thismoon/tools/t-man`, part of the thismoon
monorepo module; it has no go.mod of its own.

## How it works

### How change detection works

`internal/reconcile/reconciler.go` runs read-compare-apply on every `add`:

1. **Read** the existing plist from disk and reconstruct a `service.Definition`.
2. **Compare** `current.Hash()` against `desired.Hash()`. The hash is the
   SHA256 of the definition marshalled to JSON (`internal/service/definition.go`),
   so every field participates: command, args, env, workdir, log paths,
   sandbox profile digest, extra logs.
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
| `list` | `ls` | List managed services (NAME / STATUS / COMMAND) |
| `remove N` | `rm`, `delete` | Unload and delete a service |
| `start N` / `stop N` / `restart N` | | Control a running service |
| `status N` | | Detailed info for one service |
| `logs N` | | Tail stdout + stderr (and named extra logs) |
| `logs sandbox [N]` | | Collect `sandbox`/`sandbox-*` extra logs across services |
| `version [-o json]` | | Print the build SHA, or the full build metadata object |

Global persistent flags (all commands): `--agent` (default true), `--daemon`
(requires root, mutually exclusive with `--agent`), `--dryrun`.

`add` flags: `--name` (required), `--desc`, `--workdir`, `--env KEY=VALUE`
(repeatable), `--path` (colon-separated PATH additions), `--logs DIR`,
`--sandbox-profile PATH.sb`, `--extra-log NAME=PATH` (repeatable). `RunAtLoad`
and `KeepAlive` are always set to true on the generated plist.

## Gotchas

- **No declarative manifest.** t-man builds a `Definition` from `add` flags;
  there is no YAML/JSON service file it reads at runtime. `service-info.yaml`
  in this directory is catalog metadata, not service config.
- **`make build` writes to this directory** (`tools/t-man/t-man`), not `bin/`.
  `.gitignore` excludes `/t-man`.
- **Editing a sandbox `.sb` profile changes the hash.** The profile's content
  digest feeds `Definition.Hash()`, so the next `t-man add` re-renders the
  plist and bounces the service. An unchanged re-add stays a no-op.
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

- `README.md`: user-facing reference and quick start.
- `docs/architecture.md`: how t-man wraps launchd; the reconcile loop.
- `docs/agents-and-daemons.md`: agent vs daemon, the one-time daemon setup,
  the `--port` convention.
- `docs/troubleshooting.md`: a service that won't stay up; debugging with
  launchctl directly.
- `docs/working-on-t-man.md`: build, run, test, and debug from source.
