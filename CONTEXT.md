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

**srt**:
anthropics/sandbox-runtime, the sandbox every service and MCP server in the fleet runs under. It generates a default-deny macOS seatbelt profile from a JSON settings file and allows egress only to the hosts the settings name. The consuming repo installs it through mise (docs/adr/0028).
_Avoid_: seatbelt (for a process that runs under srt), jail, container

**Access manifest**:
What one sandboxed process may reach, declared beside its code: the `spec.access` block of a component's `service-info.yaml`, or a recipe's standalone `access.yaml`. It is the source of truth; the srt settings file is rendered from it. docs/SANDBOXING.md covers the fields.
_Avoid_: sandbox profile (the hand-written seatbelt file), policy file, settings file (the rendered JSON)

**srt settings**:
The per-process srt JSON file at `~/.config/srt/<label>.json`, rendered from the access manifest and never edited by hand. It holds the static policy, and one file serves every machine; a host that depends on the machine comes from the wrapper at launch, never from this file.
_Avoid_: profile, policy file, overlay

**Expansion layer**:
`~/.config/srt/<label>.local.json`, what one machine adds on top of a rendered settings file. srt-run merges it at launch and refuses one that does more than widen.
_Avoid_: override (it can't narrow or replace), local settings

**srt-run**:
The fleet launcher path, `~/code/bin/srt-run`: a shim (`recipes/speak/srt-run.sh`) over `srtspec run`. The launcher starts a process under srt with its srt settings. It merges the wrapper's `SRT_RUN_ALLOW_DOMAINS` hosts and the expansion layer into them, and adds the `SRT_RUN=<label>` marker sandbox-watch attributes denials by.
_Avoid_: wrapper (the per-process script that calls srt-run is the wrapper)

**Seatbelt sandbox**:
The legacy mechanism: a hand-written macOS `sandbox-exec` profile (`*.sb`) run through t-man's `--sandbox-profile`. Only a process srt can't express keeps one, with every deny rule tagged `(with message "SRT_RUN=<label>")`. The speak-tts runtime is the one case, because MLX needs Metal.
_Avoid_: srt (for the hand-written profile), jail, container

**Container build**:
The throwaway Apple `container` VM a package-manager build step (npm, pip, gem) runs in, with only the named artifacts exported out; webkit's `make build` is the reference. A different mechanism from srt and the seatbelt sandbox (docs/CONTAINER-BUILDS.md).
_Avoid_: sandbox, docker

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

**agent-memory**:
The shared memory store: a directory of one-fact markdown files at `~/.config/agent-memory/`, usually a private git clone, whose index belt injects at session start. It follows the user across agents, and across machines when something syncs it. docs/MEMORY-STACK.md covers it beside the other stores.
_Avoid_: auto memory (Claude Code's own store, a different thing), shared memory on its own

**Auto memory**:
Claude Code's built-in memory directory, one per git repository under `~/.claude/projects/`. Machine-local and Claude-only; thismoon ships nothing for it.
_Avoid_: agent-memory (the shared store above)
