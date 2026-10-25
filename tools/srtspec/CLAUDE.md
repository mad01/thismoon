# srtspec, access manifests to srt settings (CLI)

Go CLI that renders sandbox access manifests to the srt
(anthropics/sandbox-runtime) settings JSON the kernel enforces. It also checks
a committed settings file for drift and explains the effective policy srt-run
launches a label under. The manifest schema lives in `kit/access` so the
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
  Makefile          part of module github.com/mad01/thismoon (no own go.mod)
```

## Build / install / test

```bash
make build    # ./srtspec (build metadata via ldflags, from ../../buildinfo.mk)
make install  # build + cp to ~/code/bin/srtspec + codesign
make test     # go test ./...
```

From the repo root, `make sandbox-settings` runs `generate` over the whole
repo; CI runs it and fails on any diff.

## Commands

```
srtspec render <manifest> [-o out.json]
srtspec check <manifest> <settings.json>   # exit 1 + diff on drift
srtspec generate [root]                    # every manifest -> recipes/<dir>/<label>.srt.json
srtspec explain <label>                    # ~/.config/srt/<label>{,.local}.json + SRT_RUN_ALLOW_DOMAINS
srtspec docs
srtspec version [-o json]
```

## Gotchas

- `generate` maps a component manifest to `recipes/<component dir name>/`.
  A component whose recipe directory has another name can't render there;
  give the recipe directory the component's directory name.
- `explain` and srt-run implement the expansion rules twice (Go here, jq in
  `recipes/speak/srt-run.sh`). Any rule change goes into both, with a case in
  `recipes/speak/testdata/srt-local`, which both test suites run.
- `check` compares by meaning, so a committed file that only differs in key
  or list order passes. CI's `make sandbox-settings` + `git diff` is the
  byte-level gate.
