# t-man architecture

## Overview

t-man is a tool that manages macOS launchd services declaratively. One
`t-man add` describes a service; t-man renders the plist, loads it through
`launchctl`, and on later runs only touches launchd when the definition
actually changed. It manages per-user LaunchAgents by default and system
LaunchDaemons with `--daemon` (root required for every command in that mode).
The boundary is launchd: t-man generates plists and drives the system
`launchctl`, and it only ever touches plists it recognises as its own (via
`TManMetadata`) or as serviceman's. A longer walkthrough of the reconcile
loop lives in `docs/architecture.md`; this file is the component-level map.

## Structure

Four layers, top to bottom, plus two side Go packages:

```
cmd/t-man/                 entry point, hands off to internal/cli
internal/cli/              cobra commands: parse flags, build a Definition
internal/reconcile/        read-compare-apply: decide create/update/delete/none
internal/service/          the Definition struct, its Hash(), the Manager interface
internal/platform/launchd/ plist rendering (howett.net/plist) and the launchctl wrapper
internal/notify/           best-effort event emission to the local events service
```

The CLI layer never calls launchctl directly. `Manager` is an interface and
the launchctl client takes an injectable command runner, which keeps the core
logic testable without a live launchd.

## Data flow

An `add` builds a `service.Definition` entirely from flags (there is no
manifest file) and hands it to `reconcile.Reconciler`, which runs
read-compare-apply:

1. Read the existing plist for that name, if any, back into a Definition.
2. Compare `current.Hash()` against `desired.Hash()`. The hash is the SHA256
   of the JSON-marshalled struct, so every field participates, including the
   sandbox profile's content digest. The stored hash lives in the plist under
   `TManMetadata.Hash`; there is no separate state file to drift.
3. Apply only on a mismatch. The update path in
   `internal/platform/launchd/manager.go` renders the new plist to a temp
   file, `launchctl unload`s the old one, atomically renames the temp file
   into place, and `launchctl load -w`s it; if the load fails it restores the
   old plist and reloads that.

Applied changes and failures POST an event to the local events service via
`emitReconcile` in the reconciler; no-op reconciles stay silent. The control
commands (`start`, `stop`, `restart`, `status`, `list`, `logs`, `remove`) go
through the same Manager, which shells out to the legacy launchctl verbs only:
`load -w`, `unload`, `start`, `stop`, `list`, and `print gui/<uid>/<label>`
with a `list` fallback.

## Storage

- `~/Library/LaunchAgents/<label>.plist` (agent mode) or `/Library/LaunchDaemons/<label>.plist` (daemon mode): the generated plist, carrying the `TManMetadata` dict (hash, ManagedBy, version) and a `DO NOT EDIT MANUALLY` marker comment. This plist is the single source of truth; t-man keeps no state elsewhere.
- `~/Library/Logs/<name>/stdout.log` and `stderr.log` (agent mode) or `/var/log/<name>/` (daemon mode): the service's log files, created by launchd at the paths the plist points at. `--logs` overrides the directory and `--extra-log NAME=PATH` registers extra files for `t-man logs --source`.

t-man itself writes only plists; the services it manages write the log files.

## Interfaces

CLI commands: `add --name N [flags] -- CMD [args]` (idempotent
create-or-update), `list`, `remove`, `start`/`stop`/`restart`, `status`,
`logs` (with `--stdout`/`--stderr`/`--source` and `-f`), `logs sandbox`, and
`version` (bare token, or the shared four-key build metadata object with
`-o json`). Global flags: `--agent` (default), `--daemon`, `--dryrun`. The
`add` syntax is CLI-compatible with serviceman, and t-man adopts serviceman
plists for migration.

There is no config file and no web or MCP surface: callers such as ralph
recipes own the desired state and express it as `add` flags on every run.
The `--port` health-probe convention belongs to the surrounding tooling,
which reads `ProgramArguments` from the plist — t-man neither parses nor
enforces it.
