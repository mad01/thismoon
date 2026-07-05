# ADR-0006: Two-layer recipes, hard deps allowed on platform foundations

**Date:** 2026-07-05
**Status:** Accepted

**Scope:** every recipe under `recipes/`. Decided in MAD-199 after the
reminder pilot (MAD-190) and the five-service batch (MAD-191).

## Context

Ralph has no field-level overlay: `recipes_config.overrides` can enable or
disable a recipe, but a consuming machine cannot patch individual fields of a
recipe it pulls from a `[[recipe_sources]]` source. So machine-private
configuration cannot live in this repo's recipes, and anything that does live
here runs on every machine that points at the source.

Separately, the service recipes reference `packages.t_man` in `depends_on`,
and t-man's recipe currently lives in the consuming repo (dotfiles). A recipe
that hard-depends on an item key its own source doesn't define fails ralph
validation on a machine that consumes only this source. The alternative —
dropping the dep and relying on the `command -v t-man` runtime guards — keeps
recipes standalone but loses first-install ordering.

## Decision

Recipes split into two layers:

- **Public layer (this repo, `recipes/<svc>/`):** everything portable —
  package build/install with a sources-cache `working_dir`, t-man-guarded
  restart and uninstall hooks, the service's Claude skill, sandbox profiles.
  Profile gating (`profiles = ["personal"]`) is allowed here when the gate is
  a property of the service itself, not of a machine.
- **Private layer (the consuming repo, dotfiles):** the `[[recipe_sources]]`
  stanza and pin, MCP registration (`servers.json`), host filtering and
  enables, env vars, secrets, and per-machine config overlays as companion
  recipes that add their own items (the `pr-config` pattern) — never by
  patching public recipe fields.

Cross-source `depends_on` on **platform foundations** — t-man and d-man — is
allowed and kept. These are prerequisites of the `*.this` platform, not
optional integrations; a machine consuming these recipes without them is
misconfigured, and validation failing there is the correct signal. Both are
slated to migrate into this repo (MAD-204, MAD-205), which makes the dep
same-source and the question moot. Hooks keep the `command -v t-man` runtime
guards regardless, so a missing foundation degrades to a printed manual step
instead of a broken run.

Cross-source deps on anything that is not a platform foundation stay banned.

## Consequences

- All six service recipes keep `depends_on = ["packages.<svc>",
  "packages.t_man"]` unchanged; no recipe edits came out of this decision.
- Until MAD-204 lands, a machine consuming thismoon recipes without the
  dotfiles t-man recipe fails validation. Accepted: no such machine exists,
  and the failure names the missing prerequisite.
- New machine-private needs get a companion recipe in dotfiles, not a field
  in a public recipe. If field-level overlay pressure grows, that is a ralph
  feature request, not a layering exception.
