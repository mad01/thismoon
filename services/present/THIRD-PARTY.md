# Third-party notices

present's page shell loads the browser libraries and fonts below.
`scripts/cache-assets.sh` fetches them into the workdir at install, and
the image build bakes the same files into the shared instance. They are
not part of this repository's BSD-3-Clause licensed source and stay under
their own licences, listed here.

Licences were checked on 2026-10-07 against the `license` field of each
npm package and the `METADATA.pb` and `OFL.txt` of each family in the
google/fonts repository.

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

## Fonts

The woff2 files come from Google Fonts (`fonts.googleapis.com` and
`fonts.gstatic.com`). Every family is under the SIL Open Font License
(OFL), version 1.1.

- Fira Code, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/firacode
- Inter, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/inter
- Lexend, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/lexend
- Work Sans, OFL 1.1, https://github.com/google/fonts/tree/main/ofl/worksans

## Bumping a version

Change the version in `scripts/cache-assets.sh` and the script tag in
`internal/server/shell.html` together; a test fails when the two disagree.
Update the entry here in the same change. The policy behind this layout is
[ADR-0022](../../docs/adr/0022-vendored-browser-assets.md).
