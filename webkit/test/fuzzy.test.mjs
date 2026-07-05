// Pure-function unit tests for the ⌘K fuzzy matcher. Run via `node --test`.
// Imports the same source module webkit.ts uses — no logic duplication.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { fuzzyMatch, rankSites } from '../src/fuzzy.ts';

const SITES = [
  { name: 'catalog', host: 'catalog.this', url: 'http://catalog.this/' },
  { name: 'csl', host: 'csl.this', url: 'http://csl.this/' },
  { name: 'status', host: 'status.this', url: 'http://status.this/' },
  { name: 'present', host: 'present.this', url: 'http://present.this/' },
  { name: 'speak', host: 'speak.this', url: 'http://speak.this/' },
];

test('fuzzyMatch: subsequence matches and reports indices', () => {
  const m = fuzzyMatch('ctl', 'catalog');
  assert.ok(m);
  assert.deepEqual(m.indices, [0, 2, 4]); // c-a-T-a-L-og  -> c,t,l
});

test('fuzzyMatch: is case-insensitive', () => {
  assert.ok(fuzzyMatch('CSL', 'csl'));
  assert.ok(fuzzyMatch('csl', 'CSL'));
});

test('fuzzyMatch: returns null when a char is missing or out of order', () => {
  assert.equal(fuzzyMatch('xyz', 'catalog'), null);
  assert.equal(fuzzyMatch('lc', 'csl'), null); // right chars, wrong order
});

test('fuzzyMatch: empty query matches anything with score 0', () => {
  assert.deepEqual(fuzzyMatch('', 'csl'), { score: 0, indices: [] });
});

test('fuzzyMatch: a prefix scores higher than a scattered match', () => {
  const prefix = fuzzyMatch('sta', 'status');   // s-t-a contiguous from start
  const scattered = fuzzyMatch('sta', 'speak');  // no 't' after 's'+'a'? -> null
  assert.ok(prefix);
  assert.equal(scattered, null);
});

test('rankSites: ranks the strongest match first', () => {
  const out = rankSites('sta', SITES);
  assert.equal(out[0].site.name, 'status');
});

test('rankSites: empty query returns every site in original order', () => {
  const out = rankSites('', SITES);
  assert.deepEqual(out.map(o => o.site.name), SITES.map(s => s.name));
});

test('rankSites: drops non-matching sites', () => {
  const out = rankSites('cat', SITES);
  assert.deepEqual(out.map(o => o.site.name), ['catalog']);
});

test('rankSites: falls back to host when the name does not match', () => {
  // "this" is in every host but in no name -> all sites match via host.
  const out = rankSites('this', SITES);
  assert.equal(out.length, SITES.length);
});
