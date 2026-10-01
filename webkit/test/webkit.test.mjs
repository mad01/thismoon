// Pure-function unit tests (no DOM). Run via `node --test`.
// Imports the same source modules webkit.ts uses — no logic duplication.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { toFixation } from '../src/fixation.ts';
import { clampSize, SIZE_MIN, SIZE_MAX } from '../src/size.ts';

test('toFixation: single-character word is returned unchanged (length <= 1 edge)', () => {
  assert.equal(toFixation('a'), 'a');
  assert.equal(toFixation('I'), 'I');
});

test('toFixation: bolds the first half of each word in a multi-word string', () => {
  // "the" (len 3) -> ceil(3/2)=2 -> <b>th</b>e
  // "code" (len 4) -> ceil(4/2)=2 -> <b>co</b>de
  assert.equal(toFixation('the code'), '<b>th</b>e <b>co</b>de');
});

test('toFixation: leaves non-letter runs intact between words', () => {
  assert.equal(toFixation('a, bb'), 'a, <b>b</b>b');
});

test('clampSize: pins to the lower bound (12)', () => {
  assert.equal(clampSize(0), SIZE_MIN);
  assert.equal(clampSize(-100), 12);
  assert.equal(clampSize(11), 12);
});

test('clampSize: pins to the upper bound (24)', () => {
  assert.equal(clampSize(99), SIZE_MAX);
  assert.equal(clampSize(25), 24);
});

test('clampSize: passes through values within range', () => {
  assert.equal(clampSize(16), 16);
  assert.equal(clampSize(12), 12);
  assert.equal(clampSize(24), 24);
});

import { bootSnippetFor } from '../src/boot.snippet.js';

const bootSnippet = bootSnippetFor(['default']);

// The boot snippet can't import size.ts (it must stay a plain classic-script
// string), so it hardcodes the clamp bounds. This pins them to the real
// SIZE_MIN/SIZE_MAX so the two can't drift apart silently.
test('bootSnippet: clamp bounds match size.ts SIZE_MIN/SIZE_MAX', () => {
  assert.ok(
    bootSnippet.includes(`Math.max(${SIZE_MIN},Math.min(${SIZE_MAX},`),
    `bootSnippet clamp does not use [${SIZE_MIN}, ${SIZE_MAX}]`,
  );
});

import { segmentSentences, speakable } from '../src/sentences.ts';

test('segmentSentences: splits prose into trimmed sentences', () => {
  const got = segmentSentences('First sentence. Second one! Third?');
  assert.deepEqual(got.map(s => s.text), ['First sentence.', 'Second one!', 'Third?']);
});

test('segmentSentences: offsets cover the untrimmed source segments', () => {
  const src = 'One. Two.';
  const got = segmentSentences(src);
  assert.equal(got.length, 2);
  assert.equal(got[0].start, 0);
  assert.equal(src.slice(got[1].start, got[1].end).trim(), 'Two.');
});

test('segmentSentences: base shifts offsets into document coordinates', () => {
  const got = segmentSentences('Hello there.', 100);
  assert.equal(got[0].start, 100);
  assert.equal(got[0].end, 112);
});

test('segmentSentences: drops whitespace-only input', () => {
  assert.deepEqual(segmentSentences('   \n  '), []);
});

test('speakable: strips backticks and quote marks', () => {
  assert.equal(speakable('the `pkg/mixer` class'), 'the pkg/mixer class');
  assert.equal(speakable('he said “hello world”'), 'he said hello world');
  assert.equal(speakable('a "quoted string" here'), 'a quoted string here');
});

test('speakable: keeps apostrophes', () => {
  assert.equal(speakable("don't stop"), "don't stop");
});

test('speakable: collapses whitespace', () => {
  assert.equal(speakable('one\n  two\tthree '), 'one two three');
});

test('speakable: quote-only input becomes empty', () => {
  assert.equal(speakable('“”'), '');
});

// The audio toggle's default is per view: a shell names the storage key and
// the default on <html> (data-audio-key, data-audio-default), and boot.js
// resolves data-audio from them before paint, so a deck opens with the
// read-aloud controls hidden and a brief with them shown, whatever the other
// view stored. The snippet is run against stubs for the three globals it
// touches; what it set on <html> is the result.
function bootAudio({ stored = {}, attrs = {} } = {}) {
  const set = {};
  const root = {
    getAttribute: name => (name in attrs ? attrs[name] : null),
    setAttribute: (name, value) => { set[name] = value; },
    style: {},
  };
  new Function('document', 'localStorage', 'window', bootSnippet)(
    { documentElement: root },
    { getItem: key => (key in stored ? stored[key] : null) },
    {},
  );
  return set['data-audio'];
}

test('bootSnippet: audio is on when nothing is stored and the shell names no default', () => {
  assert.equal(bootAudio(), 'on');
});

test('bootSnippet: audio follows the stored value under the default key', () => {
  assert.equal(bootAudio({ stored: { 'webkit-audio': 'on' } }), 'on');
  assert.equal(bootAudio({ stored: { 'webkit-audio': 'off' } }), 'off');
});

test('bootSnippet: a deck shell opens off under its own key', () => {
  const deck = { 'data-audio-key': 'webkit-audio-deck', 'data-audio-default': 'off' };
  assert.equal(bootAudio({ attrs: deck }), 'off');
  // The brief's stored choice is another key and does not reach the deck.
  assert.equal(bootAudio({ attrs: deck, stored: { 'webkit-audio': 'on' } }), 'off');
  assert.equal(bootAudio({ attrs: deck, stored: { 'webkit-audio-deck': 'on' } }), 'on');
});

test('bootSnippet: a brief opens on whatever the deck stored', () => {
  assert.equal(bootAudio({ stored: { 'webkit-audio-deck': 'off' } }), 'on');
});

test('bootSnippet: an unknown stored value falls back to the view default', () => {
  assert.equal(bootAudio({ stored: { 'webkit-audio': 'maybe' } }), 'on');
  const deck = { 'data-audio-key': 'webkit-audio-deck', 'data-audio-default': 'off' };
  assert.equal(bootAudio({ attrs: deck, stored: { 'webkit-audio-deck': '' } }), 'off');
});
