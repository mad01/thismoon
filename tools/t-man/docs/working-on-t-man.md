# Working on t-man

How to build, run, test, and debug t-man from source.

## Build and install

```bash
git clone https://github.com/mad01/thismoon.git
cd thismoon/tools/t-man

make build      # → ./t-man in this directory
make install    # build, then copy to ~/code/bin and re-sign on macOS
```

A few things worth knowing:

- `make build` writes the binary to **this directory** as `./t-man`, not to a
  `bin/` directory. `.gitignore` excludes `/t-man`.
- `make install` copies the binary to `~/code/bin`. On macOS it then strips
  extended attributes and re-signs the binary — with the "mad01 Local Signing"
  identity when present, ad-hoc otherwise. This is required on recent macOS: a
  plain `cp` of a linker-signed Go binary gets killed on launch otherwise.
- The build stamps the version. `make build` passes
  `-ldflags "-X github.com/mad01/thismoon/tools/t-man/internal/cli.Version=<short-sha>"`,
  and `t-man version` prints that SHA. Only `cli.Version` is stamped; the
  variables in `pkg/version` exist but are not wired to the Makefile.

Requirements: macOS 10.15+ and Go 1.25+.

## Run locally

You can run the freshly built binary without installing it:

```bash
make build && ./t-man       # run with no args (prints help)
./t-man add --name test-echo -- /bin/echo "hello"
./t-man list
./t-man logs test-echo
./t-man remove test-echo
```

`--dryrun` is the safe way to see what an `add` would do without touching
launchd:

```bash
./t-man --dryrun add --name test -- /bin/echo "test"
```

## Tests

```bash
make test            # go test ./... -timeout 30s
go test ./internal/service/   # one package
```

The launchd layer is tested without a real launchd: the launchctl client takes
an injectable command runner, and the `Manager` is an interface, so plist
generation and the reconcile logic are exercised with fakes. The plist
generation tests compare against golden files under `testdata/plists/`.

> One caution when running the suite: t-man's tests touch launchd-shaped paths.
> Before running an unfamiliar test that constructs real `~/Library/LaunchAgents`
> paths, check that it points at a temp directory and not your real one. A
> system-state tool's tests can pass while quietly mutating real services if a
> path constructor uses the real home directory.

## Lint and format

```bash
make lint       # golangci-lint run ./...
make fmt        # golines (100 cols, gofumpt base)
```

## Where the code lives

```
cmd/t-man/main.go            entry point; calls cli.Execute()
internal/cli/
  root.go                    persistent flags (--agent/--daemon/--dryrun), checkSudo, Version var
  add.go                     builds a Definition from flags, resolves the command path
  list.go, control.go        list/ls; start/stop/restart/status
  remove.go, logs.go         remove/rm/delete; logs + the `logs sandbox` subcommand
  version.go
internal/service/
  definition.go              the Definition struct, Hash(), sandbox digest, validation
  manager.go                 Manager interface
internal/platform/launchd/
  plist.go                   plist XML generation + the t-man marker/metadata
  launchctl.go               the launchctl command wrapper (injectable runner)
  manager.go                 launchd Manager: create/update/delete, plist<->Definition
internal/reconcile/
  reconciler.go              read-compare-apply
  state.go                   CompareStates → create/update/delete/none
pkg/version/version.go       version vars (not stamped by the Makefile)
```

## Debugging a stuck agent

When you are developing and a service gets wedged, drop below t-man to launchd:

```bash
# What does launchd think the state is? (last exit code, run state)
launchctl print gui/$(id -u)/my-service

# Force-remove a registration t-man's remove did not clear
launchctl bootout gui/$(id -u)/my-service

# Read the raw plist t-man generated
cat ~/Library/LaunchAgents/my-service.plist
```

For a daemon, swap `gui/$(id -u)` for `system/` and prefix with `sudo`. See
`docs/troubleshooting.md` for the full crash-loop walkthrough.

## Making a change

t-man's design keeps the layers honest, so respect the boundaries when you add
to it:

- New CLI behavior goes in `internal/cli/` and should build a `Definition`,
  not call launchctl.
- Anything that changes what a service *is* belongs on the `Definition` struct.
  Add the field, give it a JSON tag, and it automatically joins the hash — so
  add a test that the hash changes when the field changes.
- Anything launchd-specific (new plist keys, new launchctl verbs) goes in
  `internal/platform/launchd/` behind the `Manager` interface, with a golden
  plist test under `testdata/`.

Add tests for new code, and make sure `make test` is green before opening a PR.
