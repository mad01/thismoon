// Theme derivation and validation, run via `node --test`. The literal default
// palette is pinned on the Go side (themes_test.go); these cover the pure
// functions and the file contract.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import {
  AUTHORED_ROLES, REFERENCE_ROLES, TONES,
  mix, alpha, luminance, deriveVariant, validateFamily, resolveFamily, renderCSS, loadFamilies,
} from '../src/themes.mjs';

const variant = {
  variant: 'Test',
  bg: '#101010', paper: '#202020', chip: '#303030', border: '#404040', 'border-hover': '#505050',
  'text-1': '#f0f0f0', 'text-2': '#c0c0c0', 'text-3': '#909090',
  primary: '#ff8000', 'on-primary': '#101010',
  red: '#ff0000', green: '#00ff00', amber: '#ffaa00', yellow: '#ffff00', blue: '#0000ff', purple: '#aa00ff',
  series: ['#111111', '#222222', '#333333', '#444444'],
};
const family = { name: 'test', label: 'Test', source: 'https://example.com/test', license: 'MIT', dark: variant };

test('mix: moves a toward b by t, in plain sRGB', () => {
  assert.equal(mix('#000000', '#ffffff', 0), '#000000');
  assert.equal(mix('#000000', '#ffffff', 1), '#ffffff');
  assert.equal(mix('#000000', '#ffffff', 0.5), '#808080');
  assert.equal(mix('#C4704B', '#FFFFFF', 0.2), '#d08d6f');
});

test('alpha: rgba string with the channels and the opacity', () => {
  assert.equal(alpha('#FAF9F7', 0.88), 'rgba(250,249,247,0.88)');
});

test('luminance: black is 0, white is 1, mid grey below 0.4', () => {
  assert.equal(luminance('#000000'), 0);
  assert.equal(luminance('#ffffff'), 1);
  assert.ok(luminance('#4A9E6B') < 0.4);
  assert.ok(luminance('#a3be8c') > 0.4);
});

test('deriveVariant: authored roles come through, series fan out, derived roles follow the rules', () => {
  const r = deriveVariant(variant, 'dark');
  for (const role of AUTHORED_ROLES) assert.equal(r[role], variant[role]);
  assert.equal(r['series-1'], '#111111');
  assert.equal(r['series-4'], '#444444');
  assert.equal(r['card-bg'], variant.paper);
  assert.equal(r['chip-active-text'], variant['on-primary']);
  assert.equal(r['code-bg'], variant.bg, 'dark code sits on the page background');
  assert.equal(deriveVariant(variant, 'light')['code-bg'], variant.paper, 'light code sits on paper');
  assert.equal(r['graph-hot'], variant.primary);
  assert.equal(r['graph-module-2'], variant.blue);
  assert.equal(r['tone-green-border'], variant.green);
  assert.equal(r['tone-neutral-bg'], variant.chip);
  assert.equal(r['topbar-bg'], 'rgba(16,16,16,0.88)');
  assert.equal(r['scrim'], 'rgba(32,32,32,0.45)', 'dark scrim is paper at 45%');
  assert.equal(r['terracotta'], variant.primary, 'legacy alias');
  assert.equal(r['on-red'], '#FFFFFF', 'white on a dark fill');
  assert.equal(r['on-yellow'], variant.bg, 'the deepest ink on a pale fill in dark mode');
  assert.equal(deriveVariant(variant, 'light')['on-yellow'], variant['text-1'], 'and text-1 in light mode');
});

test('deriveVariant: overrides win over derivation but not over authored roles', () => {
  const r = deriveVariant({ ...variant, overrides: { 'tag-a': '#123456', scrim: 'rgba(1,2,3,0.5)' } }, 'dark');
  assert.equal(r['tag-a'], '#123456');
  assert.equal(r.scrim, 'rgba(1,2,3,0.5)');
});

test('every reference role and every tone triple is derived', () => {
  const r = deriveVariant(variant, 'light');
  for (const role of REFERENCE_ROLES) assert.ok(role in r, `missing ${role}`);
  for (const t of TONES) for (const part of ['bg', 'border', 'text']) assert.ok(`tone-${t}-${part}` in r);
});

test('validateFamily: accepts a complete family and names the fault otherwise', () => {
  assert.equal(validateFamily('test.json', family), family);
  assert.throws(() => validateFamily('t.json', { ...family, name: 'Bad Name' }), /name must match/);
  assert.throws(() => validateFamily('t.json', { ...family, license: '' }), /license must be/);
  assert.throws(() => validateFamily('t.json', { name: 'x', label: 'x', source: 'x', license: 'x' }), /needs a light or a dark/);
  assert.throws(() => validateFamily('t.json', { ...family, dark: { ...variant, red: 'red' } }), /dark\.red must be a 6-digit hex/);
  assert.throws(() => validateFamily('t.json', { ...family, dark: { ...variant, series: ['#000000'] } }), /series must be 4/);
  assert.throws(() => validateFamily('t.json', { ...family, dark: { ...variant, extra: '#000000' } }), /not an authored role/);
  assert.throws(() => validateFamily('t.json', { ...family, dark: { ...variant, overrides: { bg: '#000000' } } }), /not a derived role/);
  assert.throws(() => validateFamily('t.json', { ...family, dark: { ...variant, overrides: { nope: '#000000' } } }), /not a derived role/);
  assert.throws(() => validateFamily('t.json', { ...family, dark: { ...variant, overrides: { 'tag-a': 'var(--x)' } } }), /must be hex or rgba/);
});

test('resolveFamily: refuses a canvas role that is not a hex literal', () => {
  const bad = { ...family, dark: { ...variant, overrides: { 'graph-hot': 'rgba(0,0,0,0.5)' } } };
  assert.throws(() => resolveFamily(bad), /graph-hot must be a literal hex/);
});

test('renderCSS: default doubles as :root and the bare dark selector, others are attribute-scoped', () => {
  const def = resolveFamily({ ...family, name: 'default', light: variant, dark: variant });
  const other = resolveFamily(family);
  const css = renderCSS([def, other]);
  assert.ok(css.startsWith('/* generated by build.mjs'));
  assert.ok(css.includes(':root, [data-palette="default"][data-theme="light"] {\n  --bg: #101010;'));
  assert.ok(css.includes('[data-theme="dark"], [data-palette="default"][data-theme="dark"] {'));
  assert.ok(css.includes('\n[data-palette="test"][data-theme="dark"] {'));
  assert.ok(!css.includes('[data-palette="test"][data-theme="light"]'), 'a family without a light variant gets no light block');
  assert.ok(!css.includes('var('), 'every value is a literal');
});

test('the shipped collection loads: default first, both variants, licence and source on every file', () => {
  const families = loadFamilies(new URL('../src/themes', import.meta.url).pathname);
  assert.equal(families[0].name, 'default');
  assert.ok(families[0].light && families[0].dark);
  assert.ok(families.length >= 8);
  for (const f of families) {
    assert.ok(f.license && f.source, `${f.name} lacks licence or source`);
    for (const mode of ['light', 'dark']) {
      if (!f[mode]) continue;
      for (const role of REFERENCE_ROLES) assert.match(f[mode].roles[role], /^#[0-9a-fA-F]{6}$/, `${f.name} ${mode} ${role}`);
    }
  }
});

// The gallery used to hand-paste the boot snippet and drifted from the source.
test('gallery: loads dist/boot.js instead of an inline copy of the snippet', () => {
  const html = readFileSync(new URL('../examples/gallery.html', import.meta.url), 'utf8');
  assert.ok(html.includes('<script src="../dist/boot.js"></script>'));
  assert.ok(!html.includes("localStorage.getItem('webkit-theme')"), 'gallery inlines the boot snippet');
});
