# ADR-0022: present fetches its vendored assets at install and pins them by version and checksum

**Date:** 2026-10-07
**Status:** Accepted

**Scope:** `services/present`: `scripts/cache-assets.sh`,
`internal/server/shell.html`, `THIRD-PARTY.md`, the `Dockerfile`, and the
present recipe (`recipes/present`). The catalog service keeps its own
model.

## Context

The page shell loads its browser libraries and fonts from `/assets/`.
These vendored assets stay out of git. `scripts/cache-assets.sh` fetches
them from jsDelivr and Google Fonts into the workdir at install. The ralph
recipe runs it after every apply, and the image build runs it once. The
scripts alone come to about 2.8 MB, elkjs 1.6 MB of it. Committed, that
would sit in every clone and in every diff that bumps a version. The
catalog service took the other road and embeds one committed Cytoscape
file. That suits a single small library, not eight. `/assets/` is served
with a one-year immutable cache header, so a file must never change under
its name, and the version lives in the filename.

Nothing recorded this choice. MAD-385 adds d3 as the eighth library, and
reading the script for it found two gaps. The script ran curl without
`-f`, so a 404 wrote the error body under the versioned name and exited
0. The skip check only asks that every file is non-empty, so it then
trusted that body forever. And no licence notice existed for any
vendored asset.

## Decision

Fetching at install stays, and each vendored asset keeps its version in
its filename, so a bump is a new URL the immutable cache has never seen.

Every download fails loudly. curl runs with `-f` and three retries into
a temporary file, which is renamed only on success. A failed fetch leaves
nothing behind for the skip check to trust. The stylesheet is renamed
last, after every font file it names.

A file upstream ships as-is carries a pinned sha256, checked before the
rename. d3 is the first. The others gain a pin when they are next bumped.
chart.js stays unpinned until it moves to the npm dist file. jsDelivr
generates the `chart.umd.min.js` it loads today and warns against pinning
a hash on it.

`THIRD-PARTY.md` beside the service is the licence notice, and every bump
updates it in the same change. A test keeps the page shell and the script
naming the same files. The image build runs the same script, so a shared
instance bakes the same set a local install caches. A missing file is a
404, and the client degrades instead of failing. The sankey chart, which
shows a note when its plugin is absent, is the pattern a new consumer
follows.

## Consequences

- An install needs the network once per version. Offline, a brief or deck
  loads without graphs or charts and falls back to system fonts.
- A jsDelivr outage still fails ralph's `post_apply` hook silently. The
  recipe keeps `make cache || true`, so the service starts and the next
  apply retries. Run by hand, `make cache` now fails.
- The image build fails on any failed or mismatched download, which is
  wanted: a shared instance never ships an error body as a script.
- An unpinned file is trusted on TLS to jsDelivr alone. A pin closes that
  gap one file at a time.
- d3 adds about 280 KB to the first page load until its first consumer
  lands (MAD-386, MAD-382). The immutable cache keeps it a one-time cost.
- A workdir cached before this change is fetched again in full on the
  next apply, because the skip check now wants the d3 file too. A failed
  refetch leaves the files already cached in place.
