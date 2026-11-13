// Pure-function unit tests for the time and duration formatters. Run via
// `node --test`. Every expected string below is what the hand-written copy in
// the named service printed for the same input before it moved to webkit
// (csl common.js timeAgo, events relTime, deps humanizeSince, moondb ago,
// fmtDuration and fmtSpan); the real timestamps come from those services' APIs.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatDuration, timeAgo } from '../src/format.ts';

const NOW = Date.parse('2026-11-13T10:38:30Z');
const S = 1000, M = 60 * S, H = 60 * M, D = 24 * H;

// [label, ms before NOW, csl + events, deps, moondb]
const AGO = [
  ['0', 0, '0s ago', 'just now', 'just now'],
  ['0.5s', 500, '0s ago', 'just now', 'just now'],
  ['1s', S, '1s ago', 'just now', 'just now'],
  ['59s', 59 * S, '59s ago', 'just now', 'just now'],
  ['59.999s', 59999, '59s ago', 'just now', 'just now'],
  ['60s', 60 * S, '1m ago', '1 minute ago', '1 min ago'],
  ['61s', 61 * S, '1m ago', '1 minute ago', '1 min ago'],
  ['59m', 59 * M, '59m ago', '59 minutes ago', '59 min ago'],
  ['59m59s', 59 * M + 59 * S, '59m ago', '59 minutes ago', '59 min ago'],
  ['60m', 60 * M, '1h ago', '1 hour ago', '1 h ago'],
  ['23h', 23 * H, '23h ago', '23 hours ago', '23 h ago'],
  ['23h59m59s', D - S, '23h ago', '23 hours ago', '23 h ago'],
  ['24h', D, '1d ago', '1 day ago', '24 h ago'],
  ['25h', 25 * H, '1d ago', '1 day ago', '25 h ago'],
  ['47h59m', 48 * H - M, '1d ago', '1 day ago', '47 h ago'],
  ['48h', 48 * H, '2d ago', '2 days ago', '2 d ago'],
  ['6d', 6 * D, '6d ago', '6 days ago', '6 d ago'],
  ['7d', 7 * D, '7d ago', '7 days ago', '7 d ago'],
  ['30d', 30 * D, '30d ago', '30 days ago', '30 d ago'],
  ['400d', 400 * D, '400d ago', '400 days ago', '400 d ago'],
  ['future 5s', -5 * S, '0s ago', 'just now', 'just now'],
  ['future 2h', -2 * H, '0s ago', 'just now', 'just now'],
];

// [timestamp as its API returns it, csl + events, deps, moondb]
const REAL = [
  ['2026-11-13T10:38:02.573652Z', '27s ago', 'just now', 'just now'], // events time
  ['2026-11-13T08:21:25.047781Z', '2h ago', '2 hours ago', '2 h ago'], // deps scanned_at
  ['2026-11-13T11:37:43+01:00', '47s ago', 'just now', 'just now'], // csl last_run
  ['2026-10-13T21:04:56+02:00', '30d ago', '30 days ago', '30 d ago'], // csl indexed_at
  ['2026-11-13T11:37:59.292128+01:00', '30s ago', 'just now', 'just now'], // moondb last_activity_at
];

const MOONDB = { style: 'medium', daysAfterHours: 48 };

test('timeAgo: short is what csl and events print, across every boundary', () => {
  for (const [label, off, short] of AGO) {
    const iso = new Date(NOW - off).toISOString();
    assert.equal(timeAgo(iso, { now: NOW }), short, label);
  }
});

test('timeAgo: long is what deps prints, across every boundary', () => {
  for (const [label, off, , long] of AGO) {
    assert.equal(timeAgo(new Date(NOW - off), { now: NOW, style: 'long' }), long, label);
  }
});

test('timeAgo: medium with days from 48h is what moondb prints, across every boundary', () => {
  for (const [label, off, , , medium] of AGO) {
    assert.equal(timeAgo(new Date(NOW - off), { ...MOONDB, now: new Date(NOW) }), medium, label);
  }
});

test('timeAgo: real API timestamps read as each service printed them', () => {
  for (const [iso, short, long, medium] of REAL) {
    assert.equal(timeAgo(iso, { now: NOW }), short, iso);
    assert.equal(timeAgo(iso, { now: NOW, style: 'long' }), long, iso);
    assert.equal(timeAgo(iso, { ...MOONDB, now: NOW }), medium, iso);
  }
});

test('timeAgo: a Date, an ISO string and epoch ms give the same text, and so do the now shapes', () => {
  const iso = '2026-11-13T08:21:25.047781Z';
  const ms = Date.parse(iso);
  for (const value of [iso, new Date(ms), ms]) {
    for (const now of [NOW, new Date(NOW), new Date(NOW).toISOString()]) {
      assert.equal(timeAgo(value, { now }), '2h ago');
    }
  }
});

test('timeAgo: without now it measures from the clock', () => {
  assert.equal(timeAgo(Date.now() - 5 * M), '5m ago');
});

test('timeAgo: missing or invalid input returns the fallback, never NaN', () => {
  // Before: csl printed '' for '' and 'NaNd ago' for garbage; events printed
  // 'NaNd ago' for both; a Go zero time read as '739932d ago'.
  for (const bad of ['', 'not a date', null, undefined, NaN, Infinity, '0001-01-01T00:00:00Z', {}]) {
    assert.equal(timeAgo(bad, { now: NOW }), '', String(bad));
    assert.equal(timeAgo(bad, { now: NOW, style: 'long', fallback: '-' }), '-', String(bad));
  }
  assert.equal(timeAgo(NOW - M, { now: 'not a date', fallback: 'never' }), 'never');
});

test('timeAgo: an unknown style or a bad daysAfterHours falls back to the defaults', () => {
  assert.equal(timeAgo(NOW - 3 * H, { now: NOW, style: 'loud' }), '3h ago');
  assert.equal(timeAgo(NOW - 30 * H, { now: NOW, daysAfterHours: 0 }), '1d ago');
  assert.equal(timeAgo(NOW - 30 * H, { now: NOW, daysAfterHours: NaN }), '1d ago');
});

test('timeAgo: long pluralizes every unit but one', () => {
  assert.equal(timeAgo(NOW - H, { now: NOW, style: 'long' }), '1 hour ago');
  assert.equal(timeAgo(NOW - 2 * H, { now: NOW, style: 'long' }), '2 hours ago');
  assert.equal(timeAgo(NOW - D, { now: NOW, style: 'long' }), '1 day ago');
});

// [ms, moondb fmtDuration (precise), moondb fmtSpan (coarse)]
const DURATIONS = [
  [0, '0ms', '<1m'],
  [1, '1ms', '<1m'],
  [499.5, '500ms', '<1m'],
  [850, '850ms', '<1m'],
  [999.4, '999ms', '<1m'],
  [999.6, '1000ms', '<1m'],
  [1000, '1.0s', '<1m'],
  [4200, '4.2s', '<1m'],
  [4249, '4.2s', '<1m'],
  [59000, '59.0s', '<1m'],
  [59949, '59.9s', '<1m'],
  [59950, '60.0s', '<1m'],
  [60000, '1m00s', '1m'],
  [192000, '3m12s', '3m'],
  [3599999, '59m59s', '59m'],
  [3600000, '1h00m', '1h 00m'],
  [3900000, '1h05m', '1h 05m'],
  [86399999, '23h59m', '23h 59m'],
  [86400000, '24h00m', '1d 0h'],
  [180000000, '50h00m', '2d 2h'],
  // real moondb session wall and active times
  [49228897, '13h40m', '13h 40m'],
  [2491618, '41m31s', '41m'],
  [3147987, '52m27s', '52m'],
  [710623, '11m50s', '11m'],
];

test('formatDuration: precise is what moondb fmtDuration printed', () => {
  for (const [ms, precise] of DURATIONS) {
    assert.equal(formatDuration(ms), precise, String(ms));
    assert.equal(formatDuration(ms, { style: 'precise' }), precise, String(ms));
  }
});

test('formatDuration: coarse is what moondb fmtSpan printed', () => {
  for (const [ms, , coarse] of DURATIONS) {
    assert.equal(formatDuration(ms, { style: 'coarse' }), coarse, String(ms));
  }
});

test('formatDuration: a negative span counts as zero', () => {
  // Before: fmtDuration(-5000) printed '-5000ms'; fmtSpan already printed '<1m'.
  assert.equal(formatDuration(-5000), '0ms');
  assert.equal(formatDuration(-5000, { style: 'coarse' }), '<1m');
});

test('formatDuration: a value that is not a finite number returns the fallback', () => {
  // Before: fmtDuration(undefined) printed 'NaNhNaNm', fmtSpan 'NaNd NaNh'.
  for (const bad of [undefined, null, NaN, Infinity, '1000']) {
    assert.equal(formatDuration(bad), '', String(bad));
    assert.equal(formatDuration(bad, { style: 'coarse', fallback: '-' }), '-', String(bad));
  }
});
