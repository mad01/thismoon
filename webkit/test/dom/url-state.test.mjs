// DOM test for Webkit.urlState against a real history: write pushes an entry,
// and back and forward call the subscriber with that entry's query. Runs
// against dist/webkit.js in happy-dom (see page.mjs); test/url-state.test.mjs
// covers the pure parts.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { openPage } from './page.mjs';

test('urlState: back and forward after a pushed write call the subscriber', t => {
  const { Webkit, history, location } = openPage(t, '', { url: 'http://kit.test/repos#top' });
  const seen = [];
  const stop = Webkit.urlState.subscribe(params => seen.push(params.q ?? null));

  assert.equal(Webkit.urlState.write({ q: 'moon' }, { push: true }), true);
  assert.equal(location.search, '?q=moon');
  assert.equal(location.hash, '#top');
  assert.deepEqual(seen, [], 'the page\'s own write is not a popstate');

  history.back();
  assert.equal(location.search, '');
  history.forward();
  assert.deepEqual(seen, [null, 'moon']);

  stop();
  history.back();
  assert.equal(seen.length, 2, 'a stopped subscriber is not called');
});

test('urlState: a typed write replaces the entry, an equal one adds none', t => {
  const { Webkit, history } = openPage(t, '', { url: 'http://kit.test/?a=1&b=2' });
  const before = history.length;
  assert.equal(Webkit.urlState.write({ b: '2', a: '1' }, { push: true }), false);
  Webkit.urlState.write({ a: '1', b: '3' });
  assert.equal(history.length, before);
  assert.equal(Webkit.urlState.read().b, '3');
});
