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
