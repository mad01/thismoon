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
import { bootSnippetFor } from './src/boot.snippet.js';
import { buildThemes } from './src/themes.mjs';

mkdirSync('dist', { recursive: true });

// The theme collection comes first: its family names are injected into both
// bundles (WEBKIT_FAMILIES, declared in src/globals.d.ts) so a stored palette
// name is validated against the shipped list without a fetch. The value is a
// JSON string, not an array: esbuild inlines a string literal in place, while
// an array becomes a hoisted virtual module with an init call per file.
const themes = buildThemes('src/themes');
const familyNames = themes.families.map(f => f.name);
const define = { WEBKIT_FAMILIES: JSON.stringify(JSON.stringify(familyNames)) };

await esbuild.build({
  entryPoints: ['src/webkit.ts'],
  bundle: true,
  format: 'iife',
  globalName: 'Webkit',
  target: ['es2020'],
  outfile: 'dist/webkit.js',
  minify: false,
  legalComments: 'none',
  define,
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
  define,
});

// Palette first, then the hand-written stylesheet: every --role the rules
// below read is declared by the generated blocks for every family and mode.
writeFileSync('dist/webkit.css', themes.css + '\n' + readFileSync('src/webkit.css', 'utf8'));
writeFileSync('dist/themes.json', themes.json);
copyFileSync('src/themes.html', 'dist/themes.html');

// Emit the FOUC-guard boot snippet as a standalone classic script, built for
// the same family list the bundle got, so the served file can never drift
// from Webkit.bootSnippet. Trailing newline keeps it a tidy text file.
writeFileSync('dist/boot.js', bootSnippetFor(familyNames) + '\n');

console.log(`webkit: built dist/webkit.js + dist/webkit.css (${themes.families.length} theme families) + dist/themes.{json,html,js} + dist/boot.js`);
