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

**buildinfo**:
The shared Go package holding the build metadata every component links in, injected at link time by the `buildinfo.mk` ldflags. Like webkit, an in-module package rather than a component.
_Avoid_: version package, versioninfo

**Build metadata**:
What a binary reports about its own build: version, commit, component tag, and build time. Serialized as a four-key JSON object, every key always present and `""` when unknown, printed by `<binary> version -o json` and served at `GET /version`.
_Avoid_: version info, version payload

**Recipe**:
A ralph recipe under `recipes/` that installs a component on a machine. Consumed remotely through ralph's `[[recipe_sources]]`, identified as `thismoon/<recipe>`.
_Avoid_: config, manifest

**Component tag**:
A release tag in the form `svc/vX.Y.Z` (component path prefix, slash separator), cut by release-please.
_Avoid_: version tag, release tag (ambiguous — repo-wide vs per-component)

**Clean import**:
Bringing code in from a source repo without its git history; the mapping to the source repo and SHA lives in `docs/MIGRATED-FROM.md`.
_Avoid_: migration (too broad), fork

**Assertion**:
A one-sentence, evidence-pinned statement about system behavior stored by keeper-of-facts, carrying a kind, a subject, a confidence, and a fresh/stale/retracted status.
_Avoid_: fact, memory

**Evidence pin**:
A hashed line range in a repo working tree that grounds an assertion. kof v1's only pin kind is a code pin.
_Avoid_: citation, reference

**Stale**:
A reversible assertion status set by `kof check` when a pin's content no longer hashes the same; it flips back to fresh when the content matches again.
_Avoid_: expired, invalid

**Retract**:
Terminal withdrawal of an assertion with a counter-evidence note. Retracted assertions are never re-checked.
_Avoid_: delete, cancel
