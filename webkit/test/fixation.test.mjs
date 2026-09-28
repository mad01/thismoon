// Pure-function unit tests for the fixation segmenter (no DOM). Run via
// `node --test`. The header's text-walk builds its <b> nodes from these
// segments, so what is bold here is what is bold on the page.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { fixationSegments, toFixation } from '../src/fixation.ts';

test('fixationSegments: bolds the first half (rounded up) of each word', () => {
  assert.deepEqual(fixationSegments('the code'), [
    { text: 'th', bold: true },
    { text: 'e ', bold: false },
    { text: 'co', bold: true },
    { text: 'de', bold: false },
  ]);
});

test('fixationSegments: merges the plain text around single-letter words', () => {
  // "a" and "I" stay plain and fold into the neighbouring plain runs.
  assert.deepEqual(fixationSegments('a, I am'), [
    { text: 'a, I ', bold: false },
    { text: 'a', bold: true },
    { text: 'm', bold: false },
  ]);
});

test('fixationSegments: text with nothing to bold is one plain segment', () => {
  assert.deepEqual(fixationSegments('   \n '), [{ text: '   \n ', bold: false }]);
  assert.deepEqual(fixationSegments('42 - 7'), [{ text: '42 - 7', bold: false }]);
  assert.deepEqual(fixationSegments(''), []);
});

test('fixationSegments: a letter run touching a digit or underscore stays plain', () => {
  assert.deepEqual(fixationSegments('v2 snake_case'), [{ text: 'v2 snake_case', bold: false }]);
});

test('fixationSegments: segments concatenate back to the input', () => {
  for (const text of ['Hello, world! It works.', 'don\'t stop', 'x', 'über café', '<b>&amp;</b>']) {
    assert.equal(fixationSegments(text).map(s => s.text).join(''), text);
  }
});

test('fixationSegments: never leaves an empty segment or two adjacent runs of one kind', () => {
  const segs = fixationSegments('Reading aids help many readers move faster.');
  assert.ok(segs.every(s => s.text.length > 0));
  for (let i = 1; i < segs.length; i++) assert.notEqual(segs[i].bold, segs[i - 1].bold);
});

test('toFixation: is the string form of the segments', () => {
  for (const text of ['the code', 'a, bb', 'Hello, world! It works.', '']) {
    const want = fixationSegments(text).map(s => (s.bold ? `<b>${s.text}</b>` : s.text)).join('');
    assert.equal(toFixation(text), want);
  }
  assert.equal(toFixation('Hello, world!'), '<b>Hel</b>lo, <b>wor</b>ld!');
});
