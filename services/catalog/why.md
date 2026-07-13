# why catalog

## The problem

A single developer machine accumulates repos and tools faster than anyone can
remember them. What exists, who owns it, which repo a component lives in,
whether it is production or winding down: none of that has a home once the
count passes a handful of repos. The answers exist, but only as folklore, and
folklore drifts. catalog gives that inventory one address: `service-info.yaml`
files describing Systems and Components live in the repos themselves, and
catalog indexes them into a browsable, searchable localhost UI and CLI.

## Why its own service

The data is spread across many repos, so no single repo's tooling can own the
index. The obvious off-the-shelf answer, Backstage, is a hosted platform with
its own backend, database, and plugin system; running it to catalog one
machine's repos is out of proportion to the problem. catalog borrows Backstage's entity
shape (`kind`, `metadata`, `spec`) so the files read familiarly, and stops
there. It also needs two distinct surfaces that no existing component
provides: a running web index for browsing, and a `validate` command that CI
and pre-merge checks can call against specific paths.

## Why this shape

The load-bearing decision is that catalog never owns the data: entity files
stay in their source repos, and every `list`/`validate`/`web` run rescans
them fresh into memory, with nothing cached to disk. The repos are the source
of truth, so the catalog cannot drift from them; the worst staleness is one
Refresh away. Names form a single global namespace across Systems and
Components, enforced by `catalog validate` (deliberately not by `Load`, so
the UI still renders a catalog that happens to contain a collision). And the
web UI follows the platform's client-side rendering model
(docs/adr/0005): the backend serves a static shell plus JSON at `/api/*`, and
a hand-written vanilla `app.js` builds the DOM with webkit primitives. The
only third-party JavaScript is a vendored, pinned Cytoscape.js for the System
dependency graph; there is no frontend build toolchain.

## Non-goals

catalog is not Backstage: no plugins, no scaffolding templates, no CI/CD or
deployment views, no multi-user anything. It does not track runtime state
(that is status's job) and it does not copy or migrate entity files; the one
write path is the Add form, which refuses to overwrite an existing
`service-info.yaml`. Recipes and install descriptors are not entities.
