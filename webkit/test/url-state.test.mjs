// Unit tests for the URL state helper. Run via `node --test`. parse,
// serialize, list and nextHref are pure; read, write and subscribe run against
// a small stand-in for location, history and window, which is how a browser
// tab moves between entries of one page. The URLs pinned below are the ones
// deps, events, moondb and csl wrote by hand before.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { nextHref, parse, read, serialize, subscribe, urlState, write } from '../src/url-state.ts';

const { list } = urlState;

test('parse: the first value of each param, absent ones undefined', () => {
  const p = parse('?q=foo+bar&repo=a%2Cb&level=warn&q=second');
  assert.equal(p.q, 'foo bar');
  assert.equal(p.repo, 'a,b');
  assert.equal(p.level, 'warn');
  assert.equal(p.eco, undefined);
  assert.deepEqual({ ...parse('a=1&b=') }, { a: '1', b: '' });
  assert.deepEqual({ ...parse('') }, {});
});

test('parse: a param named like an Object member is a plain key', () => {
  const p = parse('?constructor=x&__proto__=y&toString=z');
  assert.equal(p.constructor, 'x');
  assert.equal(p.__proto__, 'y');
  assert.equal(p.toString, 'z');
  assert.equal(parse('?a=1').constructor, undefined);
});

test('serialize: leaves out empty values, joins lists, writes true as 1', () => {
  assert.equal(serialize({ q: '', repo: [], eco: null, cwd: undefined, topics: false }), '');
  assert.equal(serialize({ repo: ['thismoon', 'dotfiles'], eco: ['go'], q: 'x y' }),
    'repo=thismoon%2Cdotfiles&eco=go&q=x+y'); // what deps wrote
  assert.equal(serialize({ topics: true, depth: 2 }), 'topics=1&depth=2');
  assert.equal(serialize({ limit: NaN, n: Infinity, zero: 0 }), 'zero=0');
  assert.equal(serialize({ list: ['', 'a', ''] }), 'list=a');
});

test('serialize: a value equal to its default is left out', () => {
  const defaults = { since: '7d', limit: '50' };
  assert.equal(serialize({ repo: '', cwd: '', since: '7d', limit: 50 }, defaults), '');
  assert.equal(serialize({ repo: 'mad01/thismoon', since: '24h', limit: 100 }, defaults),
    'repo=mad01%2Fthismoon&since=24h&limit=100'); // what moondb wrote
  // a defaults key that is only an Object member never matches
  assert.equal(serialize({ toString: 'x' }, {}), 'toString=x');
});

test('list: splits a comma list into its non-empty items', () => {
  assert.deepEqual(list('a,b,,c'), ['a', 'b', 'c']);
  assert.deepEqual(list(''), []);
  assert.deepEqual(list(undefined), []);
  assert.deepEqual(list(null), []);
});

test('nextHref: keeps the path and hash, null when the query would not change', () => {
  const loc = { pathname: '/', search: '?level=warn', hash: '' };
  assert.equal(nextHref({ level: 'warn' }, loc), null);
  assert.equal(nextHref({ level: 'error' }, loc), '/?level=error');
  assert.equal(nextHref({ level: '' }, loc), '/');
  assert.equal(nextHref({ q: 'x' }, { pathname: '/search', search: '', hash: '#top' }), '/search?q=x#top');
  assert.equal(nextHref({ since: '7d' }, { pathname: '/', search: '', hash: '' }, { since: '7d' }), null);
});

test('nextHref: an old link with its params in another order or unencoded is the same view', () => {
  assert.equal(nextHref({ q: 'foo', kind: 'semantic' }, { pathname: '/', search: '?kind=semantic&q=foo', hash: '' }), null);
  assert.equal(nextHref({ repo: ['a', 'b'] }, { pathname: '/', search: '?repo=a,b', hash: '' }), null);
  assert.equal(nextHref({ q: 'a b' }, { pathname: '/', search: '?q=a%20b', hash: '' }), null);
});

// fakeTab stands in for one browser tab: a history stack of URLs, location
// reading the current one, and back/forward firing popstate when the step
// stays inside the page, as a browser does.
function fakeTab(start) {
  const entries = [start];
  let at = 0;
  const handlers = [];
  const loc = {};
  function show() {
    const u = new URL(entries[at], 'http://tool.this');
    loc.pathname = u.pathname;
    loc.search = u.search;
    loc.hash = u.hash;
  }
  function go(step) {
    at += step;
    show();
    handlers.slice().forEach((h) => h({ type: 'popstate' }));
  }
  show();
  globalThis.location = loc;
  globalThis.history = {
    pushState(_s, _t, url) { entries.splice(at + 1); entries.push(url); at++; show(); },
    replaceState(_s, _t, url) { entries[at] = url; show(); },
    back() { go(-1); },
    forward() { go(1); },
  };
  globalThis.window = {
    addEventListener(type, fn) { if (type === 'popstate') handlers.push(fn); },
    removeEventListener(type, fn) {
      const i = handlers.indexOf(fn);
      if (type === 'popstate' && i >= 0) handlers.splice(i, 1);
    },
  };
  return { entries, handlers, loc, at: () => at, go };
}

test('read: parses the current query', () => {
  fakeTab('/?q=hello&sources=a,b');
  const p = read();
  assert.equal(p.q, 'hello');
  assert.deepEqual(list(p.sources), ['a', 'b']);
});

test('write: replaces by default, pushes on request, and skips an unchanged URL', () => {
  const tab = fakeTab('/');
  assert.equal(write({ q: 'f' }), true);
  assert.deepEqual(tab.entries, ['/?q=f']);
  assert.equal(write({ q: 'fo' }), true);
  assert.deepEqual(tab.entries, ['/?q=fo']);
  assert.equal(write({ q: 'fo', level: 'warn' }, { push: true }), true);
  assert.deepEqual(tab.entries, ['/?q=fo', '/?q=fo&level=warn']);
  // the same state again, as a re-render writes it, adds nothing
  assert.equal(write({ q: 'fo', level: 'warn' }, { push: true }), false);
  assert.equal(write({ level: 'warn', q: 'fo' }), false);
  assert.deepEqual(tab.entries, ['/?q=fo', '/?q=fo&level=warn']);
  assert.equal(write({}, { push: true }), true);
  assert.deepEqual(tab.entries, ['/?q=fo', '/?q=fo&level=warn', '/']);
});

test('write: keeps the hash an in-page anchor put there', () => {
  const tab = fakeTab('/?turn=t1#edit-4');
  write({ turn: 't1', view: 'raw' });
  assert.deepEqual(tab.entries, ['/?turn=t1&view=raw#edit-4']);
});

test('subscribe: back and forward bring each pushed view back', () => {
  const tab = fakeTab('/?since=7d');
  const seen = [];
  const stop = subscribe((p) => seen.push(p.since || ''));
  write({ since: '24h' }, { push: true });
  write({ since: '30d' }, { push: true });
  tab.go(-1);
  tab.go(-1);
  tab.go(1);
  assert.deepEqual(seen, ['24h', '7d', '24h']);
  stop();
  tab.go(1);
  assert.deepEqual(seen, ['24h', '7d', '24h']);
  assert.equal(tab.handlers.length, 0);
});

test('subscribe: a step that only moves the hash is not a new view', () => {
  const tab = fakeTab('/?turn=t1');
  const seen = [];
  subscribe((p) => seen.push(p.turn))();
  const stop = subscribe((p) => seen.push(p.turn));
  history.pushState(null, '', '/?turn=t1#edit-4'); // an in-page anchor click
  tab.go(-1);
  tab.go(1);
  assert.deepEqual(seen, []);
  write({ turn: 't2' }, { push: true });
  tab.go(-1);
  assert.deepEqual(seen, ['t1']);
  stop();
});

test('subscribe: every subscriber hears the same step, and stopping one keeps the rest', () => {
  const tab = fakeTab('/');
  const a = [];
  const b = [];
  const stopA = subscribe((p) => a.push(p.q || ''));
  const stopB = subscribe((p) => b.push(p.q || ''));
  write({ q: 'x' }, { push: true });
  tab.go(-1);
  assert.deepEqual(a, ['']);
  assert.deepEqual(b, ['']);
  stopA();
  stopA(); // a second stop is a no-op
  tab.go(1);
  assert.deepEqual(a, ['']);
  assert.deepEqual(b, ['', 'x']);
  stopB();
  assert.equal(tab.handlers.length, 0);
});

test('urlState: the global object carries every function', () => {
  assert.deepEqual(Object.keys(urlState).sort(),
    ['list', 'nextHref', 'parse', 'read', 'serialize', 'subscribe', 'write']);
});
