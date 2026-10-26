# srtspec, access manifests to srt settings (CLI)

Go CLI that renders sandbox access manifests to the srt
(anthropics/sandbox-runtime) settings JSON the kernel enforces. It also checks
a committed settings file for drift and explains the effective policy
srt-run launches a label under. `verify` runs every render and expansion
layer in a repo through the same checks without writing; it is the gate CI
runs. The manifest schema lives in `kit/access` so the
catalog validates the same block. Why it exists: `why.md`. Flow and
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
    layers/         explain's base + local + env merge
    verify/         the repo-wide check: renders, orphans and layers
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
- `explain` and srt-run implement the expansion rules twice (Go here, jq in
  `recipes/speak/srt-run.sh`). Any rule change goes into both, with a case in
  `recipes/speak/testdata/srt-local`, which both test suites run.
- `check` compares by meaning, so a committed file that only differs in key
  or list order passes. `verify` is the byte-level gate; `make
  sandbox-settings` re-renders.
