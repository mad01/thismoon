// Pure-function unit tests for read-aloud's part planning. Run via `node --test`.
// These mirror services/speak/internal/chunk's tests: the browser player and
// the Go side must cut parts the same way.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { group, joinParts, LIVE_RAMP, MAX_PART_CHARS, partKeys } from '../src/sentences.ts';

const SENTENCE_100 = 'x'.repeat(99) + '.';

test('LIVE_RAMP: one sentence, then up to 250, then up to 600', () => {
  assert.deepEqual([...LIVE_RAMP], [0, 250, 600]);
  assert.equal(MAX_PART_CHARS, 600);
});

// Same example as Go's TestGroupFollowsTheRamp.
test('group: follows the ramp, the last bound repeating', () => {
  const pieces = new Array(12).fill(SENTENCE_100);
  // 1 piece; 2 pieces (201 <= 250, 302 > 250); then 3 per part
  // (302 <= 400, 403 > 400).
  assert.deepEqual(group(pieces, [0, 250, 400]), [
    [0], [1, 2], [3, 4, 5], [6, 7, 8], [9, 10, 11],
  ]);
});

test('group: an oversized piece gets a part of its own', () => {
  const pieces = ['Short.', 'y'.repeat(300), 'Tail.'];
  assert.deepEqual(group(pieces, [250]), [[0], [1], [2]]);
});

test('group: the live ramp starts with a single sentence', () => {
  const pieces = ['Hello there.', 'This is the second.', 'And a third'];
  assert.deepEqual(group(pieces, LIVE_RAMP), [[0], [1, 2]]);
});

test('group: counts characters, not UTF-16 code units', () => {
  // Each emoji is one character but two code units: 2 + 1 + 2 = 5 fits.
  assert.deepEqual(group(['🙂🙂', '🙂🙂'], [5]), [[0, 1]]);
});

test('group: no pieces, no parts', () => {
  assert.deepEqual(group([], LIVE_RAMP), []);
});

test('joinParts: a heading gets a period instead of running on', () => {
  assert.equal(joinParts(['Results', 'It passed.']), 'Results. It passed.');
});

test('joinParts: keeps terminal punctuation', () => {
  assert.equal(joinParts(['Why?', 'Because!', 'Note:', 'Wait…', 'Then;']), 'Why? Because! Note: Wait… Then;');
});

test('joinParts: looks past closing brackets and quotes', () => {
  assert.equal(joinParts(['(see below.)', 'Next']), '(see below.) Next.');
  assert.equal(joinParts(['It said ’done.’']), 'It said ’done.’');
  assert.equal(joinParts(['(aside)']), '(aside).');
});

test('joinParts: drops blank pieces and trims', () => {
  assert.equal(joinParts([' ', ' One. ']), 'One.');
});

// A loose list item whose two paragraphs straddle a nested list: the outer
// item reads parts k1 and k3, the nested one k2. Block order alone says
// k1, k3, k2.
test('partKeys: plays the planned order, repeats kept', () => {
  assert.deepEqual(partKeys('k1 k2 k3 k1', ['k1 k3', 'k2']), ['k1', 'k2', 'k3', 'k1']);
});

test('partKeys: without a plan, block order with each key once', () => {
  assert.deepEqual(partKeys(undefined, ['k1', 'k1 k2', ' k3  ']), ['k1', 'k2', 'k3']);
  assert.deepEqual(partKeys('  ', ['k1']), ['k1']);
});

test('partKeys: nothing keyed, nothing to play', () => {
  assert.deepEqual(partKeys(null, []), []);
});
