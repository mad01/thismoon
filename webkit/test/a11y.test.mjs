// Pure-function unit tests for the keyboard path's decisions. Run via `node --test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  INTERACTIVE_ANCESTORS, isActivationKey, skipTarget, TITLE_CONTAINERS, wantsFilterKeys, wantsPageHeading,
} from '../src/a11y.ts';

// A stand-in for an element: its attributes, and the ancestor closest()
// finds for each selector (null when it sits outside one).
function element(attrs, ancestors = {}) {
  return {
    hasAttribute: name => Object.hasOwn(attrs, name),
    getAttribute: name => (Object.hasOwn(attrs, name) ? attrs[name] : null),
    closest: selector => ancestors[selector] ?? null,
  };
}

test('wantsPageHeading: a bare wk-title is the page heading', () => {
  assert.equal(wantsPageHeading(element({})), true);
});

test('wantsPageHeading: a title in a card or panel, or with its own role, is left alone', () => {
  assert.equal(wantsPageHeading(element({}, { [TITLE_CONTAINERS]: { tagName: 'WK-CARD' } })), false);
  assert.equal(wantsPageHeading(element({ role: 'presentation' })), false);
  assert.match(TITLE_CONTAINERS, /wk-card/);
  assert.match(TITLE_CONTAINERS, /\.wk-card/);
  assert.match(TITLE_CONTAINERS, /wk-panel/);
});

test('wantsFilterKeys: a filter badge without tabindex or role gets the keyboard', () => {
  assert.equal(wantsFilterKeys(element({ variant: 'filter' })), true);
  assert.equal(wantsFilterKeys(element({ variant: 'filter', active: '' })), true);
});

test('wantsFilterKeys: a page-wired filter badge or any other variant is left alone', () => {
  assert.equal(wantsFilterKeys(element({ variant: 'filter', tabindex: '0' })), false);
  assert.equal(wantsFilterKeys(element({ variant: 'filter', tabindex: '-1' })), false);
  assert.equal(wantsFilterKeys(element({ variant: 'filter', role: 'button' })), false);
  assert.equal(wantsFilterKeys(element({ variant: 'stat' })), false);
  assert.equal(wantsFilterKeys(element({})), false);
});

test('wantsFilterKeys: a filter badge inside a link or button stays a label', () => {
  assert.equal(wantsFilterKeys(element({ variant: 'filter' }, { [INTERACTIVE_ANCESTORS]: { tagName: 'A' } })), false);
  assert.equal(wantsFilterKeys(element({ variant: 'filter' }, { [INTERACTIVE_ANCESTORS]: { tagName: 'BUTTON' } })), false);
  assert.match(INTERACTIVE_ANCESTORS, /\ba\b/);
  assert.match(INTERACTIVE_ANCESTORS, /\bbutton\b/);
});

// The role and tabindex webkit set on a badge it manages.
const MANAGED = { variant: 'filter', role: 'button', tabindex: '0' };

test('wantsFilterKeys: a managed badge keeps the keyboard despite the role and tabindex webkit set', () => {
  assert.equal(wantsFilterKeys(element(MANAGED), true), true);
  assert.equal(wantsFilterKeys(element({ ...MANAGED, active: '' }), true), true);
  // Unmanaged, the same attributes read as the page's own wiring.
  assert.equal(wantsFilterKeys(element(MANAGED)), false);
});

test('wantsFilterKeys: a managed badge moved into a link or button is released', () => {
  assert.equal(wantsFilterKeys(element(MANAGED, { [INTERACTIVE_ANCESTORS]: { tagName: 'A' } }), true), false);
  assert.equal(wantsFilterKeys(element(MANAGED, { [INTERACTIVE_ANCESTORS]: { tagName: 'BUTTON' } }), true), false);
});

test('wantsFilterKeys: a managed badge that is no longer a filter is released', () => {
  assert.equal(wantsFilterKeys(element({ ...MANAGED, variant: 'stat' }), true), false);
  assert.equal(wantsFilterKeys(element({ role: 'button', tabindex: '0' }), true), false);
});

test('isActivationKey: Enter and Space press, other keys do not', () => {
  assert.equal(isActivationKey('Enter'), true);
  assert.equal(isActivationKey(' '), true);
  for (const key of ['Spacebar', 'Tab', 'Escape', 'a', 'ArrowDown']) {
    assert.equal(isActivationKey(key), false, key);
  }
});

// A stand-in document that holds the given selector matches.
function doc(matches) {
  return { querySelector: selector => matches[selector] ?? null };
}

test('skipTarget: the first main wins, then .wrap, then the header\'s next sibling', () => {
  const header = { nextElementSibling: 'after-header' };
  assert.equal(skipTarget(doc({ main: 'main', '.wrap': 'wrap' }), header), 'main');
  assert.equal(skipTarget(doc({ '.wrap': 'wrap' }), header), 'wrap');
  assert.equal(skipTarget(doc({}), header), 'after-header');
  assert.equal(skipTarget(doc({}), { nextElementSibling: null }), null);
});
