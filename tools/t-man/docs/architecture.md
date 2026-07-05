# Architecture

How t-man turns a one-line `add` command into a running launchd service, and
how it avoids touching launchd when nothing has changed.

## The layers

t-man is a thin, testable wrapper around launchd. The code splits into four
layers, top to bottom:

```
internal/cli/                cobra commands — parse flags, build a Definition
internal/reconcile/          read-compare-apply: decide create/update/delete/none
internal/service/            the Definition type and its Hash(); the Manager interface
internal/platform/launchd/   the macOS implementation: plist XML + launchctl calls
```

The CLI layer never talks to launchctl directly. It builds a
`service.Definition` from flags and hands it to the reconciler. The reconciler
decides what kind of change is needed and calls the platform Manager. Only the
`platform/launchd` package knows about plist files and `launchctl`.

This split is what makes the core logic testable without a real launchd: the
`Manager` is an interface, and the launchctl client takes an injectable
command runner.

## A service definition

Everything t-man knows about a service lives in one struct,
`service.Definition` (`internal/service/definition.go`):

| Field | Source | Notes |
|-------|--------|-------|
| `Name` | `--name` | Required; `[a-zA-Z0-9._-]+` |
| `Command` | first word after `--` | Must be an absolute, executable path |
| `Args` | rest after `--` | |
| `WorkingDir` | `--workdir` | Must exist and be absolute |
| `Environment` | `--env`, `--path` | Map of KEY=VALUE |
| `RunAtLoad` | always true | |
| `KeepAlive` | always true | launchd restarts the process if it exits |
| `StandardOutPath` / `StandardErrPath` | derived from `--logs` | |
| `SandboxProfile` / `SandboxProfileSHA256` | `--sandbox-profile` | Path plus content digest |
| `ExtraLogs` | `--extra-log` | Name → path, for logs written outside stdout/stderr |

There is no separate config file. The definition is assembled from flags every
time you run `add`. If you want to change a service, you re-run `add` with the
new flags.

## The reconcile loop

Every `add` runs read-compare-apply (`internal/reconcile/reconciler.go`):

1. Read. Look for an existing plist for this name. If one exists, parse it
   back into a `Definition` (`plistToDefinition`). If not, the current state
   is nil.
2. Compare. `CompareStates(current, desired)` returns one of: create (no
   current), delete (no desired), update (hashes differ), or none (hashes
   match). The comparison is purely the two hashes, described below.
3. Apply. Create, update, or delete through the launchd Manager. A "none"
   result does nothing, which is why a repeated `add` prints "already up to
   date" and leaves the running process alone.

### Why hashing instead of field comparison

`Definition.Hash()` marshals the whole struct to JSON and takes the SHA256 of
the bytes. The hash is written into the plist under `TManMetadata.Hash`. On the
next `add`, t-man reads that stored hash, computes the hash of the definition
you just described, and compares the two strings.

Because the hash covers every field, any real change (a new env var, a
different working directory, an edited sandbox profile) flips the hash and
triggers an update. Anything that leaves the definition byte-identical is a
no-op. There is no field-by-field diff to keep in sync with the struct, and no
external state database to drift out of sync with launchd. The plist on disk
is the single source of truth.

### How an update is applied

A hash mismatch is the only thing that bounces a service. The update path
(`internal/platform/launchd/manager.go`) is written to avoid leaving launchd in
a half-configured state:

1. Render the new plist to a temp file in the same directory.
2. `launchctl unload` the old plist.
3. Atomically rename the temp file over the old plist.
4. `launchctl load -w` the new plist.
5. If the load fails, restore the previous plist content and reload it, so a
   bad config does not take the service down permanently.

## How t-man wraps launchd

t-man does not reimplement launchd. It generates a plist and drives the system
`launchctl` binary. The launchctl client (`internal/platform/launchd/launchctl.go`)
uses the legacy verbs:

- `launchctl load -w <plist>` — register and enable a service
- `launchctl unload <plist>` — deregister it
- `launchctl start <label>` / `stop <label>` — control a loaded service
- `launchctl list` — enumerate loaded services
- `launchctl print gui/<uid>/<label>` — query one service's status (falls back
  to `list` if the print call fails)

The plist itself is generated in `internal/platform/launchd/plist.go` using the
howett.net/plist library.

Each managed plist carries a TManMetadata dict (the stored hash, a ManagedBy
field, the version, and the sandbox and extra-log fields) plus a
`DO NOT EDIT MANUALLY` marker comment. Those two markers are how the list and
status commands tell which plists belong to t-man and which to leave alone.

### Sandboxed services

With `--sandbox-profile`, t-man prepends the launch arguments with
`/usr/bin/sandbox-exec -D HOME=<home> -f <profile>` so the command runs inside
the seatbelt profile. `sandbox-exec` execs the target in the same process, so
`KeepAlive` and restart behavior are unchanged. The profile's content digest is
part of the definition hash, so editing the `.sb` file re-renders the plist on
the next `add`. See `docs/troubleshooting.md` for reading sandbox denial logs.

## Where this sits in the wider system

t-man is the bottom layer for a set of local `.this` services — a present
server, a text-to-speech service, a status page, code search, and a catalog,
among others. Each registers itself as a t-man agent; one front-door daemon
runs as a root LaunchDaemon. When you reason about why one of those services is
down, t-man is where the launchd truth lives: the plist it wrote, the logs it
points at, and the hash that decides whether the last `add` actually changed
anything.
