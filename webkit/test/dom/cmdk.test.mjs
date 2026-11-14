// DOM tests for the ⌘K site picker, run against dist/webkit.js in happy-dom
// (see page.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { openPage, key } from './page.mjs';

const PAGE = '<wk-header brand="kit"></wk-header><main><button id="other">other</button></main>';

function openPicker(window) {
  window.Webkit.openCmdK();
  const overlay = window.document.querySelector('.wk-cmdk-overlay');
  assert.ok(overlay, 'the picker renders');
  assert.equal(overlay.hasAttribute('hidden'), false, 'the picker is open');
  return overlay;
}

test('cmdk: Escape in the input closes the picker', t => {
  const window = openPage(t, PAGE);
  const overlay = openPicker(window);
  const e = key(window, overlay.querySelector('.wk-cmdk-input'), 'Escape');
  assert.equal(overlay.hasAttribute('hidden'), true);
  assert.equal(e.defaultPrevented, true);
});

test('cmdk: Escape closes the picker when focus has left the input', t => {
  const window = openPage(t, PAGE);
  const overlay = openPicker(window);
  window.document.getElementById('other').focus();
  key(window, window.document.activeElement, 'Escape');
  assert.equal(overlay.hasAttribute('hidden'), true);
});

test('cmdk: Escape with the picker closed is left to the page', t => {
  const window = openPage(t, PAGE);
  const e = key(window, window.document.body, 'Escape');
  assert.equal(e.defaultPrevented, false);
});
