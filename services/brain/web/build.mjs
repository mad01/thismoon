// Builds dist/brain-graph.bundle.js from entry.mjs and writes dist/SHA256SUMS
// beside it. `make bundle` runs this in a container and copies both files
// into internal/server/ui/assets/; `make bundle-local` runs it on the host.
// The output depends only on package-lock.json and the files here, so a
// rebuild from the same lockfile gives the same bytes and the same sum.
//
// The bundle ends with the licence text of every package esbuild put in it,
// read from node_modules, so the shipped file carries its own notices.
import * as esbuild from 'esbuild';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';

const NAME = 'brain-graph.bundle.js';
const OUT = 'dist/' + NAME;

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8'));
}

// bundledPackages lists the npm packages behind the bundle's inputs, sorted.
function bundledPackages(metafile) {
  const names = new Set();
  for (const input of Object.keys(metafile.inputs)) {
    const m = input.match(/^node_modules\/((?:@[^/]+\/)?[^/]+)\//);
    if (m) names.add(m[1]);
  }
  return [...names].sort();
}

// notice is one package's licence as a block comment. A "*/" in the text
// would end the comment early, so it is broken up.
function notice(name) {
  const dir = 'node_modules/' + name;
  const pkg = readJSON(dir + '/package.json');
  const file = readdirSync(dir).find((f) => /^licen[cs]e/i.test(f));
  if (!file) throw new Error(name + ' ships no licence file; add its notice by hand');
  const text = readFileSync(dir + '/' + file, 'utf8').trim().replaceAll('*/', '* /');
  return '/*! ' + name + ' ' + pkg.version + ' (' + pkg.license + ')\n\n' + text + '\n*/';
}

const pkg = readJSON('package.json');
const pinned = Object.keys(pkg.dependencies)
  .filter((name) => name !== 'graphology-types')
  .map((name) => name + ' ' + readJSON('node_modules/' + name + '/package.json').version)
  .join(', ');

mkdirSync('dist', { recursive: true });

const result = await esbuild.build({
  entryPoints: ['entry.mjs'],
  bundle: true,
  format: 'iife',
  target: ['es2020'],
  platform: 'browser',
  minify: true,
  legalComments: 'none',
  metafile: true,
  write: false,
  outfile: OUT,
});

const packages = bundledPackages(result.metafile);
const banner = '/* brain-graph bundle: ' + pinned + '. Built by services/brain/web/build.mjs ' +
  'from web/package-lock.json; licences at the end of this file and in services/brain/THIRD-PARTY.md. ' +
  'Do not edit. */\n';
const bytes = Buffer.from(banner + result.outputFiles[0].text + packages.map(notice).join('\n') + '\n');
writeFileSync(OUT, bytes);

const sum = createHash('sha256').update(bytes).digest('hex');
writeFileSync('dist/SHA256SUMS', sum + '  ' + NAME + '\n');
console.log('brain-graph: built ' + OUT + ' (' + bytes.length + ' bytes), sha256 ' + sum);
console.log('brain-graph: bundled ' + packages.join(', '));
