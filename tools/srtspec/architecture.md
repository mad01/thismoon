# architecture srtspec

```
srtspec/
  cmd/srtspec/          entrypoint
  internal/
    cli/                cobra commands: render, check, generate, explain, docs, version
    discover/           finds the manifests in a repo tree and the settings file each renders to
    drift/              compares a settings file to a render by meaning (canonical JSON + diff)
    layers/             base + local + env merge for explain, with srt-run's refusal rules
kit/access/             (shared) manifest model, strict parsing, validation, Render
```

## Data flow

render and check: `access.Load` reads a service-info.yaml (the one Component
with spec.access) or a standalone access.yaml, decodes the block strictly,
and validates it. `access.Render` builds the settings tree from the fleet
default, the declared fields and `extra`, checks the fleet invariants on the
result, and encodes it with sorted keys and 2-space indent. check
canonicalizes both sides (sorted keys, sorted string lists) and diffs them.

generate: `discover.Find` globs `*/`, `services/*/` and `tools/*/` for
service-info.yaml and `recipes/*/` for access.yaml, maps each srt manifest to
`recipes/<dir>/<label>.srt.json`, and refuses duplicate outputs. The CLI
renders each, writes only on change, and fails on any recipes/*/*.srt.json no
manifest renders.

explain: `layers.Resolve` reads `~/.config/srt/<label>.json`, checks and adds
`<label>.local.json`, then adds `SRT_RUN_ALLOW_DOMAINS`, recording the layer
of every entry.

## Invariants

- The manifest is the source of truth; a settings file in a recipe directory
  is always a render (docs/adr/0027).
- Rendering is a pure function of the manifest: same input, same bytes.
- `layers.CheckLocal` and srt-run's jq check accept and refuse the same
  inputs; `recipes/speak/testdata/srt-local` holds the cases both run.
