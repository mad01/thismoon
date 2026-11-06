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
  assert.deepEqual(G.COLOR_MODES, ['vault', 'tier', 'evidence', 'status', 'staleness', 'cluster']);
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

test('activeFilters counts the filters away from the defaults', () => {
  assert.equal(G.activeFilters(G.DEFAULT_FILTERS), 0);
  assert.equal(G.DEFAULT_FILTERS.dangling, false, 'dangling targets start hidden');
  assert.equal(G.activeFilters({ ...G.DEFAULT_FILTERS, vault: 'me', dangling: true }), 2);
  assert.equal(G.activeFilters({ ...G.DEFAULT_FILTERS, orphans: false }), 1);
});

test('coversAll: a filtered set does not cover the global layout, a wider one does', () => {
  const global = G.visibleIds(nodes, edges, G.DEFAULT_FILTERS);
  assert.equal(G.coversAll(global, global), true);
  assert.equal(G.coversAll(G.visibleIds(nodes, edges, { ...G.DEFAULT_FILTERS, vault: 'me' }), global), false);
  assert.equal(G.coversAll(G.visibleIds(nodes, edges, all), global), true, 'dangling on adds nodes');
  assert.equal(G.coversAll({}, {}), true);
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

test('nodeRadius grows with degree, is flat with byDegree off, small for dangling', () => {
  assert.equal(G.maxDegree(nodes), 3);
  assert.equal(G.nodeRadius({ id: 'x', degree: 3 }, 3, true), 40);
  assert.equal(G.nodeRadius({ id: 'x', degree: 1 }, 4, true), 28);
  assert.equal(G.nodeRadius({ id: 'x', degree: 0 }, 3, true), 16);
  assert.equal(G.nodeRadius({ id: 'x', degree: 2 }, 0, true), 16);
  assert.equal(G.nodeRadius({ id: 'x', degree: 1 }, 3, false), 25);
  assert.equal(G.nodeRadius({ id: 'x', degree: 3 }, 3, false), 25);
  assert.equal(G.nodeRadius(nodes[4], 3, true), 12);
  assert.equal(G.nodeRadius(nodes[4], 3, false), 12);
});

test('captionShown and captionsFlip follow the on-screen radius', () => {
  assert.equal(G.captionShown(22, 1, 22), true);
  assert.equal(G.captionShown(21, 1, 22), false);
  assert.equal(G.captionShown(16, 0.5, 22), true);
  assert.equal(G.captionsFlip([16, 25, 40], 22, 1, 1), false);
  assert.equal(G.captionsFlip([16, 25, 40], 22, 1, 0.9), false, '16 is still too small at 0.9');
  assert.equal(G.captionsFlip([16, 25, 40], 22, 1, 0.7), true, '16 shows from 0.72 down');
  assert.equal(G.captionsFlip([25, 40], 22, 1, 1.2), true, '25 / 1.2 drops under 22');
  assert.equal(G.captionsFlip([], 22, 1, 3), false);
});

test('radiiOf lists the distinct radii, sorted', () => {
  assert.deepEqual(G.radiiOf(nodes, 3, false), [12, 25]);
  const byDegree = G.radiiOf(nodes, 3, true);
  assert.equal(byDegree[0], 12);
  assert.equal(byDegree[byDegree.length - 1], 40);
  assert.deepEqual(byDegree, byDegree.slice().sort((a, b) => a - b));
});

test('captionSize scales with the radius within 6 to 13', () => {
  assert.equal(G.captionSize(25), 8.5);
  assert.equal(G.captionSize(10), 6);
  assert.equal(G.captionSize(40), 13);
});

test('caption fits the circle and cuts with an ellipsis', () => {
  assert.equal(G.caption('Postgres failover runbook', 25, 8.5), 'Postgres\u2026');
  assert.equal(G.caption('A', 25, 8.5), 'A');
  assert.equal(G.caption('  two \n  words ', 25, 8.5), 'two words');
  assert.equal(G.caption('ab, cdefghijk', 10, 6), 'ab\u2026', 'trailing punctuation goes before the ellipsis');
  assert.equal(G.caption('anything', 2, 13), '', 'no room for even one letter and the ellipsis');
  assert.equal(G.caption(undefined, 25, 8.5), '');
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
  assert.equal(G.colorRole(nodes[0], 'tier', ctx), 'node-blue');
  assert.equal(G.colorRole(nodes[2], 'tier', ctx), 'node-yellow');
  assert.equal(G.colorRole(nodes[0], 'vault', ctx), 'node-blue');
  assert.equal(G.colorRole(nodes[2], 'vault', ctx), 'node-orange');
  assert.equal(G.colorRole({ id: 'x', vault: 'other' }, 'vault', ctx), 'node-grey');
  assert.equal(G.colorRole(nodes[0], 'evidence', ctx), 'node-green');
  assert.equal(G.colorRole(nodes[1], 'evidence', ctx), 'node-grey');
  assert.equal(G.colorRole(nodes[1], 'status', ctx), 'node-orange');
  assert.equal(G.colorRole(nodes[1], 'staleness', ctx), 'node-red');
  assert.equal(G.colorRole(nodes[4], 'tier', ctx), 'node-beige');
  assert.equal(G.colorRole(nodes[5], 'status', ctx), 'node-teal');
});

test('vault roles cycle the node palette', () => {
  const vaults = G.NODE_ROLES.map((_, i) => 'v' + i).concat(['wrap']);
  assert.equal(G.colorRole({ id: 'x', vault: 'wrap' }, 'vault', { vaults }), G.NODE_ROLES[0]);
  assert.equal(new Set(G.NODE_ROLES).size, G.NODE_ROLES.length);
});

test('borderRole names the ring of a fill role', () => {
  assert.equal(G.borderRole('node-blue'), 'node-blue-border');
});

test('luminance and captionColor: white on dark fills, dark on light ones', () => {
  assert.equal(G.luminance('#FFFFFF'), 1);
  assert.equal(G.luminance('#000000'), 0);
  assert.equal(G.luminance('not a colour'), 0);
  assert.equal(G.captionColor('#4C8EDA'), '#FFFFFF', 'blue');
  assert.equal(G.captionColor('#F79767'), '#FFFFFF', 'orange');
  assert.equal(G.captionColor('#604A0E'), '#FFFFFF', 'brown');
  assert.equal(G.captionColor('#57C7E3'), '#2A2C34', 'cyan');
  assert.equal(G.captionColor('#FFC454'), '#2A2C34', 'yellow');
  assert.equal(G.captionColor('#8DCC93'), '#2A2C34', 'green');
});

test('edgeCaption is the kind in capitals', () => {
  assert.equal(G.edgeCaption('supersedes'), 'SUPERSEDES');
  assert.equal(G.edgeCaption(''), 'LINK');
});

test('legend lists the mode values and adds none when a note has no value', () => {
  const ctx = { vaults: ['me', 'team'] };
  assert.deepEqual(G.legend('vault', nodes, ctx), [
    { label: 'me', role: 'node-blue' }, { label: 'team', role: 'node-orange' },
  ]);
  const tiers = G.legend('tier', nodes, ctx);
  assert.deepEqual(tiers.map((e) => e.label), ['inbox', 'notes', 'outputs', 'MOCs']);
  const evidence = G.legend('evidence', nodes, ctx);
  assert.equal(evidence[evidence.length - 1].label, 'none');
  assert.equal(G.legend('staleness', nodes, ctx).length, 4);
});

test('cluster colours: one palette colour per community, grey for unlinked', () => {
  const ctx = { community: { 'id:a': 0, 'id:b': 0, 'team:notes/c.md': 1, 'id:d': G.LOOSE } };
  assert.equal(G.colorRole(nodes[0], 'cluster', ctx), G.NODE_ROLES[0]);
  assert.equal(G.colorRole(nodes[2], 'cluster', ctx), G.NODE_ROLES[1]);
  assert.equal(G.colorRole(nodes[3], 'cluster', ctx), 'node-grey');
  assert.equal(G.colorRole(nodes[5], 'cluster', ctx), G.colorRole(nodes[5], 'tier', ctx), 'a tag keeps its colour');
  assert.equal(G.clusterRole(G.NODE_ROLES.length), G.NODE_ROLES[0], 'the palette wraps');
  assert.equal(G.colorRole(nodes[0], 'cluster', {}), 'node-grey', 'no communities yet');
});

test('cluster legend names each community after its most linked note', () => {
  const ctx = { community: { 'id:a': 0, 'id:b': 0, 'team:notes/c.md': 1, 'id:d': G.LOOSE } };
  assert.deepEqual(G.legend('cluster', nodes, ctx), [
    { label: 'A', role: G.NODE_ROLES[0] },
    { label: 'c', role: G.NODE_ROLES[1] },
    { label: 'unlinked', role: 'node-grey' },
  ]);
  const many = [];
  const community = {};
  for (let i = 0; i < G.NODE_ROLES.length + 2; i++) {
    many.push({ id: 'n' + i, title: 'Note ' + i, degree: 1 });
    community['n' + i] = i;
  }
  const out = G.legend('cluster', many, { community });
  assert.equal(out.length, G.NODE_ROLES.length + 1);
  assert.deepEqual(out[out.length - 1], { label: '+2 more', role: '' });
});

test('legend adds the tag and dangling colours only with their toggles on', () => {
  const labels = (ctx) => G.legend('tier', nodes, ctx).map((e) => e.label);
  assert.deepEqual(labels({}), ['inbox', 'notes', 'outputs', 'MOCs']);
  const on = G.legend('vault', nodes, { vaults: ['me'], topics: true, dangling: true });
  assert.deepEqual(on.slice(-2), [{ label: 'tag', role: 'node-teal' }, { label: 'dangling', role: 'node-beige' }]);
  const notes = nodes.filter((n) => G.nodeKind(n) === 'note');
  assert.deepEqual(G.legend('tier', notes, { topics: true, dangling: true }).length, 4, 'none of either kind');
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

const near = (a, b, eps = 1e-9) => Math.abs(a - b) < eps;

test('spiral spreads points outward from the origin, skip drops the inner ones', () => {
  const pts = G.spiral(5, 36);
  assert.equal(pts.length, 5);
  assert.ok(near(pts[0][0], 36 * Math.sqrt(0.5)) && near(pts[0][1], 0));
  const dist = pts.map((p) => Math.hypot(p[0], p[1]));
  for (let i = 1; i < dist.length; i++) assert.ok(dist[i] > dist[i - 1]);
  assert.deepEqual(G.spiral(1, 36, 1)[0], G.spiral(2, 36)[1]);
  assert.deepEqual(G.spiral(0, 36), []);
});

test('placeFresh keeps known positions and starts new nodes near their neighbours', () => {
  const adj = G.adjacency([{ source: 'k', target: 'n' }]);
  assert.deepEqual(G.placeFresh([], adj, { k: [1, 2] }), { k: [1, 2] });
  const out = G.placeFresh(['n'], adj, { k: [100, 100] });
  assert.deepEqual(out.k, [100, 100]);
  assert.ok(Math.hypot(out.n[0] - 100, out.n[1] - 100) < 80, 'n starts next to k');
  assert.ok(Math.hypot(out.n[0] - 100, out.n[1] - 100) > 10, 'but not on top of it');
  const all = G.placeFresh(['a', 'b', 'c'], {}, {});
  assert.deepEqual(Object.keys(all).sort(), ['a', 'b', 'c']);
  assert.deepEqual(all.a, G.spiral(1, 36)[0]);
});

test('simParams: the global graph puts the island settings on top of the base ones', () => {
  const local = G.simParams();
  const global = G.simParams(true);
  assert.equal(local.islandPull, undefined, 'a local graph has no islands');
  assert.equal(global.gravity, 0, 'islands replace the gravity');
  assert.ok(global.islandPull > 0 && global.islandWall > 0 && global.islandCell > 0);
  assert.ok(global.islandJoin > 2 * global.collidePad, 'neighbours on one island are within islandJoin');
  assert.ok(global.islandJoin < global.islandGapMin, 'two islands are further apart than islandJoin');
  assert.equal(global.charge, local.charge);
  assert.ok(near(Math.pow(1 - global.alphaDecay, global.coolTicks), global.alphaMin, 1e-6));
});

test('simParams: the base settings, decay from coolTicks', () => {
  const p = G.simParams();
  assert.equal(p.charge, -200);
  assert.equal(p.chargeDistanceMax, 2500);
  assert.equal(p.linkGap, 20);
  assert.equal(p.linkBase, 0.5);
  assert.equal(p.linkPerShared, 1);
  assert.equal(p.gravity, 0.01);
  assert.equal(p.velocityDecay, 0.3);
  assert.ok(p.calmVelocityDecay > p.velocityDecay, 'a drag moves the graph slower than a settle');
  assert.ok(p.dragAlphaTarget < p.reheatAlpha, 'a drag heats the graph less than a reheat');
  assert.ok(near(Math.pow(1 - p.alphaDecay, p.coolTicks), p.alphaMin, 1e-6), 'cools to alphaMin in coolTicks');
  p.charge = 0;
  assert.equal(G.simParams().charge, -200, 'each call returns a fresh copy');
});

test('placeFresh starts new nodes with no placed neighbour at their island home', () => {
  const homes = { a: [1000, 0, 200], b: [1000, 0, 200], c: [-1000, 0, 200] };
  const out = G.placeFresh(['a', 'b', 'c'], {}, {}, homes);
  assert.ok(Math.hypot(out.a[0] - 1000, out.a[1]) < 100, 'a starts on its island');
  assert.ok(Math.hypot(out.b[0] - 1000, out.b[1]) < 100, 'b starts on the same island');
  assert.notDeepEqual(out.a, out.b, 'not on one point');
  assert.ok(Math.hypot(out.c[0] + 1000, out.c[1]) < 100, 'c starts on the other island');
  // A new node with a placed neighbour still starts beside it.
  const adj = G.adjacency([{ source: 'k', target: 'n' }]);
  const near2 = G.placeFresh(['n'], adj, { k: [5, 5] }, { n: [900, 900, 100] });
  assert.ok(Math.hypot(near2.n[0] - 5, near2.n[1] - 5) < 80);
});

test('linkStrengths: links inside a group pull harder than a link between groups', () => {
  const P = G.simParams();
  // Two triangles, a-b-c and d-e-f, joined by c-d.
  const links = [
    { source: 'a', target: 'b' }, { source: 'b', target: 'c' }, { source: 'c', target: 'a' },
    { source: 'c', target: 'd' },
    { source: 'd', target: 'e' }, { source: 'e', target: 'f' }, { source: 'f', target: 'd' },
  ];
  const got = G.linkStrengths(links, P);
  assert.equal(got.length, links.length);
  // a-b share c; each end has two links: (0.5 + 1) / 2.
  assert.equal(got[0], 0.75);
  // c-a, in the order given, shares b; c has three links, a two.
  assert.equal(got[2], 0.75);
  // c-d shares nothing and both ends have three links: 0.5 / 3.
  assert.ok(near(got[3], 0.5 / 3));
  assert.ok(got.every((s, i) => i === 3 || s > got[3]), 'the bridge pulls weakest');
  // A link to a note with no other links shares nothing: linkBase over 1.
  assert.deepEqual(G.linkStrengths([{ source: 'a', target: 'z' }], P), [0.5]);
  assert.deepEqual(G.linkStrengths([], P), []);
});

test('linkStrengths: a link between two communities pulls crossLink times as hard', () => {
  const P = G.simParams(true);
  const links = [{ source: 'a', target: 'b' }, { source: 'b', target: 'c' }];
  const plain = G.linkStrengths(links, P);
  const got = G.linkStrengths(links, P, { a: 0, b: 0, c: 1 });
  assert.equal(got[0], plain[0]);
  assert.ok(near(got[1], plain[1] * P.crossLink));
  assert.ok(P.crossLink < 0.1, 'a cross link barely pulls');
});

test('linkStrengths: never above 1', () => {
  const P = Object.assign(G.simParams(), { linkBase: 2 });
  const got = G.linkStrengths([{ source: 'a', target: 'b' }, { source: 'b', target: 'c' }, { source: 'c', target: 'a' }], P);
  assert.deepEqual(got, [1, 1, 1]);
});

test('startAlpha: cold on a full cache, gentle on old positions, hotter with more new nodes', () => {
  assert.equal(G.startAlpha(0, 0, false), 0);
  assert.equal(G.startAlpha(10, 0, true), 0);
  assert.equal(G.startAlpha(10, 0, false), 0.1);
  assert.equal(G.startAlpha(100, 25, true), 0.55);
  assert.equal(G.startAlpha(100, 100, false), 1);
});

test('simLinks: one link per shown pair, no self links', () => {
  const shown = { a: true, b: true, c: true };
  const got = G.simLinks([
    { source: 'a', target: 'b' }, { source: 'b', target: 'a' }, { source: 'a', target: 'a' },
    { source: 'a', target: 'x' }, { source: 'b', target: 'c' },
  ], shown);
  assert.deepEqual(got, [{ source: 'a', target: 'b' }, { source: 'b', target: 'c' }]);
});

test('boundsOf and fitRatio frame circles in the viewport', () => {
  assert.equal(G.boundsOf([]), null);
  const b = G.boundsOf([{ x: 0, y: 0, r: 10 }, { x: 100, y: 50, r: 5 }]);
  assert.deepEqual(b, { minX: -10, minY: -10, maxX: 105, maxY: 55 });
  const box = { minX: 0, minY: 0, maxX: 200, maxY: 100 };
  assert.equal(G.fitRatio(box, { width: 100, height: 100 }, 0, 0.1, 20), 2);
  assert.ok(near(G.fitRatio(box, { width: 100, height: 100 }, 0.1, 0.1, 20), 2.4));
  assert.equal(G.fitRatio({ minX: 0, minY: 0, maxX: 0, maxY: 0 }, { width: 100, height: 100 }, 0, 0.1, 20), 1);
  assert.equal(G.fitRatio({ minX: 0, minY: 0, maxX: 1e6, maxY: 1 }, { width: 100, height: 100 }, 0, 0.1, 20), 20);
});

test('easeOut, tweenValue and dimOpacity drive the hover fade', () => {
  assert.equal(G.easeOut(0), 0);
  assert.equal(G.easeOut(1), 1);
  assert.equal(G.easeOut(0.5), 0.875);
  assert.equal(G.tweenValue(0, 1, 100, 100, 200), 0);
  assert.equal(G.tweenValue(0, 1, 100, 200, 200), 0.875);
  assert.equal(G.tweenValue(0, 1, 100, 300, 200), 1);
  assert.equal(G.tweenValue(1, 0, 100, 150, 0), 0);
  assert.equal(G.dimOpacity(0, 0.12), 1);
  assert.equal(G.dimOpacity(1, 0.12), 0.12);
});

test('searchHits maps literal files and hybrid hits to node ids, ranked, once each', () => {
  const refs = G.refIndex(nodes);
  const literal = { files: [
    { vault: 'me', path: 'notes/b.md' }, { vault: 'me', path: 'notes/a.md' },
    { vault: 'me', path: 'notes/b.md' }, { vault: 'gone', path: 'x.md' },
  ] };
  assert.deepEqual(G.searchHits(literal, refs), ['id:b', 'id:a']);
  assert.deepEqual(G.searchHits({ hits: [{ vault: 'team', path: 'notes/c.md' }] }, refs), ['team:notes/c.md']);
  assert.deepEqual(G.searchHits(null, refs), []);
  assert.deepEqual(G.searchHits({}, refs), []);
});

test('restrictTo narrows a shown set to a search, or keeps it without one', () => {
  const shown = { a: true, b: true };
  assert.equal(G.restrictTo(shown, null), shown);
  assert.deepEqual(G.restrictTo(shown, ['b', 'c']), { b: true });
  assert.deepEqual(G.restrictTo(shown, []), {});
});

test('countNotes counts notes only, not tags or dangling targets', () => {
  const ids = {};
  nodes.forEach((n) => { ids[n.id] = true; });
  assert.equal(G.countNotes(nodes, ids), 4);
  assert.equal(G.countNotes(nodes, { 'id:a': true, 'tag:db': true }), 1);
  assert.equal(G.countNotes(nodes, {}), 0);
});

test('reviewCount sums the actionable items and skips failed queues', () => {
  assert.equal(G.reviewCount({ promotions: { count: 2, actionable: 2 }, stale: { count: 3, actionable: 3 }, check: { error: new Error('x') } }), 5);
  // 1100 check warnings stay in the Check tab; only the 6 errors count.
  assert.equal(G.reviewCount({
    promotions: { count: 1, actionable: 1 }, stale: { count: 0, actionable: 0 }, check: { count: 1106, actionable: 6 },
  }), 7);
  assert.equal(G.reviewCount({ promotions: null, stale: null, check: null }), 0);
  assert.equal(G.reviewCount(null), 0);
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

// ── communities and islands ──

// A detect stub that answers one label per connected component, so the
// tests pin what communities does with any finder's answer.
function components(ids, links) {
  const adj = G.adjacency(links);
  const out = {};
  let next = 100;
  ids.forEach((id) => {
    if (out[id] !== undefined) return;
    const label = 'c' + next++;
    const stack = [id];
    while (stack.length) {
      const x = stack.pop();
      if (out[x] !== undefined) continue;
      out[x] = label;
      G.neighbours(adj, x).forEach((nb) => { if (ids.includes(nb)) stack.push(nb); });
    }
  });
  return out;
}

const L = (s, t) => ({ source: s, target: t });

test('seededRandom: one seed gives one sequence in [0, 1)', () => {
  const a = G.seededRandom(G.COMMUNITY_SEED);
  const b = G.seededRandom(G.COMMUNITY_SEED);
  const xs = Array.from({ length: 50 }, () => a());
  assert.deepEqual(Array.from({ length: 50 }, () => b()), xs);
  assert.ok(xs.every((x) => x >= 0 && x < 1));
  assert.notEqual(G.seededRandom(1)(), G.seededRandom(2)());
});

test('communities: numbered by size from 0, unlinked notes LOOSE', () => {
  const ids = ['s1', 's2', 'b1', 'b2', 'b3', 'lone'];
  const links = [L('s1', 's2'), L('b1', 'b2'), L('b2', 'b3')];
  const got = G.communities(ids, links, components, null);
  assert.deepEqual(got, { b1: 0, b2: 0, b3: 0, s1: 1, s2: 1, lone: G.LOOSE });
  assert.deepEqual(G.communities([], [], components, null), {});
});

test('communities: with the last answer nothing moves and a new note joins its neighbours', () => {
  const ids = ['a1', 'a2', 'a3', 'b1', 'b2', 'new'];
  const links = [L('a1', 'a2'), L('a2', 'a3'), L('b1', 'b2'), L('a3', 'b1'), L('new', 'b1'), L('new', 'b2'), L('new', 'a1')];
  const previous = { a1: 0, a2: 0, a3: 0, b1: 1, b2: 1 };
  // The finder would now merge everything; the previous answer wins.
  const all = () => Object.fromEntries(ids.map((id) => [id, 'x']));
  const got = G.communities(ids, links, all, previous);
  assert.deepEqual(got, { a1: 0, a2: 0, a3: 0, b1: 1, b2: 1, new: 1 });
});

test('communities: new notes linked only among themselves form a new community', () => {
  const ids = ['a1', 'a2', 'b1', 'b2', 'n1', 'n2', 'n3', 'was'];
  const links = [L('a1', 'a2'), L('b1', 'b2'), L('n1', 'n2'), L('n2', 'n3'), L('was', 'a1')];
  const previous = { a1: 0, a2: 0, b1: 3, b2: 3, was: G.LOOSE, gone: 7 };
  const got = G.communities(ids, links, components, previous);
  assert.equal(got.n1, 4, 'numbered after the communities still in use');
  assert.equal(got.n2, 4);
  assert.equal(got.n3, 4);
  assert.equal(got.was, 0, 'a note that was unlinked joins once it links');
  assert.equal(got.gone, undefined, 'a note no longer in ids drops out');
});

test('joinCommunities: majority of the assigned neighbours, the smaller number on a tie', () => {
  const adj = G.adjacency([L('x', 'a'), L('x', 'b'), L('x', 'c'), L('y', 'a'), L('y', 'c'), L('z', 'x'), L('w', 'q')]);
  const assigned = { a: 1, b: 2, c: 2 };
  const left = G.joinCommunities(['x', 'y', 'z', 'w'], adj, assigned);
  assert.equal(assigned.x, 2);
  assert.equal(assigned.y, 1);
  assert.equal(assigned.z, 2, 'z waits for x, then follows it');
  assert.deepEqual(left, ['w']);
});

test('islandsFromLayout: circles close together share an island, a lone one has none', () => {
  const P = G.simParams(true);
  const r = () => 20;
  const known = {};
  // Three circles in a row 40 apart border to border, two more far away,
  // and one alone further still.
  ['a', 'b', 'c'].forEach((id, i) => { known[id] = [i * 80, 0]; });
  known.d = [2000, 0];
  known.e = [2080, 0];
  known.f = [5000, 5000];
  const got = G.islandsFromLayout(known, r, P);
  assert.deepEqual(got, { a: 0, b: 0, c: 0, d: 1, e: 1 });
  // Bigger circles at the same centres touch: the border gap counts.
  assert.equal(G.islandsFromLayout({ p: [0, 0], q: [300, 0] }, () => 120, P).q, 0);
  assert.deepEqual(G.islandsFromLayout({}, r, P), {});
});

function discsOf(homes) {
  const seen = {};
  Object.values(homes).forEach((h) => { seen[h.join(',')] = h; });
  return Object.values(seen);
}

test('islandHomes: islands sized by their members and packed with a gap between each pair', () => {
  const P = G.simParams(true);
  const community = {};
  [12, 7, 7, 3, 2, 1].forEach((n, c) => { for (let i = 0; i < n; i++) community[c + '-' + i] = c; });
  const homes = G.islandHomes(community, () => 20, {}, P);
  assert.deepEqual(Object.keys(homes).sort(), Object.keys(community).sort());
  const discs = discsOf(homes);
  assert.equal(discs.length, 6);
  for (let i = 0; i < discs.length; i++) {
    for (let j = i + 1; j < discs.length; j++) {
      const [a, b] = [discs[i], discs[j]];
      const d = Math.hypot(a[0] - b[0], a[1] - b[1]);
      assert.ok(d >= a[2] + b[2] + G.islandGap(a[2], b[2], P) - 1e-6, 'islands ' + i + ' and ' + j + ' keep their gap');
    }
  }
  const big = homes['0-0'];
  const small = homes['5-0'];
  assert.ok(big[2] > small[2], 'more members, a bigger island');
  assert.ok(near(big[2], G.islandRadius(Array(12).fill(20), P)));
  assert.deepEqual(G.islandHomes(community, () => 20, {}, P), homes, 'the same input packs the same way');
});

test('islandHomes: a placed island stays at its members, a new one keeps clear of it', () => {
  const P = G.simParams(true);
  const community = { a1: 0, a2: 0, b1: 1, b2: 1, n1: 2, n2: 2 };
  const known = { a1: [-1500, 0], a2: [-1300, 0], b1: [1300, 0], b2: [1500, 0] };
  const homes = G.islandHomes(community, () => 20, known, P);
  assert.deepEqual(homes.a1.slice(0, 2), [-1400, 0]);
  assert.deepEqual(homes.b1.slice(0, 2), [1400, 0]);
  [homes.a1, homes.b1].forEach((h) => {
    const d = Math.hypot(homes.n1[0] - h[0], homes.n1[1] - h[1]);
    assert.ok(d >= h[2] + homes.n1[2] + G.islandGap(h[2], homes.n1[2], P) - 1e-6);
  });
  // Two placed islands on top of each other are pushed apart.
  const both = G.islandHomes({ a: 0, b: 1 }, () => 20, { a: [0, 0], b: [10, 0] }, P);
  const d = Math.hypot(both.a[0] - both.b[0], both.a[1] - both.b[1]);
  assert.ok(d >= both.a[2] + both.b[2] + G.islandGap(both.a[2], both.b[2], P) - 1e-6);
});

test('islandHomes: only counted ids size an island, the rest share its home', () => {
  const P = G.simParams(true);
  const community = { a: 0, b: 0, ghost1: 0, ghost2: 0, ghost3: 0 };
  const homes = G.islandHomes(community, () => 20, {}, P, { a: true, b: true });
  assert.deepEqual(homes.ghost1, homes.a);
  assert.ok(near(homes.a[2], G.islandRadius([20, 20], P)));
});

test('islandForce: pulls toward the island centre, harder from outside the edge', () => {
  const P = G.simParams(true);
  const inside = { x: 50, y: 0, vx: 0, vy: 0, home: [0, 0, 100] };
  const outside = { x: 300, y: 0, vx: 0, vy: 0, home: [0, 0, 100] };
  const free = { x: 300, y: 0, vx: 0, vy: 0, home: null };
  const force = G.islandForce(P);
  force.initialize([inside, outside, free]);
  force(1);
  assert.ok(inside.vx < 0 && inside.vy === 0, 'inside: toward the centre');
  assert.ok(near(inside.vx, -50 * P.islandPull));
  assert.ok(outside.vx < inside.vx * 6, 'outside: the wall adds to the pull');
  assert.equal(free.vx, 0, 'a node with no home is left alone');
});
