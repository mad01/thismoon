// Builds the shared webkit bundle that the Go tools embed.
//   src/webkit.ts       -> dist/webkit.js   (IIFE exposing window.Webkit)
//   src/webkit.css      -> dist/webkit.css  (copied verbatim)
//   src/boot.snippet.js -> dist/boot.js     (the FOUC-guard IIFE, served at
//                                            GET /webkit/boot.js)
// dist/ is committed so `go:embed` works without a node toolchain in consumers.
import * as esbuild from 'esbuild';
import { copyFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { bootSnippet } from './src/boot.snippet.js';

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

copyFileSync('src/webkit.css', 'dist/webkit.css');

// Emit the FOUC-guard boot snippet as a standalone classic script. Same source
// string as Webkit.bootSnippet, so the served file can never drift from the
// JS export. Trailing newline keeps it a tidy text file.
writeFileSync('dist/boot.js', bootSnippet + '\n');

console.log('webkit: built dist/webkit.js + dist/webkit.css + dist/boot.js');
