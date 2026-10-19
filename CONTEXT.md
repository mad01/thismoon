# thismoon

The `*.this` platform in one repo: services, tools, the shared web UI package, and the ralph recipes that install them.

This file holds the repo-wide terms and applies to every file in the repo. The
terms of one service or skill live in a CONTEXT.md beside it and apply only
under that directory; CONTEXT-MAP.md lists them.

## Language

**Component**:
A releasable unit — a directory under `services/` or `tools/` with its own Makefile and its own semver line.
_Avoid_: module, package (both mean something else in Go)

**Service**:
A component under `services/` that runs locally and serves a `*.this` address.
_Avoid_: app, daemon

**Tool**:
A component under `tools/` — a CLI installed to the local bin, not a running process.
_Avoid_: binary, utility

**webkit**:
The shared Go web UI package that services import in-module. Not a component; it has no release line of its own.
_Avoid_: web kit, ui-lib

**buildinfo**:
The shared Go package holding the build metadata every component links in, injected at link time by the `buildinfo.mk` ldflags. Like webkit, an in-module package rather than a component.
_Avoid_: version package, versioninfo

**Build metadata**:
What a binary reports about its own build: version, commit, component tag, and build time. Serialized as a four-key JSON object, every key always present and `""` when unknown, printed by `<binary> version -o json` and served at `GET /version`.
_Avoid_: version info, version payload

**Recipe**:
A ralph recipe under `recipes/` that installs a component on a machine. Consumed remotely through ralph's `[[recipe_sources]]`, identified as `thismoon/<recipe>`.
_Avoid_: config, manifest

**Mod**:
A Claude Code plugin under `mods/<name>/` whose hooks are TypeScript functions running in-process: it draws a band or a pane and observes events. The root `thismoon` marketplace lists it. Additive only: nothing in the repo may require one, and it is never a component.
_Avoid_: plugin on its own (ambiguous with command-hook plugins and MCP plugins), extension, hook module

**Component tag**:
A release tag in the form `svc/vX.Y.Z` (component path prefix, slash separator), cut by release-please.
_Avoid_: version tag, release tag (ambiguous — repo-wide vs per-component)

**Clean import**:
Bringing code in from a source repo without its git history; the import commit message records the source repo and SHA.
_Avoid_: migration (too broad), fork
