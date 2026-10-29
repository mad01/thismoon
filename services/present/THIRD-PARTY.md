# Third-party notices

present's page shell loads the browser libraries and fonts below.
`scripts/cache-assets.sh` fetches them into the workdir at install, and
the image build bakes the same files into the shared instance. They are
not part of this repository's BSD-3-Clause licensed source and stay under
their own licences, listed here.

Licences were checked on 2026-10-07 against the `license` field of each
npm package and the `METADATA.pb` and `OFL.txt` of each family in the
google/fonts repository. The two xterm entries were checked the same way
on 2026-10-19.

## Browser libraries

Each file comes from `https://cdn.jsdelivr.net/npm/<package>@<version>/`.

- cytoscape 3.31.0, MIT, https://github.com/cytoscape/cytoscape.js
- dagre 0.8.5, MIT, https://github.com/dagrejs/dagre
- cytoscape-dagre 2.5.0, MIT, https://github.com/cytoscape/cytoscape.js-dagre
- elkjs 0.12.0, EPL-2.0 OR GPL-3.0-or-later, https://github.com/kieler/elkjs
- cytoscape-elk 2.3.0, MIT, https://github.com/cytoscape/cytoscape.js-elk
- chart.js 4.4.6, MIT, https://github.com/chartjs/Chart.js
- chartjs-chart-sankey 0.15.3, MIT, https://github.com/kurkle/chartjs-chart-sankey
- d3 7.9.0, ISC, https://github.com/d3/d3
- @xterm/xterm 6.0.0, MIT, https://github.com/xtermjs/xterm.js
- @xterm/addon-fit 0.11.0, MIT, https://github.com/xtermjs/xterm.js

## Fonts

The woff2 files come from Google Fonts (`fonts.googleapis.com` and
`fonts.gstatic.com`). Every family is under the SIL Open Font License
(OFL), version 1.1.

- Fira Code, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/firacode
- Inter, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/inter
- Lexend, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/lexend
- Work Sans, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/worksans

## In an exported page

`present export` and the `present_export` tool write a page as one HTML
file ([ADR-0029](../../docs/adr/0029-present-standalone-export.md)). That
file does not carry the browser libraries. It names the ones the page uses
by their jsDelivr URL, each pinned by an integrity hash of the cached file,
and the reader's browser fetches them. Each stays under its licence above.
elkjs is dual-licensed under EPL-2.0 or GPL-3.0-or-later, and its source
for version 0.12.0 is at https://github.com/kieler/elkjs/tree/0.12.0.

The file does carry the latin faces of the four font families, as data
URIs. The OFL 1.1 allows that with the copyright notice and the licence,
https://openfontlicense.org/open-font-license-official-text/. The comment
at the head of every exported file gives that link and the copyright line
of each family, as the first line of its `OFL.txt` in google/fonts reads:

- Fira Code: Copyright 2014-2020 The Fira Code Project Authors (https://github.com/tonsky/FiraCode)
- Inter: Copyright 2020 The Inter Project Authors (https://github.com/rsms/inter)
- Lexend: Copyright 2018 The Lexend Project Authors (https://github.com/googlefonts/lexend), with Reserved Font Name “RevReading Lexend”.
- Work Sans: Copyright 2019 The Work Sans Project Authors (https://github.com/weiweihuanghuang/Work-Sans)

The file also inlines webkit's bundle, which includes Prism
(https://github.com/PrismJS/prism), MIT, Copyright (c) 2012 Lea Verou. The
head comment carries Prism's licence text in full, from the `LICENSE` file
of the prismjs npm package. `internal/export/notices.go` holds these
notices, and a test keeps them in step with this file.

## Bumping a version

Change the version in `scripts/cache-assets.sh`, the script tag in
`internal/server/shell.html`, and the library table in
`internal/export/libraries.go` together; tests fail when they disagree.
Every library takes a new sha256 with it. That is the sum of the file
jsDelivr serves at the URL the script fetches. For an npm file, those are
the bytes of the file inside the package tarball. Fetch the npm file, never a
`.min.js` jsDelivr generates, since the export pins its hash. Update the
entry here in the same change. The policy behind this layout is
[ADR-0022](../../docs/adr/0022-vendored-browser-assets.md).
