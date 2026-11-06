// Pure graph helpers for the brain.this UI (docs/serve.md, Browser globals).
// No DOM access: app.js hands these functions the /api/graph nodes and edges
// and turns their answers into sigma state. node --test runs them against a
// fake window (test/graph.test.mjs).
//
// Colours come back as palette role names ("node-blue", "edge"), never as
// hex. app.js reads the role from the page's CSS variables (shell.html holds
// the node palette and its dark variants), so the graph follows the theme
// without this file knowing about it. Each node role has a border role, the
// same name with "-border".
(function () {
  'use strict';

  // The lists below are JSON on one line each: ui_test.go reads TIERS and
  // STATUSES out of this file and checks them against the Go lists.
  var TIERS = ["inbox", "notes", "outputs", "MOCs"];
  var STATUSES = ["raw", "validated", "promoting", "disputed", "superseded", "retracted", "withdrawn"];
  var EVIDENCE = ["A", "B", "C", "D", "mixed"];
  var EDGE_KINDS = ["link", "tag", "supersedes", "source", "moc"];
  var COLOR_MODES = ["vault", "tier", "evidence", "status", "staleness", "cluster"];
  var STALENESS = ["stale", "due", "fresh", "none"];

  var DEPTH_MIN = 1;
  var DEPTH_MAX = 5;
  // A note whose review_by falls within this many days reads as "due".
  var DUE_DAYS = 30;

  var TAG_PREFIX = 'tag:';
  var DANGLING_PREFIX = 'dangling:';

  // The node palette, in the order a categorical mode hands it out.
  var NODE_ROLES = [
    'node-blue', 'node-orange', 'node-cyan', 'node-green', 'node-purple', 'node-yellow',
    'node-red', 'node-beige', 'node-pink', 'node-rose', 'node-teal', 'node-brown',
  ];
  var TIER_ROLES = { inbox: 'node-yellow', notes: 'node-blue', outputs: 'node-green', MOCs: 'node-purple' };
  var EVIDENCE_ROLES = { A: 'node-green', B: 'node-blue', C: 'node-yellow', D: 'node-red', mixed: 'node-purple' };
  var STATUS_ROLES = {
    raw: 'node-yellow', validated: 'node-green', promoting: 'node-blue', disputed: 'node-orange',
    superseded: 'node-purple', retracted: 'node-red', withdrawn: 'node-grey',
  };
  var STALENESS_ROLES = { stale: 'node-red', due: 'node-orange', fresh: 'node-green', none: 'node-grey' };
  var STALENESS_LABELS = { stale: 'stale', due: 'review due soon', fresh: 'reviewed', none: 'no review date' };
  // Edges are one neutral grey, as their captions name the kind; the two
  // kinds that point at another note's lifecycle keep a hue.
  var EDGE_ROLES = { link: 'edge', tag: 'edge', supersedes: 'node-orange', source: 'node-blue', moc: 'edge' };
  var NONE_ROLE = 'node-grey';
  var DANGLING_ROLE = 'node-beige';
  var TAG_ROLE = 'node-teal';

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

  // DEFAULT_FILTERS is the filter state the page opens with. The global
  // layout the server caches lays out the nodes it shows, so dangling
  // targets, hidden by default, are not part of it.
  var DEFAULT_FILTERS = { vault: '', tier: '', tag: '', status: '', orphans: true, dangling: false };

  // activeFilters counts the filters of f set away from DEFAULT_FILTERS,
  // the number the Filters button shows.
  function activeFilters(f) {
    var n = 0;
    Object.keys(DEFAULT_FILTERS).forEach(function (k) {
      if (f && f[k] !== DEFAULT_FILTERS[k]) n++;
    });
    return n;
  }

  // coversAll says whether every id of need (a set) is in have (a set). A
  // simulation lays out the whole global layout only when its nodes cover
  // the ones DEFAULT_FILTERS shows.
  function coversAll(have, need) {
    return Object.keys(need).every(function (id) { return !!have[id]; });
  }

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

  // Node radii and caption sizes are graph units: app.js draws one unit as
  // one pixel at camera ratio 1, so a caption grows with its circle as the
  // camera zooms in, the way Neo4j Browser draws its nodes.
  var RADIUS_FLAT = 25;
  var RADIUS_MIN = 16;
  var RADIUS_MAX = 40;
  var RADIUS_DANGLING = 12;

  // nodeRadius grows with the square root of degree, so hubs stand out
  // without swallowing their neighbours. With byDegree off every note is
  // RADIUS_FLAT; a dangling target is always small.
  function nodeRadius(n, max, byDegree) {
    if (nodeKind(n) === 'dangling') return RADIUS_DANGLING;
    if (!byDegree) return RADIUS_FLAT;
    if (!max || !n.degree) return RADIUS_MIN;
    return RADIUS_MIN + (RADIUS_MAX - RADIUS_MIN) * Math.sqrt(n.degree / max);
  }

  // captionSize is the font size of the caption inside a circle of radius.
  function captionSize(radius) {
    return Math.max(6, Math.min(13, radius * 0.34));
  }

  // CHAR_EM is an average sans-serif glyph width in ems; captions are
  // measured with it, since this file has no canvas to measure text on.
  var CHAR_EM = 0.52;
  // A caption may span this share of the circle's diameter.
  var CAPTION_FILL = 0.88;

  // caption is the text drawn inside a node: the label, cut with an
  // ellipsis to the width the circle leaves at the caption's font size.
  function caption(text, radius, fontSize) {
    var s = String(text || '').replace(/\s+/g, ' ').trim();
    var fit = Math.floor((2 * radius * CAPTION_FILL) / (fontSize * CHAR_EM));
    if (s.length <= fit) return s;
    if (fit < 2) return '';
    return s.slice(0, fit - 1).replace(/[\s.,;:-]+$/, '') + '\u2026';
  }

  // captionShown says whether a caption is drawn: once its circle of
  // radius graph units is at least minPx pixels at camera ratio.
  function captionShown(radius, ratio, minPx) {
    return radius / ratio >= minPx;
  }

  // captionsFlip says whether zooming from ratio r0 to r1 shows or hides
  // the caption of a node of any of radii.
  function captionsFlip(radii, minPx, r0, r1) {
    if (r0 === r1) return false;
    return radii.some(function (r) { return captionShown(r, r0, minPx) !== captionShown(r, r1, minPx); });
  }

  // radiiOf lists the distinct radii nodeRadius gives nodes, sorted.
  function radiiOf(nodes, max, byDegree) {
    var seen = {};
    nodes.forEach(function (n) { seen[nodeRadius(n, max, byDegree)] = true; });
    return Object.keys(seen).map(Number).sort(function (a, b) { return a - b; });
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
    return NODE_ROLES[i % NODE_ROLES.length];
  }

  // borderRole is the darker ring drawn around a node of fill role.
  function borderRole(role) {
    return role + '-border';
  }

  // luminance is the WCAG relative luminance of a #rrggbb colour, 0 to 1.
  function luminance(hex) {
    var m = /^#?([0-9a-f]{6})$/i.exec(String(hex).trim());
    if (!m) return 0;
    var v = parseInt(m[1], 16);
    function lin(c) {
      c /= 255;
      return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
    }
    return 0.2126 * lin((v >> 16) & 255) + 0.7152 * lin((v >> 8) & 255) + 0.0722 * lin(v & 255);
  }

  // A caption is white on a fill darker than CAPTION_LUMINANCE and dark
  // grey on a lighter one. The threshold sits where the palette's warm
  // orange still takes white and its cyan already takes dark text.
  var CAPTION_LIGHT = '#FFFFFF';
  var CAPTION_DARK = '#2A2C34';
  var CAPTION_LUMINANCE = 0.46;
  function captionColor(fill) {
    return luminance(fill) > CAPTION_LUMINANCE ? CAPTION_DARK : CAPTION_LIGHT;
  }

  // clusterRole is the role of community c: the node palette in order, so
  // the largest communities differ; LOOSE and no community are grey.
  function clusterRole(c) {
    if (c === undefined || c === null || c === LOOSE) return NONE_ROLE;
    return NODE_ROLES[c % NODE_ROLES.length];
  }

  // colorRole is the palette role of a node under a colour mode. ctx is
  // {vaults: [names in config order], today: "YYYY-MM-DD", community: {id:
  // community number}}.
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
      case 'cluster': return clusterRole(ctx.community && ctx.community[n.id]);
      default: return TIER_ROLES[n.tier] || NONE_ROLE;
    }
  }

  function edgeRole(kind) {
    return EDGE_ROLES[kind] || EDGE_ROLES.link;
  }

  // legend lists {label, role} for a colour mode: every value the mode
  // knows, then "none" when some note has no value. Tag nodes and dangling
  // targets keep one colour in every mode; ctx.topics and ctx.dangling (the
  // toggles that show them) add those entries when the graph holds any.
  function legend(mode, nodes, ctx) {
    ctx = ctx || {};
    var out = modeLegend(mode, nodes, ctx);
    function has(kind) { return nodes.some(function (n) { return nodeKind(n) === kind; }); }
    if (ctx.topics && has('tag')) out.push({ label: 'tag', role: TAG_ROLE });
    if (ctx.dangling && has('dangling')) out.push({ label: 'dangling', role: DANGLING_ROLE });
    return out;
  }

  function modeLegend(mode, nodes, ctx) {
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
      case 'cluster': return clusterLegend(notes, ctx.community || {});
      default: return entries(TIERS, TIER_ROLES, 'tier');
    }
  }

  // clusterLegend names each community after its note with the most links,
  // in community order, one entry per palette colour; the rest are counted
  // in a last entry with no swatch. "unlinked" is the grey of LOOSE notes.
  var LEGEND_LABEL_MAX = 24;
  function clusterLegend(notes, community) {
    var hub = {};
    var loose = false;
    notes.forEach(function (n) {
      var c = community[n.id];
      if (c === undefined) return;
      if (c === LOOSE) { loose = true; return; }
      if (!hub[c] || (n.degree || 0) > (hub[c].degree || 0)) hub[c] = n;
    });
    var keys = Object.keys(hub).map(Number).sort(function (a, b) { return a - b; });
    var out = keys.slice(0, NODE_ROLES.length).map(function (c) {
      var label = nodeLabel(hub[c]);
      if (label.length > LEGEND_LABEL_MAX) label = label.slice(0, LEGEND_LABEL_MAX - 1).trim() + '\u2026';
      return { label: label, role: clusterRole(c) };
    });
    var more = keys.length - out.length;
    if (more > 0) out.push({ label: '+' + more + ' more', role: '' });
    if (loose) out.push({ label: 'unlinked', role: NONE_ROLE });
    return out;
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
  // note that links it in. What is left starts at its home (homes, an id
  // to its island's [x, y, radius]) when it has one, else at the centroid
  // of the known layout.
  function anchorsFor(fresh, adj, known, homes) {
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
    todo.forEach(function (id) {
      var h = homes && homes[id];
      anchors[id] = h ? [h[0], h[1]] : middle;
    });
    return anchors;
  }

  // groupByAnchor groups new nodes that share an anchor point, so each
  // group starts on one small spiral around it.
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

  // spiral lays count points on a sunflower spiral around the origin, about
  // step apart: the start d3-force gives a node with no position, scaled to
  // circles of RADIUS_FLAT. skip leaves out the innermost points, so a ring
  // around an anchor node does not start on top of it.
  var GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));
  var PLACE_STEP = 36;
  function spiral(count, step, skip) {
    var out = [];
    for (var i = 0; i < count; i++) {
      var k = i + (skip || 0);
      var r = step * Math.sqrt(0.5 + k);
      out.push([r * Math.cos(k * GOLDEN_ANGLE), r * Math.sin(k * GOLDEN_ANGLE)]);
    }
    return out;
  }

  // placeFresh gives every id in fresh a start point (docs/serve.md,
  // Layout, step 2) and returns it with the known positions. With nothing
  // known and no homes the whole graph starts on one spiral; otherwise
  // each group of new nodes spirals out from the neighbours it links to,
  // or from its island's centre (homes, as anchorsFor takes them).
  function placeFresh(fresh, adj, known, homes) {
    var out = {};
    Object.keys(known).forEach(function (id) { out[id] = known[id]; });
    if (!fresh.length) return out;
    var none = !Object.keys(known).length;
    if (none && !homes) {
      spiral(fresh.length, PLACE_STEP).forEach(function (p, i) { out[fresh[i]] = p; });
      return out;
    }
    groupByAnchor(anchorsFor(fresh, adj, known, homes)).forEach(function (grp) {
      spiral(grp.ids.length, PLACE_STEP, none ? 0 : 1).forEach(function (p, i) {
        out[grp.ids[i]] = [grp.anchor[0] + p[0], grp.anchor[1] + p[1]];
      });
    });
    return out;
  }

  // ── physics ──

  // simParams are the d3-force settings. The proportions follow Neo4j
  // Browser (link length measured border to border, a charge that keeps
  // captions apart), the feel follows Obsidian (a low velocity decay, so a
  // dragged node pulls its neighbours along and the graph swings back when
  // it is let go), and the shape follows ForceAtlas2: notes that link to
  // each other gather in groups with space between the groups.
  //   - charge repels every pair up to chargeDistanceMax, far enough that
  //     whole groups push each other apart.
  //   - linkGap is the rest length of a link between the two borders. It
  //     is short, so a link holds its ends close and a group stays tight;
  //     the charge still spreads the group to about Neo4j's spacing.
  //   - linkBase and linkPerShared set how hard a link pulls
  //     (linkStrengths).
  //   - collidePad keeps that much space around each circle.
  //   - gravity pulls every node toward the origin on x and y, which brings
  //     unlinked notes back in. It is weak: a strong pull packs the groups
  //     into one even disc.
  //   - alphaDecay cools the graph from alpha 1 to alphaMin in coolTicks.
  //   - warmTicks and warmMs bound the synchronous run before the first
  //     draw, so the page opens on a settled graph.
  var SIM = {
    charge: -200,
    chargeDistanceMax: 2500,
    linkGap: 20,
    linkBase: 0.5,
    linkPerShared: 1,
    collidePad: 10,
    collideIterations: 2,
    gravity: 0.01,
    velocityDecay: 0.3,
    alphaMin: 0.02,
    coolTicks: 220,
    dragAlphaTarget: 0.3,
    reheatAlpha: 0.3,
    warmTicks: 300,
    warmMs: 250,
  };
  // ISLANDS replaces some of SIM in the global graph, where every node
  // has a home island (islandHomes) instead of the gravity:
  //   - islandPull draws each node to its island's centre; against the
  //     charge it spreads the members evenly over the disc.
  //   - islandWall pulls a node that sits beyond its island's edge back,
  //     by that much per unit outside, so no node drifts into a gap.
  //   - islandCell is the area one note takes on its island, which sets
  //     the island's radius (islandRadius).
  //   - islandGap is the empty space between two islands, as a share of
  //     their mean radius, and at least islandGapMin.
  //   - islandJoin: circles closer than this sit on one island when a
  //     reload reads the islands out of a saved layout (islandsFromLayout).
  //   - crossLink scales the pull of a link between two communities, so
  //     it does not drag notes out into the gap.
  //   - chargeDistanceMax keeps the charge within about one island.
  //   - collidePad and linkGap are wider than in a local graph: a global
  //     graph is seen zoomed out, where circles need more room between
  //     them to read as apart.
  var ISLANDS = {
    islandPull: 0.06,
    islandWall: 0.3,
    islandCell: 9000,
    islandGap: 0.5,
    islandGapMin: 120,
    islandJoin: 100,
    crossLink: 0.02,
    chargeDistanceMax: 900,
    collidePad: 20,
    linkGap: 60,
    gravity: 0,
  };

  // simParams are the settings for one simulation: SIM, with ISLANDS on
  // top for the global graph (islands true).
  function simParams(islands) {
    var p = {};
    Object.keys(SIM).forEach(function (k) { p[k] = SIM[k]; });
    if (islands) Object.keys(ISLANDS).forEach(function (k) { p[k] = ISLANDS[k]; });
    p.alphaDecay = 1 - Math.pow(p.alphaMin, 1 / p.coolTicks);
    return p;
  }

  // startAlpha is how hot a fresh simulation starts: cold (0, nothing runs)
  // when the cached layout covers every node, a gentle settle when the
  // positions are an older key's, and hotter the larger the share of new
  // nodes, up to a full run when nothing was placed before.
  function startAlpha(total, fresh, cached) {
    if (!total) return 0;
    if (!fresh) return cached ? 0 : 0.1;
    return Math.min(1, 0.1 + 0.9 * Math.sqrt(fresh / total));
  }

  // simLinks lists one undirected link per linked pair of shown ids, so
  // two edges between the same notes (a link and a moc edge) pull no harder
  // than one. edges carry source and target ids.
  function simLinks(edges, shown) {
    var seen = {};
    var out = [];
    edges.forEach(function (e) {
      var s = e.source;
      var t = e.target;
      if (s === t || !shown[s] || !shown[t]) return;
      var k = s < t ? s + '\u0000' + t : t + '\u0000' + s;
      if (seen[k]) return;
      seen[k] = true;
      out.push({ source: s, target: t });
    });
    return out;
  }

  // linkStrengths is how hard each of links (simLinks, ids not yet
  // resolved) pulls, in the order given. d3-force's default is 1 over the
  // link count of the end with fewer links; this scales it by P.linkBase,
  // plus P.linkPerShared for each neighbour the two ends share. A
  // link inside a group of notes that link among themselves shares
  // neighbours and pulls hard; one across groups shares none and pulls
  // weakly, so the groups come apart. Never above 1, where d3 overshoots.
  // With community (an id to its community), a link between two
  // communities pulls P.crossLink times as hard.
  function linkStrengths(links, P, community) {
    var nb = {};
    function add(a, b) {
      if (!nb[a]) nb[a] = {};
      nb[a][b] = true;
    }
    links.forEach(function (l) {
      add(l.source, l.target);
      add(l.target, l.source);
    });
    var count = {};
    Object.keys(nb).forEach(function (id) { count[id] = Object.keys(nb[id]).length; });
    return links.map(function (l) {
      var s = l.source;
      var t = l.target;
      var few = count[s] <= count[t] ? s : t;
      var many = nb[few === s ? t : s];
      var shared = 0;
      Object.keys(nb[few]).forEach(function (x) { if (many[x]) shared++; });
      var w = P.linkBase + P.linkPerShared * shared;
      var pull = Math.min(1, w / Math.min(count[s], count[t]));
      if (community && community[s] !== community[t]) pull *= P.crossLink;
      return pull;
    });
  }

  // ── communities and islands ──
  // The global graph groups its notes by community: Louvain splits the
  // notes the default filters show into groups that link more inside than
  // out, each group gets an island (a disc sized to hold it), and a force
  // holds every node on its island (docs/serve.md, Layout).

  // LOOSE is the community of a node with no link: the unlinked notes
  // share one island instead of each floating alone.
  var LOOSE = -1;
  // COMMUNITY_SEED seeds Louvain's random walk, so the same graph splits
  // the same way on every load.
  var COMMUNITY_SEED = 20261106;

  // seededRandom returns a deterministic generator of numbers in [0, 1)
  // (mulberry32), the rng app.js hands Louvain.
  function seededRandom(seed) {
    var a = seed >>> 0;
    return function () {
      a = (a + 0x6D2B79F5) >>> 0;
      var t = Math.imul(a ^ (a >>> 15), a | 1);
      t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  // numberBySize renumbers raw community labels from base up, the largest
  // community first; a tie goes to the one whose first member comes first
  // in ids.
  function numberBySize(raw, ids, base) {
    var size = {};
    var first = {};
    ids.forEach(function (id, i) {
      var c = raw[id];
      if (c === undefined) return;
      size[c] = (size[c] || 0) + 1;
      if (first[c] === undefined) first[c] = i;
    });
    var order = Object.keys(size).sort(function (a, b) { return size[b] - size[a] || first[a] - first[b]; });
    var num = {};
    order.forEach(function (c, i) { num[c] = base + i; });
    var out = {};
    ids.forEach(function (id) { if (raw[id] !== undefined) out[id] = num[raw[id]]; });
    return out;
  }

  // joinCommunities gives each id of fresh the community most of its
  // assigned neighbours have (assigned, a map it adds to), the smaller
  // number on a tie. A node whose neighbours are all new waits for them,
  // as anchorsFor does; it returns the ids no neighbour placed.
  function joinCommunities(fresh, adj, assigned) {
    var todo = fresh.slice();
    var progress = true;
    while (todo.length && progress) {
      progress = false;
      var wait = [];
      var round = {};
      todo.forEach(function (id) {
        var votes = {};
        neighbours(adj, id).forEach(function (nb) {
          var c = assigned[nb];
          if (c !== undefined && c !== LOOSE) votes[c] = (votes[c] || 0) + 1;
        });
        var best = null;
        Object.keys(votes).forEach(function (c) {
          var n = Number(c);
          if (best === null || votes[c] > votes[best] || (votes[c] === votes[best] && n < best)) best = n;
        });
        if (best === null) wait.push(id);
        else round[id] = best;
      });
      Object.keys(round).forEach(function (id) {
        assigned[id] = round[id];
        progress = true;
      });
      todo = wait;
    }
    return todo;
  }

  // communities numbers the community of each of ids (the notes the global
  // layout groups) over links (simLinks among them). detect(ids, links)
  // is the community finder (Louvain in app.js), answering any label per
  // id. Without previous it splits the whole graph, numbered by size from
  // 0. With previous (the last load's answer) every id it holds keeps its
  // community, so a poll moves nothing: a new note joins its neighbours'
  // community, and new notes linked only among themselves form new ones
  // numbered after the rest. A node with no link is LOOSE.
  function communities(ids, links, detect, previous) {
    var adj = adjacency(links);
    var out = {};
    var linked = ids.filter(function (id) { return neighbours(adj, id).length; });
    ids.forEach(function (id) { if (!adj[id]) out[id] = LOOSE; });
    var fresh = linked;
    if (previous) {
      fresh = linked.filter(function (id) {
        var c = previous[id];
        if (c === undefined || c === LOOSE) return true;
        out[id] = c;
        return false;
      });
    }
    if (!fresh.length) return out;
    var left = fresh.length === linked.length ? fresh : joinCommunities(fresh, adj, out);
    if (!left.length) return out;
    var inLeft = {};
    left.forEach(function (id) { inLeft[id] = true; });
    var among = links.filter(function (l) { return inLeft[l.source] && inLeft[l.target]; });
    var base = 0;
    Object.keys(out).forEach(function (id) { if (out[id] >= base) base = out[id] + 1; });
    var found = numberBySize(detect(left, among) || {}, left, base);
    left.forEach(function (id) { out[id] = found[id] === undefined ? LOOSE : found[id]; });
    return out;
  }

  // islandsFromLayout reads the islands back out of a laid out graph, so a
  // reload keeps the groups the layout shows even when the graph changed
  // and Louvain would split it another way. known maps ids to positions,
  // radiusOf(id) is a node's circle radius. Two nodes whose circles are
  // less than P.islandJoin apart sit on one island: inside an island the
  // circles are about 2 * collidePad apart, between islands at least
  // islandGapMin. Islands are numbered by size from 0; a node alone keeps
  // no number, so it joins its neighbours' community (communities).
  function islandsFromLayout(known, radiusOf, P) {
    var ids = Object.keys(known);
    var maxR = 0;
    ids.forEach(function (id) { maxR = Math.max(maxR, radiusOf(id)); });
    var cell = 2 * maxR + P.islandJoin;
    var grid = {};
    function key(cx, cy) { return cx + ',' + cy; }
    ids.forEach(function (id) {
      var k = key(Math.floor(known[id][0] / cell), Math.floor(known[id][1] / cell));
      (grid[k] = grid[k] || []).push(id);
    });
    var parent = {};
    ids.forEach(function (id) { parent[id] = id; });
    function root(id) {
      while (parent[id] !== id) {
        parent[id] = parent[parent[id]];
        id = parent[id];
      }
      return id;
    }
    ids.forEach(function (a) {
      var pa = known[a];
      var cx = Math.floor(pa[0] / cell);
      var cy = Math.floor(pa[1] / cell);
      for (var dx = -1; dx <= 1; dx++) {
        for (var dy = -1; dy <= 1; dy++) {
          (grid[key(cx + dx, cy + dy)] || []).forEach(function (b) {
            if (b <= a) return;
            var pb = known[b];
            var gap = Math.hypot(pa[0] - pb[0], pa[1] - pb[1]) - radiusOf(a) - radiusOf(b);
            if (gap < P.islandJoin) parent[root(a)] = root(b);
          });
        }
      }
    });
    var raw = {};
    var size = {};
    ids.forEach(function (id) {
      var r = root(id);
      raw[id] = r;
      size[r] = (size[r] || 0) + 1;
    });
    var grouped = ids.filter(function (id) { return size[raw[id]] > 1; });
    return numberBySize(raw, grouped, 0);
  }

  // islandRadius is the radius of the disc a community fills: each member
  // takes the area P.islandCell, or its own circle and padding when that is
  // larger. radii are the members' circle radii.
  function islandRadius(radii, P) {
    var area = 0;
    radii.forEach(function (r) {
      var own = Math.PI * Math.pow(r + P.collidePad, 2);
      area += Math.max(P.islandCell, own);
    });
    return Math.sqrt(area / Math.PI);
  }

  // islandGap is the empty space kept between two islands of radii a and b.
  function islandGap(a, b, P) {
    return Math.max(P.islandGapMin, P.islandGap * (a + b) / 2);
  }

  // islandAnchors gives each island its centre. islands are [{key, r,
  // ids}]; known maps an id to the position it already has. An island with
  // placed members starts at their centroid and stays there unless it
  // overlaps another; the rest are packed around them, the largest
  // nearest the middle, each kept islandGap clear of every other. The
  // answer depends only on its input, so a reload packs the same way.
  var ISLAND_ROUNDS = 400;
  var ISLAND_PULL = 0.05;
  function islandAnchors(islands, known, P) {
    var discs = islands.map(function (isl) {
      var pts = isl.ids.filter(function (id) { return known[id]; }).map(function (id) { return known[id]; });
      var c = pts.length ? centroid(pts) : null;
      return { key: isl.key, r: isl.r, fixed: !!c, x: c ? c[0] : 0, y: c ? c[1] : 0 };
    });
    var fixed = discs.filter(function (d) { return d.fixed; });
    var middle = fixed.length ? centroid(fixed.map(function (d) { return [d.x, d.y]; })) : [0, 0];
    var loose = discs.filter(function (d) { return !d.fixed; }).sort(function (a, b) { return b.r - a.r || (a.key < b.key ? -1 : 1); });
    // The first free island starts in the middle when nothing is placed,
    // the others on a sunflower spiral that grows with the area so far.
    var area = fixed.reduce(function (s, d) { return s + Math.pow(d.r + islandGap(d.r, d.r, P), 2); }, 0);
    loose.forEach(function (d, i) {
      var rho = Math.sqrt(area) + (area ? d.r : 0);
      d.x = middle[0] + rho * Math.cos(i * GOLDEN_ANGLE);
      d.y = middle[1] + rho * Math.sin(i * GOLDEN_ANGLE);
      area += Math.pow(d.r + islandGap(d.r, d.r, P), 2);
    });
    for (var round = 0; round < ISLAND_ROUNDS; round++) {
      var pull = round < ISLAND_ROUNDS / 2 ? ISLAND_PULL : 0;
      loose.forEach(function (d) {
        d.x += (middle[0] - d.x) * pull;
        d.y += (middle[1] - d.y) * pull;
      });
      if (!separate(discs, P) && !pull) break;
    }
    var out = {};
    discs.forEach(function (d) { out[d.key] = [d.x, d.y]; });
    return out;
  }

  // separate pushes apart every pair of discs closer than their radii and
  // islandGap allow, and reports whether any pair was. A free disc gives
  // way to a placed one; two of a kind share the move by area.
  function separate(discs, P) {
    var moved = false;
    for (var i = 0; i < discs.length; i++) {
      for (var j = i + 1; j < discs.length; j++) {
        var a = discs[i];
        var b = discs[j];
        var want = a.r + b.r + islandGap(a.r, b.r, P);
        var dx = b.x - a.x;
        var dy = b.y - a.y;
        var d = Math.sqrt(dx * dx + dy * dy);
        if (d >= want) continue;
        moved = true;
        if (d < 1e-6) {
          // Two discs on one point part along a fixed direction.
          dx = Math.cos((i + j) * GOLDEN_ANGLE);
          dy = Math.sin((i + j) * GOLDEN_ANGLE);
          d = 1;
        }
        var push = (want - d) / d;
        var wa = a.r * a.r;
        var wb = b.r * b.r;
        var shareA = a.fixed === b.fixed ? wb / (wa + wb) : a.fixed ? 0 : 1;
        a.x -= dx * push * shareA;
        a.y -= dy * push * shareA;
        b.x += dx * push * (1 - shareA);
        b.y += dy * push * (1 - shareA);
      }
    }
    return moved;
  }

  // islandHomes lays out the islands and gives each id of community its
  // home, [x, y, radius] of its island. radiusOf(id) is a node's circle
  // radius; known maps ids to the positions they already have. With
  // counted (a set of ids), only those size and place an island, so nodes
  // the page hides by default (dangling targets) share a home without
  // making their island bigger.
  function islandHomes(community, radiusOf, known, P, counted) {
    var byKey = {};
    Object.keys(community).forEach(function (id) {
      var c = community[id];
      if (!byKey[c]) byKey[c] = { key: c, ids: [], all: [], radii: [] };
      byKey[c].all.push(id);
      if (counted && !counted[id]) return;
      byKey[c].ids.push(id);
      byKey[c].radii.push(radiusOf(id));
    });
    var islands = Object.keys(byKey).map(function (k) {
      var isl = byKey[k];
      return { key: isl.key, ids: isl.ids, r: islandRadius(isl.radii, P) };
    });
    var at = islandAnchors(islands, known || {}, P);
    var homes = {};
    islands.forEach(function (isl) {
      byKey[isl.key].all.forEach(function (id) { homes[id] = [at[isl.key][0], at[isl.key][1], isl.r]; });
    });
    return homes;
  }

  // islandForce is a d3-force force that pulls each node with a home
  // ([x, y, radius]) toward its island's centre by P.islandPull, and back
  // inside the island by P.islandWall for every unit it sits beyond the
  // edge. Charge spreads the members over the disc against the pull.
  function islandForce(P) {
    var nodes = [];
    function force(alpha) {
      for (var i = 0; i < nodes.length; i++) {
        var d = nodes[i];
        var h = d.home;
        if (!h) continue;
        var dx = h[0] - d.x;
        var dy = h[1] - d.y;
        var dist = Math.sqrt(dx * dx + dy * dy);
        var k = P.islandPull;
        if (dist > h[2]) k += P.islandWall * (dist - h[2]) / dist;
        d.vx += dx * k * alpha;
        d.vy += dy * k * alpha;
      }
    }
    force.initialize = function (n) { nodes = n; };
    return force;
  }

  // ── camera ──

  // boundsOf is the box around circles [{x, y, r}], or null for none.
  function boundsOf(circles) {
    if (!circles.length) return null;
    var b = { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity };
    circles.forEach(function (c) {
      var r = c.r || 0;
      if (c.x - r < b.minX) b.minX = c.x - r;
      if (c.y - r < b.minY) b.minY = c.y - r;
      if (c.x + r > b.maxX) b.maxX = c.x + r;
      if (c.y + r > b.maxY) b.maxY = c.y + r;
    });
    return b;
  }

  // fitRatio is the camera ratio that shows bounds with margin to spare on
  // each side, given how many graph units the viewport spans at ratio 1.
  function fitRatio(bounds, span, margin, min, max) {
    var w = bounds.maxX - bounds.minX;
    var h = bounds.maxY - bounds.minY;
    var m = 1 + 2 * (margin || 0);
    var r = Math.max(w / span.width, h / span.height) * m;
    if (!isFinite(r) || r <= 0) r = 1;
    return Math.min(max, Math.max(min, r));
  }

  // ── hover tween ──

  // easeOut is a cubic ease-out over t in 0..1.
  function easeOut(t) {
    var u = 1 - Math.min(1, Math.max(0, t));
    return 1 - u * u * u;
  }

  // tweenValue is a tween's value at now: from at start, to after ms.
  function tweenValue(from, to, start, now, ms) {
    if (ms <= 0 || now >= start + ms) return to;
    return from + (to - from) * easeOut((now - start) / ms);
  }

  // dimOpacity is the opacity of a node outside the hover's focus while
  // the focus fades in by t (0 none, 1 full).
  function dimOpacity(t, dim) {
    return 1 - (1 - dim) * t;
  }

  // ── search ──

  // searchHits maps an /api/search response to the node ids it names, in
  // the order the server ranked them, each once. A literal search answers
  // files, a hybrid one hits; lines are an older literal shape. refs is
  // refIndex's map.
  function searchHits(res, refs) {
    var rows = (res && (res.hits || res.lines || res.files)) || [];
    var seen = {};
    var out = [];
    rows.forEach(function (r) {
      var id = refs[r.vault + ':' + r.path] || (r.id && refs[r.id]);
      if (!id || seen[id]) return;
      seen[id] = true;
      out.push(id);
    });
    return out;
  }

  // restrictTo keeps the ids of shown that are also in only (a list), or
  // shown itself when only is null.
  function restrictTo(shown, only) {
    if (!only) return shown;
    var out = {};
    only.forEach(function (id) { if (shown[id]) out[id] = true; });
    return out;
  }

  // countNotes counts the note nodes among ids (a set of node ids), so the
  // status line never counts a tag or a dangling target as a note.
  function countNotes(nodes, ids) {
    var n = 0;
    nodes.forEach(function (node) { if (ids[node.id] && nodeKind(node) === 'note') n++; });
    return n;
  }

  // reviewCount is the number on the Review button: the items someone has
  // to act on, each queue's actionable count (promotions, stale notes and
  // check errors). Check warnings show in the Check tab and do not count.
  // A queue still loading or failed counts nothing.
  function reviewCount(queues) {
    var n = 0;
    ['promotions', 'stale', 'check'].forEach(function (k) {
      var q = queues && queues[k];
      if (q && !q.error && typeof q.actionable === 'number') n += q.actionable;
    });
    return n;
  }

  // edgeCaption is the text drawn along an edge: its kind, in capitals as
  // Neo4j Browser draws relationship types.
  function edgeCaption(kind) {
    return String(kind || 'link').toUpperCase();
  }

  // wheelRatio is the camera ratio after one wheel event. It scales by the
  // event's own delta, so a trackpad's stream of small events zooms smoothly
  // and a mouse notch (about 100 px) takes one step of about 1.2x. A pinch
  // arrives as ctrl+wheel with small deltas, so it weighs more per pixel.
  // Line-mode deltas, which some mice send, count as LINE_PX pixels each.
  var WHEEL_ZOOM_PER_PX = 0.002;
  var PINCH_ZOOM_PER_PX = 0.01;
  var LINE_PX = 16;
  function wheelRatio(ratio, deltaY, deltaMode, pinch, min, max) {
    var px = deltaMode === 1 ? deltaY * LINE_PX : deltaY;
    var next = ratio * Math.exp(px * (pinch ? PINCH_ZOOM_PER_PX : WHEEL_ZOOM_PER_PX));
    return Math.min(max, Math.max(min, next));
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
    NODE_ROLES: NODE_ROLES,
    DEPTH_MIN: DEPTH_MIN,
    DEPTH_MAX: DEPTH_MAX,
    nodeKind: nodeKind,
    nodeLabel: nodeLabel,
    noteRef: noteRef,
    refIndex: refIndex,
    facets: facets,
    DEFAULT_FILTERS: DEFAULT_FILTERS,
    activeFilters: activeFilters,
    coversAll: coversAll,
    matchesFilters: matchesFilters,
    visibleIds: visibleIds,
    isOrphan: isOrphan,
    isDangling: isDangling,
    adjacency: adjacency,
    neighbours: neighbours,
    depthSearch: depthSearch,
    clampDepth: clampDepth,
    maxDegree: maxDegree,
    nodeRadius: nodeRadius,
    captionSize: captionSize,
    caption: caption,
    captionShown: captionShown,
    captionsFlip: captionsFlip,
    radiiOf: radiiOf,
    addDays: addDays,
    staleness: staleness,
    colorRole: colorRole,
    clusterRole: clusterRole,
    borderRole: borderRole,
    luminance: luminance,
    captionColor: captionColor,
    edgeRole: edgeRole,
    edgeCaption: edgeCaption,
    legend: legend,
    knownPositions: knownPositions,
    anchorsFor: anchorsFor,
    groupByAnchor: groupByAnchor,
    spiral: spiral,
    placeFresh: placeFresh,
    LOOSE: LOOSE,
    COMMUNITY_SEED: COMMUNITY_SEED,
    seededRandom: seededRandom,
    joinCommunities: joinCommunities,
    communities: communities,
    islandsFromLayout: islandsFromLayout,
    islandRadius: islandRadius,
    islandGap: islandGap,
    islandAnchors: islandAnchors,
    islandHomes: islandHomes,
    islandForce: islandForce,
    simParams: simParams,
    startAlpha: startAlpha,
    simLinks: simLinks,
    linkStrengths: linkStrengths,
    boundsOf: boundsOf,
    fitRatio: fitRatio,
    easeOut: easeOut,
    tweenValue: tweenValue,
    dimOpacity: dimOpacity,
    searchHits: searchHits,
    restrictTo: restrictTo,
    countNotes: countNotes,
    reviewCount: reviewCount,
    wheelRatio: wheelRatio,
    layoutPayload: layoutPayload,
    wikilinkLabel: wikilinkLabel,
    linkIndex: linkIndex,
  };
})();
