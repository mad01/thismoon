// Pure-function unit tests for read-aloud's prepared mode. Run via `node --test`.
// The status shapes mirror services/speak/internal/web/docs.go (DocStatus,
// SectionStatus, PartCounts).
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  audioExt, barView, BlockCollector, blockText, downloadName, inProgress, pageLine, parseDocStatus,
  parseRegistration, POLL_MAX_MS, POLL_MS, pollDelay, prepareQuery, readRequest, sectionView,
  statusOfSection,
} from '../src/prepare.ts';

const ZERO = { parts: 0, ready: 0, generating: 0, queued: 0, retrying: 0, idle: 0, failed: 0 };

function counts(over) {
  return { ...ZERO, ...over };
}

function section(n, over) {
  return { section: n, reason: '', ...counts(over) };
}

test('blockText: collapses whitespace in each run and joins runs with a space', () => {
  assert.equal(blockText(['  A heading\n\n  with   a\tbreak ']), 'A heading with a break');
  assert.equal(blockText(['line one', 'line two']), 'line one line two');
  assert.equal(blockText([' a ', '', '\n', 'c']), 'a c');
  assert.equal(blockText([' \n\t']), '');
  assert.equal(blockText([]), '');
});

// <p>the <code>pkg</code> class</p>: inline elements split text nodes but not runs.
test('BlockCollector: text within one run keeps its own spacing', () => {
  const c = new BlockCollector();
  c.text('p', 'the ');
  c.text('p', 'pkg');
  c.text('p', ' class');
  assert.deepEqual(c.blocks(), [{ el: 'p', text: 'the pkg class' }]);
});

// <p>line one<br>line two</p> and <li>a<ul>…</ul>c</li>: a cut is where live
// mode's collectRuns flushes, and the runs join with a space instead of fusing.
test('BlockCollector: runs cut by a nested block or <br> join with a space', () => {
  const c = new BlockCollector();
  c.text('p', 'line one');
  c.cut('p');
  c.text('p', 'line two');
  c.cut('p');
  c.cut('p'); // a childless <br> after another cut adds no run
  assert.deepEqual(c.blocks(), [{ el: 'p', text: 'line one line two' }]);
});

// <li>\n  <ul><li>first on screen</li></ul>\n  then loose text</li>: the outer
// item's whitespace before its nested list must not place it first.
test('BlockCollector: a block takes its place at its first non-blank text', () => {
  const c = new BlockCollector();
  c.text('outer', '\n  ');
  c.cut('outer');
  c.text('inner', 'first on screen');
  c.text('outer', '\n  then loose text');
  assert.deepEqual(c.blocks(), [
    { el: 'inner', text: 'first on screen' },
    { el: 'outer', text: 'then loose text' },
  ]);
});

// <li>a<ul><li>b</li></ul>c</li>: loose text before the nested list does place
// the outer item first, and both of its runs belong to it.
test('BlockCollector: loose text before a nested block keeps the outer block first', () => {
  const c = new BlockCollector();
  c.text('outer', 'a');
  c.cut('outer');
  c.text('inner', 'b');
  c.text('outer', 'c');
  assert.deepEqual(c.blocks(), [{ el: 'outer', text: 'a c' }, { el: 'inner', text: 'b' }]);
});

test('BlockCollector: blank blocks are omitted and a cut before any text is a no-op', () => {
  const c = new BlockCollector();
  c.cut('ul');
  c.text('ul', '\n  ');
  c.cut('ul');
  c.text('ul', ' ');
  c.text('li', 'item');
  assert.deepEqual(c.blocks(), [{ el: 'li', text: 'item' }]);
  assert.deepEqual(new BlockCollector().blocks(), []);
});

test('pollDelay: doubles per consecutive failure from the poll cadence, capped', () => {
  assert.equal(POLL_MS, 2000);
  assert.equal(POLL_MAX_MS, 60_000);
  assert.deepEqual([0, 1, 2, 3, 4].map(pollDelay), [2000, 4000, 8000, 16_000, 32_000]);
  assert.equal(pollDelay(5), 60_000, '64 s is capped');
  assert.equal(pollDelay(40), 60_000, 'no overflow past the cap');
  assert.equal(pollDelay(-1), 2000, 'never below the cadence');
});

test('readRequest: one blocks list per section, copied', () => {
  const blocks = ['Title', 'First paragraph.'];
  const body = readRequest('brief', [blocks, []]);
  assert.deepEqual(body, { name: 'brief', sections: [{ blocks: ['Title', 'First paragraph.'] }, { blocks: [] }] });
  blocks.push('later');
  assert.equal(body.sections[0].blocks.length, 2);
});

test('parseDocStatus: reads the shape, missing counts as zero', () => {
  const st = parseDocStatus({
    id: 'abc', name: 'brief', total: { parts: 3, ready: 1 },
    sections: [{ section: 1, parts: 3, ready: 1, reason: 'retrying after 1 attempt: boom' }],
  });
  assert.equal(st.id, 'abc');
  assert.equal(st.name, 'brief');
  assert.deepEqual(st.total, counts({ parts: 3, ready: 1 }));
  assert.deepEqual(st.sections, [section(1, { parts: 3, ready: 1, reason: 'retrying after 1 attempt: boom' })]);
  assert.equal(st.reason, '');
});

test('parseDocStatus: null without an id', () => {
  assert.equal(parseDocStatus({ name: 'x' }), null);
  assert.equal(parseDocStatus({ id: '' }), null);
  assert.equal(parseDocStatus(null), null);
  assert.equal(parseDocStatus('abc'), null);
});

const REGISTRATION = {
  name: 'brief',
  doc: { id: 'abc', name: 'brief', total: { parts: 3 }, sections: [{ section: 1, parts: 3 }, { section: 2 }] },
  sections: [
    { parts: ['k1', 'k2', 'k3', 'k1'], blocks: [['k1'], [], ['k2', 'k3', 'k1']] },
    { parts: [], blocks: [] },
  ],
};

test('parseRegistration: keys per block and per section, mirroring the request', () => {
  const reg = parseRegistration(REGISTRATION, [3, 0]);
  assert.equal(reg.name, 'brief');
  assert.equal(reg.doc.id, 'abc');
  assert.deepEqual(reg.sections[0].parts, ['k1', 'k2', 'k3', 'k1']);
  assert.deepEqual(reg.sections[0].blocks, [['k1'], [], ['k2', 'k3', 'k1']]);
  assert.deepEqual(reg.sections[1], { parts: [], blocks: [] });
});

test('parseRegistration: an answer that does not mirror the request is rejected', () => {
  assert.equal(parseRegistration(REGISTRATION, [3]), null, 'section count');
  assert.equal(parseRegistration(REGISTRATION, [2, 0]), null, 'block count');
  assert.equal(parseRegistration({ ...REGISTRATION, doc: {} }, [3, 0]), null, 'doc without id');
  assert.equal(parseRegistration({ ...REGISTRATION, sections: 'nope' }, [3, 0]), null, 'sections not a list');
  const badKeys = { ...REGISTRATION, sections: [{ parts: [1], blocks: [[], [], []] }, { parts: [], blocks: [] }] };
  assert.equal(parseRegistration(badKeys, [3, 0]), null, 'keys not strings');
  assert.equal(parseRegistration(null, []), null);
});

test('parseRegistration: the name falls back to the document name', () => {
  const reg = parseRegistration({ ...REGISTRATION, name: undefined }, [3, 0]);
  assert.equal(reg.name, 'brief');
});

test('inProgress: queued, generating and retrying parts keep the poll going', () => {
  assert.equal(inProgress(counts({ queued: 1, generating: 2, retrying: 3, idle: 9, failed: 9 })), 6);
  assert.equal(inProgress(counts({ ready: 4, idle: 1, failed: 1 })), 0);
});

test('statusOfSection: looks up by 1-based section number', () => {
  const st = parseDocStatus({ id: 'abc', sections: [{ section: 2, parts: 1 }, { section: 1, parts: 5 }] });
  assert.equal(statusOfSection(st, 0).parts, 5);
  assert.equal(statusOfSection(st, 1).parts, 1);
  assert.equal(statusOfSection(st, 2), undefined);
});

test('pageLine: ready first, then what the rest are doing', () => {
  assert.equal(pageLine(counts({ parts: 12, ready: 3, queued: 2, generating: 1, retrying: 1, idle: 4, failed: 1 })),
    'Audio: 3 of 12 parts ready, 3 preparing, 1 retrying, 4 not prepared, 1 failed');
  assert.equal(pageLine(counts({ parts: 2, ready: 2 })), 'Audio: 2 of 2 parts ready');
});

test('barView: actions follow the counts', () => {
  const doc = (total, reason = '') => ({ id: 'abc', name: 'brief', total: counts(total), sections: [], reason });
  const idle = barView(doc({ parts: 4, idle: 4 }));
  assert.equal(idle.canPrepare, true);
  assert.equal(idle.failed, 0);
  assert.equal(idle.canDownload, false);
  assert.equal(idle.reason, '');

  const failed = barView(doc({ parts: 4, ready: 2, failed: 2 }, 'failed after 3 attempts: no engine'));
  assert.equal(failed.canPrepare, true);
  assert.equal(failed.failed, 2);
  assert.equal(failed.reason, 'failed after 3 attempts: no engine');

  const working = barView(doc({ parts: 4, ready: 2, generating: 2 }, 'retrying after 1 attempt: x'));
  assert.equal(working.canPrepare, false);
  assert.equal(working.reason, '', 'a reason shows only behind a failure');

  const ready = barView(doc({ parts: 4, ready: 4 }));
  assert.equal(ready.canPrepare, false);
  assert.equal(ready.canDownload, true);
  assert.equal(ready.line, 'Audio: 4 of 4 parts ready');

  assert.equal(barView(doc({})).canDownload, false, 'nothing to download');
});

test('sectionView: most useful fact first', () => {
  assert.deepEqual(sectionView(section(1, { parts: 3, ready: 3 })),
    { variant: 'ok', text: 'audio ready', title: 'audio ready', retry: false, download: true });
  assert.deepEqual(sectionView(section(1, { parts: 3, ready: 1, generating: 1, queued: 1 })),
    { variant: 'info', text: 'generating 2 of 3', title: 'generating 2 of 3', retry: false, download: false });
  assert.equal(sectionView(section(1, { parts: 3, ready: 3, generating: 1 })).text, 'audio ready');
  assert.equal(sectionView(section(1, { parts: 3, queued: 3 })).text, 'queued, 0 of 3 ready');
  const retrying = sectionView(section(1, { parts: 3, ready: 1, retrying: 1, reason: 'retrying after 1 attempt: boom' }));
  assert.equal(retrying.variant, 'warn');
  assert.equal(retrying.text, 'retrying, 1 of 3 ready');
  assert.equal(retrying.title, 'retrying, 1 of 3 ready (last failure: retrying after 1 attempt: boom)');
  assert.equal(sectionView(section(1, { parts: 3, ready: 1, idle: 2 })).text, '1 of 3 ready');
  assert.deepEqual(sectionView(section(1, { parts: 3, idle: 3 })),
    { variant: 'outline', text: 'not prepared', title: 'not prepared', retry: false, download: false });
});

test('sectionView: a failure keeps speak\'s reason as is', () => {
  const failed = sectionView(section(2, { parts: 2, ready: 1, failed: 1, reason: 'failed after 3 attempts: no engine' }));
  assert.equal(failed.variant, 'error');
  assert.equal(failed.text, 'failed after 3 attempts: no engine');
  assert.equal(failed.title, failed.text);
  assert.equal(failed.retry, true);
  assert.equal(failed.download, false);
  assert.equal(sectionView(section(2, { parts: 2, failed: 2, reason: 'no engine' })).text, 'failed: no engine');
  assert.equal(sectionView(section(2, { parts: 2, failed: 2 })).text, 'failed: unknown reason');
});

test('prepareQuery: section and failed flags', () => {
  assert.equal(prepareQuery(0, false), '');
  assert.equal(prepareQuery(0, true), '?failed=1');
  assert.equal(prepareQuery(3, false), '?section=3');
  assert.equal(prepareQuery(3, true), '?section=3&failed=1');
});

test('audioExt: mp3 for mpeg, wav for everything else', () => {
  assert.equal(audioExt('audio/mpeg'), '.mp3');
  assert.equal(audioExt('audio/mp3; charset=binary'), '.mp3');
  assert.equal(audioExt('audio/wav'), '.wav');
  assert.equal(audioExt('audio/x-wav'), '.wav');
  assert.equal(audioExt(''), '.wav');
});

test('downloadName: filename-safe name, section suffix, extension from the type', () => {
  assert.equal(downloadName('Q3 review: what shipped', 0, 'audio/wav'), 'Q3-review--what-shipped.wav');
  assert.equal(downloadName('Q3 review', 2, 'audio/mpeg'), 'Q3-review-section-2.mp3');
  assert.equal(downloadName('notes.md', 0, 'audio/wav'), 'notes.wav');
  assert.equal(downloadName('v1.2 plan', 0, 'audio/wav'), 'v1.2-plan.wav', 'a title keeps its dots');
  assert.equal(downloadName('Städer & åar', 0, 'audio/wav'), 'Städer---åar.wav');
  assert.equal(downloadName('---', 0, 'audio/wav'), 'speak.wav');
  assert.equal(downloadName('', 1, 'audio/wav'), 'speak-section-1.wav');
});
