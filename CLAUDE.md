# CLAUDE.md - thismoon

Monorepo for the `*.this` platform: local web services, CLI tools, the shared webkit package, and their ralph recipes. A macOS-first local development toolbox — every service is a web UI + CLI for the human and (where it makes sense) an MCP server for agents, all on local data. Private for now, planned to go public — every commit sits on top of the BSD-3-Clause LICENSE (the root commit).

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
kit/         shared Go packages for cross-tool functionality (one subpackage
             per concern, e.g. kit/repofind for git repo discovery)
buildinfo/   shared build-metadata package (ldflags targets, /version handler)
buildinfo.mk Makefile fragment every component includes to inject it
recipes/     ralph recipes, consumed remotely via [[recipe_sources]]
skills/      agent skills (Claude + Codex), one directory per skill; each is
             symlinked into ~/.claude/skills and ~/.agents/skills by its
             paired recipe (skill-only recipes for skills with no binary)
examples/    worked private-config-repo layout (overlay recipes, example
             global CLAUDE.md) — the reference for the machine-private layer
docs/adr/    architecture decision records
docs/GETTING-STARTED.md install guide: one tool via brew vs the fleet via ralph
docs/HOW-IT-FITS-TOGETHER.md fleet pitch, ralph vocabulary, rollout order
docs/RELEASING.md       release process (release-please, tags, artifacts, verification)
```

## Conventions

- Single Go module: `github.com/mad01/thismoon`, go 1.26.2. No nested go.mod files.
- A component is a directory under `services/` or `tools/` with its own Makefile exposing `build`, `test`, and `install` targets. The root Makefile discovers and delegates to them.
- Releases are per-component semver with `svc/vX.Y.Z` tags, cut by release-please manifest mode on merge to main. Artifact builds are a plain CI matrix job.
- Build metadata is shared: a component Makefile sets `COMPONENT := <name>`, does `include ../../buildinfo.mk`, and links with `$(BUILDINFO_LDFLAGS)`. Every component then reports the same four-key object (`version`, `commit`, `tag`, `build_time`) from `GET /version` and `<binary> version -o json`, while plain `<binary> version` stays a bare token that status parses.
- Build targets: darwin/arm64 only — the platform is macOS-focused (see docs/adr/0007; supersedes the target list in 0004).
- Release artifacts ship with checksums.txt and cosign keyless signatures; local installs use the "mad01 Local Signing" codesign identity.
- Code imported from another repo comes in clean (no git history); the import commit message records the source repo and SHA it came from.
- Recipes under `recipes/` must use absolute or `~`-prefixed `working_dir` in builds/packages — ralph resolves remote recipe paths against its sources cache, not the consuming machine's checkout.
- Recipes are the **public layer** only: portable build/install, t-man-guarded hooks, skills. Machine-private wiring (`[[recipe_sources]]` pins, MCP registration, host enables, env/secrets, config overlays) lives in the consuming repo as companion recipes. Hard `depends_on` on the platform foundations t-man and d-man is allowed; on anything else cross-source deps are banned. See docs/adr/0006.
- Install the secret-scanning pre-commit hook after cloning: `suspenders hook install`.
- Every `CLAUDE.md` in the repo has a sibling `AGENTS.md` symlink pointing at it, so non-Claude agents (Codex and others that read `AGENTS.md`) discover the same instructions; new components get the symlink alongside the `CLAUDE.md`.

## Skills

The repo ships six agent skills under `skills/`, one directory per skill. Each is a `SKILL.md` that Claude Code and Codex load on demand; the paired recipe under `recipes/<skill>/` symlinks it into `~/.claude/skills` and `~/.agents/skills` when the fleet applies recipes, so a provisioned machine has it in every session. Invoke one explicitly with `/<skill-name>`, or let the agent load it when the task matches the skill's description.

| Skill | What it does | Backed by |
|-------|--------------|-----------|
| `commit-pipeline` | Coordinates commits when several sessions share one working tree: requesters queue commit request files under `~/.commits/`, and a single committer session processes them serially with exact staging. | nothing (guidance only; writes to `~/.commits/`) |
| `golang-style` | Idiomatic Go review and authoring — naming, package layout, error handling, the HTTP/CLI/store patterns used across this codebase. Covers every Go component here. | nothing (guidance only) |
| `handoff` | Writes a cold-start handoff document and persists key learnings to memory so the next agent or session can continue work without re-discovering context. | nothing (guidance only; writes to `~/.claude/handoffs/` and durable memory) |
| `humanizer` | Strips AI-writing tells from prose before it lands in docs, PR descriptions, or commit bodies. | the `humanizer` MCP for detection and voice profiling, plus a headless `claude -p` pass on Haiku for holistic judgment |
| `present` | Generates a scrollable briefing page with fixation reading, graphs, and inline charts for digesting a work summary or research. | the `present` service (`services/present`) |
| `worklog` | Saves and resumes cross-session work state keyed by ticket or topic, not by working directory. | the `worklog` MCP, with the `worklog` CLI as fallback |

Prerequisites: `commit-pipeline`, `golang-style`, and `handoff` need nothing beyond the checkout. The other three need their backing MCP or service registered — that wiring is machine-private and lives in the consuming repo alongside the recipe (see docs/adr/0006), not here. Without the backing MCP the skill still loads, but its tool-backed steps are unavailable.

## Release process

Full reference: `docs/RELEASING.md`. The short version for working here:

- release-please (manifest mode) runs on merge to main, keyed by the `packages` map in `release-please-config.json`. Conventional commits scoped by path drive per-component bumps; merging a release PR cuts the `name/vX.Y.Z` tag + GitHub Release, and the artifacts matrix builds darwin/arm64 tarballs with checksums.txt + cosign keyless bundle, natively on a macOS arm64 runner (csl builds with cgo for its tree-sitter grammars; everything else stays CGO_ENABLED=0).
- Release PRs can sit unmerged; merge = release. Never hand-edit `.release-please-manifest.json`.
- The workflow authenticates with the `RELEASE_PLEASE_TOKEN` fine-grained PAT (Contents + Pull requests read-write). "Error adding to tree" or a missing release PR usually means the PAT expired or lost write — it needs renewal at most yearly.
- New component → must be added to `release-please-config.json`; CI fails the PR otherwise.

## Shipping a change to machines

Two delivery paths, independent of each other:

- **Fleet (ralph):** consuming machines point a `[[recipe_sources]]` stanza at this repo (`ref = "main"`, `update = true`), so recipes merge as `thismoon/<recipe>` and every `ralph up` pulls main, rebuilds from source, and restarts services via t-man. Merging to main IS the deploy; no release tag needed. After merging recipe changes, verify with the service's `/version` — ralph can report ok while an old binary keeps running.
- **Artifacts (releases):** the per-component tags/tarballs above serve `go install` and manual downloads; the fleet does not consume them.

Machine-private wiring (which `.this` hosts exist, MCP registration, env/secrets, config overlays) lives in the consuming repo as companion recipes layered over these — never add it here (docs/adr/0006).

## Importing a service from another repo

The migration playbook, applied to every service brought in so far. Follow it
in order; each step earned its place:

1. **Import clean.** `git archive HEAD:<svc>` from the source repo, extract
   into `services/<svc>` (or `tools/<name>`). No git history comes along; the
   import commit message records the source repo and SHA.
2. **Fold into the module.** Delete the imported `go.mod`/`go.sum`, rewrite
   module paths to `github.com/mad01/thismoon/...`, switch webkit imports to
   the in-module package.
3. **Drop webkit versioning.** Delete `webkit_version.go`, remove the `webkit`
   field from `/version`, delete the `update-webkit` Makefile target — there is
   no pin here, the service compiles against the webkit committed beside it.
4. **Rewrite docs.** `service-info.yaml` gets `spec.system: thismoon`; strip
   pin/bump language from the service CLAUDE.md; scan for names and hosts that
   must not appear in a public repo (help text and code comments too, not just
   docs).
5. **Gates.** Root `go mod tidy`, then per-service build/vet/test/lint, root
   `make test`, and a clean-checkout gate: `git archive HEAD | tar -x` into a
   temp dir, build and test there.
6. **Register for release.** Add the component to the `packages` map in
   `release-please-config.json` — CI fails the PR if a Makefile-bearing
   component is missing from it.
7. **Recipe beside the service.** `recipes/<svc>/recipe.toml` with a
   sources-cache `working_dir` (see Conventions), then the scratch-config
   `ralph up --dry-run` gate: commit first, move the real sources cache aside,
   remove the test cache between runs, and put a `config.local.toml` with the
   right profiles beside the scratch config.
8. **Two commits per service:** one for the import, one for the recipe.

Cutover happens in the consuming repo (dotfiles): delete the old source dir +
recipe in one PR, keep item keys identical so ralph state carries over, then a
single `ralph up` swaps the fleet. After it, pull the sources cache manually if
the recipes are newly merged, and verify each service's `/version` — ralph can
report ok while leaving the old binary in place.

## Domain language

See `CONTEXT.md` for the glossary. Check `docs/adr/` before changing anything structural — the founding decisions are recorded there and are deliberate.
