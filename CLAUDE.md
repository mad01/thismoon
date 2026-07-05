# CLAUDE.md - thismoon

Monorepo for the `*.this` platform: local web services, CLI tools, the shared webkit package, and their ralph recipes. Private for now, planned to go public — every commit sits on top of the Apache-2.0 LICENSE (the root commit).

## Quick reference

| Task | Command |
|------|---------|
| Build all components | `make build` |
| Test everything | `make test` |
| Install all components | `make install-all` |
| List components | `make components` |

## Layout

```
services/    *.this local web services (one directory per service)
tools/       CLI tools installed to the local bin
webkit/      shared Go web UI package (in-module, no separate versioning)
recipes/     ralph recipes, consumed remotely via [[recipe_sources]]
docs/adr/    architecture decision records
docs/MIGRATED-FROM.md   maps each imported directory to its source repo + SHA
```

## Conventions

- Single Go module: `github.com/mad01/thismoon`, go 1.26.2. No nested go.mod files.
- A component is a directory under `services/` or `tools/` with its own Makefile exposing `build`, `test`, and `install` targets. The root Makefile discovers and delegates to them.
- Releases are per-component semver with `svc/vX.Y.Z` tags, cut by release-please manifest mode on merge to main. Artifact builds are a plain CI matrix job.
- Build targets: darwin/arm64, linux/amd64, linux/arm64. No Windows, by design (see docs/adr/0004).
- Release artifacts ship with checksums.txt and cosign keyless signatures; local installs use the "mad01 Local Signing" codesign identity.
- Code imported from another repo comes in clean (no git history) and gets a row in `docs/MIGRATED-FROM.md` with the source repo and SHA it came from.
- Recipes under `recipes/` must use absolute or `~`-prefixed `working_dir` in builds/packages — ralph resolves remote recipe paths against its sources cache, not the consuming machine's checkout.
- Install the secret-scanning pre-commit hook after cloning: `suspenders hook install`.

## Domain language

See `CONTEXT.md` for the glossary. Check `docs/adr/` before changing anything structural — the founding decisions are recorded there and are deliberate.
