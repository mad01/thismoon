// Pure graph helpers for the brain.this UI (docs/serve.md, Browser globals).
// No DOM access: app.js hands these functions the /api/graph nodes and edges
// and turns their answers into sigma state. node --test runs them against a
// fake window (test/graph.test.mjs).
//
// Colours come back as palette role names ("green", "series-2"), never as
// hex. app.js reads the role from the page's CSS variables, so the graph
// follows the webkit theme without this file knowing about it.
(function () {
  'use strict';

  // The lists below are JSON on one line each: ui_test.go reads TIERS and
  // STATUSES out of this file and checks them against the Go lists.
  var TIERS = ["inbox", "notes", "outputs", "MOCs"];
  var STATUSES = ["raw", "validated", "promoting", "disputed", "superseded", "retracted", "withdrawn"];
  var EVIDENCE = ["A", "B", "C", "D", "mixed"];
  var EDGE_KINDS = ["link", "tag", "supersedes", "source", "moc"];
  var COLOR_MODES = ["vault", "tier", "evidence", "status", "staleness"];
  var STALENESS = ["stale", "due", "fresh", "none"];

  var DEPTH_MIN = 1;
  var DEPTH_MAX = 5;
  // A note whose review_by falls within this many days reads as "due".
  var DUE_DAYS = 30;

  var TAG_PREFIX = 'tag:';
  var DANGLING_PREFIX = 'dangling:';

  var VAULT_ROLES = ['series-1', 'series-2', 'series-3', 'series-4', 'purple', 'amber', 'red', 'yellow'];
  var TIER_ROLES = { inbox: 'amber', notes: 'blue', outputs: 'green', MOCs: 'purple' };
  var EVIDENCE_ROLES = { A: 'green', B: 'blue', C: 'yellow', D: 'red', mixed: 'purple' };
  var STATUS_ROLES = {
    raw: 'yellow', validated: 'green', promoting: 'blue', disputed: 'amber',
    superseded: 'purple', retracted: 'red', withdrawn: 'text-3',
  };
  var STALENESS_ROLES = { stale: 'red', due: 'amber', fresh: 'green', none: 'text-3' };
  var STALENESS_LABELS = { stale: 'stale', due: 'review due soon', fresh: 'reviewed', none: 'no review date' };
  var EDGE_ROLES = { link: 'border-hover', tag: 'border', supersedes: 'amber', source: 'blue', moc: 'primary' };
  var NONE_ROLE = 'text-3';
  var DANGLING_ROLE = 'text-3';
  var TAG_ROLE = 'text-2';

  // ── nodes ──

  // nodeKind is "note", "dangling" (a link target no note matches) or "tag"
  // (a topic node, only with topics=1).
  function nodeKind(n) {
    if (n.dangling) return 'dangling';
    if (String(n.id).indexOf(TAG_PREFIX) === 0) return 'tag';
    return 'note';
  }

  function nodeLabel(n) {
    if (n.title) return n.title;
    if (n.stem) return n.stem;
    var id = String(n.id);
    if (id.indexOf(TAG_PREFIX) === 0) return '#' + id.slice(TAG_PREFIX.length);
    if (id.indexOf(DANGLING_PREFIX) === 0) return id.slice(DANGLING_PREFIX.length);
    return id;
  }

  // noteRef names a note node the way /api/note takes it: vault:path when
  // the node carries both, else its id (id:<ulid> is a note ref too). A tag
  // or dangling node has no ref.
  function noteRef(n) {
    if (nodeKind(n) !== 'note') return '';
    if (n.vault && n.path) return n.vault + ':' + n.path;
    return String(n.id);
  }

  // refIndex maps every ref a note node answers to (vault:path and its id)
  // to the node id, so a ref from a list or a note link finds its node.
  function refIndex(nodes) {
    var idx = {};
    nodes.forEach(function (n) {
      if (nodeKind(n) !== 'note') return;
      idx[n.id] = n.id;
      if (n.vault && n.path) idx[n.vault + ':' + n.path] = n.id;
    });
    return idx;
  }

  // facets lists the values the filters offer, from the note nodes. Tiers
  // keep vault order, statuses keep lifecycle order, the rest sort.
  function facets(nodes) {
    var seen = { vault: {}, tier: {}, tag: {}, status: {} };
    nodes.forEach(function (n) {
      if (nodeKind(n) !== 'note') return;
      if (n.vault) seen.vault[n.vault] = true;
      if (n.tier) seen.tier[n.tier] = true;
      if (n.status) seen.status[n.status] = true;
      (n.tags || []).forEach(function (t) { seen.tag[t] = true; });
    });
    function ordered(set, order) {
      var known = order.filter(function (v) { return set[v]; });
      var rest = Object.keys(set).filter(function (v) { return order.indexOf(v) < 0; }).sort();
      return known.concat(rest);
    }
    return {
      vaults: Object.keys(seen.vault).sort(),
      tiers: ordered(seen.tier, TIERS),
      tags: Object.keys(seen.tag).sort(),
      statuses: ordered(seen.status, STATUSES),
    };
  }

  // ── filters ──

  // matchesFilters says whether a note node passes the vault, tier, tag and
  // status filters. An empty filter value matches everything.
  function matchesFilters(n, f) {
    if (f.vault && n.vault !== f.vault) return false;
    if (f.tier && n.tier !== f.tier) return false;
    if (f.status && n.status !== f.status) return false;
    if (f.tag && (n.tags || []).indexOf(f.tag) < 0) return false;
    return true;
  }

  function isOrphan(n) {
    return !n.degree;
  }

  function isDangling(n) {
    return nodeKind(n) === 'dangling';
  }

  // visibleIds returns the set of node ids to draw. Hiding never moves a
  // node: app.js keeps every node in the sigma graph and hides the rest.
  //   - A note shows when it passes the filters, and, with orphans off, when
  //     it touches at least one edge.
  //   - A dangling target shows with dangling on, when a shown note links to it.
  //   - A tag shows when a shown note carries it.
  // f is {vault, tier, tag, status, orphans, dangling}.
  function visibleIds(nodes, edges, f) {
    var byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });
    var shown = {};
    nodes.forEach(function (n) {
      if (nodeKind(n) !== 'note') return;
      if (!matchesFilters(n, f)) return;
      if (!f.orphans && isOrphan(n)) return;
      shown[n.id] = true;
    });
    edges.forEach(function (e) {
      if (!shown[e.source]) return;
      var t = byId[e.target];
      if (!t) return;
      var kind = nodeKind(t);
      if (kind === 'tag' || (kind === 'dangling' && f.dangling)) shown[t.id] = true;
    });
    return shown;
  }

  // ── adjacency and depth search ──

  // adjacency maps each node id to its neighbours, edges followed in either
  // direction.
  function adjacency(edges) {
    var adj = {};
    function add(a, b) {
      if (!adj[a]) adj[a] = {};
      adj[a][b] = true;
    }
    edges.forEach(function (e) {
      if (e.source === e.target) return;
      add(e.source, e.target);
      add(e.target, e.source);
    });
    return adj;
  }

  function neighbours(adj, id) {
    return Object.keys(adj[id] || {});
  }

  function clampDepth(depth) {
    var d = Math.round(Number(depth));
    if (!isFinite(d) || d < DEPTH_MIN) return DEPTH_MIN;
    return d > DEPTH_MAX ? DEPTH_MAX : d;
  }

  // depthSearch walks out from center up to depth hops and returns each
  // reached id with its distance; center is 0. depth is clamped to 1..5.
  function depthSearch(adj, center, depth) {
    var max = clampDepth(depth);
    var dist = {};
    dist[center] = 0;
    var frontier = [center];
    for (var hop = 1; hop <= max && frontier.length; hop++) {
      var next = [];
      frontier.forEach(function (id) {
        neighbours(adj, id).forEach(function (nb) {
          if (dist[nb] !== undefined) return;
          dist[nb] = hop;
          next.push(nb);
        });
      });
      frontier = next;
    }
    return dist;
  }

  // ── sizes ──

  function maxDegree(nodes) {
    return nodes.reduce(function (m, n) { return n.degree > m ? n.degree : m; }, 0);
  }

  // nodeSize grows with the square root of degree, so hubs stand out without
  // swallowing their neighbours. With byDegree off every node is one size.
  function nodeSize(degree, max, byDegree) {
    var base = 2;
    var spread = 7;
    if (!byDegree) return base + 1;
    if (!max || !degree) return base;
    return base + spread * Math.sqrt(degree / max);
  }

  // ── colours ──

  // addDays returns the YYYY-MM-DD date n days after ymd.
  function addDays(ymd, n) {
    var p = String(ymd).split('-').map(Number);
    var d = new Date(Date.UTC(p[0], p[1] - 1, p[2] + n));
    return d.toISOString().slice(0, 10);
  }

  // staleness buckets a note for the staleness colour: stale (the server put
  // it in the stale queue), due (review_by within DUE_DAYS of today), fresh
  // (a later review_by) or none (no review_by).
  function staleness(n, today) {
    if (n.stale) return 'stale';
    if (!n.review_by) return 'none';
    if (today && n.review_by <= addDays(today, DUE_DAYS)) return 'due';
    return 'fresh';
  }

  function vaultRole(name, vaults) {
    var i = (vaults || []).indexOf(name);
    if (i < 0) return NONE_ROLE;
    return VAULT_ROLES[i % VAULT_ROLES.length];
  }

  // colorRole is the palette role of a node under a colour mode. ctx is
  // {vaults: [names in config order], today: "YYYY-MM-DD"}.
  function colorRole(n, mode, ctx) {
    var kind = nodeKind(n);
    if (kind === 'dangling') return DANGLING_ROLE;
    if (kind === 'tag') return TAG_ROLE;
    ctx = ctx || {};
    switch (mode) {
      case 'vault': return vaultRole(n.vault, ctx.vaults);
      case 'evidence': return EVIDENCE_ROLES[n.evidence] || NONE_ROLE;
      case 'status': return STATUS_ROLES[n.status] || NONE_ROLE;
      case 'staleness': return STALENESS_ROLES[staleness(n, ctx.today)];
      default: return TIER_ROLES[n.tier] || NONE_ROLE;
    }
  }

  function edgeRole(kind) {
    return EDGE_ROLES[kind] || EDGE_ROLES.link;
  }

  // legend lists {label, role} for a colour mode: every value the mode
  // knows, then "none" when some note has no value.
  function legend(mode, nodes, ctx) {
    ctx = ctx || {};
    var notes = nodes.filter(function (n) { return nodeKind(n) === 'note'; });
    function entries(values, roles, field) {
      var out = values.map(function (v) { return { label: v, role: roles[v] }; });
      if (notes.some(function (n) { return !roles[n[field]]; })) out.push({ label: 'none', role: NONE_ROLE });
      return out;
    }
    switch (mode) {
      case 'vault':
        return (ctx.vaults || []).map(function (v) { return { label: v, role: vaultRole(v, ctx.vaults) }; });
      case 'evidence': return entries(EVIDENCE, EVIDENCE_ROLES, 'evidence');
      case 'status': return entries(STATUSES, STATUS_ROLES, 'status');
      case 'staleness':
        return STALENESS.map(function (s) { return { label: STALENESS_LABELS[s], role: STALENESS_ROLES[s] }; });
      default: return entries(TIERS, TIER_ROLES, 'tier');
    }
  }

  // ── layout ──

  // knownPositions collects a position for every node it can: the x, y
  // /api/graph returned, else the position the page last drew the node at.
  function knownPositions(nodes, previous) {
    var out = {};
    previous = previous || {};
    nodes.forEach(function (n) {
      if (typeof n.x === 'number' && typeof n.y === 'number') out[n.id] = [n.x, n.y];
      else if (previous[n.id]) out[n.id] = previous[n.id].slice(0, 2);
    });
    return out;
  }

  function centroid(points) {
    if (!points.length) return [0, 0];
    var sx = 0;
    var sy = 0;
    points.forEach(function (p) { sx += p[0]; sy += p[1]; });
    return [sx / points.length, sy / points.length];
  }

  // anchorsFor gives each new node (fresh, a list of ids) the point it
  // should start near: the centroid of its placed neighbours. A node whose
  // neighbours are all new waits for them, so a new cluster hangs off the
  // note that links it in. What is left starts at the centroid of the
  // known layout.
  function anchorsFor(fresh, adj, known) {
    var placed = {};
    Object.keys(known).forEach(function (id) { placed[id] = known[id]; });
    var anchors = {};
    var todo = fresh.slice();
    var progress = true;
    while (todo.length && progress) {
      progress = false;
      var wait = [];
      var round = {};
      todo.forEach(function (id) {
        var pts = neighbours(adj, id)
          .filter(function (nb) { return placed[nb]; })
          .map(function (nb) { return placed[nb]; });
        if (!pts.length) { wait.push(id); return; }
        round[id] = centroid(pts);
      });
      Object.keys(round).forEach(function (id) {
        anchors[id] = round[id];
        placed[id] = round[id];
        progress = true;
      });
      todo = wait;
    }
    var middle = centroid(Object.keys(known).map(function (id) { return known[id]; }));
    todo.forEach(function (id) { anchors[id] = middle; });
    return anchors;
  }

  // groupByAnchor groups new nodes that share an anchor point, so app.js
  // lays each group out on one small circle around it.
  function groupByAnchor(anchors) {
    var groups = {};
    Object.keys(anchors).forEach(function (id) {
      var a = anchors[id];
      var k = a[0].toFixed(3) + ',' + a[1].toFixed(3);
      if (!groups[k]) groups[k] = { anchor: a, ids: [] };
      groups[k].ids.push(id);
    });
    return Object.keys(groups).sort().map(function (k) { return groups[k]; });
  }

  // spacing is a typical gap between known nodes: the side of the layout's
  // bounding box over the square root of the node count. New nodes start
  // that far from their anchor.
  function spacing(known) {
    var ids = Object.keys(known);
    if (ids.length < 2) return 1;
    var minX = Infinity;
    var minY = Infinity;
    var maxX = -Infinity;
    var maxY = -Infinity;
    ids.forEach(function (id) {
      var p = known[id];
      if (p[0] < minX) minX = p[0];
      if (p[0] > maxX) maxX = p[0];
      if (p[1] < minY) minY = p[1];
      if (p[1] > maxY) maxY = p[1];
    });
    var side = Math.max(maxX - minX, maxY - minY);
    return side > 0 ? side / Math.sqrt(ids.length) : 1;
  }

  // layoutMs is how long ForceAtlas2 runs: nothing when every node kept its
  // position, at least a short settle when a few are new, and a share of
  // the full run, longer for a bigger graph, when more are.
  function layoutMs(total, fresh) {
    if (!fresh || !total) return 0;
    var full = Math.min(1500 + total * 4, 8000);
    var share = Math.min(fresh / total, 1);
    return Math.round(Math.max(800, full * share));
  }

  function round3(v) {
    return Math.round(v * 1000) / 1000;
  }

  // layoutPayload is the POST /api/layout body: the key the graph came with
  // and each node's [x, y], rounded to keep the file small.
  function layoutPayload(key, positions) {
    var out = {};
    Object.keys(positions).forEach(function (id) {
      var p = positions[id];
      if (!isFinite(p[0]) || !isFinite(p[1])) return;
      out[id] = [round3(p[0]), round3(p[1])];
    });
    return { key: key, positions: out };
  }

  // ── note links ──

  // wikilinkLabel is the text a wikilink shows: the alias after "|", else
  // the target, with a heading shown as "note > heading".
  function wikilinkLabel(text) {
    var inner = String(text).replace(/^!?\[\[/, '').replace(/\]\]$/, '');
    var bar = inner.indexOf('|');
    if (bar >= 0) return inner.slice(bar + 1).trim();
    var hash = inner.indexOf('#');
    if (hash >= 0) return (inner.slice(0, hash) + ' > ' + inner.slice(hash + 1)).trim();
    return inner.trim();
  }

  // linkIndex maps a wikilink as written to its /api/note links entry, so
  // the renderer turns "[[x|y]]" into an anchor by matching text. The first
  // entry for a text wins.
  function linkIndex(links) {
    var idx = {};
    (links || []).forEach(function (l) {
      if (!Object.prototype.hasOwnProperty.call(idx, l.text)) idx[l.text] = l;
    });
    return idx;
  }

  window.BrainGraph = {
    TIERS: TIERS,
    STATUSES: STATUSES,
    EVIDENCE: EVIDENCE,
    EDGE_KINDS: EDGE_KINDS,
    COLOR_MODES: COLOR_MODES,
    DEPTH_MIN: DEPTH_MIN,
    DEPTH_MAX: DEPTH_MAX,
    nodeKind: nodeKind,
    nodeLabel: nodeLabel,
    noteRef: noteRef,
    refIndex: refIndex,
    facets: facets,
    matchesFilters: matchesFilters,
    visibleIds: visibleIds,
    isOrphan: isOrphan,
    isDangling: isDangling,
    adjacency: adjacency,
    neighbours: neighbours,
    depthSearch: depthSearch,
    clampDepth: clampDepth,
    maxDegree: maxDegree,
    nodeSize: nodeSize,
    addDays: addDays,
    staleness: staleness,
    colorRole: colorRole,
    edgeRole: edgeRole,
    legend: legend,
    knownPositions: knownPositions,
    anchorsFor: anchorsFor,
    groupByAnchor: groupByAnchor,
    spacing: spacing,
    layoutMs: layoutMs,
    layoutPayload: layoutPayload,
    wikilinkLabel: wikilinkLabel,
    linkIndex: linkIndex,
  };
})();
