// viz.js helpers, run via `node --test`. The file is a classic browser script,
// so it runs against a fake window and the tests read window.PresentViz.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { homedir } from 'node:os';
import { join } from 'node:path';

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

// The ribbon's text and the lengths that follow it. A slide's unit at
// 1440x900 framed: u0 12.13 px, the floor 1.1 u0.
const slideAt1440 = { u0: 12.13, u: 12.13, floor: 13.343 };

test('ribbonFontSizes: the brief stays at 11 px whatever the root; a slide sizes like a slide chart', () => {
  assert.deepEqual(PresentViz.ribbonFontSizes(16, null), { tick: 11, legend: 11 });
  assert.deepEqual(PresentViz.ribbonFontSizes(24, null), { tick: 11, legend: 11 }, 'the reader size leaves the brief alone');
  assert.deepEqual(PresentViz.ribbonFontSizes(16, slideAt1440), PresentViz.chartFontSizes(16, slideAt1440));
  const low = PresentViz.ribbonFontSizes(16, { u0: 12.13, u: 12.13 * 0.7, floor: 13.343 });
  assert.ok(low.tick >= 13.343 && low.legend >= 13.343, 'a low fit holds the text at the floor');
});

test('ribbonMetrics: at 11 px the brief layout, and every text length scales with the text', () => {
  const brief = {
    rightPx: 4, charPx: 6.5, labelMinPx: 16, labelPadPx: 8, leftMinPx: 40, axisAirPx: 10, tickPadPx: 6,
    topPx: 8, unitTopPx: 22, unitYPx: -10, bottomPx: 22, periodYPx: 16
  };
  assert.deepEqual(PresentViz.ribbonMetrics(11), brief);
  assert.deepEqual(PresentViz.ribbonMetrics(undefined), brief, 'no size reads as the brief');
  const twice = PresentViz.ribbonMetrics(22);
  Object.keys(brief).forEach((k) => assert.equal(twice[k], k === 'rightPx' ? 4 : 2 * brief[k], k));
});

test('ribbonMargins: the axis widens for its widest tick label, the top for the unit', () => {
  const m11 = PresentViz.ribbonMetrics(11);
  const ticks = ['0', '50', '100', '150', '200'];
  assert.deepEqual(PresentViz.ribbonMargins(ticks, true, m11), { top: 22, right: 4, bottom: 22, left: 40 });
  assert.deepEqual(PresentViz.ribbonMargins(ticks, false, m11), { top: 8, right: 4, bottom: 22, left: 40 });
  assert.equal(PresentViz.ribbonMargins(['0', '500,000', '1,000,000'], false, m11).left, Math.ceil(9 * 6.5) + 10);
  const m22 = PresentViz.ribbonMetrics(22);
  assert.deepEqual(PresentViz.ribbonMargins(ticks, true, m22), { top: 44, right: 4, bottom: 44, left: 80 });
  assert.equal(PresentViz.ribbonMargins(['1,000,000'], true, m22).left, Math.ceil(9 * 13) + 20);
});

test('ribbonLabelFits: a segment names its series when a line fits its height and the name its width', () => {
  const m11 = PresentViz.ribbonMetrics(11), m22 = PresentViz.ribbonMetrics(22);
  assert.equal(PresentViz.ribbonLabelFits('search', 10, 26, 60, m11), true);
  assert.equal(PresentViz.ribbonLabelFits('search', 10, 25, 60, m11), false, 'one px short of a line');
  assert.equal(PresentViz.ribbonLabelFits('search', 10, 26, 46, m11), false, 'the name is wider than the column');
  assert.equal(PresentViz.ribbonLabelFits('search', 10, 26, 60, m22), false, 'twice the text needs twice the room');
  assert.equal(PresentViz.ribbonLabelFits('search', 10, 42, 94, m22), true);
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
    { from: 'web', to: 'db' },
  ],
  steps: [{ caption: 'one' }, { caption: 'two', focus: ['api'] }, { caption: 'three', focus: ['data'] }],
};
// wrapped checks what wrapText promises: no line wider than the limit once
// its hanging closing marks are left out, and every character kept.
function wrapped(text, max) {
  const lines = PresentViz.wrapText(text, max, measure, 'text');
  lines.forEach((l) => assert.ok(measure(l.replace(/[,.;:)\]]{1,2}$/, '')) <= max, `${JSON.stringify(l)} is wider than ${max}`));
  assert.equal(lines.join('').replace(/\s+/g, ''), String(text).replace(/\s+/g, ''), 'nothing dropped');
  return lines;
}

test('wrapText: fills lines by measured width and keeps every character', () => {
  assert.deepEqual(wrapped('Go, validates and prices', 28 * 7), ['Go, validates and prices']);
  assert.deepEqual(wrapped('Next.js, renders the checkout page for the browser', 20 * 7), ['Next.js, renders the', 'checkout page for', 'the browser']);
  assert.deepEqual(PresentViz.wrapText('', 100, measure, 'text'), []);
  assert.deepEqual(PresentViz.wrapText(undefined, 100, measure, 'text'), []);
  const roles = [];
  PresentViz.wrapText('a b', 100, (s, role) => { roles.push(role); return s.length; }, 'label');
  assert.ok(roles.every((r) => r === 'label'), 'measures in the role it is given');
});

test('wrapText: a path or dotted name breaks after a separator in its second half', () => {
  assert.deepEqual(wrapped('~/.config/present/assets/elk.bundled.js', 18 * 7), ['~/.config/present/', 'assets/elk.bundled.', 'js'], 'the dot hangs');
  assert.deepEqual(wrapped('services/present/internal', 12 * 7), ['services/', 'present/', 'internal']);
});

test('wrapText: one long word with no separator breaks at the last character that fits', () => {
  assert.deepEqual(wrapped('x'.repeat(25), 10 * 7), ['x'.repeat(10), 'x'.repeat(10), 'x'.repeat(5)]);
});

test('wrapText: one or two closing marks hang and never start a line', () => {
  assert.deepEqual(wrapped('abcd efghi),', 10 * 7), ['abcd efghi),']);
  assert.deepEqual(wrapped('abcdefghij)', 10 * 7), ['abcdefghij)']);
  assert.deepEqual(wrapped('first second third.', 12 * 7), ['first second', 'third.']);
  const lines = wrapped('one, two; three: four. five) six] seven,', 6 * 7);
  lines.forEach((l) => assert.ok(!/^[,.;:)\]]/.test(l), `${JSON.stringify(l)} starts with a closing mark`));
  assert.deepEqual(wrapped('ab.)cd.)ef', 3 * 7).filter((l) => /^[,.;:)\]]/.test(l)), []);
});

test('diagramBox: each density wraps inside its widest box', () => {
  const web = checkout.nodes[0];
  assert.deepEqual(PresentViz.diagramBox(web, measure, 'full'), {
    width: 29 * 7 + 28, height: 20 + 18 + 2 * 15,
    label: ['Web app'], lines: ['Next.js, renders the checkout', 'page for the browser'],
  });
  assert.deepEqual(PresentViz.diagramBox(web, measure, 'compact'), {
    width: 21 * 7 + 28, height: 20 + 18 + 3 * 15,
    label: ['Web app'], lines: ['Next.js, renders the', 'checkout page for the', 'browser'],
  });
  assert.deepEqual(PresentViz.diagramBox(web, measure, 'names'), { width: 120, height: 38, label: ['Web app'], lines: [] });
  assert.deepEqual(PresentViz.diagramBox({ label: 'API' }, measure), PresentViz.diagramBox({ label: 'API' }, measure, 'full'));
});

test('diagramBox: a long name takes two lines, and the width stays inside 120 and the density limit', () => {
  const two = PresentViz.diagramBox({ label: 'Release please manifest config parser' }, measure, 'full');
  assert.deepEqual(two.label, ['Release please manifest config', 'parser']);
  assert.equal(two.height, 20 + 2 * 18);
  [['full', 240], ['compact', 190], ['names', 240]].forEach(([density, max]) => {
    [{ label: 'A' }, { label: 'x'.repeat(60) }, { label: 'Store', text: 'y '.repeat(40) }].forEach((node) => {
      const b = PresentViz.diagramBox(node, measure, density);
      assert.ok(b.width >= 120 && b.width <= max, `${density}: ${b.width}`);
    });
  });
});

test('edgeLabelBox: one line in full; two in compact and names only over 84 px', () => {
  assert.deepEqual(PresentViz.edgeLabelBox('POST /checkout', measure, 'full'), { lines: ['POST /checkout'], w: 14 * 7 + 8, h: 16 });
  assert.deepEqual(PresentViz.edgeLabelBox('insert', measure, 'compact'), { lines: ['insert'], w: 6 * 7 + 8, h: 16 });
  assert.deepEqual(PresentViz.edgeLabelBox('twelve chars', measure, 'compact').lines, ['twelve chars'], '84 px is not over 84');
  assert.deepEqual(PresentViz.edgeLabelBox('POST /checkout', measure, 'compact'), { lines: ['POST', '/checkout'], w: 9 * 7 + 8, h: 30 });
  assert.deepEqual(PresentViz.edgeLabelBox('POST /checkout', measure, 'names').lines, ['POST', '/checkout']);
  assert.deepEqual(PresentViz.edgeLabelBox('unsplittablelabel', measure, 'compact').lines, ['unsplittablelabel']);
});

test('edgeLabelBox: the split leaves the shorter longest line', () => {
  assert.deepEqual(PresentViz.edgeLabelBox('writes the event log', measure, 'compact'), { lines: ['writes the', 'event log'], w: 10 * 7 + 8, h: 30 });
});

test('diagramElk: every labelled edge carries one inline label with its box', () => {
  const { graph, labels } = PresentViz.diagramElk(checkout, measure, { direction: 'LR', density: 'compact' });
  assert.deepEqual(graph.edges[0], {
    id: 'e0', sources: ['web'], targets: ['api'],
    labels: [{ id: 'l0', text: 'POST /checkout', width: 71, height: 30, layoutOptions: { 'elk.edgeLabels.inline': 'true' } }],
  });
  assert.deepEqual(labels[0], PresentViz.edgeLabelBox('POST /checkout', measure, 'compact'));
  assert.equal(graph.edges[1].labels.length, 1);
  assert.deepEqual(graph.edges[2], { id: 'e2', sources: ['web'], targets: ['db'] });
  assert.equal(labels[2], undefined);
});

test('diagramElk: the form sets the direction, and the spacing set sits on the root and every group', () => {
  const lr = PresentViz.diagramElk(checkout, measure, { direction: 'LR', density: 'full' }).graph;
  const tb = PresentViz.diagramElk(checkout, measure, { direction: 'TB', density: 'full' }).graph;
  assert.equal(lr.layoutOptions['elk.direction'], 'RIGHT');
  assert.equal(tb.layoutOptions['elk.direction'], 'DOWN');
  assert.equal(PresentViz.diagramElk({ direction: 'TB', nodes: [] }, measure).graph.layoutOptions['elk.direction'], 'DOWN', 'no form: the authored direction');
  ['elk.hierarchyHandling', 'elk.edgeRouting', 'elk.json.edgeCoords', 'elk.json.shapeCoords', 'elk.layered.nodePlacement.strategy'].forEach((k) => {
    assert.deepEqual([k, lr.layoutOptions[k]], [k, { 'elk.hierarchyHandling': 'INCLUDE_CHILDREN', 'elk.edgeRouting': 'ORTHOGONAL', 'elk.json.edgeCoords': 'ROOT', 'elk.json.shapeCoords': 'ROOT', 'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX' }[k]]);
  });
  const spacing = {
    'elk.layered.spacing.nodeNodeBetweenLayers': '32', 'elk.layered.spacing.edgeNodeBetweenLayers': '12',
    'elk.spacing.nodeNode': '28', 'elk.spacing.edgeNode': '24', 'elk.spacing.edgeEdge': '14',
  };
  const prod = lr.children[0], data = prod.children[0];
  // ELK spaces siblings by their parent's options, so a group, nested or not, carries the root's.
  [lr, prod, data].forEach((el) => Object.entries(spacing).forEach(([k, v]) => assert.equal(el.layoutOptions[k], v, `${el.id} ${k}`)));
});

test('diagramElk: groups nest and are at least as wide as their label; boxes grow by their shape room', () => {
  const { graph, boxes } = PresentViz.diagramElk(checkout, measure, { direction: 'LR', density: 'full' });
  assert.deepEqual(graph.children.map((c) => c.id), ['prod', 'web']);
  const prod = graph.children[0];
  assert.deepEqual(prod.children.map((c) => c.id), ['data', 'api']);
  assert.deepEqual(prod.children[0].children.map((c) => c.id), ['db']);
  assert.equal(prod.layoutOptions['elk.padding'], '[top=36,left=16,bottom=16,right=16]');
  assert.equal(prod.layoutOptions['elk.nodeSize.constraints'], 'MINIMUM_SIZE');
  assert.equal(prod.layoutOptions['elk.nodeSize.minimum'], '(' + ('prod cluster'.length * 7 + 24) + ',0)', 'measured as written, not in capitals');
  assert.equal(prod.width, undefined);
  const web = graph.children[1], db = prod.children[0].children[0];
  assert.deepEqual(boxes.web, PresentViz.diagramBox(checkout.nodes[0], measure, 'full'));
  assert.equal(web.width, boxes.web.width + 14, 'a person grows by its figure');
  assert.equal(web.height, boxes.web.height);
  assert.equal(db.width, 120);
  assert.equal(db.height, 38 + 6, 'a store grows by its cap');
  assert.equal(web.layoutOptions, undefined);
});

test('diagramElk: top to bottom a group carries its least width second, since ELK turns the minimum with the layout', () => {
  const tb = PresentViz.diagramElk(checkout, measure, { direction: 'TB', density: 'full' }).graph;
  assert.equal(tb.children[0].layoutOptions['elk.nodeSize.minimum'], '(0,' + ('prod cluster'.length * 7 + 24) + ')');
});

test('diagramReturnEdges: the edges a depth-first walk from the nodes in order finds pointing back into its path', () => {
  const spec = (ids, ...edges) => ({
    nodes: ids.split(' ').map((id) => ({ id })),
    edges: edges.map((e) => ({ from: e[0], to: e[2] })),
  });
  assert.deepEqual(PresentViz.diagramReturnEdges(spec('a b c', 'a>b', 'b>c', 'c>a')), [false, false, true]);
  assert.deepEqual(PresentViz.diagramReturnEdges(spec('a b c', 'a>b', 'a>c', 'b>c')), [false, false, false], 'two ways down is no loop');
  assert.deepEqual(PresentViz.diagramReturnEdges(spec('c a b', 'a>b', 'b>c', 'c>a')), [false, true, false], 'the walk starts at the first node');
  assert.deepEqual(PresentViz.diagramReturnEdges({ nodes: [{ id: 'a' }] }), []);
});

test('diagramElk: a labelled return edge puts its label in the layer next to the box it leaves', () => {
  const loop = {
    nodes: [{ id: 'a', label: 'Build' }, { id: 'b', label: 'Test' }, { id: 'c', label: 'Deploy' }],
    edges: [{ from: 'a', to: 'b', label: 'image' }, { from: 'b', to: 'c' }, { from: 'c', to: 'a', label: 'rollback' }],
  };
  const { graph } = PresentViz.diagramElk(loop, measure, { direction: 'LR', density: 'full' });
  assert.deepEqual(graph.edges[0].labels[0].layoutOptions, { 'elk.edgeLabels.inline': 'true' });
  assert.deepEqual(graph.edges[2].labels[0].layoutOptions, {
    'elk.edgeLabels.inline': 'true', 'elk.layered.edgeLabels.centerLabelPlacementStrategy': 'TAIL_LAYER',
  });
});

test('diagramGroupLabelSpots: a group label moves right past a route or an edge label in its strip', () => {
  const group = (x) => ({ id: 'g', x, y: 100, width: 300, height: 120, children: [] });
  const spec = { groups: [{ id: 'g', label: 'Backend' }] }; // 7 * 7 = 49 px wide
  const down = (x) => ({ sections: [{ startPoint: { x, y: 40 }, endPoint: { x, y: 140 } }] });
  const at = (layout) => PresentViz.diagramGroupLabelSpots(spec, layout, measure).g;
  assert.deepEqual(at({ children: [group(0)], edges: [] }), PresentViz.diagramGroupLabel, 'nothing crosses: the corner');
  assert.deepEqual(at({ children: [group(0)], edges: [down(200)] }), { x: 12, y: 20 }, 'a route clear of it leaves it');
  assert.deepEqual(at({ children: [group(50)], edges: [down(80)] }), { x: 36, y: 20 }, 'past the route, 6 px clear, from the group edge');
  assert.deepEqual(at({ children: [group(0)], edges: [down(30), down(90)] }), { x: 96, y: 20 }, 'past the second when the first gap is too narrow');
  const label = { sections: [], labels: [{ x: 10, y: 110, width: 60, height: 16 }] };
  assert.deepEqual(at({ children: [group(0)], edges: [label] }), { x: 76, y: 20 }, 'past an edge label in the strip');
  const below = { sections: [{ startPoint: { x: 0, y: 130 }, endPoint: { x: 300, y: 130 } }] };
  assert.deepEqual(at({ children: [group(0)], edges: [below] }), { x: 12, y: 20 }, 'a run under the strip leaves it');
  const across = { sections: [{ startPoint: { x: 0, y: 115 }, endPoint: { x: 300, y: 115 } }] };
  assert.deepEqual(at({ children: [group(0)], edges: [across] }), { x: 12, y: 20 }, 'no clear place: the corner');
});

test('diagramLabelSpots: the top left corner ELK gave each label, by edge index', () => {
  const spots = PresentViz.diagramLabelSpots({ edges: [
    { id: 'e2', labels: [{ id: 'l2', x: 10, y: 20, width: 5, height: 5 }] },
    { id: 'e0' },
    { id: 'e1', labels: [] },
  ] });
  assert.equal(spots.length, 3);
  assert.deepEqual(spots[2], { x: 10, y: 20 });
  assert.equal(spots[0], undefined);
  assert.equal(spots[1], undefined);
  assert.deepEqual(PresentViz.diagramLabelSpots({}), []);
});

test('diagramLadder: the authored direction first, then the other, names only on a slide', () => {
  const tag = (forms) => forms.map((f) => `${f.direction} ${f.density} ${f.group}`);
  assert.deepEqual(tag(PresentViz.diagramLadder('LR', false)), ['LR full 0', 'LR compact 0', 'TB full 1', 'TB compact 1']);
  assert.deepEqual(tag(PresentViz.diagramLadder('TB', false)), ['TB full 0', 'TB compact 0', 'LR full 1', 'LR compact 1']);
  assert.deepEqual(tag(PresentViz.diagramLadder(undefined, false)), tag(PresentViz.diagramLadder('LR', false)), 'no direction is left to right');
  assert.deepEqual(tag(PresentViz.diagramLadder('TB', true)), ['TB full 0', 'TB compact 0', 'LR full 1', 'LR compact 1', 'TB names 2', 'LR names 2']);
});

// form builds one laid-out form for diagramPlan: its size, group and density.
const form = (width, height, group, density) => ({ width, height, group, density });
const ladder = (sizes, onSlide) => PresentViz.diagramLadder('LR', onSlide).map((f, i) => form(sizes[i][0], sizes[i][1], f.group, f.density));
const BRIEF = { room: { w: 1000, h: null, soft: 720 }, lim: { floor: 1, aim: 13 / 11.5, max: 16 / 13 } };
const SLIDE = { room: { w: 1200, h: 400, soft: null }, lim: { floor: 1.4, aim: 1.4, max: 2.4 } };
const near = (a, b) => Math.abs(a - b) < 1e-9;

test('diagramPlan: picks the form and scale for the room', () => {
  const cases = [
    { name: 'the authored form at the aim, capped at the largest scale', at: BRIEF,
      sizes: [[800, 300], [700, 300], [400, 900], [350, 950]], want: { index: 0, scale: 16 / 13 } },
    { name: 'compact when full is under the aim', at: BRIEF,
      sizes: [[950, 300], [850, 400], [400, 900], [350, 950]], want: { index: 1, scale: 1000 / 850 } },
    { name: "the group's largest when it reaches the floor but not the aim", at: BRIEF,
      sizes: [[950, 300], [980, 300], [400, 300], [350, 300]], want: { index: 0, scale: 1000 / 950 } },
    { name: 'the other direction when the authored one is under the floor', at: BRIEF,
      sizes: [[1500, 300], [1200, 400], [700, 500], [650, 540]], want: { index: 2, scale: 16 / 13 } },
    { name: 'the brief scrolls at the floor when every form is under it', at: BRIEF,
      sizes: [[1500, 300], [1300, 400], [1400, 600], [1250, 700]], want: { index: 3, scale: 1, underFloor: true, scroll: true } },
    { name: 'a tall drawing shrinks toward 45rem, not below the aim', at: BRIEF,
      sizes: [[500, 1000], [480, 1100], [900, 300], [800, 400]], want: { index: 0, scale: 13 / 11.5 } },
    { name: 'a tall drawing stops at 45rem when that is above the aim', at: BRIEF,
      sizes: [[500, 600], [480, 700], [900, 300], [800, 400]], want: { index: 0, scale: 720 / 600 } },
    { name: 'a slide fits the height too', at: SLIDE,
      sizes: [[600, 500], [500, 250], [300, 900], [250, 950], [400, 200], [200, 600]], want: { index: 1, scale: 1.6 } },
    { name: 'a slide grows no further than its largest scale', at: SLIDE,
      sizes: [[300, 100], [280, 100], [100, 300], [90, 300], [200, 80], [80, 200]], want: { index: 0, scale: 2.4 } },
    { name: 'a slide draws under the floor at the ratio and marks it', at: SLIDE,
      sizes: [[2000, 300], [1800, 320], [600, 900], [550, 1000], [1700, 300], [500, 800]], want: { index: 1, scale: 1200 / 1800, underFloor: true, scroll: false } },
    { name: 'names when only names reaches the floor', at: SLIDE,
      sizes: [[2000, 300], [1800, 320], [600, 900], [550, 1000], [800, 280], [500, 800]], want: { index: 4, scale: 400 / 280 } },
    { name: 'under the floor, names only with a 1.15 gain', at: SLIDE,
      sizes: [[2000, 300], [1800, 320], [600, 900], [550, 1000], [1600, 300], [500, 800]], want: { index: 1, scale: 1200 / 1800, underFloor: true } },
    { name: 'under the floor, names with more than a 1.15 gain', at: SLIDE,
      sizes: [[2000, 300], [1800, 320], [600, 900], [550, 1000], [1500, 300], [500, 800]], want: { index: 4, scale: 1200 / 1500, underFloor: true } },
  ];
  cases.forEach((c) => {
    const plan = PresentViz.diagramPlan(ladder(c.sizes, c.at.room.h !== null), c.at.room, c.at.lim);
    const want = { underFloor: false, scroll: false, ...c.want };
    assert.equal(plan.index, want.index, `${c.name}: index`);
    assert.ok(near(plan.scale, want.scale), `${c.name}: scale ${plan.scale}, want ${want.scale}`);
    assert.equal(plan.underFloor, want.underFloor, `${c.name}: underFloor`);
    assert.equal(plan.scroll, want.scroll, `${c.name}: scroll`);
  });
});

test('diagramPlan: a form whose layout failed is left out, and the pick is among the rest', () => {
  // laidExcept lays out every rung but the dropped ones, as diagramLayout
  // does when ELK fails on a form, and keeps each form's name.
  const laidExcept = (sizes, onSlide, dropped) => PresentViz.diagramLadder('LR', onSlide)
    .map((f, i) => ({ ...f, width: sizes[i][0], height: sizes[i][1] }))
    .filter((f) => !dropped.includes(`${f.direction} ${f.density}`));
  const cases = [
    { name: 'the authored full form failed: its compact form', at: BRIEF, dropped: ['LR full'],
      sizes: [[800, 300], [700, 300], [400, 900], [350, 950]], want: 'LR compact' },
    { name: 'both authored forms failed: the other direction', at: BRIEF, dropped: ['LR full', 'LR compact'],
      sizes: [[800, 300], [700, 300], [400, 900], [350, 950]], want: 'TB full' },
    { name: 'one form left, under the floor: drawn at the floor and scrolled', at: BRIEF, dropped: ['LR full', 'LR compact', 'TB full'],
      sizes: [[800, 300], [700, 300], [400, 900], [1350, 950]], want: 'TB compact', underFloor: true, scroll: true },
    { name: 'on a slide only the names forms laid out', at: SLIDE, dropped: ['LR full', 'LR compact', 'TB full', 'TB compact'],
      sizes: [[600, 500], [500, 250], [300, 900], [250, 950], [400, 200], [200, 600]], want: 'LR names' },
  ];
  cases.forEach((c) => {
    const forms = laidExcept(c.sizes, c.at.room.h !== null, c.dropped);
    const plan = PresentViz.diagramPlan(forms, c.at.room, c.at.lim);
    const picked = forms[plan.index];
    assert.equal(picked && `${picked.direction} ${picked.density}`, c.want, `${c.name}: form`);
    assert.equal(plan.underFloor, !!c.underFloor, `${c.name}: underFloor`);
    assert.equal(plan.scroll, !!c.scroll, `${c.name}: scroll`);
  });
});

test('diagramLimits: the slide floor comes from the canvas width, the largest box name from a 2.2u wrapper', () => {
  const at1440 = PresentViz.diagramLimits({ onSlide: true, wrapWidth: 1238, maxHeight: 468, canvasWidth: 1440, wrapFontPx: 2.2 * 14.4 });
  assert.deepEqual(at1440.room, { w: 1238, h: 468, soft: null });
  assert.equal((11.5 * at1440.lim.floor).toFixed(1), '15.8', 'box text at 1.1% of 1440');
  assert.equal((13 * at1440.lim.floor).toFixed(1), '17.9');
  assert.equal(at1440.lim.aim, at1440.lim.floor);
  assert.equal((13 * at1440.lim.max).toFixed(2), '31.68', 'box names at most 2.2% of 1440');
  const at1920 = PresentViz.diagramLimits({ onSlide: true, wrapWidth: 1651, maxHeight: 562, canvasWidth: 1920, wrapFontPx: 2.2 * 19.2 });
  assert.equal((11.5 * at1920.lim.floor).toFixed(2), '21.12');
  assert.equal((13 * at1920.lim.max).toFixed(2), '42.24');
});

test('diagramLimits: on a slide the largest box name follows the wrapper font, so it shrinks with the fit and the floor does not', () => {
  const room = { onSlide: true, wrapWidth: 1238, maxHeight: 468, canvasWidth: 1440 };
  const fit1 = PresentViz.diagramLimits({ ...room, wrapFontPx: 2.2 * 14.4 });
  const fit08 = PresentViz.diagramLimits({ ...room, wrapFontPx: 2.2 * 14.4 * 0.8 });
  assert.equal((13 * fit1.lim.max).toFixed(2), '31.68', 'a 2.2u wrapper at fit 1 is the 2.2% cap');
  assert.equal((13 * fit08.lim.max).toFixed(2), '25.34');
  assert.equal(fit08.lim.floor, fit1.lim.floor);
  assert.equal((11.5 * fit08.lim.floor).toFixed(1), '15.8');
  const tiny = PresentViz.diagramLimits({ ...room, wrapFontPx: 4 });
  assert.equal(tiny.lim.max, tiny.lim.floor, 'never under the floor');
});

test('fitSteps: the first fit that holds wins, from 1 down in steps of 0.05', () => {
  const tried = [];
  assert.deepEqual(PresentViz.fitSteps((fit) => { tried.push(fit); return false; }), { fit: 1, overflow: false });
  assert.deepEqual(tried, [1], 'a slide that holds at 1 is measured once');
  tried.length = 0;
  assert.deepEqual(PresentViz.fitSteps((fit) => { tried.push(fit); return fit > 0.85; }), { fit: 0.85, overflow: false });
  assert.deepEqual(tried, [1, 0.95, 0.9, 0.85]);
});

test('fitSteps: stops at 0.7 and reports a slide that still overflows', () => {
  const tried = [];
  assert.deepEqual(PresentViz.fitSteps((fit) => { tried.push(fit); return true; }), { fit: 0.7, overflow: true });
  assert.deepEqual(tried, [1, 0.95, 0.9, 0.85, 0.8, 0.75, 0.7], 'six steps down, no further');
  assert.deepEqual(PresentViz.fitSteps((fit) => fit > 0.7), { fit: 0.7, overflow: false }, 'holding at 0.7 is no overflow');
});

test('diagramLimits: the brief has no height limit, a floor at 1 and an aim of 13 px', () => {
  const root16 = PresentViz.diagramLimits({ onSlide: false, wrapWidth: 1032, wrapFontPx: 16, rootPx: 16 });
  assert.deepEqual(root16.room, { w: 1032, h: null, soft: 720 });
  assert.equal(root16.lim.floor, 1);
  assert.ok(near(root16.lim.aim, 13 / 11.5));
  assert.ok(near(root16.lim.max, 16 / 13), 'box names no larger than the wrapper font');
  const root20 = PresentViz.diagramLimits({ onSlide: false, wrapWidth: 1032, wrapFontPx: 20, rootPx: 20 });
  assert.equal(root20.room.soft, 900);
  assert.ok(near(root20.lim.max, 20 / 13));
  const small = PresentViz.diagramLimits({ onSlide: false, wrapWidth: 600, wrapFontPx: 12, rootPx: 16 });
  assert.ok(near(small.lim.max, 13 / 11.5), 'never under the aim');
});

// plain strips the prototype-free maps the helpers return, so deepEqual compares values.
const plain = (v) => JSON.parse(JSON.stringify(v));

test('diagramShown: step 0 shows the unstepped elements, step n adds the rest in order, null shows all', () => {
  assert.deepEqual(plain(PresentViz.diagramShown(checkout, 0)), { groups: { prod: true, data: false }, nodes: { web: true, api: false, db: false }, edges: [false, false, true] });
  assert.deepEqual(plain(PresentViz.diagramShown(checkout, 2).nodes), { web: true, api: true, db: false });
  assert.deepEqual(PresentViz.diagramShown(checkout, 2).edges, [true, false, true]);
  assert.deepEqual(PresentViz.diagramShown(checkout, null).edges, [true, true, true]);
  assert.deepEqual(plain(PresentViz.diagramShown(checkout, null).groups), { prod: true, data: true });
});

test('diagramFocus: the focus ids, everything in a focused group, and the edges touching them', () => {
  assert.equal(PresentViz.diagramFocus(checkout, 1), null);
  assert.equal(PresentViz.diagramFocus(checkout, null), null);
  assert.deepEqual(plain(PresentViz.diagramFocus(checkout, 2)), { groups: { prod: false, data: false }, nodes: { web: false, api: true, db: false }, edges: [true, true, false] });
  assert.deepEqual(plain(PresentViz.diagramFocus(checkout, 3)), { groups: { prod: false, data: true }, nodes: { web: false, api: false, db: true }, edges: [false, true, true] });
});

test('diagramFocus: no focus at rest, whatever the step', () => {
  assert.equal(PresentViz.diagramFocus(checkout, 3, true), null);
  assert.equal(PresentViz.diagramFocus(checkout, 2, true), null);
  assert.notEqual(PresentViz.diagramFocus(checkout, 3, false), null);
});

test('flowSpeed: 6 px/s for the lightest, 20 px/s for the heaviest, the middle without weights', () => {
  assert.equal(PresentViz.flowSpeed(0, 100), 0.006);
  assert.equal(PresentViz.flowSpeed(100, 100), 0.02);
  assert.ok(Math.abs(PresentViz.flowSpeed(undefined, 0) - 0.013) < 1e-9);
});

test('diagramText: one row per name line, then one per text line, moved past the shape room', () => {
  const box = (label, lines) => ({ label, lines });
  assert.deepEqual(PresentViz.diagramText({ kind: 'service' }, box(['A'], ['x', 'y']), 160), { x: 80, label: [19], lines: [35.5, 50.5] });
  assert.deepEqual(PresentViz.diagramText({}, box(['A'], []), 120), { x: 60, label: [19], lines: [] });
  assert.deepEqual(PresentViz.diagramText({}, box(['A', 'B'], ['x']), 120), { x: 60, label: [19, 37], lines: [53.5] });
  assert.deepEqual(PresentViz.diagramText({ kind: 'person' }, box(['A'], ['x']), 196), { x: 105, label: [19], lines: [35.5] });
  assert.deepEqual(PresentViz.diagramText({ kind: 'store' }, box(['A'], ['x']), 120), { x: 60, label: [25], lines: [41.5] });
  assert.deepEqual(PresentViz.diagramText({ kind: 'toString' }, box(['A'], []), 120), { x: 60, label: [19], lines: [] });
});

test('edgeLabelRows: the middle of each label line inside its halo', () => {
  assert.deepEqual(PresentViz.edgeLabelRows({ lines: ['a'] }), [8]);
  assert.deepEqual(PresentViz.edgeLabelRows({ lines: ['a', 'b'] }), [8, 22]);
});

test('diagram helpers: an id named like an Object.prototype key is just an id', () => {
  const spec = { groups: [{ id: 'constructor', label: 'C' }], nodes: [{ id: 'toString', label: 'T', group: 'constructor' }, { id: '__proto__', label: 'P', step: 2 }], edges: [{ from: 'toString', to: '__proto__' }], steps: [{ caption: 'one' }, { caption: 'two', focus: ['constructor'] }] };
  const { graph, boxes } = PresentViz.diagramElk(spec, (t) => t.length * 7, { direction: 'LR', density: 'full' });
  assert.deepEqual(graph.children.map((c) => c.id), ['constructor', '__proto__']);
  assert.deepEqual(graph.children[0].children.map((c) => c.id), ['toString']);
  assert.deepEqual(Object.keys(boxes).sort(), ['__proto__', 'toString']);
  const shown = PresentViz.diagramShown(spec, 1).nodes, lit = PresentViz.diagramFocus(spec, 2).nodes;
  assert.equal(shown.toString, true);
  assert.equal(shown['__proto__'], false);
  assert.equal(lit.toString, true);
  assert.equal(lit['__proto__'], false);
});

// Chart helpers.
const CHART_KINDS = ['bar', 'stacked-bar', 'horizontal-bar', 'line', 'area', 'sparkline', 'scatter', 'doughnut', 'sankey', 'ribbon'];

test('chartLine: straight lines for every kind drawn with one, nothing for the rest', () => {
  assert.deepEqual(Object.fromEntries(CHART_KINDS.map((k) => [k, PresentViz.chartLine(k)])), {
    bar: {}, 'stacked-bar': {}, 'horizontal-bar': {}, line: { tension: 0 }, area: { tension: 0 }, sparkline: { tension: 0 },
    scatter: {}, doughnut: {}, sankey: {}, ribbon: {},
  });
});

test('chartValueAxis: bars and areas from zero, lines and scatters fitted to the data', () => {
  const zero = { beginAtZero: true }, fitted = { beginAtZero: false, grace: '5%' };
  assert.deepEqual(Object.fromEntries(CHART_KINDS.map((k) => [k, PresentViz.chartValueAxis(k)])), {
    bar: zero, 'stacked-bar': zero, 'horizontal-bar': zero, area: zero,
    line: fitted, sparkline: fitted, scatter: fitted, doughnut: null, sankey: null, ribbon: null,
  });
  assert.equal(PresentViz.chartValueAxis('constructor'), null);
});

test('chartTicks: whole ticks when every value on the axis is a whole number', () => {
  const runs = [{ points: [{ x: 'a', y: 3 }, { x: 'b', y: 0 }] }, { points: [{ x: 'a', y: 2 }, { x: 'b', y: 1 }] }];
  assert.deepEqual(PresentViz.chartTicks(runs, 'y'), { precision: 0 }, 'runs passed out of 3');
  assert.deepEqual(PresentViz.chartTicks([{ points: [{ y: 0.031 }, { y: 2 }] }], 'y'), {}, 'one share is enough for steps');
  assert.deepEqual(PresentViz.chartTicks([{ points: [{ x: '1998', y: 0.5 }, { x: '656', y: 1 }] }], 'x'), { precision: 0 },
    'a scatter x arrives as a decimal string');
  assert.deepEqual(PresentViz.chartTicks([{ points: [{ x: '12.5', y: 1 }] }], 'x'), {});
  assert.deepEqual(PresentViz.chartTicks([], 'y'), {}, 'no values');
  assert.deepEqual(PresentViz.chartTicks(undefined, 'y'), {});
});

// mono measures like a monospaced face: 10 px a character.
const mono = (s) => s.length * 10;

test('barAxisLabels: a name that fits its category stays one line', () => {
  // 2 categories on 240 px at 12 px ticks: 120 - 6 - 12 = 102 px, 10 characters.
  assert.deepEqual(PresentViz.barAxisLabels(['speak', 'csl-index'], 240, 12, mono), ['speak', 'csl-index']);
});

test('barAxisLabels: a long name wraps after its separators to its category', () => {
  const names = ['belt-soft-mode-guards', 'belt-sweep-readsize-cross', 'present-chart-diagram-kinds', 'speak-pause'];
  const room = 1000 / names.length - 6 - 12; // 232 px: 23 characters
  const got = PresentViz.barAxisLabels(names, 1000, 12, mono);
  assert.deepEqual(got, ['belt-soft-mode-guards', ['belt-sweep-readsize-', 'cross'], ['present-chart-diagram-', 'kinds'], 'speak-pause']);
  got.flat().forEach((line) => assert.ok(mono(line) <= room, line + ' is wider than its category'));
  assert.equal(got.flat().join(''), names.join(''), 'nothing cut, nothing added');
  assert.deepEqual(PresentViz.barAxisLabels(['two words here'], 112, 12, mono), [['two words', 'here']]);
});

test('barAxisLabels: a line never starts with a separator; a piece wider than a line is cut', () => {
  // One category, 90 px of room: 9 characters.
  assert.deepEqual(PresentViz.barAxisLabels(['csl-index-flags'], 108, 12, mono), [['csl-', 'index-', 'flags']],
    'not csl-index and -flags, which reads as a flag');
  assert.deepEqual(PresentViz.barAxisLabels(['src/web/app.go'], 108, 12, mono), [['src/web/', 'app.go']]);
  // 50 px of room: 5 characters; the space after the cut word stays.
  assert.deepEqual(PresentViz.barAxisLabels(['longword x'], 68, 12, mono), [['longw', 'ord x']]);
});

test('barAxisLabels: past four lines the axis keeps its rotation', () => {
  assert.equal(PresentViz.barAxisLabels(['present-chart-diagram-kinds'], 60, 12, mono), null, '4 characters a line');
  assert.deepEqual(PresentViz.barAxisLabels([], 600, 12, mono), []);
  assert.deepEqual(PresentViz.barAxisLabels([undefined, 7], 600, 12, mono), ['', '7']);
});

test('chartPointRoom: a point keeps its hover radius and ring free; kinds without points none', () => {
  assert.deepEqual(Object.fromEntries(CHART_KINDS.map((k) => [k, PresentViz.chartPointRoom(k, 2)])), {
    bar: 0, 'stacked-bar': 0, 'horizontal-bar': 0, line: 7, area: 6, sparkline: 0,
    scatter: 9, doughnut: 0, sankey: 0, ribbon: 0,
  });
  assert.equal(PresentViz.chartPointRoom('scatter', 1), 8, 'the scatter draws a 1 px ring');
  assert.equal(PresentViz.chartPointRoom('line'), 5, 'no ring');
  assert.deepEqual(PresentViz.chartPoint('line'), { radius: 0, hover: 5 }, 'a line marks a point on hover only');
  assert.equal(PresentViz.chartPoint('constructor'), null);
});

test('chartFontSizes: ticks at 0.75 of the root, kept between 11 and 15; the legend at 0.8125', () => {
  assert.deepEqual(PresentViz.chartFontSizes(16), { tick: 12, legend: 13 });
  assert.deepEqual(PresentViz.chartFontSizes(12), { tick: 11, legend: 9.75 });
  assert.deepEqual(PresentViz.chartFontSizes(24), { tick: 15, legend: 19.5 });
  assert.deepEqual(PresentViz.chartFontSizes(NaN), PresentViz.chartFontSizes(16), 'no root size reads as 16');
});

test('chartFontSizes: on a slide, ticks at 1.5u and the legend at 1.7u, held at the floor', () => {
  const near2 = (got, want, msg) => assert.ok(Math.abs(got - want) < 1e-9, (msg ? msg + ': ' : '') + got + ' != ' + want);
  const at1440 = PresentViz.chartFontSizes(16, { u0: 14.4, u: 14.4, floor: 15.84 });
  near2(at1440.tick, 21.6);
  near2(at1440.legend, 24.48);
  const fitted = PresentViz.chartFontSizes(16, { u0: 14.4, u: 14.4 * 0.7, floor: 15.84 });
  near2(fitted.tick, 15.84);
  near2(fitted.legend, 17.136);
  const bumped = PresentViz.chartFontSizes(20, { u0: 14.4, u: 14.4, floor: 15.84 });
  near2(bumped.tick, 25.6);
  near2(bumped.legend, 28.48);
  const small = PresentViz.chartFontSizes(12, { u0: 14.4, u: 14.4 * 0.8 });
  near2(small.tick, 1.1 * 14.4, 'small text at a low fit: the floor from u0 when none is given');
});

// slideFonts is what chartFonts in app.js gives on a slide of u0 at fit.
function slideFonts(rootPx, fit, family = 'Fira Code', floor = 15.84) {
  const s = PresentViz.chartFontSizes(rootPx, { u0: 14.4, u: 14.4 * fit, floor });
  return { tick: { family, size: s.tick }, legend: { family, size: s.legend } };
}

test('chartFontsChanged: a lower fit, the reader\'s size or face rebuilds a slide chart; the same fit does not', () => {
  const { chartFontsChanged } = PresentViz;
  assert.equal(chartFontsChanged(slideFonts(16, 1), slideFonts(16, 0.9)), true, 'the fit went from 1 to 0.9');
  assert.equal(chartFontsChanged(slideFonts(16, 0.9), slideFonts(16, 0.9)), false, 'the same fit');
  assert.equal(chartFontsChanged(slideFonts(16, 0.9), slideFonts(19, 0.9)), true, 'three size steps up');
  assert.equal(chartFontsChanged(slideFonts(16, 0.9), slideFonts(16, 0.9, 'Inter')), true, 'another face');
  assert.equal(chartFontsChanged(slideFonts(16, 0.75, 'Fira Code', 20), slideFonts(16, 0.7, 'Fira Code', 20)), false,
    'a fit that leaves every size at the floor');
  const a = slideFonts(16, 0.9), b = slideFonts(16, 0.9);
  b.tick.size += 0.005;
  assert.equal(chartFontsChanged(a, b), false, 'float noise');
  assert.equal(chartFontsChanged(undefined, a), true, 'no fonts on record');
});

test('readingChanged: the probe box moved in either direction; float noise and a held box did not', () => {
  const { readingChanged } = PresentViz;
  const box = { width: 412.5, height: 19 };
  assert.equal(readingChanged(null, box), true, 'no earlier read');
  assert.equal(readingChanged(box, { width: 412.5, height: 19 }), false);
  assert.equal(readingChanged(box, { width: 412.505, height: 19 }), false, 'float noise');
  assert.equal(readingChanged(box, { width: 389.2, height: 19 }), true, 'a narrower face');
  assert.equal(readingChanged(box, { width: 412.5, height: 22 }), true, 'a face with taller lines');
  assert.ok(PresentViz.readingSample.length > 20, 'the sample is a line, not a glyph');
});

test('settle: a burst of calls runs fn once, ms after the last; a later call runs it again', (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  let runs = 0;
  const later = PresentViz.settle(() => { runs++; }, 150);
  later();
  t.mock.timers.tick(80);
  later();
  t.mock.timers.tick(80);
  later();
  t.mock.timers.tick(149);
  assert.equal(runs, 0, 'three presses 80 ms apart are one burst');
  t.mock.timers.tick(1);
  assert.equal(runs, 1);
  later();
  t.mock.timers.tick(150);
  assert.equal(runs, 2);
});

test('seriesExtent: the smallest and largest point over every series', () => {
  assert.deepEqual(PresentViz.seriesExtent([{ points: [{ y: 0.91 }, { y: 0.84 }] }, { points: [{ y: 0.97 }, { y: '0.9' }] }]), { min: 0.84, max: 0.97 });
  assert.deepEqual(PresentViz.seriesExtent([{ points: [{ y: -3 }] }, {}]), { min: -3, max: -3 });
  assert.equal(PresentViz.seriesExtent([]), null);
  assert.equal(PresentViz.seriesExtent(undefined), null);
});

test('changedSectionTargets: changed and added sections, by id or by index', () => {
  assert.deepEqual(PresentViz.changedSectionTargets({
    changed: ['results'], added: ['index:3', 'next-steps'], removed: ['old'],
  }), [{ id: 'results' }, { index: 3 }, { id: 'next-steps' }]);
  assert.deepEqual(PresentViz.changedSectionTargets(undefined), []);
  assert.deepEqual(PresentViz.changedSectionTargets({ changed: [], added: [] }), []);
});

// Layout with the real engine: the 13 diagrams of the looks eval's baseline
// run (skills/present/evals/looks), each in every form a slide can draw,
// through the vendored ELK. Skipped on a machine whose asset cache (make
// cache) has no ELK bundle. CI fetches the bundle with
// scripts/cache-assets.sh, so there a missing bundle fails the test.
const elkFile = join(process.env.PRESENT_WORKDIR || join(homedir(), '.config', 'present'), 'assets', 'js', 'elk-0.12.0.bundled.js');
const elkSkip = existsSync(elkFile) || process.env.CI === 'true' ? false : `no ELK bundle at ${elkFile} (run make cache)`;
const fixtures = new URL('./fixtures/diagrams/', import.meta.url);
// faceMeasure stands in for the canvas measure: a width per character close
// to Fira Code at the diagram's sizes.
const FACE_PX = { label: 7.8, text: 6.9, group: 6.9 };
const faceMeasure = (s, role) => String(s).length * (FACE_PX[role] || FACE_PX.text);
const TOUCH = 0.5; // boxes closer than this do not overlap, and a route this near a label touches it

function overlaps(a, b) {
  return a.x < b.x + b.w - TOUCH && b.x < a.x + a.w - TOUCH && a.y < b.y + b.h - TOUCH && b.y < a.y + a.h - TOUCH;
}
// runsThrough says whether one straight run of a route (orthogonal, so
// one of its axes is fixed) meets the rectangle r.
function runsThrough(p, q, r) {
  const x0 = Math.min(p.x, q.x) - TOUCH, x1 = Math.max(p.x, q.x) + TOUCH, y0 = Math.min(p.y, q.y) - TOUCH, y1 = Math.max(p.y, q.y) + TOUCH;
  return x0 <= r.x + r.w && r.x <= x1 && y0 <= r.y + r.h && r.y <= y1;
}
function touchesRoute(edge, r) {
  return (edge.sections || []).some((sec) => {
    const pts = [sec.startPoint, ...(sec.bendPoints || []), sec.endPoint];
    return pts.slice(1).some((q, i) => runsThrough(pts[i], q, r));
  });
}
// GROUP_PAD_BOTTOM is the padding under a group's lowest box (DIAGRAM_GROUP_PAD).
const GROUP_PAD_BOTTOM = 16;
// laidOut walks ELK's result into node boxes, group label boxes where
// diagramGroupLabelSpots puts them, and the room under each group's lowest
// box, in root coordinates.
function laidOut(layout, spec) {
  const group = Object.create(null), nodes = [], groupLabels = [], rooms = [];
  const spots = PresentViz.diagramGroupLabelSpots(spec, layout, faceMeasure);
  (spec.groups || []).forEach((g) => { group[g.id] = g; });
  (function walk(children) {
    (children || []).forEach((c) => {
      if (group[c.id]) {
        const at = spots[c.id];
        groupLabels.push({ id: c.id, x: c.x + at.x, y: c.y + at.y - 12, w: faceMeasure(group[c.id].label || '', 'group'), h: 16 });
        rooms.push({ id: c.id, under: c.y + c.height - Math.max(...(c.children || []).map((k) => k.y + k.height)) });
        walk(c.children);
      } else nodes.push({ id: c.id, x: c.x, y: c.y, w: c.width, h: c.height });
    });
  })(layout.children);
  return { nodes, groupLabels, rooms };
}

test('layout: on every baseline diagram and form, each label sits on its own route and clear of boxes and labels, no route crosses a group label, and no group leaves room under its boxes', { skip: elkSkip }, async () => {
  assert.ok(existsSync(elkFile), `no ELK bundle at ${elkFile}: CI runs scripts/cache-assets.sh with PRESENT_WORKDIR before this test`);
  const ELK = createRequire(import.meta.url)(elkFile);
  const elk = new ELK();
  const files = readdirSync(fixtures).filter((f) => f.endsWith('.json')).sort();
  assert.equal(files.length, 13);
  const faults = [];
  for (const file of files) {
    const spec = JSON.parse(readFileSync(new URL(file, fixtures), 'utf8'));
    for (const form of PresentViz.diagramLadder(spec.direction, true)) {
      const where = `${file} ${form.direction} ${form.density}`;
      const built = PresentViz.diagramElk(spec, faceMeasure, form);
      const layout = await elk.layout(built.graph);
      const spots = PresentViz.diagramLabelSpots(layout);
      const { nodes, groupLabels, rooms } = laidOut(layout, spec);
      rooms.forEach((r) => { if (r.under > GROUP_PAD_BOTTOM + TOUCH) faults.push(`${where}: group ${r.id} has ${Math.round(r.under)} px under its boxes`); });
      layout.edges.forEach((e) => groupLabels.forEach((g) => { if (touchesRoute(e, g)) faults.push(`${where}: route ${e.id} crosses group label ${g.id}`); }));
      const labels = [];
      (spec.edges || []).forEach((e, k) => {
        if (!e.label) return;
        if (!spots[k]) { faults.push(`${where}: edge ${k} has no label spot`); return; }
        const box = built.labels[k];
        labels.push({ k, x: spots[k].x, y: spots[k].y, w: box.w, h: box.h, edge: layout.edges.find((x) => x.id === 'e' + k) });
      });
      labels.forEach((a, i) => {
        if (!touchesRoute(a.edge, a)) faults.push(`${where}: label ${a.k} is off its route`);
        nodes.forEach((n) => { if (overlaps(a, n)) faults.push(`${where}: label ${a.k} overlaps box ${n.id}`); });
        groupLabels.forEach((g) => { if (overlaps(a, g)) faults.push(`${where}: label ${a.k} overlaps group label ${g.id}`); });
        labels.slice(i + 1).forEach((b) => { if (overlaps(a, b)) faults.push(`${where}: label ${a.k} overlaps label ${b.k}`); });
      });
    }
  }
  assert.deepEqual(faults, []);
});
