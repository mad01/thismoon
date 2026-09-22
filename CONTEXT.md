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
Bringing code in from a source repo without its git history; the import commit message records the source repo and SHA.
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

**Loom**:
The worktree coordination pattern the loom skill runs: each agent session claims its own git worktree under `~/.worktrees/` and commits there, and one weaver lands the finished branches in order. All state is derived from git; nothing is stored.
_Avoid_: commit pipeline (the retired queue model), merge queue

**Claimed worktree**:
A linked git worktree at `~/.worktrees/<repo>/<slug>`, created explicitly for one task and owned by exactly one session until its branch lands and the worktree is removed.
_Avoid_: sandbox, workspace, checkout (that means the canonical one)

**Weaver**:
The single session that integrates a loom's ready branches: rebase onto the default branch tip, gates, then PR or fast-forward merge by repo policy. A role, not a process — nothing enforces its exclusivity.
_Avoid_: committer (the retired queue model's role), merger

**Shared instance**:
A present running with `--shared`: it serves pages by id only, takes writes only with an author key, and keeps no index or listing. Where a local present shares pages to.
_Avoid_: shared server, remote present

**Author key**:
A self-issued secret a client sends to a shared instance as a bearer token; the instance stores only its hash, which becomes the page's author. Minted with `present key new`.
_Avoid_: token (the bearer token is how the key travels, not what it is), password, API key

**Ephemeral page**:
A shared page that expires 30 days after its last write or share and is then purged by the sweeper.
_Avoid_: temporary page, TTL page

**Share**:
Pushing a copy of a local page to a shared instance. Sharing the same page again replaces the copy under the same link.
_Avoid_: publish, upload, sync
