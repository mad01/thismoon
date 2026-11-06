# Third-party notices

The brain.this web UI loads one script built from npm packages:
`internal/server/ui/assets/brain-graph.bundle.js`. `web/build.mjs` builds
it with esbuild from the versions `web/package.json` pins and
`web/package-lock.json` locks, and the brain binary embeds it. These
packages are not part of this repository's BSD-3-Clause licensed source and
stay under their own licences, listed here. The bundle also ends with the
full licence text of each package in it.

Licences were checked on 2026-11-05 against the `license` field of each
npm package. The policy behind the committed bundle is
[ADR-0031](../../docs/adr/0031-brain-graph-bundle.md).

## In the bundle

The packages the UI imports:

- sigma 4.0.4, MIT, https://github.com/jacomyal/sigma.js
- graphology 0.26.0, MIT, https://github.com/graphology/graphology
- graphology-layout-forceatlas2 0.10.1, MIT, https://github.com/graphology/graphology
- graphology-layout 0.6.1, MIT, https://github.com/graphology/graphology
- graphology-utils 2.5.2, MIT, https://github.com/graphology/graphology
- markdown-it 15.0.3, MIT, https://github.com/markdown-it/markdown-it

Their dependencies that esbuild puts in the bundle:

- events 3.3.0, MIT, https://github.com/Gozala/events
- entities 8.1.0, BSD-2-Clause, https://github.com/fb55/entities
- linkify-it 6.1.0, MIT, https://github.com/markdown-it/linkify-it
- mdurl 2.1.0, MIT, https://github.com/markdown-it/mdurl
- uc.micro 3.0.0, MIT, https://github.com/markdown-it/uc.micro
- punycode.js 2.3.1, MIT, https://github.com/mathiasbynens/punycode.js

## Build only

These are in the lockfile but not in the bundle:

- esbuild 0.28.2, MIT, https://github.com/evanw/esbuild
- graphology-types 0.24.8, MIT, https://github.com/graphology/graphology
  (type declarations)
- argparse, mnemonist, obliterator and pandemonium, dependencies of the
  packages above that the UI's imports never reach

## Bumping a version

Change the exact version in `web/package.json`, then refresh the lockfile
without running install scripts:

```sh
cd services/brain/web && npm install --package-lock-only --ignore-scripts
```

Run `make bundle` from `services/brain` and commit the lockfile, the
bundle and `assets/SHA256SUMS` together. Update the lists above in the same
change; the build prints the packages it bundled. A test in the `ui`
package fails while the bundle and its sum disagree.
