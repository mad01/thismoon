# ADR-0029: a present export inlines our files and loads the third-party libraries from a pinned CDN

**Date:** 2026-10-29
**Status:** Accepted

**Scope:** `services/present`: `internal/export`, `present export`, the
`present_export` tool, the standalone branch in `internal/server/app.js`,
and `THIRD-PARTY.md`. Builds on
[ADR-0022](0022-vendored-browser-assets.md). Decided in MAD-520.

## Context

A page could reach someone else only through a shared instance. That needs
a reachable instance and an author key, and it refuses pages with local
images. Saving the page from the browser keeps only the empty shell, since
app.js fetches the page data and builds the body after load.

One file that opens anywhere has to carry the body as markup, and app.js
has to read the page data from the file instead of the API. The question
is what else goes into it. Our own files are webkit's stylesheet and
scripts, the shell's stylesheet, app.js, and viz.js, about 460 KB in all.
The third-party libraries add up to about 2.8 MB, and elkjs alone is
1.6 MB. They come under their own licences (MIT, ISC, and EPL-2.0 or
GPL-3.0 for elkjs).

## Decision

Hybrid. The export inlines what is ours, together with the body, the page
data, and the page's local images as data URIs. Prose and fixation reading
then work offline and with scripts off.

The libraries load from jsDelivr's npm mirror, at the versions the asset
cache holds. Each script tag carries an integrity hash computed from the
cached file, and the export adds only the libraries the page's blocks use.
jsDelivr serves an npm file byte for byte, so the hash matches the CDN
response. A browser refuses any other bytes, and the block shows its
"needs the asset" note. The cached Chart.js file is the one exception: it
is a file jsDelivr generates, which is its banner comment followed by the
npm file `dist/chart.umd.js`. The export strips the banner and loads that
npm file, whose bytes are fixed.

The fonts' latin faces are inlined as data URIs. The families are not on
npm in the bytes Google Fonts serves, and a stylesheet cannot pin a font
file by hash.

Our files are not loaded from a CDN that serves this repository from
GitHub. Release tags contain a slash, and a file inlined at export time
cannot drift from the binary that wrote it.

## Consequences

- A prose page exports to about 0.9 MB, and the latin fonts are half of
  that. Graph, chart, and diagram pages add only a script tag per library.
- Graphs, charts, and diagrams need the network when the file opens.
  Offline they show the note and the rest of the page reads.
- A library bump changes the export along with the shell: the library
  table in `internal/export` names the same files, and a test holds it to
  `scripts/cache-assets.sh`.
- An export references the libraries without redistributing them. It does
  carry the fonts, which the SIL Open Font License allows with the notice
  that `THIRD-PARTY.md` and the file's head comment give.
- A terminal block cannot work in a file, because it needs a local
  pseudo-terminal behind a WebSocket. It shows a note instead.
- A deck shows no default logo in an export: the repo logo is 340 KB and
  would make the file half again as large. A logo URL in the deck's chrome
  still shows.
