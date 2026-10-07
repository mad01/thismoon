// viz.js helpers, run via `node --test`. The file is a classic browser script,
// so it runs against a fake window and the tests read window.PresentViz.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('../viz.js', import.meta.url), 'utf8');
const fakeWindow = {};
new Function('window', src)(fakeWindow);
const { stepPlan, chartStepVisibility } = fakeWindow.PresentViz;
const PresentViz = fakeWindow.PresentViz;

test('stepPlan: a reveal slide steps each fragment, then the stepped chart in-block', () => {
  const p = { name: 'p' };
  const chart = { name: 'chart' };
  assert.deepEqual(stepPlan([
    { el: p, fragment: true, steps: 0 },
    { el: chart, fragment: true, steps: 3 },
  ]), [
    { kind: 'fragment', el: p },
    { kind: 'fragment', el: chart },
    { kind: 'block', el: chart, n: 1 },
    { kind: 'block', el: chart, n: 2 },
    { kind: 'block', el: chart, n: 3 },
  ]);
});

test('stepPlan: a chart inside a columns fragment steps after the columns fragment', () => {
  const cols = { name: 'columns' };
  const chart = { name: 'chart' };
  assert.deepEqual(stepPlan([
    { el: cols, fragment: true, children: [{ el: chart, steps: 2 }] },
  ]), [
    { kind: 'fragment', el: cols },
    { kind: 'block', el: chart, n: 1 },
    { kind: 'block', el: chart, n: 2 },
  ]);
});

test('stepPlan: a non-reveal slide contributes only block entries', () => {
  const p = { name: 'p' };
  const chart = { name: 'chart' };
  assert.deepEqual(stepPlan([
    { el: p, fragment: false },
    { el: chart, fragment: false, steps: 2 },
  ]), [
    { kind: 'block', el: chart, n: 1 },
    { kind: 'block', el: chart, n: 2 },
  ]);
});

test('stepPlan: a non-reveal slide with nothing stepped has no steps', () => {
  assert.deepEqual(stepPlan([
    { el: { name: 'p' }, fragment: false },
    { el: { name: 'table' }, fragment: false, steps: 0, children: [] },
  ]), []);
  assert.deepEqual(stepPlan([]), []);
});

test('stepPlan: top-level list items are one fragment each', () => {
  const li1 = { name: 'li1' };
  const li2 = { name: 'li2' };
  const li3 = { name: 'li3' };
  assert.deepEqual(stepPlan([
    { el: li1, fragment: true },
    { el: li2, fragment: true },
    { el: li3, fragment: true },
  ]), [
    { kind: 'fragment', el: li1 },
    { kind: 'fragment', el: li2 },
    { kind: 'fragment', el: li3 },
  ]);
});

test('stepPlan: the input is not mutated', () => {
  const items = [
    { el: { name: 'cols' }, fragment: true, steps: 1, children: [{ el: { name: 'chart' }, steps: 2 }] },
    { el: { name: 'p' }, fragment: true },
  ];
  const before = structuredClone(items);
  const plan = stepPlan(items);
  assert.equal(plan.length, 5);
  assert.deepEqual(items, before);
});

test('chartStepVisibility: no stepAt means every series shows', () => {
  const series = [{ step: 1 }, { step: 3 }, {}];
  assert.deepEqual(chartStepVisibility(series, null), [true, true, true]);
  assert.deepEqual(chartStepVisibility(series, undefined), [true, true, true]);
});

test('chartStepVisibility: stepAt 0 hides stepped series and keeps unstepped ones', () => {
  assert.deepEqual(chartStepVisibility([{ step: 1 }, {}, { step: 2 }, { step: 0 }], 0), [false, true, false, true]);
});

test('chartStepVisibility: a series shows once stepAt reaches its step', () => {
  assert.deepEqual(chartStepVisibility([{ step: 1 }, { step: 2 }, { step: 3 }], 2), [true, true, false]);
});

test('chartStepVisibility: a zero, negative, or non-integer step always shows', () => {
  const series = [{ step: 0 }, { step: -1 }, { step: 1.5 }, { step: '2' }, { step: null }];
  assert.deepEqual(chartStepVisibility(series, 0), [true, true, true, true, true]);
  assert.deepEqual(chartStepVisibility(series, 1), [true, true, true, true, true]);
});

test('blockStepsAt: one entry per stepped block at the highest step below n', () => {
  const p = {}, chart = {}, other = {};
  const plan = PresentViz.stepPlan([
    { el: p, fragment: true },
    { el: chart, fragment: true, steps: 2 },
    { el: other, fragment: false, steps: 1 },
  ]);
  const at = (n) => PresentViz.blockStepsAt(plan, n).map((r) => [r.el === chart ? 'chart' : 'other', r.n]);
  assert.deepEqual(at(0), [['chart', 0], ['other', 0]]);
  assert.deepEqual(at(2), [['chart', 0], ['other', 0]]);
  assert.deepEqual(at(3), [['chart', 1], ['other', 0]]);
  assert.deepEqual(at(4), [['chart', 2], ['other', 0]]);
  assert.deepEqual(at(5), [['chart', 2], ['other', 1]]);
  assert.deepEqual(at(3), [['chart', 1], ['other', 0]], 'stepping back lowers the step again');
});

test('blockStepsAt: a chart inside a columns fragment follows that fragment', () => {
  const cols = {}, chart = {};
  const plan = PresentViz.stepPlan([{ el: cols, fragment: true, children: [{ el: chart, steps: 2 }] }]);
  assert.deepEqual(PresentViz.blockStepsAt(plan, 1), [{ el: chart, n: 0 }]);
  assert.deepEqual(PresentViz.blockStepsAt(plan, 3), [{ el: chart, n: 2 }]);
  assert.deepEqual(PresentViz.blockStepsAt(plan, 0), [{ el: chart, n: 0 }]);
});

test('seriesMax: largest positive point, or the largest stacked sum of positives', () => {
  const series = [
    { points: [{ x: 'a', y: 100 }, { x: 'b', y: 10 }] },
    { points: [{ x: 'a', y: -50 }, { x: 'b', y: 30 }] },
    { points: [{ x: 'a', y: 100 }, { x: 'b', y: 5 }] },
  ];
  assert.equal(PresentViz.seriesMax(series, false), 100);
  assert.equal(PresentViz.seriesMax(series, true), 200, 'negatives stack on their own in Chart.js');
  assert.equal(PresentViz.seriesMax([], true), 0);
  assert.equal(PresentViz.seriesMax(null, false), 0);
});
