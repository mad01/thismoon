# srtspec, access manifests to srt settings (CLI)

Go CLI that renders sandbox access manifests to the srt
(anthropics/sandbox-runtime) settings JSON the kernel enforces. It also checks
a committed settings file for drift and explains the effective policy a
label launches under. `verify` runs every render and expansion layer in a
repo through the same checks without writing; it is the gate CI runs. `run`
is the launcher: `~/code/bin/srt-run` (speak recipe) is a shim that execs
it, so every sandboxed service and MCP server starts through this binary. The manifest schema lives in `kit/access`, apart
from the binary. Why it exists: `why.md`. Flow and
invariants: `architecture.md`. The full authoring guide is the "Declaring
access" section of `docs/SANDBOXING.md`; the decision is docs/adr/0027.

## Module layout

```
srtspec/
  cmd/srtspec/      entrypoint
  internal/
    cli/            cobra commands
    discover/       manifest discovery and the recipe-dir mapping
    drift/          canonical JSON and the drift diff
    layers/         the base + local + env merge run and explain share
    verify/         the repo-wide check: renders, orphans and layers
    launch/         what run does: settings, label, merged copy, runtime, the exec
  Makefile          part of module github.com/mad01/thismoon (no own go.mod)
```

## Build / install / test

```bash
make build    # ./srtspec (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/srtspec + codesign
make test     # go test ./...
```

From the repo root, `make sandbox-settings` runs `generate` over the repo and
the public example. `make sandbox-verify` runs `verify` over both; CI runs it
and fails on any stale render, orphan or narrowing layer.

## Commands

```
srtspec render <manifest> [-o out.json]
srtspec check <manifest> <settings.json>   # exit 1 + diff on drift
srtspec generate [root]                    # every manifest -> recipes/<dir>/<label>.srt.json
srtspec verify [root]                      # exit 1 + one FAIL line per stale render, orphan or narrowing layer
srtspec run <settings> [--] <cmd> [args]   # merge ~/.config/srt/<label>.local.json + SRT_RUN_ALLOW_DOMAINS, exec srt
srtspec explain <label>                    # ~/.config/srt/<label>{,.local}.json + SRT_RUN_ALLOW_DOMAINS
srtspec docs
srtspec version [-o json]
```

## Gotchas

- `generate` and `verify` map a component manifest to
  `recipes/<component dir name>/`. A component whose recipe directory has
  another name can't render there; give the recipe directory the
  component's directory name. Both skip anything under a `testdata`
  directory, so the refuse-case layers in `recipes/speak/testdata/srt-local`
  never count as repo layers.
- `verify` checks a `<label>.local.json` against the `<label>.srt.json`
  beside it. When the repo ships only the layer (the base renders in another
  repo, as dotfiles' `speak-web.local.json` does), it checks against an
  empty base. Every rule holds except the one that catches an allow carving
  back a deny, and the ok line says so.
- `run` and `explain` share `internal/layers`, so a rule change is one
  edit plus a case in `recipes/speak/testdata/srt-local`, which the launch
  and layers tests both run.
- `run` replaces the process (`syscall.Exec`), so nothing after it runs and
  the srt-run shim stays a one-line exec. Tests go through `launch.Plan`,
  which returns the exec (path, argv, env) without performing it; only
  `mise which srt` runs as a subprocess, against a stand-in in the tests.
- `run` has `DisableFlagParsing`, so the confined command's own flags pass
  through untouched; `--` after the settings file is accepted and dropped.
  Exit codes follow the srt-run contract: 2 usage, 127 a missing srt or
  runtime, 1 a refused layer; `cli.ExitCode` maps them for main.
- `check` compares by meaning, so a committed file that only differs in key
  or list order passes. `verify` is the byte-level gate; `make
  sandbox-settings` re-renders.
