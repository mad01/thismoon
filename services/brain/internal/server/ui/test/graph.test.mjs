// graph.js helpers, run via `node --test` (make test-js). The file is a
// classic browser script, so it runs against a fake window and the tests
// read window.BrainGraph.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('../graph.js', import.meta.url), 'utf8');
const fakeWindow = {};
new Function('window', src)(fakeWindow);
const G = fakeWindow.BrainGraph;

// A small vault: a links b and a dangling target, b links c, d is an orphan,
// and with topics a carries the tag "db".
const nodes = [
  { id: 'id:a', vault: 'me', path: 'notes/a.md', title: 'A', tier: 'notes', status: 'validated', tags: ['db'], evidence: 'A', degree: 3, dangling: false },
  { id: 'id:b', vault: 'me', path: 'notes/b.md', title: 'B', tier: 'notes', status: 'disputed', degree: 2, dangling: false, stale: true },
  { id: 'team:notes/c.md', vault: 'team', path: 'notes/c.md', stem: 'c', tier: 'inbox', status: 'raw', degree: 1, dangling: false },
  { id: 'id:d', vault: 'me', path: 'MOCs/d.md', title: 'D', tier: 'MOCs', degree: 0, dangling: false },
  { id: 'dangling:missing', stem: 'missing', degree: 1, dangling: true },
  { id: 'tag:db', title: 'db', degree: 1, dangling: false },
];
const edges = [
  { source: 'id:a', target: 'id:b', kind: 'link', resolved: true },
  { source: 'id:a', target: 'dangling:missing', kind: 'link', resolved: false },
  { source: 'id:b', target: 'team:notes/c.md', kind: 'source', resolved: true },
  { source: 'id:a', target: 'tag:db', kind: 'tag', resolved: true },
];
const all = { vault: '', tier: '', tag: '', status: '', orphans: true, dangling: true };

test('exported lists: tiers, statuses, evidence, edge kinds and colour modes', () => {
  assert.deepEqual(G.TIERS, ['inbox', 'notes', 'outputs', 'MOCs']);
  assert.deepEqual(G.STATUSES, ['raw', 'validated', 'promoting', 'disputed', 'superseded', 'retracted', 'withdrawn']);
  assert.deepEqual(G.EVIDENCE, ['A', 'B', 'C', 'D', 'mixed']);
  assert.deepEqual(G.EDGE_KINDS, ['link', 'tag', 'supersedes', 'source', 'moc']);
  assert.deepEqual(G.COLOR_MODES, ['vault', 'tier', 'evidence', 'status', 'staleness']);
  assert.equal(G.DEPTH_MIN, 1);
  assert.equal(G.DEPTH_MAX, 5);
});

test('every colour mode and edge kind maps to a role', () => {
  for (const mode of G.COLOR_MODES) {
    for (const n of nodes) assert.ok(G.colorRole(n, mode, { vaults: ['me', 'team'], today: '2026-11-05' }), mode + ' ' + n.id);
  }
  for (const kind of G.EDGE_KINDS) assert.ok(G.edgeRole(kind), kind);
  assert.equal(G.edgeRole('unknown'), G.edgeRole('link'));
});

test('nodeKind, nodeLabel and noteRef', () => {
  assert.equal(G.nodeKind(nodes[0]), 'note');
  assert.equal(G.nodeKind(nodes[4]), 'dangling');
  assert.equal(G.nodeKind(nodes[5]), 'tag');
  assert.equal(G.nodeLabel(nodes[0]), 'A');
  assert.equal(G.nodeLabel(nodes[2]), 'c');
  assert.equal(G.nodeLabel({ id: 'tag:x' }), '#x');
  assert.equal(G.nodeLabel({ id: 'dangling:y', dangling: true }), 'y');
  assert.equal(G.noteRef(nodes[0]), 'me:notes/a.md');
  assert.equal(G.noteRef({ id: 'id:z' }), 'id:z');
  assert.equal(G.noteRef(nodes[4]), '');
  assert.equal(G.noteRef(nodes[5]), '');
});

test('refIndex finds a note node by vault:path and by id', () => {
  const idx = G.refIndex(nodes);
  assert.equal(idx['me:notes/a.md'], 'id:a');
  assert.equal(idx['id:a'], 'id:a');
  assert.equal(idx['team:notes/c.md'], 'team:notes/c.md');
  assert.equal(idx['dangling:missing'], undefined);
});

test('facets: tiers in vault order, statuses in lifecycle order, notes only', () => {
  assert.deepEqual(G.facets(nodes), {
    vaults: ['me', 'team'],
    tiers: ['inbox', 'notes', 'MOCs'],
    tags: ['db'],
    statuses: ['raw', 'validated', 'disputed'],
  });
});

test('visibleIds: no filter shows everything', () => {
  assert.deepEqual(Object.keys(G.visibleIds(nodes, edges, all)).sort(), nodes.map((n) => n.id).sort());
});

test('visibleIds: the orphan and dangling toggles', () => {
  const v = G.visibleIds(nodes, edges, { ...all, orphans: false, dangling: false });
  assert.equal(v['id:d'], undefined, 'orphan hidden');
  assert.equal(v['dangling:missing'], undefined, 'dangling hidden');
  assert.equal(v['id:a'], true);
  assert.equal(v['tag:db'], true);
});

test('visibleIds: filters hide notes, and a tag or dangling target follows its notes', () => {
  const byVault = G.visibleIds(nodes, edges, { ...all, vault: 'team' });
  assert.deepEqual(Object.keys(byVault), ['team:notes/c.md']);

  const byTag = G.visibleIds(nodes, edges, { ...all, tag: 'db' });
  assert.deepEqual(Object.keys(byTag).sort(), ['dangling:missing', 'id:a', 'tag:db']);

  const byStatus = G.visibleIds(nodes, edges, { ...all, status: 'disputed' });
  assert.deepEqual(Object.keys(byStatus), ['id:b']);

  const byTier = G.visibleIds(nodes, edges, { ...all, tier: 'MOCs' });
  assert.deepEqual(Object.keys(byTier), ['id:d']);
});

test('matchesFilters: an empty value matches everything', () => {
  assert.equal(G.matchesFilters(nodes[0], {}), true);
  assert.equal(G.matchesFilters(nodes[0], { tag: 'db', tier: 'notes' }), true);
  assert.equal(G.matchesFilters(nodes[1], { tag: 'db' }), false);
});

test('isOrphan and isDangling', () => {
  assert.equal(G.isOrphan(nodes[3]), true);
  assert.equal(G.isOrphan(nodes[0]), false);
  assert.equal(G.isDangling(nodes[4]), true);
  assert.equal(G.isDangling(nodes[0]), false);
});

test('adjacency follows edges in both directions and skips self links', () => {
  const adj = G.adjacency(edges.concat([{ source: 'id:d', target: 'id:d', kind: 'link' }]));
  assert.deepEqual(G.neighbours(adj, 'id:b').sort(), ['id:a', 'team:notes/c.md']);
  assert.deepEqual(G.neighbours(adj, 'id:d'), []);
  assert.deepEqual(G.neighbours(adj, 'nope'), []);
});

test('depthSearch returns hop distances and clamps depth to 1..5', () => {
  const adj = G.adjacency(edges);
  assert.deepEqual(G.depthSearch(adj, 'team:notes/c.md', 1), { 'team:notes/c.md': 0, 'id:b': 1 });
  assert.deepEqual(G.depthSearch(adj, 'team:notes/c.md', 2), {
    'team:notes/c.md': 0, 'id:b': 1, 'id:a': 2,
  });
  assert.deepEqual(Object.keys(G.depthSearch(adj, 'team:notes/c.md', 9)).length, 5);
  assert.deepEqual(G.depthSearch(adj, 'team:notes/c.md', 0), G.depthSearch(adj, 'team:notes/c.md', 1));

  // A chain of seven: depth 9 stops at five hops.
  const chain = [];
  for (let i = 0; i < 7; i++) chain.push({ source: 'n' + i, target: 'n' + (i + 1) });
  const far = G.depthSearch(G.adjacency(chain), 'n0', 9);
  assert.equal(far.n5, 5);
  assert.equal(far.n6, undefined);
});

test('clampDepth', () => {
  assert.equal(G.clampDepth('3'), 3);
  assert.equal(G.clampDepth(0), 1);
  assert.equal(G.clampDepth(7), 5);
  assert.equal(G.clampDepth('x'), 1);
});

test('nodeSize grows with degree and is flat with byDegree off', () => {
  assert.equal(G.maxDegree(nodes), 3);
  const small = G.nodeSize(1, 3, true);
  const big = G.nodeSize(3, 3, true);
  assert.ok(big > small);
  assert.equal(G.nodeSize(0, 3, true), 2);
  assert.equal(G.nodeSize(0, 0, true), 2);
  assert.equal(G.nodeSize(3, 3, true), 9);
  assert.equal(G.nodeSize(1, 3, false), G.nodeSize(3, 3, false));
});

test('staleness: stale, due within 30 days, fresh, none', () => {
  const today = '2026-11-05';
  assert.equal(G.addDays(today, 30), '2026-12-05');
  assert.equal(G.addDays('2026-12-20', 30), '2027-01-19');
  assert.equal(G.staleness({ stale: true, review_by: '2027-06-01' }, today), 'stale');
  assert.equal(G.staleness({ review_by: '2026-11-20' }, today), 'due');
  assert.equal(G.staleness({ review_by: '2027-06-01' }, today), 'fresh');
  assert.equal(G.staleness({}, today), 'none');
});

test('colorRole per mode, with dangling and tag nodes in fixed roles', () => {
  const ctx = { vaults: ['me', 'team'], today: '2026-11-05' };
  assert.equal(G.colorRole(nodes[0], 'tier', ctx), 'blue');
  assert.equal(G.colorRole(nodes[2], 'tier', ctx), 'amber');
  assert.equal(G.colorRole(nodes[0], 'vault', ctx), 'series-1');
  assert.equal(G.colorRole(nodes[2], 'vault', ctx), 'series-2');
  assert.equal(G.colorRole({ id: 'x', vault: 'other' }, 'vault', ctx), 'text-3');
  assert.equal(G.colorRole(nodes[0], 'evidence', ctx), 'green');
  assert.equal(G.colorRole(nodes[1], 'evidence', ctx), 'text-3');
  assert.equal(G.colorRole(nodes[1], 'status', ctx), 'amber');
  assert.equal(G.colorRole(nodes[1], 'staleness', ctx), 'red');
  assert.equal(G.colorRole(nodes[4], 'tier', ctx), 'text-3');
  assert.equal(G.colorRole(nodes[5], 'status', ctx), 'text-2');
});

test('legend lists the mode values and adds none when a note has no value', () => {
  const ctx = { vaults: ['me', 'team'] };
  assert.deepEqual(G.legend('vault', nodes, ctx), [
    { label: 'me', role: 'series-1' }, { label: 'team', role: 'series-2' },
  ]);
  const tiers = G.legend('tier', nodes, ctx);
  assert.deepEqual(tiers.map((e) => e.label), ['inbox', 'notes', 'outputs', 'MOCs']);
  const evidence = G.legend('evidence', nodes, ctx);
  assert.equal(evidence[evidence.length - 1].label, 'none');
  assert.equal(G.legend('staleness', nodes, ctx).length, 4);
});

test('knownPositions prefers the server position, then the last drawn one', () => {
  const got = G.knownPositions(
    [{ id: 'a', x: 1, y: 2 }, { id: 'b' }, { id: 'c' }],
    { a: [9, 9], b: [3, 4] },
  );
  assert.deepEqual(got, { a: [1, 2], b: [3, 4] });
});

test('anchorsFor: a new node starts at its placed neighbours, a new chain follows', () => {
  const adj = G.adjacency([
    { source: 'k1', target: 'n1' },
    { source: 'k2', target: 'n1' },
    { source: 'n1', target: 'n2' },
  ]);
  const known = { k1: [0, 0], k2: [10, 0] };
  const anchors = G.anchorsFor(['n2', 'n1', 'lone'], adj, known);
  assert.deepEqual(anchors.n1, [5, 0]);
  assert.deepEqual(anchors.n2, [5, 0]);
  assert.deepEqual(anchors.lone, [5, 0], 'no neighbours: the layout centroid');
  assert.deepEqual(G.anchorsFor(['x'], {}, {}).x, [0, 0]);
});

test('groupByAnchor groups new nodes that share an anchor', () => {
  const groups = G.groupByAnchor({ a: [1, 1], b: [1, 1], c: [2, 0] });
  assert.equal(groups.length, 2);
  assert.deepEqual(groups.map((g) => g.ids.sort()), [['a', 'b'], ['c']]);
});

test('spacing is the layout side over the square root of the count', () => {
  assert.equal(G.spacing({}), 1);
  assert.equal(G.spacing({ a: [0, 0], b: [0, 0] }), 1);
  assert.equal(G.spacing({ a: [0, 0], b: [8, 2], c: [4, 4], d: [0, 8] }), 4);
});

test('layoutMs: none without new nodes, a short settle for a few, more for many', () => {
  assert.equal(G.layoutMs(100, 0), 0);
  assert.equal(G.layoutMs(0, 0), 0);
  assert.equal(G.layoutMs(100, 1), 800);
  assert.equal(G.layoutMs(100, 100), 1900);
  assert.equal(G.layoutMs(5000, 5000), 8000);
});

test('wheelRatio: scales by the delta, adds up across events, weights pinches, clamps', () => {
  const close = (a, b) => assert.ok(Math.abs(a - b) < 1e-9, `${a} != ${b}`);
  assert.equal(G.wheelRatio(1, 0, 0, false, 0.02, 20), 1);
  assert.ok(G.wheelRatio(1, -100, 0, false, 0.02, 20) < 1, 'scrolling up zooms in');
  assert.ok(G.wheelRatio(1, 100, 0, false, 0.02, 20) > 1, 'scrolling down zooms out');
  let r = 1;
  for (let i = 0; i < 100; i++) r = G.wheelRatio(r, 1, 0, false, 0.02, 20);
  close(r, G.wheelRatio(1, 100, 0, false, 0.02, 20));
  close(G.wheelRatio(1, 3, 1, false, 0.02, 20), G.wheelRatio(1, 48, 0, false, 0.02, 20));
  assert.ok(G.wheelRatio(1, 10, 0, true, 0.02, 20) > G.wheelRatio(1, 10, 0, false, 0.02, 20));
  assert.equal(G.wheelRatio(1, -1e6, 0, false, 0.02, 20), 0.02);
  assert.equal(G.wheelRatio(1, 1e6, 0, false, 0.02, 20), 20);
});

test('layoutPayload rounds positions and drops non-finite ones', () => {
  assert.deepEqual(G.layoutPayload('k1', { a: [1.23456, -2.00049], b: [NaN, 1] }), {
    key: 'k1',
    positions: { a: [1.235, -2] },
  });
});

test('wikilinkLabel: alias, heading and plain target', () => {
  assert.equal(G.wikilinkLabel('[[pg-failover|Failover]]'), 'Failover');
  assert.equal(G.wikilinkLabel('[[pg-failover#Steps]]'), 'pg-failover > Steps');
  assert.equal(G.wikilinkLabel('[[pg-failover]]'), 'pg-failover');
  assert.equal(G.wikilinkLabel('![[diagram]]'), 'diagram');
});

test('linkIndex keys links by text, first entry wins', () => {
  const idx = G.linkIndex([
    { text: '[[a]]', target: 'id:a', resolved: true },
    { text: '[[a]]', target: 'id:other', resolved: true },
    { text: '[[gone]]', target: 'dangling:gone', resolved: false },
  ]);
  assert.equal(idx['[[a]]'].target, 'id:a');
  assert.equal(idx['[[gone]]'].resolved, false);
  assert.deepEqual(G.linkIndex(null), {});
});
