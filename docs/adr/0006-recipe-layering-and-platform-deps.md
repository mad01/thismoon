# ADR-0006: Two-layer recipes, hard deps allowed on platform foundations

**Date:** 2026-07-05
**Status:** Accepted, amended 2026-08-29

**Scope:** every recipe under `recipes/`. Decided after the reminder pilot
and the five-service batch that established the recipe pattern.

## Context

Ralph has no field-level overlay: `recipes_config.overrides` can enable or
disable a recipe, but a consuming machine cannot patch individual fields of a
recipe it pulls from a `[[recipe_sources]]` source. So machine-private
configuration cannot live in this repo's recipes, and anything that does live
here runs on every machine that points at the source.

Separately, the service recipes reference `packages.t_man` in `depends_on`.
At the time of this decision, t-man's own recipe lived outside this repo, in
the private repo that wires up a machine, so a service recipe hard-depending
on it named an item key its own source didn't define, and that fails ralph
validation on a machine that consumes only this source. The alternative,
dropping the dep and relying on the `command -v t-man` runtime guards, keeps
recipes standalone but loses first-install ordering.

## Decision

Recipes split into two layers:

- **Public layer** (this repo, `recipes/<svc>/`) carries everything portable:
  package build/install with a sources-cache `working_dir`, t-man-guarded
  restart and uninstall hooks, the service's Claude skill, sandbox profiles.
  No profile gating: a public recipe never carries `profiles`. Where a recipe
  runs is the consuming layer's call (see the 2026-09-08 amendment; the
  original text allowed a gate when it was "a property of the service").
- **Private layer** (the consuming repo) carries the `[[recipe_sources]]`
  stanza and pin, MCP registration (`servers.json`), host filtering and
  enables, env vars, secrets, and per-machine config overlays as companion
  recipes that add their own items, never by patching public recipe fields.

Cross-source `depends_on` on **platform foundations** (t-man and d-man) is
allowed and kept. These are prerequisites of the `*.this` platform, not
optional integrations; a machine consuming these recipes without them is
misconfigured, and validation failing there is the correct signal. Hooks keep
the `command -v t-man` runtime guards regardless, so a missing foundation
degrades to a printed manual step instead of a broken run.

Cross-source deps on anything that is not a platform foundation stay banned.

## Consequences

- Every service recipe keeps `depends_on = ["packages.<svc>",
  "packages.t_man"]` unchanged; no recipe edits came out of this decision.
- At the time of this decision, a machine consuming thismoon recipes without
  the private repo's t-man recipe failed validation, and the failure named
  the missing prerequisite. See the amendment below: this no longer applies,
  now that t-man ships from this repo.
- New machine-private needs get a companion recipe in the consuming repo, not
  a field in a public recipe. If field-level overlay pressure grows, that is
  a ralph feature request, not a layering exception.

## Amendment (2026-08-29)

t-man and d-man have both migrated into this repo (`recipes/t-man/`,
`recipes/d-man/`). The cross-source dependency this ADR carved out an
exception for is now same-source, so the exception is dormant rather than
load-bearing. The layering decision itself is unchanged: the public/private
split, and the ban on cross-source deps for anything that is not a platform
foundation, still govern every recipe added since.

This repo is also headed toward a public release, which flips the default
assumption in the original Context: a fresh machine with no private
consuming repo at all becomes the common case, not the hypothetical one.

## Amendment (2026-09-08)

The public layer no longer carries `profiles` at all. The original decision
allowed a gate when it described the service rather than a machine, and
five recipes used that allowance (catalog, prs, reminder, wire, deps). In
practice every one of those gates was a machine-class decision, and the
consuming repos had to mirror each one by hand: the work-side overlay had to
know which thismoon recipes were personal-only, and the base config had to
restate the gate whenever it switched a recipe off.

The gate moved to where the decision lives. ralph now reads an
`overrides.toml` from any active recipe source (mad01/ralph#42), so the
layering is: this repo declares what exists, the consuming base config sets
the defaults with `[recipes_config.overrides."thismoon/<name>"]`, and a
profile-gated role source ships an `overrides.toml` that switches recipes off
(or on) for its machines only. Precedence per key is base config, then active
sources, then the machine-local overlay.

Consequence for recipes here: never add `profiles` to a public recipe. A
recipe that should not run on some machine class is withheld from the
consuming side, not gated from this one.
