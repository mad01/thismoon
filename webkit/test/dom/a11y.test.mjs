// DOM tests for the behaviour wk-title and wk-badge carry, run against
// dist/webkit.js in happy-dom (see page.mjs). test/a11y.test.mjs holds the
// decisions on stand-in elements; these check the elements act on them.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { clicks, key, openPage } from './page.mjs';

test('wk-title: outside a card it is the level 1 page heading', t => {
  const { document } = openPage(t, '<wk-page-header><wk-title>Repos</wk-title></wk-page-header>');
  const title = document.querySelector('wk-title');
  assert.equal(title.getAttribute('role'), 'heading');
  assert.equal(title.getAttribute('aria-level'), '1');
});

test('wk-title: in a card, or with a role of its own, it is left alone', t => {
  const { document } = openPage(t,
    '<wk-card><wk-title id="card">Card</wk-title></wk-card>' +
    '<wk-title id="own" role="presentation">Own</wk-title>' +
    '<wk-title id="level" aria-level="2">Level</wk-title>');
  assert.equal(document.getElementById('card').hasAttribute('role'), false);
  assert.equal(document.getElementById('own').getAttribute('role'), 'presentation');
  assert.equal(document.getElementById('own').hasAttribute('aria-level'), false);
  assert.equal(document.getElementById('level').getAttribute('aria-level'), '2');
});

test('wk-badge filter: a button the keyboard presses with Enter and Space', t => {
  const window = openPage(t, '<wk-chips><wk-badge variant="filter">go</wk-badge></wk-chips>');
  const badge = window.document.querySelector('wk-badge');
  assert.equal(badge.getAttribute('role'), 'button');
  assert.equal(badge.getAttribute('tabindex'), '0');

  const pressed = clicks(badge);
  assert.equal(key(window, badge, 'Enter').defaultPrevented, true);
  key(window, badge, ' ');
  key(window, badge, 'a');
  assert.equal(pressed.count, 2);
});

test('wk-badge: other variants and badges the page wired stay as they are', t => {
  const window = openPage(t,
    '<wk-badge id="plain" variant="ok">ok</wk-badge>' +
    '<wk-badge id="own" variant="filter" tabindex="-1">own</wk-badge>' +
    '<a href="/x"><wk-badge id="linked" variant="filter">linked</wk-badge></a>');
  const { document } = window;
  assert.equal(document.getElementById('plain').hasAttribute('role'), false);
  assert.equal(document.getElementById('own').getAttribute('tabindex'), '-1');
  assert.equal(document.getElementById('own').hasAttribute('role'), false);
  assert.equal(document.getElementById('linked').hasAttribute('role'), false);

  const own = document.getElementById('own');
  const pressed = clicks(own);
  key(window, own, 'Enter');
  assert.equal(pressed.count, 0);
});

test('wk-badge filter: released in a link or button, managed again when moved out', t => {
  const window = openPage(t,
    '<wk-chips id="chips"><wk-badge variant="filter">go</wk-badge></wk-chips>' +
    '<a id="link" href="/x"></a><button id="btn"></button>');
  const { document } = window;
  const badge = document.querySelector('wk-badge');
  const pressed = clicks(badge);

  document.getElementById('link').append(badge);
  assert.equal(badge.hasAttribute('role'), false);
  assert.equal(badge.hasAttribute('tabindex'), false);
  key(window, badge, 'Enter');
  assert.equal(pressed.count, 0, 'a released badge does not press on Enter');

  document.getElementById('chips').append(badge);
  assert.equal(badge.getAttribute('role'), 'button');
  assert.equal(badge.getAttribute('tabindex'), '0');
  key(window, badge, 'Enter');
  assert.equal(pressed.count, 1);

  document.getElementById('btn').append(badge);
  assert.equal(badge.hasAttribute('role'), false);
});

test('wk-badge: a variant change to or from filter takes or hands back the keys', t => {
  const { document } = openPage(t, '<wk-badge variant="outline">x</wk-badge>');
  const badge = document.querySelector('wk-badge');
  assert.equal(badge.hasAttribute('role'), false);
  badge.setAttribute('variant', 'filter');
  assert.equal(badge.getAttribute('role'), 'button');
  badge.setAttribute('variant', 'outline');
  assert.equal(badge.hasAttribute('role'), false);
  assert.equal(badge.hasAttribute('tabindex'), false);
});

test('wk-badge filter: a badge inserted later is wired on connect', t => {
  const { document } = openPage(t, '<wk-chips></wk-chips>');
  const badge = document.createElement('wk-badge');
  badge.setAttribute('variant', 'filter');
  document.querySelector('wk-chips').append(badge);
  assert.equal(badge.getAttribute('role'), 'button');
});
