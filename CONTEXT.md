# thismoon

The `*.this` platform in one repo: services, tools, the shared web UI package, and the ralph recipes that install them.

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

**Recipe**:
A ralph recipe under `recipes/` that installs a component on a machine. Consumed remotely through ralph's `[[recipe_sources]]`, identified as `thismoon/<recipe>`.
_Avoid_: config, manifest

**Component tag**:
A release tag in the form `svc/vX.Y.Z` (component path prefix, slash separator), cut by release-please.
_Avoid_: version tag, release tag (ambiguous — repo-wide vs per-component)

**Clean import**:
Bringing code in from a source repo without its git history; the mapping to the source repo and SHA lives in `docs/MIGRATED-FROM.md`.
_Avoid_: migration (too broad), fork
