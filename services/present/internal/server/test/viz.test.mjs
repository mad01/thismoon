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

// ribbonLayout: the ribbon chart's columns, segments and ribbons in value units.
const traffic = [
  { name: 'search', points: [{ x: 'Q1', y: 40 }, { x: 'Q2', y: 35 }] },
  { name: 'social', points: [{ x: 'Q1', y: 20 }, { x: 'Q2', y: 45 }, { x: 'Q3', y: 50 }] },
  { name: 'direct', points: [{ x: 'Q1', y: 20 }, { x: 'Q2', y: 0 }, { x: 'Q3', y: 10 }] },
];

test('ribbonLayout: periods come in order of first appearance across the series', () => {
  const l = PresentViz.ribbonLayout([
    { points: [{ x: 'Q2', y: 1 }, { x: 'Q1', y: 1 }] },
    { points: [{ x: 'Q1', y: 1 }, { x: 'Q3', y: 1 }] },
  ]);
  assert.deepEqual(l.periods, ['Q2', 'Q1', 'Q3']);
  assert.deepEqual(l.columns.map((c) => c.segments.map((s) => s.series)), [[0], [0, 1], [1]]);
});

test('ribbonLayout: rank order puts the largest on top and keeps ties as given', () => {
  const l = PresentViz.ribbonLayout(traffic);
  const q1 = l.columns[0];
  assert.equal(q1.period, 'Q1');
  assert.equal(q1.total, 80);
  assert.deepEqual(q1.segments, [
    { series: 0, value: 40, y0: 40, y1: 80 },
    { series: 1, value: 20, y0: 20, y1: 40 },
    { series: 2, value: 20, y0: 0, y1: 20 },
  ]);
  assert.deepEqual(l.columns[1].segments.map((s) => s.series), [1, 0]);
});

test('ribbonLayout: given order keeps the series order in every column', () => {
  const l = PresentViz.ribbonLayout(traffic, 'given');
  assert.deepEqual(l.columns[1].segments.map((s) => s.series), [0, 1]);
  assert.deepEqual(l.columns[1].segments[0], { series: 0, value: 35, y0: 45, y1: 80 });
});

test('ribbonLayout: a zero or missing value leaves no segment and no ribbon', () => {
  const l = PresentViz.ribbonLayout(traffic);
  assert.deepEqual(l.columns[1].segments.map((s) => s.series), [1, 0]);
  assert.deepEqual(l.columns[2].segments.map((s) => s.series), [1, 2]);
  assert.equal(l.ribbons.filter((r) => r.series === 2).length, 0);
  assert.equal(l.ribbons.filter((r) => r.series === 0 && r.col === 2).length, 0);
});

test('ribbonLayout: a ribbon joins a series across adjacent columns and ends at col', () => {
  const l = PresentViz.ribbonLayout(traffic);
  assert.deepEqual(l.ribbons, [
    { series: 1, col: 1, a: { y0: 20, y1: 40 }, b: { y0: 35, y1: 80 } },
    { series: 0, col: 1, a: { y0: 40, y1: 80 }, b: { y0: 0, y1: 35 } },
    { series: 1, col: 2, a: { y0: 35, y1: 80 }, b: { y0: 10, y1: 60 } },
  ]);
});

test('ribbonLayout: max is the largest column total, and nothing in gives nothing out', () => {
  assert.equal(PresentViz.ribbonLayout(traffic).max, 80);
  assert.deepEqual(PresentViz.ribbonLayout([]), { periods: [], columns: [], ribbons: [], max: 0 });
  assert.deepEqual(PresentViz.ribbonLayout(undefined).periods, []);
});

test('ribbonLayout: the period order matches the Go side (testdata/ribbon-order.json)', () => {
  const vec = JSON.parse(readFileSync(new URL('../../render/testdata/ribbon-order.json', import.meta.url), 'utf8'));
  assert.deepEqual(PresentViz.ribbonLayout(vec.series).periods, vec.periods);
});

// Diagram helpers.
const measure = (t) => String(t).length * 7;
const checkout = {
  direction: 'LR',
  groups: [{ id: 'prod', label: 'prod cluster', tone: 'blue' }, { id: 'data', label: 'data', group: 'prod', step: 3 }],
  nodes: [
    { id: 'web', label: 'Web app', text: 'Next.js, renders the checkout page for the browser', kind: 'person' },
    { id: 'api', label: 'Checkout API', text: 'Go', group: 'prod', step: 2 },
    { id: 'db', label: 'Orders DB', kind: 'store', group: 'data', step: 3 },
  ],
  edges: [
    { from: 'web', to: 'api', label: 'POST /checkout', flow: true, weight: 120, step: 2 },
    { from: 'api', to: 'db', label: 'insert', step: 3 },
  ],
  steps: [{ caption: 'one' }, { caption: 'two', focus: ['api'] }, { caption: 'three', focus: ['data'] }],
};

test('wrapLines: breaks on spaces into at most two lines and cuts the rest', () => {
  assert.deepEqual(PresentViz.wrapLines('Go, validates and prices', 28), ['Go, validates and prices']);
  assert.deepEqual(PresentViz.wrapLines('Next.js, renders the checkout page for the browser', 28), ['Next.js, renders the', 'checkout page for the\u2026']);
  assert.deepEqual(PresentViz.wrapLines('a b c d e f g h i j k l m n o p q r s t u v w x y z aa bb cc dd', 10), ['a b c d e', 'f g h i j\u2026']);
  assert.deepEqual(PresentViz.wrapLines('', 28), []);
});

test('diagramBox: sized by the widest line within the bounds, one line per text line', () => {
  const small = PresentViz.diagramBox({ label: 'API' }, measure);
  assert.deepEqual(small, { width: 120, height: 38, lines: [] });
  const two = PresentViz.diagramBox(checkout.nodes[0], measure);
  assert.equal(two.lines.length, 2);
  assert.equal(two.width, 22 * 7 + 28);
  assert.equal(two.height, 20 + 18 + 30);
  const wide = PresentViz.diagramBox({ label: 'x'.repeat(60) }, measure);
  assert.equal(wide.width, 240);
});

test('diagramElk: groups nest, nodes sit in their group, edges sit at the root without labels', () => {
  const g = PresentViz.diagramElk(checkout, measure);
  assert.equal(g.layoutOptions['elk.direction'], 'RIGHT');
  assert.equal(g.layoutOptions['elk.hierarchyHandling'], 'INCLUDE_CHILDREN');
  assert.equal(g.layoutOptions['elk.json.edgeCoords'], 'ROOT');
  assert.deepEqual(g.children.map((c) => c.id), ['prod', 'web']);
  const prod = g.children[0];
  assert.deepEqual(prod.children.map((c) => c.id), ['data', 'api']);
  assert.deepEqual(prod.children[0].children.map((c) => c.id), ['db']);
  assert.equal(prod.layoutOptions['elk.padding'], '[top=36,left=16,bottom=16,right=16]');
  assert.equal(g.children[1].width, 182);
  assert.deepEqual(g.edges[0], { id: 'e0', sources: ['web'], targets: ['api'] });
  assert.equal(g.layoutOptions['elk.edgeLabels.inline'], undefined);
  assert.equal(g.layoutOptions['elk.layered.spacing.nodeNodeBetweenLayers'], String(14 * 7 + 24));
  assert.equal(PresentViz.diagramElk({ nodes: [{ id: 'a', label: 'A' }] }, measure).layoutOptions['elk.layered.spacing.nodeNodeBetweenLayers'], '56');
  assert.equal(PresentViz.diagramElk({ direction: 'TB', nodes: [] }, measure).layoutOptions['elk.direction'], 'DOWN');
});

test('routeLabelPoint: the middle of the longest straight run, null without a route', () => {
  const route = { sections: [
    { startPoint: { x: 0, y: 10 }, bendPoints: [{ x: 8, y: 10 }, { x: 8, y: 40 }], endPoint: { x: 60, y: 40 } },
  ] };
  assert.deepEqual(PresentViz.routeLabelPoint(route), { x: 34, y: 40 });
  assert.equal(PresentViz.routeLabelPoint({}), null);
});

test('elkPath: one M/L run per section with the bend points in order', () => {
  const d = PresentViz.elkPath({ sections: [
    { startPoint: { x: 0, y: 1 }, bendPoints: [{ x: 10, y: 1 }, { x: 10, y: 20 }], endPoint: { x: 30, y: 20 } },
    { startPoint: { x: 30, y: 20 }, endPoint: { x: 40, y: 20 } },
  ] });
  assert.equal(d, 'M0,1L10,1L10,20L30,20M30,20L40,20');
  assert.equal(PresentViz.elkPath({}), '');
});

test('diagramShown: step 0 shows the unstepped elements, step n adds the rest in order, null shows all', () => {
  assert.deepEqual(PresentViz.diagramShown(checkout, 0), { groups: { prod: true, data: false }, nodes: { web: true, api: false, db: false }, edges: [false, false] });
  assert.deepEqual(PresentViz.diagramShown(checkout, 2).nodes, { web: true, api: true, db: false });
  assert.deepEqual(PresentViz.diagramShown(checkout, 2).edges, [true, false]);
  assert.deepEqual(PresentViz.diagramShown(checkout, null).edges, [true, true]);
  assert.deepEqual(PresentViz.diagramShown(checkout, null).groups, { prod: true, data: true });
});

test('diagramFocus: the focus ids, everything in a focused group, and the edges touching them', () => {
  assert.equal(PresentViz.diagramFocus(checkout, 1), null);
  assert.equal(PresentViz.diagramFocus(checkout, null), null);
  assert.deepEqual(PresentViz.diagramFocus(checkout, 2), { groups: { prod: false, data: false }, nodes: { web: false, api: true, db: false }, edges: [true, true] });
  assert.deepEqual(PresentViz.diagramFocus(checkout, 3), { groups: { prod: false, data: true }, nodes: { web: false, api: false, db: true }, edges: [false, true] });
});

test('flowSpeed: 6 px/s for the lightest, 20 px/s for the heaviest, the middle without weights', () => {
  assert.equal(PresentViz.flowSpeed(0, 100), 0.006);
  assert.equal(PresentViz.flowSpeed(100, 100), 0.02);
  assert.ok(Math.abs(PresentViz.flowSpeed(undefined, 0) - 0.013) < 1e-9);
});

test('diagramGraph: a person box grows for its figure, a store for its cap', () => {
  const plain = PresentViz.diagramElk(checkout, measure);
  const g = PresentViz.diagramGraph(checkout, measure);
  assert.equal(g.children[1].id, 'web');
  assert.equal(g.children[1].width, plain.children[1].width + 14);
  assert.equal(g.children[1].height, plain.children[1].height);
  const db = g.children[0].children[0].children[0];
  assert.equal(db.id, 'db');
  assert.equal(db.height, 38 + 6);
  assert.equal(db.width, 120);
  assert.equal(g.children[0].children[1].width, 120);
  assert.equal(g.children[0].width, undefined);
  assert.equal(g.children[0].layoutOptions['elk.nodeSize.constraints'], 'MINIMUM_SIZE');
  assert.equal(g.children[0].layoutOptions['elk.nodeSize.minimum'], '(' + ('PROD CLUSTER'.length * 7 + 24) + ',0)');
  assert.equal(g.children[0].layoutOptions['elk.padding'], '[top=36,left=16,bottom=16,right=16]');
});

test('diagramText: the label row then one row per line, moved past the shape room', () => {
  assert.deepEqual(PresentViz.diagramText({ kind: 'service' }, 2, 160), { x: 80, label: 19, lines: [35.5, 50.5] });
  assert.deepEqual(PresentViz.diagramText({}, 0, 120), { x: 60, label: 19, lines: [] });
  assert.deepEqual(PresentViz.diagramText({ kind: 'person' }, 1, 196), { x: 105, label: 19, lines: [35.5] });
  assert.deepEqual(PresentViz.diagramText({ kind: 'store' }, 1, 120), { x: 60, label: 25, lines: [41.5] });
});
