# thismoon

Monorepo for the `*.this` platform: a fleet of local web services and CLI
tools that run on your own machine behind `http://<name>.this/` hostnames,
plus the shared web UI kit and the [ralph](https://github.com/mad01/ralph)
recipes that install them.

## Layout

| Path | Contents |
|------|----------|
| `services/` | Local web services, one directory per service (catalog, csl, d-man, deps, events, pr, present, reminder, speak, status) |
| `tools/` | CLI tools (t-man) |
| `webkit/` | Shared Go web UI package, compiled in — no separate versioning |
| `recipes/` | ralph recipes, consumed remotely via `[[recipe_sources]]` |
| `docs/adr/` | Architecture decision records |

Everything is one Go module: `github.com/mad01/thismoon`. Each component
under `services/` or `tools/` has its own Makefile with `build`, `test`, and
`install` targets; the root Makefile discovers and delegates to them
(`make components` lists them).

## Install

Build from a checkout:

```sh
make -C services/present install    # one component
make install-all                    # everything
```

Or install a single tool straight from the module path:

```sh
go install github.com/mad01/thismoon/services/present/cmd/present@latest
```

Releases are per-component semver tags (`present/v1.2.3`) with checksums and
cosign keyless signatures on the artifacts. csl ships no prebuilt artifacts —
it needs cgo (tree-sitter) and local ONNX libraries, so build it from a
checkout.

The full fleet (services registered as launchd agents, config symlinks,
`.this` routing) installs through ralph's `[[recipe_sources]]` pointing at
this repo. See `recipes/` and ralph's configuration reference.

### While this repo is private

`go install` and module fetches need git-over-SSH plus a `GOPRIVATE` entry:

```sh
export GOPRIVATE=github.com/mad01/*
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

Once the repo is public, neither is needed for this module — drop the
`insteadOf` rewrite and trim `GOPRIVATE` to whatever private repos remain.
The ralph source stanza works unchanged in both worlds; it always clones over
SSH.

## License

Apache-2.0 — see [LICENSE](LICENSE). No per-file headers; the root license
covers the repo.
