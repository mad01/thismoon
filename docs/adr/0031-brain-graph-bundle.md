# ADR-0031: brain commits its graph bundle as first-party build output

**Date:** 2026-11-05
**Status:** Accepted

**Scope:** `services/brain`: `web/` (the package, lockfile, build script
and container build), `internal/server/ui/assets/`, the `bundle` and
`bundle-local` targets of its Makefile, and `THIRD-PARTY.md`. Sits beside
[ADR-0022](0022-vendored-browser-assets.md).

## Context

The brain.this UI draws the note graph with sigma and graphology, lays it
out with ForceAtlas2 in a web worker, and renders notes with markdown-it.
Those are npm packages with dependencies of their own, written as modules.
The page has no module loader and no CDN, so something has to turn them
into one script. ADR-0022 fetches each library at install, as the file
upstream ships, and pins it by version and checksum. That fits a file used
as it is. It does not fit eleven packages that only work once a bundler
has joined them.

## Decision

The bundle is our build output, committed the way `webkit/dist` is.
`web/package.json` pins every direct dependency to an exact version and
`web/package-lock.json` locks the rest. `make bundle` runs `npm ci` and
esbuild in a throwaway Apple container and copies out two files:
`assets/brain-graph.bundle.js`, one plain script that sets
`window.BrainSigma`, and `assets/SHA256SUMS`. The brain binary embeds
both, so the UI works offline and needs no install step. A test fails when
the bundle and its sum disagree, so a hand edit cannot pass as a build.

The lockfile is what makes the bundle visible to the deps scanner, which
checks every lockfile entry against the Open Source Vulnerabilities
database. The bundle sits in `assets/`, not under a directory named
`static`, where the scanner would look its hash up on jsDelivr and flag a
file jsDelivr never served.

ADR-0022 still covers upstream files shipped as they are.

## Consequences

- A version bump is a lockfile change, a container build and a new
  bundle in one commit. `THIRD-PARTY.md` and the notices the build appends
  to the bundle change with it.
- The bundle is minified, about 580 KB, so review reads the lockfile diff,
  not the bundle diff.
- `make bundle-local` builds on the host when the container runtime is
  missing. It runs install scripts on the host, so it is the fallback.
