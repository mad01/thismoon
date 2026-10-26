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
the private repo that wires up a machine. So a service recipe hard-depending
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
  enables, env vars, secrets, and per-machine config overlays. It carries
  them as companion recipes that add their own items, never by patching
  public recipe fields.

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
assumption in the original Context. A fresh machine with no private consuming
repo at all becomes the common case, not the hypothetical one.

## Amendment (2026-09-08)

The public layer no longer carries `profiles` at all. The original decision
allowed a gate when it described the service rather than a machine, and
five recipes used that allowance (catalog, prs, reminder, wire, deps). In
practice every one of those gates was a machine-class decision, and the
consuming repos had to mirror each one by hand. The work-side overlay had to
know which thismoon recipes were personal-only, and the base config had to
restate the gate whenever it switched a recipe off.

The gate moved to where the decision lives. ralph now reads an
`overrides.toml` from any active recipe source (mad01/ralph#42), so the
layering is: this repo declares what exists, the consuming base config sets
the defaults with `[recipes_config.overrides."thismoon/<name>"]`. And a
profile-gated role source ships an `overrides.toml` that switches recipes off
(or on) for its machines only. Precedence per key is base config, then active
sources, then the machine-local overlay.

Consequence for recipes here: never add `profiles` to a public recipe. A
recipe that should not run on some machine class is withheld from the
consuming side, not gated from this one.

## Amendment (2026-10-25)

Sandbox settings moved from the public layer to the private one. The
Decision lists "sandbox profiles" among the public layer's contents, and
for a first-party service that meant a seatbelt `.sb` file beside its
recipe (present's `present.sb`). The fleet now sandboxes through
anthropics/sandbox-runtime (srt), and a process's posture is a JSON
settings file at `~/.config/srt/<tool>.json`. What a process may reach is a
per-machine choice: the shared host present pushes to, the code roots
humanizer may read. So the consuming repo ships every settings file as a
config overlay, in a directory no sandbox may write. Hosts that differ per
machine are merged in at launch from the wrapper's own inputs rather than
written into the file. The public layer keeps what is portable: the shared
`srt-run` launcher (speak recipe) and each tool's spawn wrapper, which
names the settings path and fails closed when the file is absent.

## Amendment (2026-10-25, t-man services)

The amendment above put every srt settings file in the consuming repo. That
fits the MCP servers and install steps it covered, whose reach differs per
machine. It doesn't fit the first-party t-man services now run under srt:
events, keeper-of-facts, moondb, catalog-web and present serve. Their
settings name the binary, the service's own workdir and the code roots the
fleet works in (`~/code`, `~/.worktrees`, the ralph sources cache). Nothing
in them differs per machine.

So generic, machine-independent settings for a public-layer service may ship
beside its recipe as `<label>.srt.json`, linked to
`~/.config/srt/<label>.json` by that recipe. Since ADR-0027 that file is
rendered from the service's `spec.access` manifest, not written by hand. Machine-specific egress stays in
the consuming repo: a host reaches the allow-list at launch through
`SRT_RUN_ALLOW_DOMAINS`, from inputs the consuming repo controls, the way
present serve gets its shared host. A settings file that has to name a
machine-specific path ships from the consuming repo as before.

Three t-man services stay outside srt. csl-web pulls repos over ssh, which
srt can't proxy. ssh ignores the proxy settings srt hands its child, and it
needs the user's keys or SSH agent, which the sandbox exists to keep out.
sandbox-watch needs logd to stream the unified log and the notification
services to post banners, and it is the monitor that reports every other
sandbox's denials. d-man is a root LaunchDaemon that rewrites `/etc/hosts`
and binds port 80, outside what a per-user srt sandbox covers.

## Amendment (2026-10-26)

srtspec joins t-man and d-man as a platform foundation. `~/code/bin/srt-run`
is a shim over `srtspec run`, so no sandboxed service or MCP server starts
without the binary. The srtspec recipe sits in wave 0, every service
registration here names `packages.srtspec` in `depends_on`, and a consuming
repo's hook that registers a sandboxed process may name it across sources,
as it already names `packages.t_man`.
