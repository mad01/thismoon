// Builds the shared webkit bundle that the Go tools embed.
//   src/webkit.ts        -> dist/webkit.js    (IIFE exposing window.Webkit)
//   src/themes/*.json    -> palette CSS blocks prepended to dist/webkit.css,
//                           and dist/themes.json (the resolved collection)
//   src/webkit.css       -> dist/webkit.css   (after the generated palette)
//   src/boot.snippet.js  -> dist/boot.js      (the FOUC-guard IIFE, served at
//                                              GET /webkit/boot.js)
//   src/themes-page.ts   -> dist/themes.js    (the picker, served with
//   src/themes.html      -> dist/themes.html   GET /webkit/themes)
// dist/ is committed so `go:embed` works without a node toolchain in consumers.
import * as esbuild from 'esbuild';
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { bootSnippet } from './src/boot.snippet.js';
import { buildThemes } from './src/themes.mjs';

mkdirSync('dist', { recursive: true });

await esbuild.build({
  entryPoints: ['src/webkit.ts'],
  bundle: true,
  format: 'iife',
  globalName: 'Webkit',
  target: ['es2020'],
  outfile: 'dist/webkit.js',
  minify: false,
  legalComments: 'none',
});

// The themes page script runs beside webkit.js and uses the Webkit global, so
// it bundles nothing of the kit itself.
await esbuild.build({
  entryPoints: ['src/themes-page.ts'],
  bundle: true,
  format: 'iife',
  target: ['es2020'],
  outfile: 'dist/themes.js',
  minify: false,
  legalComments: 'none',
});

// Palette first, then the hand-written stylesheet: every --role the rules
// below read is declared by the generated blocks for every family and mode.
const themes = buildThemes('src/themes');
writeFileSync('dist/webkit.css', themes.css + '\n' + readFileSync('src/webkit.css', 'utf8'));
writeFileSync('dist/themes.json', themes.json);
copyFileSync('src/themes.html', 'dist/themes.html');

// Emit the FOUC-guard boot snippet as a standalone classic script. Same source
// string as Webkit.bootSnippet, so the served file can never drift from the
// JS export. Trailing newline keeps it a tidy text file.
writeFileSync('dist/boot.js', bootSnippet + '\n');

console.log(`webkit: built dist/webkit.js + dist/webkit.css (${themes.families.length} theme families) + dist/themes.{json,html,js} + dist/boot.js`);
