// DOM tests for <wk-header> and the theme API, run against dist/webkit.js in
// happy-dom (see page.mjs). Run via `npm run test:dom` after `npm ci`, or in the
// container with `make test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { openPage } from './page.mjs';

const DEFAULT_CONTROLS = [
  'webkit-cmdk', 'webkit-font', 'webkit-fixation', 'webkit-size-down', 'webkit-size-up',
  'webkit-reload', 'webkit-theme', 'webkit-help',
];

test('wk-header: renders the default controls, each with a label', t => {
  const { document } = openPage(t, '<wk-header brand="kit"></wk-header><main>content</main>');
  const controls = document.querySelector('wk-header .topbar-controls');
  assert.ok(controls, 'the controls row renders');
  for (const id of DEFAULT_CONTROLS) {
    const el = controls.querySelector('#' + id);
    assert.ok(el, `${id} renders`);
    assert.ok(el.getAttribute('aria-label'), `${id} has an aria-label`);
  }
  assert.equal(document.querySelector('wk-header .topbar-brand')?.textContent.trim().length > 0, true);
});

test('wk-header: controls lists exactly the controls that render', t => {
  const { document } = openPage(t, '<wk-header controls="size,theme"></wk-header>');
  const ids = [...document.querySelectorAll('wk-header .topbar-controls [id]')].map(el => el.id);
  assert.deepEqual(ids, ['webkit-size-down', 'webkit-size-up', 'webkit-theme']);
});

test('wk-header: the skip link is the first child and moves focus to main', t => {
  const { document } = openPage(t, '<wk-header brand="kit"></wk-header><main>content</main>');
  const header = document.querySelector('wk-header');
  const skip = header.firstElementChild;
  assert.equal(skip.matches('a.wk-skip-link'), true);
  assert.equal(skip.textContent, 'Skip to content');

  skip.click();
  const main = document.querySelector('main');
  assert.equal(document.activeElement, main);
  assert.equal(main.getAttribute('tabindex'), '-1');
});

test('wk-header: skip-link="off" leaves the skip link out', t => {
  const { document } = openPage(t, '<wk-header brand="kit" skip-link="off"></wk-header><main></main>');
  assert.equal(document.querySelector('.wk-skip-link'), null);
  assert.equal(document.querySelector('wk-header').firstElementChild.className, 'topbar');
});

test('wk-header: nav links and extra buttons move into the bar', t => {
  const { document } = openPage(t,
    '<wk-header brand="kit" controls="theme">' +
    '<a data-nav href="/a">A</a><a data-nav href="/b">B</a>' +
    '<button data-extra id="share">Share</button>' +
    '</wk-header>');
  const nav = [...document.querySelectorAll('wk-header .topbar-left .topbar-nav a')].map(a => a.textContent);
  assert.deepEqual(nav, ['A', 'B']);
  assert.ok(document.querySelector('wk-header .topbar-controls #share'));
});

test('wk-header: a move does not render the bar again', t => {
  const { document } = openPage(t, '<wk-header brand="kit" controls="theme"></wk-header><div id="elsewhere"></div>');
  const header = document.querySelector('wk-header');
  const bar = header.querySelector('.topbar');
  document.getElementById('elsewhere').append(header);
  assert.equal(header.querySelector('.topbar'), bar);
  assert.equal(header.querySelectorAll('.topbar').length, 1);
});

test('theme toggle: a click fires wk-themechange and stores the mode', t => {
  const { document, localStorage } = openPage(t, '<wk-header controls="theme"></wk-header>');
  const toggle = document.getElementById('webkit-theme');
  const seen = [];
  document.addEventListener('wk-themechange', e => seen.push({ ...e.detail }));

  toggle.click();
  assert.deepEqual(seen, [{ mode: 'dark', theme: 'dark', palette: 'default' }]);
  assert.equal(localStorage.getItem('webkit-theme'), 'dark');
  assert.equal(document.documentElement.getAttribute('data-theme'), 'dark');
  assert.equal(toggle.getAttribute('aria-pressed'), 'true');
  assert.equal(toggle.getAttribute('aria-label'), 'Switch to light mode');

  toggle.click();
  assert.equal(seen.at(-1).theme, 'light');
  assert.equal(localStorage.getItem('webkit-theme'), 'light');
  assert.equal(toggle.getAttribute('aria-pressed'), 'false');
});

test('theme API: setThemeMode and setPalette fire wk-themechange with the resolved state', t => {
  const window = openPage(t, '', { storage: { 'webkit-theme': 'dark' } });
  const { document, localStorage, Webkit } = window;
  const seen = [];
  document.addEventListener('wk-themechange', e => seen.push({ ...e.detail }));

  Webkit.setPalette('dark', 'nord');
  assert.deepEqual(seen.at(-1), { mode: 'dark', theme: 'dark', palette: 'nord' });
  assert.equal(localStorage.getItem('webkit-palette-dark'), 'nord');
  assert.equal(document.documentElement.getAttribute('data-palette'), 'nord');

  Webkit.setThemeMode('light');
  assert.deepEqual(seen.at(-1), { mode: 'light', theme: 'light', palette: 'default' });
  assert.equal(document.documentElement.hasAttribute('data-palette'), false);

  Webkit.setPalette('light', 'no-such-family');
  assert.equal(localStorage.getItem('webkit-palette-light'), null);
});

test('theme: a stored mode is applied when the header connects', t => {
  const { document } = openPage(t, '<wk-header controls="theme"></wk-header>', {
    storage: { 'webkit-theme': 'dark' },
  });
  assert.equal(document.documentElement.getAttribute('data-theme'), 'dark');
  assert.equal(document.getElementById('webkit-theme').getAttribute('aria-pressed'), 'true');
});
