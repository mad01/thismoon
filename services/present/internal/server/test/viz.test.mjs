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

test('diagramLimits and canvasWidth: the slide floor and largest box name come from the canvas width', () => {
  assert.equal(PresentViz.canvasWidth(1440, 900, false), 1440);
  assert.equal(PresentViz.canvasWidth(1440, 900, true), 1440, 'wider than 16:9 is limited by the width');
  assert.ok(near(PresentViz.canvasWidth(1440, 700, true), 700 * 16 / 9), 'a short window is limited by the height');
  assert.equal(PresentViz.canvasWidth(1920, 1080, true), 1920);
  const at1440 = PresentViz.diagramLimits({ onSlide: true, wrapWidth: 1238, maxHeight: 468, canvasWidth: 1440 });
  assert.deepEqual(at1440.room, { w: 1238, h: 468, soft: null });
  assert.equal((11.5 * at1440.lim.floor).toFixed(1), '15.8', 'box text at 1.1% of 1440');
  assert.equal((13 * at1440.lim.floor).toFixed(1), '17.9');
  assert.equal(at1440.lim.aim, at1440.lim.floor);
  assert.equal((13 * at1440.lim.max).toFixed(2), '31.68', 'box names at most 2.2% of 1440');
  const at1920 = PresentViz.diagramLimits({ onSlide: true, wrapWidth: 1651, maxHeight: 562, canvasWidth: 1920 });
  assert.equal((11.5 * at1920.lim.floor).toFixed(2), '21.12');
  assert.equal((13 * at1920.lim.max).toFixed(2), '42.24');
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

test('chartFontSizes: ticks at 0.75 of the root, kept between 11 and 15; the legend at 0.8125', () => {
  assert.deepEqual(PresentViz.chartFontSizes(16), { tick: 12, legend: 13 });
  assert.deepEqual(PresentViz.chartFontSizes(12), { tick: 11, legend: 9.75 });
  assert.deepEqual(PresentViz.chartFontSizes(24), { tick: 15, legend: 19.5 });
  assert.deepEqual(PresentViz.chartFontSizes(NaN), PresentViz.chartFontSizes(16), 'no root size reads as 16');
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
// laidOut walks ELK's result into node boxes and group label boxes, in root coordinates.
function laidOut(layout, spec) {
  const group = Object.create(null), nodes = [], groupLabels = [];
  (spec.groups || []).forEach((g) => { group[g.id] = g; });
  (function walk(children) {
    (children || []).forEach((c) => {
      if (group[c.id]) {
        const at = PresentViz.diagramGroupLabel;
        groupLabels.push({ id: c.id, x: c.x + at.x, y: c.y + at.y - 12, w: faceMeasure(group[c.id].label || '', 'group'), h: 16 });
        walk(c.children);
      } else nodes.push({ id: c.id, x: c.x, y: c.y, w: c.width, h: c.height });
    });
  })(layout.children);
  return { nodes, groupLabels };
}

test('layout: on every baseline diagram and form, each label sits on its own route and clear of boxes and labels', { skip: elkSkip }, async () => {
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
      const { nodes, groupLabels } = laidOut(layout, spec);
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
