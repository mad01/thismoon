// DOM tests for the settings sync's wiring in webkit.js, run against
// dist/webkit.js in happy-dom (see page.mjs): a page on a .this host whose
// fetch answers /__this/settings.json. The sync's own rules are covered with
// stubs in test/settings-sync.test.mjs; these check what a pull does to the
// page. Run via `npm run test:dom` after `npm ci`, or in the container with
// `make test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { openPage } from './page.mjs';

const SETTINGS_PATH = '/__this/settings.json';
const THIS_HOST = 'http://kit.this/';

function deferred() {
  let resolve;
  const promise = new Promise(r => { resolve = r; });
  return { promise, resolve };
}

// A fetch that answers the settings GET with the settings `answer` resolves
// to, a PATCH with an empty document, and anything else with a 404. `seen`
// records each request.
function settingsFetch(answer, seen = []) {
  const json = (window, body) =>
    new window.Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
  return async (window, url, init) => {
    const method = init.method ?? 'GET';
    seen.push({ url, method });
    if (!url.endsWith(SETTINGS_PATH)) return new window.Response('', { status: 404 });
    if (method !== 'GET') return json(window, { v: 1, settings: {} });
    return json(window, { v: 1, settings: await answer });
  };
}

// Waits for the pull to run its course (the fetch, the body read, the apply).
async function until(check) {
  for (let i = 0; i < 100 && !check(); i++) await new Promise(r => setTimeout(r, 5));
  return check();
}

test('settings sync: a pulled theme is applied, fires wk-themechange and moves the toggle', async t => {
  const answer = deferred();
  const { document, localStorage } = openPage(t, '<wk-header controls="theme"></wk-header>', {
    url: THIS_HOST, fetch: settingsFetch(answer.promise),
  });
  const seen = [];
  document.addEventListener('wk-themechange', e => seen.push({ ...e.detail }));
  answer.resolve({ 'webkit-theme': 'dark', 'webkit-palette-dark': 'nord' });

  assert.equal(await until(() => seen.length > 0), true, 'wk-themechange fires');
  assert.deepEqual(seen, [{ mode: 'dark', theme: 'dark', palette: 'nord' }]);
  assert.equal(localStorage.getItem('webkit-theme'), 'dark');
  assert.equal(document.documentElement.getAttribute('data-theme'), 'dark');
  assert.equal(document.documentElement.getAttribute('data-palette'), 'nord');
  assert.equal(document.getElementById('webkit-theme').getAttribute('aria-pressed'), 'true');
});

for (const putBack of [false, true]) {
  const name = putBack
    ? 'a header taken off the page and put back applies a pulled size again'
    : 'a header taken off the page no longer applies a pulled size';
  test(`settings sync: ${name}`, async t => {
    const answer = deferred();
    const { document, localStorage } = openPage(t, '<div id="slot"><wk-header controls="size"></wk-header></div>', {
      url: THIS_HOST, fetch: settingsFetch(answer.promise),
    });
    assert.equal(document.documentElement.style.fontSize, '16px');
    const header = document.querySelector('wk-header');
    header.remove();
    if (putBack) document.getElementById('slot').append(header);
    answer.resolve({ 'webkit-size': '20' });

    // The pull writes storage and runs the refreshers in one step.
    assert.equal(await until(() => localStorage.getItem('webkit-size') === '20'), true, 'the pull wrote the size');
    assert.equal(document.documentElement.style.fontSize, putBack ? '20px' : '16px');
  });
}

test('settings sync: with site storage blocked the kit still evaluates and sends no settings request', t => {
  const seen = [];
  const window = openPage(t, '', { url: THIS_HOST, fetch: settingsFetch(Promise.resolve({}), seen), blockStorage: true });
  assert.ok(window.customElements.get('wk-header'), 'wk-header is defined');
  assert.equal(typeof window.Webkit?.setThemeMode, 'function');
  assert.deepEqual(seen.filter(c => c.url.endsWith(SETTINGS_PATH)), []);
});
