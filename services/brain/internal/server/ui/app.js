'use strict';
// The brain.this UI (docs/serve.md). shell.html loads webkit.js, the graph
// bundle (window.BrainSigma) and graph.js (window.BrainGraph) first; this
// IIFE builds the page into #app: a sidebar (vaults, search, tier browsing,
// the queues), the sigma graph with its toolbar, and the note panel.
//
// Every decision about what to show (filters, colours, sizes, depth, where
// a new node starts) is a pure helper in graph.js. This file owns the DOM,
// the fetches, sigma and the ForceAtlas2 runs.
(function () {
  var B = window.BrainSigma;
  var BG = window.BrainGraph;
  var el = Webkit.el;
  var app = document.getElementById('app');
  var toastHost = document.getElementById('toast-host');
  if (!B || !BG) {
    app.textContent = 'The graph bundle did not load, so the brain UI cannot start. Check /assets/brain-graph.bundle.js.';
    return;
  }

  var POLL_MS = 10000;
  var LOCAL_LAYOUT_MS = 1500;
  var SEARCH_DEBOUNCE_MS = 300;
  var LIST_LIMIT = 100;
  var SEARCH_LIMIT = 30;
  var DIM = 0.12;
  // A local graph, or a hovered node's neighbourhood, this small shows every
  // label; a bigger one leaves the label grid to pick.
  var LOCAL_LABELS_MAX = 60;
  // Camera zoom bounds, shared by sigma's settings and the wheel handler.
  var MIN_CAMERA_RATIO = 0.02;
  var MAX_CAMERA_RATIO = 20;
  // A hover from rest draws once the cursor has stayed on a node this long,
  // so a cursor passing over nodes does not dim the graph. Leaving a node
  // clears the hover after HOVER_LINGER_MS, so crossing the gap between two
  // nodes does not un-dim and re-dim it.
  var HOVER_INTENT_MS = 80;
  var HOVER_LINGER_MS = 150;
  // A dropped node stays pinned while the rest settles for this long.
  var DROP_LAYOUT_MS = 1200;

  // ── state ──
  // stateKey is the last /api/state key; a new one refetches everything.
  // global is the graphology graph of every note, fetched with the topics
  // flag globalFlag; view is what sigma shows (global, or local around
  // centerId). prev keeps the last drawn positions per topics flag, so a
  // refetch starts from them.
  var S = {
    stateKey: null,
    vaults: [],
    vaultNames: [],
    vault: '',
    data: null,
    global: null,
    globalFlag: 0,
    adj: {},
    refs: {},
    maxDeg: 0,
    visible: {},
    prev: { 0: {}, 1: {} },
    view: 'global',
    centerId: null,
    localSmall: false,
    depth: 1,
    colorBy: 'tier',
    filters: { tier: '', tag: '', status: '' },
    orphans: true,
    dangling: false,
    byDegree: true,
    topics: false,
    hovered: null,
    hoverTimer: null,
    pointer: null,
    focus: null,
    focusCrowded: false,
    drag: null,
    selected: null,
    noteRef: '',
    noteSeq: 0,
    gen: 0,
    runs: {},
    layoutNote: '',
    tier: 'notes',
    searchMode: 'literal',
    query: '',
    queue: 'promotions',
    queues: { promotions: null, stale: null, check: null },
    sigma: null,
    today: today(),
  };

  function today() {
    var d = new Date();
    function p(n) { return n < 10 ? '0' + n : '' + n; }
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate());
  }

  // ── URL state: ?vault=&color=&note=&view=local&depth=&topics=1 ──
  (function readURL() {
    var p = new URLSearchParams(location.search);
    S.vault = p.get('vault') || '';
    if (BG.COLOR_MODES.indexOf(p.get('color')) >= 0) S.colorBy = p.get('color');
    S.noteRef = p.get('note') || '';
    S.view = p.get('view') === 'local' && S.noteRef ? 'local' : 'global';
    S.depth = BG.clampDepth(p.get('depth') || 1);
    S.topics = p.get('topics') === '1';
  })();

  function syncURL() {
    var p = new URLSearchParams();
    if (S.vault) p.set('vault', S.vault);
    if (S.colorBy !== 'tier') p.set('color', S.colorBy);
    if (S.noteRef) p.set('note', S.noteRef);
    if (S.view === 'local') p.set('view', 'local');
    if (S.depth !== 1) p.set('depth', String(S.depth));
    if (S.topics) p.set('topics', '1');
    var qs = p.toString();
    history.replaceState(null, '', qs ? '?' + qs : location.pathname);
  }

  // ── fetch helpers ──
  function query(params) {
    var p = new URLSearchParams();
    Object.keys(params).forEach(function (k) {
      var v = params[k];
      if (v !== '' && v !== null && v !== undefined && v !== false) p.set(k, String(v));
    });
    var qs = p.toString();
    return qs ? '?' + qs : '';
  }

  // getJSON throws an Error carrying the API's error code and HTTP status.
  async function getJSON(url) {
    var res = await fetch(url, { headers: { accept: 'application/json' } });
    var body = null;
    try { body = await res.json(); } catch (_) { body = null; }
    if (!res.ok) {
      var apiErr = (body && body.error) || {};
      var err = new Error(apiErr.message || 'request failed (' + res.status + ')');
      err.code = apiErr.code || '';
      err.status = res.status;
      throw err;
    }
    return body;
  }

  function toast(msg, variant) {
    var t = document.createElement('wk-toast');
    t.setAttribute('variant', variant || 'ok');
    t.textContent = msg;
    toastHost.appendChild(t);
    setTimeout(function () { t.remove(); }, 3200);
  }

  function errText(err) {
    return (err && err.message) || String(err);
  }

  // ── palette: graph.js speaks in roles, the page's CSS variables hold them ──
  var palette = {};
  function color(role) {
    if (!(role in palette)) {
      var v = getComputedStyle(document.documentElement).getPropertyValue('--' + role).trim();
      palette[role] = v || '#888888';
    }
    return palette[role];
  }
  document.addEventListener('wk-themechange', function () {
    palette = {};
    renderLegend();
    refreshGraph();
  });

  // ── page skeleton ──
  var side = el('aside', { class: 'side' });
  var stage = el('section', { class: 'stage' });
  var note = el('aside', { class: 'note', hidden: '' });
  var toolbar = el('div', { class: 'toolbar' });
  var graphBox = el('div', { id: 'graph' });
  var sigmaBox = el('div', { class: 'sigma' });
  var statusEl = el('div', { class: 'overlay status' });
  var legendEl = el('div', { class: 'overlay legend' });
  var emptyEl = el('div', { class: 'empty', hidden: '' });
  var zoomEl = el('div', { class: 'overlay zoom' }, [
    el('button', { title: 'Zoom in', 'aria-label': 'Zoom in', onclick: function () { camera('in'); } }, '+'),
    el('button', { title: 'Zoom out', 'aria-label': 'Zoom out', onclick: function () { camera('out'); } }, '−'),
    el('button', { title: 'Fit the graph', 'aria-label': 'Fit the graph', onclick: function () { camera('fit'); } }, '▢'),
  ]);
  graphBox.append(sigmaBox, statusEl, legendEl, zoomEl, emptyEl);
  stage.append(toolbar, graphBox);
  app.append(side, stage, note);

  var sideParts = {
    vaults: el('div', { class: 'sec' }),
    search: el('div', { class: 'sec' }),
    browse: el('div', { class: 'sec' }),
    queues: el('div', { class: 'sec' }),
  };
  side.append(sideParts.vaults, sideParts.search, sideParts.browse, sideParts.queues);

  function setStatus(text) {
    statusEl.textContent = text;
  }

  function showEmpty(content) {
    emptyEl.innerHTML = '';
    if (!content) { emptyEl.setAttribute('hidden', ''); return; }
    emptyEl.appendChild(typeof content === 'string' ? el('div', {}, content) : content);
    emptyEl.removeAttribute('hidden');
  }

  function seg(options, active, onPick) {
    return el('wk-seg', {}, options.map(function (o) {
      return el('button', {
        class: o.value === active ? 'active' : '',
        type: 'button',
        onclick: function () { onPick(o.value); },
      }, o.label);
    }));
  }

  function badge(text, variant) {
    return el('wk-badge', { variant: variant || 'outline' }, text);
  }

  function statusBadge(status) {
    if (!status) return null;
    var v = { validated: 'ok', disputed: 'warn', retracted: 'error', superseded: 'outline', raw: 'c', promoting: 'info', withdrawn: 'outline' };
    return badge(status, v[status]);
  }

  function noteRow(opts) {
    var kids = [el('span', { class: 't' }, opts.title)];
    if (opts.meta) kids.push(el('span', { class: 'm' }, opts.meta));
    var top = el('span', { style: 'display:flex;gap:0.4rem;align-items:baseline;min-width:0' }, kids);
    var lines = [top];
    if (opts.sub) lines.push(el('span', { class: 'sub' }, opts.sub));
    if (opts.snip) lines.push(el('span', { class: 'snip' }, opts.snip));
    return el('button', {
      class: 'row multi' + (opts.ref && opts.ref === S.noteRef ? ' active' : ''),
      type: 'button',
      title: opts.ref || '',
      onclick: function () { if (opts.ref) openNote(opts.ref); },
    }, lines);
  }

  function basename(path) {
    var s = String(path || '');
    return s.slice(s.lastIndexOf('/') + 1).replace(/\.md$/, '');
  }

  // ── graph: build, reducers, sigma ──

  function ctx() {
    return { vaults: S.vaultNames, today: S.today };
  }

  function nodeOf(id) {
    var g = S.global;
    return g && g.hasNode(id) ? g.getNodeAttribute(id, 'n') : null;
  }

  function buildGraph(data, positions) {
    var g = new B.Graph({ multi: true, type: 'directed', allowSelfLoops: false });
    data.nodes.forEach(function (n) {
      var p = positions[n.id] || [0, 0];
      g.addNode(n.id, { x: p[0], y: p[1], size: 3, label: BG.nodeLabel(n), n: n });
    });
    data.edges.forEach(function (e) {
      if (e.source === e.target || !g.hasNode(e.source) || !g.hasNode(e.target)) return;
      var key = e.source + '>' + e.target + '#' + e.kind;
      if (g.hasEdge(key)) return;
      g.addDirectedEdgeWithKey(key, e.source, e.target, { kind: e.kind, resolved: e.resolved, s: e.source, t: e.target });
    });
    return g;
  }

  function nodeReducer(key, data, attrs) {
    var n = attrs.n;
    var out = {
      color: color(BG.colorRole(n, S.colorBy, ctx())),
      size: BG.nodeSize(n.degree, S.maxDeg, S.byDegree),
      labelColor: color('text-1'),
      backdropColor: color('paper'),
      cursor: 'pointer',
    };
    if (!S.visible[key]) {
      out.visibility = 'hidden';
      out.labelVisibility = 'hidden';
      return out;
    }
    if (S.view === 'local' && S.localSmall) out.labelVisibility = 'visible';
    if (key === S.selected || key === S.centerId) {
      out.size = out.size * 1.5;
      out.labelVisibility = 'visible';
      out.backdropVisibility = 'visible';
      out.zIndex = 2;
    }
    if (S.focus) {
      if (S.focus[key] === undefined) {
        out.opacity = DIM;
        out.labelVisibility = 'hidden';
        out.zIndex = 0;
      } else {
        if (key === S.hovered || !S.focusCrowded) out.labelVisibility = 'visible';
        out.zIndex = key === S.hovered ? 3 : 1;
      }
    }
    return out;
  }

  function edgeReducer(key, data, attrs) {
    if (!S.visible[attrs.s] || !S.visible[attrs.t]) return { visibility: 'hidden' };
    var out = { color: color(BG.edgeRole(attrs.kind)), size: attrs.kind === 'link' ? 1 : 1.5 };
    if (!attrs.resolved) out.opacity = 0.6;
    if (S.focus) {
      if (attrs.s === S.hovered || attrs.t === S.hovered) {
        out.size = 2.5;
        out.zIndex = 1;
      } else {
        out.opacity = DIM / 2;
      }
    }
    return out;
  }

  function ensureSigma(graph) {
    if (S.sigma) {
      clearHover();
      S.drag = null;
      S.sigma.setCustomBBox(null);
      S.sigma.setGraph(graph);
      return;
    }
    S.sigma = new B.Sigma(graph, sigmaBox, {
      settings: {
        // sigma 4 sizes nodes in graph units by default; screen pixels keep
        // a node the same size whatever the layout's scale.
        itemSizesReference: 'screen',
        autoRescaleContent: 'nodes',
        stagePadding: 40,
        renderEdgeLabels: false,
        hideEdgesOnMove: graph.size > 3000,
        labelDensity: 0.5,
        labelRenderedSizeThreshold: 7,
        allowInvalidContainer: true,
        minCameraRatio: MIN_CAMERA_RATIO,
        maxCameraRatio: MAX_CAMERA_RATIO,
        enableNodeDrag: true,
      },
      nodeReducer: nodeReducer,
      edgeReducer: edgeReducer,
    });
    // pointer follows sigma's hover even mid-drag; the drawn hover waits
    // for the drop.
    S.sigma.on('enterNode', function (e) {
      S.pointer = e.node;
      if (!S.drag) enterHover(e.node);
    });
    S.sigma.on('leaveNode', function () {
      S.pointer = null;
      if (!S.drag) lingerHover();
    });
    S.sigma.on('clickNode', function (e) { clickNode(e.node); });
    S.sigma.on('doubleClickNode', function (e) {
      e.preventSigmaDefault();
      showLocal(e.node);
    });
    S.sigma.on('nodeDragStart', function (e) { startDrag(e.node); });
    S.sigma.on('nodeDrag', function () { if (S.drag) S.drag.moves++; });
    S.sigma.on('nodeDragEnd', endDrag);
    // Capture runs before sigma's own wheel listener on its canvas, and
    // stopPropagation keeps the event from reaching it.
    sigmaBox.addEventListener('wheel', wheelZoom, { passive: false, capture: true });
    // sigma refreshes on a window resize; the note panel opening resizes
    // the stage without one.
    new ResizeObserver(function () {
      if (S.sigma) S.sigma.scheduleRefresh();
    }).observe(sigmaBox);
  }

  function refreshGraph() {
    if (S.sigma) S.sigma.refresh({ skipIndexation: true });
  }

  function clearHover() {
    clearTimeout(S.hoverTimer);
    S.hoverTimer = null;
    S.hovered = null;
    S.pointer = null;
    S.focus = null;
    S.focusCrowded = false;
  }

  function setHover(id) {
    clearTimeout(S.hoverTimer);
    S.hoverTimer = null;
    if (id === S.hovered) return;
    S.hovered = id;
    S.focus = id ? BG.depthSearch(S.adj, id, 1) : null;
    S.focusCrowded = !!S.focus && Object.keys(S.focus).length > LOCAL_LABELS_MAX;
    refreshGraph();
  }

  // enterHover moves a drawn hover at once (the graph stays dimmed, only the
  // focus moves) and draws one from rest after HOVER_INTENT_MS.
  function enterHover(id) {
    clearTimeout(S.hoverTimer);
    if (S.hovered) { setHover(id); return; }
    S.hoverTimer = setTimeout(function () { setHover(id); }, HOVER_INTENT_MS);
  }

  // lingerHover clears the hover after HOVER_LINGER_MS unless the cursor
  // reaches another node first; it also drops a pending enterHover.
  function lingerHover() {
    clearTimeout(S.hoverTimer);
    S.hoverTimer = setTimeout(function () { setHover(null); }, HOVER_LINGER_MS);
  }

  // wheelZoom replaces sigma's wheel zoom, which takes one animated 1.7x
  // step per event and drops events under 50 ms apart, while a trackpad
  // sends one every frame. Each event here zooms by its own delta, around
  // the cursor, with no animation.
  function wheelZoom(e) {
    if (!S.sigma) return;
    e.preventDefault();
    e.stopPropagation();
    var cam = S.sigma.getCamera();
    var ratio = BG.wheelRatio(cam.ratio, e.deltaY, e.deltaMode, e.ctrlKey, MIN_CAMERA_RATIO, MAX_CAMERA_RATIO);
    if (ratio === cam.ratio) return;
    var box = sigmaBox.getBoundingClientRect();
    cam.setState(S.sigma.getViewportZoomedState({ x: e.clientX - box.left, y: e.clientY - box.top }, ratio));
  }

  // ── node drag ──
  // sigma moves the dragged node. startDrag stops the view's layout so it
  // does not fight the cursor, and pins the graph's frame, so the rest does
  // not rescale while the node crosses the edge. A drag that interrupted a
  // layout resumes it on the drop, even when the node barely moved.
  function startDrag(id) {
    var run = S.runs[S.view];
    S.drag = { node: id, moves: 0, resumeMs: run ? Math.max(0, run.endsAt - Date.now()) : 0 };
    stopLayout(S.view);
    if (!S.sigma.getCustomBBox()) S.sigma.setCustomBBox(S.sigma.getBBox());
  }

  function endDrag() {
    var d = S.drag;
    S.drag = null;
    if (!d) return;
    if (S.pointer) setHover(S.pointer);
    else lingerHover();
    // Under sigma's drag tolerance the press counts as a click, not a move.
    var moved = d.moves >= S.sigma.getSetting('draggedEventsTolerance');
    if (moved || d.resumeMs) settle(S.sigma.getGraph(), moved ? d.node : null, Math.max(DROP_LAYOUT_MS, d.resumeMs));
  }

  // settle runs ForceAtlas2 for ms with the dropped node, if any, pinned.
  // A global layout that ran to the end is saved, so a moved node keeps its
  // place across reloads; a local layout is never posted.
  function settle(g, pinned, ms) {
    var gen = S.gen;
    if (pinned) g.setNodeAttribute(pinned, 'fixed', true);
    runLayout(g, ms, S.view).then(function (done) {
      if (pinned && g.hasNode(pinned)) g.removeNodeAttribute(pinned, 'fixed');
      if (done && g === S.global && gen === S.gen) saveLayout(S.data.key, positionsOf(g), gen);
    });
  }

  function clickNode(id) {
    var n = nodeOf(id);
    if (!n) return;
    var kind = BG.nodeKind(n);
    if (kind === 'note') { openNote(BG.noteRef(n)); return; }
    if (kind === 'tag') {
      S.filters.tag = S.filters.tag === n.title ? '' : n.title;
      applyFilters();
      renderToolbar();
      return;
    }
    toast('No note matches [[' + BG.nodeLabel(n) + ']] yet', 'warn');
  }

  function camera(how) {
    if (!S.sigma) return;
    var cam = S.sigma.getCamera();
    if (how === 'in') cam.zoomIn();
    else if (how === 'out') cam.zoomOut();
    else {
      // A drag pins the frame; fitting frames what the graph spans now.
      S.sigma.setCustomBBox(null);
      cam.reset();
    }
  }

  function applyFilters() {
    if (!S.data) return;
    var f = {
      vault: S.vault,
      tier: S.filters.tier,
      tag: S.filters.tag,
      status: S.filters.status,
      orphans: S.orphans,
      dangling: S.dangling,
    };
    S.visible = BG.visibleIds(S.data.nodes, S.data.edges, f);
    refreshGraph();
    renderStatus();
  }

  function positionsOf(g) {
    var out = {};
    g.forEachNode(function (id, a) { out[id] = [a.x, a.y]; });
    return out;
  }

  // ── layout ──

  // runLayout runs ForceAtlas2 in its worker for ms, then stops it. slot
  // names the run ("global" or "local", as S.view does), so a newer run in
  // the same slot stops the older one. The promise resolves true when the
  // run lasted its ms, false at once when something stopped it first.
  function runLayout(g, ms, slot) {
    stopLayout(slot);
    var fa2 = new B.FA2Layout(g, { settings: B.forceAtlas2.inferSettings(g) });
    return new Promise(function (resolve) {
      var run = { endsAt: Date.now() + ms };
      run.stop = function (done) {
        clearTimeout(run.timer);
        fa2.kill();
        if (S.runs[slot] === run) S.runs[slot] = null;
        resolve(done);
      };
      run.timer = setTimeout(function () { run.stop(true); }, ms);
      S.runs[slot] = run;
      fa2.start();
    });
  }

  function stopLayout(slot) {
    if (S.runs[slot]) S.runs[slot].stop(false);
  }

  // placeFresh gives every node without a position a start point
  // (docs/serve.md, Layout, step 2). With nothing known the whole graph
  // starts on one circle; otherwise each group of new nodes sits on a small
  // circle around the neighbours it links to.
  function placeFresh(data, known, fresh) {
    if (!fresh.length) return known;
    var out = Object.assign({}, known);
    if (!Object.keys(known).length) {
      var all = new B.Graph();
      data.nodes.forEach(function (n) { all.addNode(n.id); });
      var ring = B.circular(all, { scale: Math.max(10, Math.sqrt(data.nodes.length) * 10) });
      Object.keys(ring).forEach(function (id) { out[id] = [ring[id].x, ring[id].y]; });
      return out;
    }
    var gap = BG.spacing(known);
    BG.groupByAnchor(BG.anchorsFor(fresh, S.adj, known)).forEach(function (grp) {
      var small = new B.Graph();
      grp.ids.forEach(function (id) { small.addNode(id); });
      var r = gap * (grp.ids.length > 1 ? 1 : 0.5);
      var pts = B.circular(small, { scale: r });
      grp.ids.forEach(function (id) {
        out[id] = [grp.anchor[0] + pts[id].x, grp.anchor[1] + pts[id].y];
      });
    });
    return out;
  }

  // layoutGlobal follows the cache-miss steps of docs/serve.md: positions
  // the server returned or the page last drew, new nodes placed near their
  // neighbours, ForceAtlas2 in the worker, then POST with the key.
  async function layoutGlobal(data, g, fresh, gen) {
    var cached = data.layout && data.layout.cached;
    if (cached && !fresh.length) { S.layoutNote = 'layout cached'; renderStatus(); return; }
    var ms = BG.layoutMs(g.order, fresh.length);
    if (ms) {
      S.layoutNote = 'laying out ' + fresh.length + ' new';
      renderStatus();
      // A stopped run is either stale (a refetch) or resumed by a drop,
      // which saves when it ends.
      var done = await runLayout(g, ms, 'global');
      if (!done || gen !== S.gen) return;
    }
    await saveLayout(data.key, positionsOf(g), gen);
  }

  async function saveLayout(key, positions, gen) {
    var res;
    try {
      res = await fetch('/api/layout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(BG.layoutPayload(key, positions)),
      });
    } catch (err) {
      S.layoutNote = 'layout not saved: ' + errText(err);
      renderStatus();
      return;
    }
    if (gen !== S.gen) return;
    if (res.status === 204 || res.ok) S.layoutNote = 'layout saved';
    else if (res.status === 409) S.layoutNote = 'layout key moved on, refetching';
    else S.layoutNote = 'layout not saved (' + res.status + ')';
    renderStatus();
  }

  async function loadGraph() {
    var gen = ++S.gen;
    stopLayout('global');
    if (S.global) S.prev[S.globalFlag] = positionsOf(S.global);
    var data = await getJSON('/api/graph' + query({ topics: S.topics ? 1 : 0 }));
    if (gen !== S.gen) return;
    data.nodes = data.nodes || [];
    data.edges = data.edges || [];
    S.data = data;
    S.adj = BG.adjacency(data.edges);
    S.refs = BG.refIndex(data.nodes);
    S.maxDeg = BG.maxDegree(data.nodes);
    var flag = S.topics ? 1 : 0;
    // The other topics flag's positions place the notes the first time the
    // topics toggle flips; tag nodes then start near their notes.
    var previous = Object.assign({}, S.prev[1 - flag], S.prev[flag]);
    var known = BG.knownPositions(data.nodes, previous);
    var fresh = data.nodes.filter(function (n) { return !known[n.id]; }).map(function (n) { return n.id; });
    var g = buildGraph(data, placeFresh(data, known, fresh));
    S.global = g;
    S.globalFlag = flag;
    S.selected = S.noteRef ? S.refs[S.noteRef] || null : null;
    applyFiltersSilently();
    renderToolbar();
    renderLegend();
    if (!data.nodes.length) {
      showEmpty(S.vaults.length ? 'No notes in the configured vaults yet.' : null);
    } else {
      showEmpty(null);
    }
    var first = !S.sigma;
    var center = S.view === 'local' ? S.centerId || S.selected : null;
    if (center && g.hasNode(center)) showLocal(center, !first);
    else showGlobal(!first);
    layoutGlobal(data, g, fresh, gen).catch(function (err) {
      S.layoutNote = 'layout failed: ' + errText(err);
      renderStatus();
    });
  }

  function applyFiltersSilently() {
    var f = {
      vault: S.vault, tier: S.filters.tier, tag: S.filters.tag, status: S.filters.status,
      orphans: S.orphans, dangling: S.dangling,
    };
    S.visible = BG.visibleIds(S.data.nodes, S.data.edges, f);
  }

  // showGlobal draws every note. keepCamera holds the view on a refetch.
  function showGlobal(keepCamera) {
    stopLayout('local');
    S.view = 'global';
    S.centerId = null;
    if (!S.global) return;
    ensureSigma(S.global);
    if (keepCamera !== true) camera('fit');
    syncURL();
    renderToolbar();
    renderStatus();
  }

  // showLocal draws the notes within depth hops of id as their own graph,
  // starting from their global positions, and settles it briefly. A local
  // layout is never posted: the cache holds the global one.
  function showLocal(id, keepCamera) {
    if (!S.global || !S.global.hasNode(id)) return;
    stopLayout('local');
    S.view = 'local';
    S.centerId = id;
    var dist = BG.depthSearch(S.adj, id, S.depth);
    var lg = new B.Graph({ multi: true, type: 'directed', allowSelfLoops: false });
    Object.keys(dist).forEach(function (nid) {
      if (S.global.hasNode(nid)) lg.addNode(nid, Object.assign({}, S.global.getNodeAttributes(nid)));
    });
    S.global.forEachEdge(function (key, a, s, t) {
      if (lg.hasNode(s) && lg.hasNode(t)) lg.addDirectedEdgeWithKey(key, s, t, Object.assign({}, a));
    });
    S.localSmall = lg.order <= LOCAL_LABELS_MAX;
    ensureSigma(lg);
    if (keepCamera !== true) camera('fit');
    syncURL();
    renderToolbar();
    renderStatus();
    if (lg.order > 1) {
      runLayout(lg, LOCAL_LAYOUT_MS, 'local').then(function (done) {
        if (done && keepCamera !== true && S.view === 'local' && S.centerId === id) camera('fit');
      });
    }
  }

  function renderStatus() {
    if (!S.data) { setStatus(''); return; }
    var g = S.sigma ? S.sigma.getGraph() : null;
    var shown = 0;
    if (g) g.forEachNode(function (id) { if (S.visible[id]) shown++; });
    var notes = S.data.nodes.filter(function (n) { return BG.nodeKind(n) === 'note'; }).length;
    var parts = [];
    if (S.view === 'local' && S.centerId) {
      parts.push('local graph of ' + BG.nodeLabel(nodeOf(S.centerId) || { id: S.centerId }) + ', depth ' + S.depth);
    }
    parts.push(shown + ' shown of ' + notes + ' notes');
    if (S.layoutNote) parts.push(S.layoutNote);
    setStatus(parts.join(' · '));
  }

  // ── toolbar ──

  function select(label, value, options, onChange) {
    var s = el('select', { 'aria-label': label, onchange: function () { onChange(s.value); } },
      options.map(function (o) { return el('option', { value: o.value }, o.label); }));
    s.value = value;
    return s;
  }

  function check(label, on, onChange, title) {
    var box = el('input', { type: 'checkbox', onchange: function () { onChange(box.checked); } });
    box.checked = on;
    return el('label', { title: title || '' }, [box, label]);
  }

  function anyOf(label, values) {
    return [{ value: '', label: label }].concat(values.map(function (v) { return { value: v, label: v }; }));
  }

  function renderToolbar() {
    toolbar.innerHTML = '';
    var fac = S.data ? BG.facets(S.data.nodes) : { tiers: [], tags: [], statuses: [] };
    var canLocal = !!(S.selected || S.centerId);
    var depth = el('input', {
      type: 'range', min: String(BG.DEPTH_MIN), max: String(BG.DEPTH_MAX), step: '1',
      'aria-label': 'Depth',
      oninput: function () {
        S.depth = BG.clampDepth(depth.value);
        depthOut.textContent = String(S.depth);
      },
      onchange: function () { if (S.view === 'local' && S.centerId) showLocal(S.centerId); else syncURL(); },
    });
    depth.value = String(S.depth);
    var depthOut = el('span', {}, String(S.depth));
    toolbar.append(
      seg([{ value: 'global', label: 'Global' }, { value: 'local', label: 'Local' }], S.view, function (v) {
        if (v === 'global') showGlobal();
        else if (canLocal) showLocal(S.centerId || S.selected);
        else toast('Open a note first, then show its local graph', 'warn');
      }),
      el('label', { class: S.view === 'local' ? '' : 'off', title: 'Hops around the note in the local graph' }, ['Depth', depth, depthOut]),
      el('label', {}, ['Color', select('Color by', S.colorBy, BG.COLOR_MODES.map(function (m) { return { value: m, label: m }; }), function (v) {
        S.colorBy = v;
        syncURL();
        renderLegend();
        refreshGraph();
      })]),
      select('Tier', S.filters.tier, anyOf('any tier', fac.tiers), filterSetter('tier')),
      select('Tag', S.filters.tag, anyOf('any tag', withCurrent(fac.tags, S.filters.tag)), filterSetter('tag')),
      select('Status', S.filters.status, anyOf('any status', fac.statuses), filterSetter('status')),
      check('Orphans', S.orphans, function (on) { S.orphans = on; applyFilters(); }, 'Notes with no links'),
      check('Dangling', S.dangling, function (on) { S.dangling = on; applyFilters(); }, 'Link targets no note matches'),
      check('Size by links', S.byDegree, function (on) { S.byDegree = on; refreshGraph(); }),
      check('Topics', S.topics, function (on) {
        S.topics = on;
        syncURL();
        loadGraph().catch(showLoadError);
      }, 'A node per tag, linked to its notes'),
    );
  }

  function withCurrent(values, current) {
    return current && values.indexOf(current) < 0 ? values.concat([current]) : values;
  }

  function filterSetter(name) {
    return function (v) {
      S.filters[name] = v;
      applyFilters();
      loadBrowse();
    };
  }

  function renderLegend() {
    legendEl.innerHTML = '';
    if (!S.data || !S.data.nodes.length) return;
    BG.legend(S.colorBy, S.data.nodes, ctx()).forEach(function (e) {
      legendEl.appendChild(el('span', {}, [el('i', { style: 'background:' + color(e.role) }), e.label]));
    });
    var kinds = {};
    S.data.edges.forEach(function (e) { kinds[e.kind] = true; });
    BG.EDGE_KINDS.filter(function (k) { return kinds[k] && k !== 'link'; }).forEach(function (k) {
      legendEl.appendChild(el('span', {}, [el('i', { class: 'line', style: 'background:' + color(BG.edgeRole(k)) }), k]));
    });
  }

  // ── sidebar: vaults ──

  function renderVaults() {
    var box = sideParts.vaults;
    box.innerHTML = '';
    var total = S.vaults.reduce(function (s, v) { return s + (v.notes || 0); }, 0);
    box.appendChild(el('div', { class: 'sec-title' }, 'Vaults'));
    var rows = [vaultRow('', 'All vaults', total, [])];
    S.vaults.forEach(function (v) {
      var tags = [];
      if (v.shared) tags.push(badge('shared', 'info'));
      if (v.default) tags.push(badge('default', 'outline'));
      rows.push(vaultRow(v.name, v.name, v.notes, tags));
    });
    box.appendChild(el('div', { class: 'list' }, rows));
  }

  function vaultRow(name, label, count, tags) {
    var kids = [el('span', { class: 't' }, label)].concat(tags);
    kids.push(el('span', { class: 'm' }, String(count || 0)));
    return el('button', {
      class: 'row' + (S.vault === name ? ' active' : ''),
      type: 'button',
      onclick: function () { pickVault(name); },
    }, kids);
  }

  function pickVault(name) {
    S.vault = name;
    syncURL();
    renderVaults();
    applyFilters();
    loadBrowse();
    runSearch();
  }

  async function loadVaults() {
    var res = await getJSON('/api/vaults');
    S.vaults = res.vaults || [];
    S.vaultNames = S.vaults.map(function (v) { return v.name; });
    if (S.vault && S.vaultNames.indexOf(S.vault) < 0) S.vault = '';
    renderVaults();
    if (!S.vaults.length) await showDoctor();
  }

  // showDoctor explains an empty vault list: serve starts with no vault
  // configured and says so here instead of exiting.
  async function showDoctor() {
    var doc = null;
    try { doc = await getJSON('/api/doctor'); } catch (_) { doc = null; }
    var lines = [el('p', {}, 'brain serve found no vault to show. Add one to the config, then reload; brain doctor says what it sees.')];
    if (doc) {
      lines.push(el('p', {}, 'State dir: ' + (doc.state_dir || 'unknown') +
        (doc.layout_dir_writable === false ? ' (layout dir not writable)' : '')));
      (doc.vaults || []).forEach(function (v) {
        lines.push(el('p', {}, v.name + ': ' + (v.error || (v.readable ? 'readable' : 'not readable')) + ' (' + (v.resolved || v.path || '') + ')'));
      });
    }
    showEmpty(el('wk-callout', { variant: 'warn' }, lines));
  }

  // ── sidebar: search ──

  var searchTimer = null;
  function renderSearch() {
    var box = sideParts.search;
    box.innerHTML = '';
    var input = el('input', {
      type: 'search', placeholder: 'Search notes', 'aria-label': 'Search notes',
      oninput: function () {
        S.query = input.value;
        clearTimeout(searchTimer);
        searchTimer = setTimeout(runSearch, SEARCH_DEBOUNCE_MS);
      },
      onkeydown: function (e) { if (e.key === 'Enter') { clearTimeout(searchTimer); runSearch(); } },
    });
    input.value = S.query;
    box.append(
      el('div', { class: 'sec-title' }, 'Search'),
      input,
      seg([{ value: 'literal', label: 'Literal' }, { value: 'hybrid', label: 'Hybrid' }], S.searchMode, function (m) {
        S.searchMode = m;
        renderSearch();
        runSearch();
      }),
      el('div', { id: 'search-results' }),
    );
  }

  var searchSeq = 0;
  async function runSearch() {
    var out = document.getElementById('search-results');
    if (!out) return;
    var q = S.query.trim();
    var seq = ++searchSeq;
    if (!q) { out.innerHTML = ''; return; }
    var url = '/api/search' + query({ q: q, mode: S.searchMode, vault: S.vault, limit: SEARCH_LIMIT });
    var res;
    try {
      res = await getJSON(url);
    } catch (err) {
      if (seq !== searchSeq) return;
      out.innerHTML = '';
      out.appendChild(el('div', { class: 'hint' }, err.code === 'semantic_unavailable'
        ? 'Hybrid search needs a semantic index, and brain serve has none. Literal search still works.'
        : 'Search failed: ' + errText(err)));
      return;
    }
    if (seq !== searchSeq) return;
    out.innerHTML = '';
    var rows = searchRows(res);
    out.appendChild(el('div', { class: 'hint' }, rows.length ? rows.length + ' of ' + (res.total || rows.length) : 'No matches.'));
    (res.warnings || []).forEach(function (w) { out.appendChild(el('div', { class: 'hint' }, w)); });
    out.appendChild(el('div', { class: 'list' }, rows));
  }

  function searchRows(res) {
    if (res.hits) {
      return res.hits.map(function (h) {
        return noteRow({ ref: h.vault + ':' + h.path, title: h.title || basename(h.path), meta: h.vault, sub: h.heading_path || h.path, snip: h.snippet });
      });
    }
    if (res.lines) {
      return res.lines.map(function (l) {
        return noteRow({ ref: l.vault + ':' + l.path, title: basename(l.path), meta: l.vault + ':' + l.line, snip: l.text });
      });
    }
    return (res.files || []).map(function (f) {
      var meta = [f.vault, f.status, f.stale ? 'stale' : ''].filter(Boolean).join(' · ');
      return noteRow({ ref: f.vault + ':' + f.path, title: titleFor(f.vault, f.path), meta: meta, sub: f.path });
    });
  }

  // titleFor uses the graph's title for a note when the graph has it.
  function titleFor(vault, path) {
    var id = S.refs[vault + ':' + path];
    var n = id ? nodeOf(id) : null;
    return (n && n.title) || basename(path);
  }

  // ── sidebar: tier browsing ──

  function renderBrowse(listing, err) {
    var box = sideParts.browse;
    box.innerHTML = '';
    var tiers = BG.TIERS.map(function (t) { return { value: t, label: t }; });
    box.append(
      el('div', { class: 'sec-title' }, ['Browse', listing ? el('span', { class: 'n' }, String(listing.total)) : null]),
      seg(tiers, S.tier, function (t) { S.tier = t; loadBrowse(); }),
    );
    if (err) { box.appendChild(el('div', { class: 'hint' }, 'Could not list notes: ' + errText(err))); return; }
    if (!listing) return;
    var notes = listing.notes || [];
    if (!notes.length) { box.appendChild(el('div', { class: 'hint' }, 'No notes in ' + S.tier + ' match.')); return; }
    if (listing.total > notes.length) box.appendChild(el('div', { class: 'hint' }, 'Newest ' + notes.length + ' of ' + listing.total));
    box.appendChild(el('div', { class: 'list' }, notes.map(function (n) {
      var meta = [S.vault ? '' : n.vault, n.status].filter(Boolean).join(' · ');
      return noteRow({ ref: n.vault + ':' + n.path, title: n.title || basename(n.path), meta: meta });
    })));
  }

  var browseSeq = 0;
  async function loadBrowse() {
    var seq = ++browseSeq;
    renderBrowse(null, null);
    if (!S.vaults.length) return;
    var url = '/api/notes' + query({
      vault: S.vault, tier: S.tier, tag: S.filters.tag, status: S.filters.status, limit: LIST_LIMIT,
    });
    try {
      var listing = await getJSON(url);
      if (seq === browseSeq) renderBrowse(listing, null);
    } catch (err) {
      if (seq === browseSeq) renderBrowse(null, err);
    }
  }

  // ── sidebar: the queues ──

  function renderQueues() {
    var box = sideParts.queues;
    box.innerHTML = '';
    var q = S.queues;
    function count(v) { return v && !v.error ? ' ' + v.count : ''; }
    box.append(
      el('div', { class: 'sec-title' }, 'Queues'),
      seg([
        { value: 'promotions', label: 'Promotions' + count(q.promotions) },
        { value: 'stale', label: 'Stale' + count(q.stale) },
        { value: 'check', label: 'Check' + count(q.check) },
      ], S.queue, function (v) { S.queue = v; renderQueues(); }),
    );
    var cur = q[S.queue];
    if (!cur) { box.appendChild(el('div', { class: 'hint' }, 'Loading…')); return; }
    if (cur.error) { box.appendChild(el('div', { class: 'hint' }, 'Could not load: ' + errText(cur.error))); return; }
    box.appendChild(({ promotions: promotionsView, stale: staleView, check: checkView })[S.queue](cur.data));
  }

  function promotionsView(data) {
    var list = data.promotions || [];
    var kids = [el('div', { class: 'hint' }, 'Notes with status promoting, on their way into a shared vault.')];
    if (!list.length) kids.push(el('div', { class: 'hint' }, 'Nothing is being promoted.'));
    kids.push(el('div', { class: 'list' }, list.map(function (p) {
      var row = noteRow({ ref: p.vault + ':' + p.path, title: p.title || basename(p.path), meta: p.vault, sub: p.status_at ? 'since ' + p.status_at : '' });
      if (!p.shared_pr) return row;
      return el('div', {}, [row, el('a', { class: 'hint', href: p.shared_pr, target: '_blank', rel: 'noopener noreferrer' }, 'pull request')]);
    })));
    return el('div', {}, kids);
  }

  function staleView(data) {
    var kids = [el('div', { class: 'sec-title' }, 'Stale queue, without drift')];
    kids.push(el('div', { class: 'hint' }, ['Past review_by, disputed, or resting on a revoked note. Code and source drift is not checked here; run ', el('code', {}, 'brain stale --drift'), '.']));
    var groups = data.groups || [];
    if (!groups.length) kids.push(el('div', { class: 'hint' }, 'Nothing is stale.'));
    groups.forEach(function (g) {
      var title = g.domain + (g.from ? ' (' + g.from + ')' : '') + ' · ' + g.total;
      kids.push(el('div', { class: 'group' }, [
        el('div', { class: 'group-title' }, title),
        el('div', { class: 'list' }, (g.notes || []).map(function (n) {
          var why = (n.reasons || []).map(function (r) { return r.kind + (r.since ? ' since ' + r.since : '') + (r.detail ? ': ' + r.detail : ''); }).join('; ');
          return noteRow({ ref: n.vault + ':' + n.path, title: n.title || basename(n.path), meta: n.status, sub: why });
        })),
      ]));
    });
    return el('div', {}, kids);
  }

  function checkView(r) {
    var head = r.ok ? 'Check passes' : 'Check fails';
    var kids = [el('div', { class: 'hint' }, head + ': ' + r.checked + ' notes, ' + r.errors + ' errors, ' + r.warnings + ' warnings.')];
    kids.push(el('div', { class: 'list' }, (r.findings || []).map(function (f) {
      var where = f.vault + ':' + f.path + (f.line ? ':' + f.line : '');
      return noteRow({ ref: f.vault + ':' + f.path, title: f.message, meta: f.level, sub: f.rule + ' · ' + where });
    })));
    return el('div', {}, kids);
  }

  // mergeChecks folds a list of per-vault reports into one, in case the
  // server answers with one report per vault instead of a merged one.
  function mergeChecks(res) {
    if (!Array.isArray(res)) return res;
    return res.reduce(function (m, r) {
      return {
        ok: m.ok && r.ok,
        checked: m.checked + (r.checked || 0),
        errors: m.errors + (r.errors || 0),
        warnings: m.warnings + (r.warnings || 0),
        findings: m.findings.concat(r.findings || []),
      };
    }, { ok: true, checked: 0, errors: 0, warnings: 0, findings: [] });
  }

  async function loadQueues() {
    function settle(name, url, count, shape) {
      return getJSON(url).then(function (data) {
        var d = shape ? shape(data) : data;
        S.queues[name] = { data: d, count: count(d) };
      }, function (err) { S.queues[name] = { error: err }; });
    }
    await Promise.all([
      settle('promotions', '/api/promotions', function (d) { return (d.promotions || []).length; }),
      settle('stale', '/api/stale', function (d) { return d.total || 0; }),
      settle('check', '/api/check', function (d) { return (d.errors || 0) + (d.warnings || 0); }, mergeChecks),
    ]);
    renderQueues();
  }

  // ── note panel ──

  var md = new B.MarkdownIt({ html: false, linkify: true });
  md.inline.ruler.before('link', 'wikilink', wikilinkRule);
  md.renderer.rules.wikilink = renderWikilink;
  var defaultLinkOpen = md.renderer.rules.link_open || function (tokens, idx, options, env, self) {
    return self.renderToken(tokens, idx, options);
  };
  md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
    var href = tokens[idx].attrGet('href') || '';
    if (/^https?:/i.test(href)) {
      tokens[idx].attrSet('target', '_blank');
      tokens[idx].attrSet('rel', 'noopener noreferrer');
    }
    return defaultLinkOpen(tokens, idx, options, env, self);
  };

  // wikilinkRule is a markdown-it inline rule for [[target|alias]] and
  // ![[embed]]. It only cuts the token; renderWikilink resolves it by the
  // text /api/note listed, so link resolution stays in Go.
  function wikilinkRule(state, silent) {
    var src = state.src;
    var pos = state.pos;
    var start = src.charCodeAt(pos) === 0x21 ? pos + 1 : pos;
    if (src.charCodeAt(start) !== 0x5B || src.charCodeAt(start + 1) !== 0x5B) return false;
    var end = src.indexOf(']]', start + 2);
    if (end < 0 || end > state.posMax) return false;
    var inner = src.slice(start + 2, end);
    if (!inner || /[\n[\]]/.test(inner)) return false;
    if (!silent) {
      var tok = state.push('wikilink', '', 0);
      tok.content = src.slice(pos, end + 2);
    }
    state.pos = end + 2;
    return true;
  }

  function renderWikilink(tokens, idx, options, env) {
    var text = tokens[idx].content;
    var links = (env && env.links) || {};
    var l = links[text] || links[text.replace(/^!/, '')];
    var label = md.utils.escapeHtml(BG.wikilinkLabel(text));
    if (l && l.resolved && l.target) {
      var ref = md.utils.escapeHtml(l.target);
      return '<a class="wikilink" href="?note=' + md.utils.escapeHtml(encodeURIComponent(l.target)) + '" data-ref="' + ref + '">' + label + '</a>';
    }
    return '<span class="wikilink dangling" title="No note matches this link">' + label + '</span>';
  }

  note.addEventListener('click', function (e) {
    var a = e.target.closest('a.wikilink[data-ref]');
    if (a) { e.preventDefault(); openNote(a.dataset.ref); return; }
    var tag = e.target.closest('wk-badge[data-tag]');
    if (tag) {
      S.filters.tag = tag.dataset.tag;
      applyFilters();
      renderToolbar();
      loadBrowse();
    }
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && S.noteRef && !e.target.closest('input,select,textarea')) closeNote();
  });

  function setNoteOpen(open) {
    if (open) { note.removeAttribute('hidden'); app.classList.add('with-note'); } else { note.setAttribute('hidden', ''); app.classList.remove('with-note'); }
  }

  async function openNote(ref, quiet) {
    if (!ref) return;
    var seq = ++S.noteSeq;
    S.noteRef = ref;
    S.selected = S.refs[ref] || null;
    syncURL();
    setNoteOpen(true);
    if (!quiet) { note.innerHTML = ''; note.appendChild(el('div', { class: 'hint' }, 'Loading ' + ref + '…')); }
    refreshGraph();
    renderToolbar();
    var got;
    try {
      got = await Promise.all([
        getJSON('/api/note' + query({ ref: ref })),
        getJSON('/api/backlinks' + query({ ref: ref })).catch(function () { return null; }),
      ]);
    } catch (err) {
      if (seq !== S.noteSeq) return;
      note.innerHTML = '';
      note.append(noteHead(ref, null), el('wk-callout', { variant: 'error' }, err.status === 404 ? 'No note ' + ref + '.' : 'Could not read ' + ref + ': ' + errText(err)));
      return;
    }
    if (seq !== S.noteSeq) return;
    var n = got[0];
    S.selected = S.refs[n.ref] || S.refs[n.vault + ':' + n.path] || S.selected;
    renderNote(n, got[1]);
    refreshGraph();
    renderToolbar();
  }

  function closeNote() {
    S.noteSeq++;
    S.noteRef = '';
    S.selected = null;
    setNoteOpen(false);
    syncURL();
    refreshGraph();
    renderToolbar();
  }

  function noteHead(ref, n) {
    var fm = (n && n.frontmatter) || {};
    var title = fm.title || (n ? basename(n.path) : ref);
    return el('div', {}, [
      el('div', { class: 'note-head' }, [
        el('h2', { class: 'note-title' }, title),
        el('button', { class: 'btn-x', 'aria-label': 'Close the note', title: 'Close (Esc)', onclick: closeNote }, '✕'),
      ]),
      el('div', { class: 'note-ref' }, n ? n.vault + ':' + n.path : ref),
    ]);
  }

  function renderNote(n, back) {
    var fm = n.frontmatter || {};
    var meta = [statusBadge(n.status || fm.status)];
    if (fm.evidence) meta.push(badge('evidence ' + fm.evidence, 'c'));
    if (fm.review_by) meta.push(badge('review by ' + fm.review_by, n.status === 'validated' && fm.review_by < S.today ? 'warn' : 'outline'));
    (Array.isArray(fm.tags) ? fm.tags : []).forEach(function (t) {
      var b = badge('#' + t, 'filter');
      b.setAttribute('data-tag', t);
      b.setAttribute('title', 'Filter the graph by this tag');
      meta.push(b);
    });
    var id = S.refs[n.ref] || S.refs[n.vault + ':' + n.path];
    var actions = [];
    if (id) {
      actions.push(el('wk-button', { variant: 'ghost', onclick: function () { showLocal(id); } }, 'Local graph'));
    }
    var kids = [noteHead(n.ref, n), el('div', { class: 'note-meta' }, meta)];
    if (actions.length) kids.push(el('div', { class: 'note-actions' }, actions));
    if (n.banner) kids.push(el('wk-callout', { variant: 'warn' }, n.banner));
    kids.push(el('div', { class: 'wk-prose note-body', html: md.render(n.body || '', { links: BG.linkIndex(n.links) }) }));
    kids.push(frontmatterView(fm));
    if (back) kids.push(backlinksView(back));
    note.innerHTML = '';
    note.append.apply(note, kids.filter(Boolean));
  }

  function fmValue(v) {
    if (Array.isArray(v)) return v.map(fmValue).join(', ');
    if (v && typeof v === 'object') return JSON.stringify(v);
    return String(v);
  }

  function frontmatterView(fm) {
    var rows = [];
    Object.keys(fm).sort().forEach(function (k) {
      rows.push(el('dt', {}, k), el('dd', {}, fmValue(fm[k])));
    });
    if (!rows.length) return null;
    return el('details', { class: 'fm' }, [el('summary', {}, 'Frontmatter'), el('dl', {}, rows)]);
  }

  function backlinksView(back) {
    var inbound = back.inbound || [];
    var dangling = (back.outbound || []).filter(function (o) { return o.dangling; });
    var kids = [el('div', { class: 'sec-title' }, ['Backlinks', el('span', { class: 'n' }, String(inbound.length))])];
    if (!inbound.length) kids.push(el('div', { class: 'hint' }, 'No note links here.'));
    kids.push(el('div', { class: 'list' }, inbound.map(function (b) {
      return noteRow({ ref: b.vault + ':' + b.path, title: titleFor(b.vault, b.path), meta: b.vault + ':' + b.line, snip: b.excerpt });
    })));
    if (dangling.length) {
      kids.push(el('div', { class: 'sec-title', style: 'margin-top:1rem' }, ['Unresolved links', el('span', { class: 'n' }, String(dangling.length))]));
      kids.push(el('div', { class: 'hint' }, dangling.map(function (d) { return d.target + ' (line ' + d.line + ')'; }).join(', ')));
    }
    return el('div', { class: 'sec' }, kids);
  }

  // ── loading and polling ──

  function showLoadError(err) {
    setStatus('Failed to load: ' + errText(err));
  }

  async function loadAll() {
    await loadVaults();
    var work = [loadQueues(), loadBrowse()];
    if (S.vaults.length) work.push(loadGraph());
    await Promise.all(work);
    runSearch();
    if (S.noteRef) openNote(S.noteRef, true);
  }

  // reload runs loadAll once at a time; a key change while one runs queues
  // exactly one more.
  var running = null;
  var again = false;
  function reload() {
    if (running) { again = true; return; }
    running = loadAll().catch(showLoadError).finally(function () {
      running = null;
      if (again) { again = false; reload(); }
    });
  }

  function onState(st) {
    if (!st || st.key === S.stateKey) return;
    S.stateKey = st.key;
    reload();
  }

  renderSearch();
  renderBrowse(null, null);
  renderQueues();
  renderToolbar();
  Webkit.poll('/api/state', onState, POLL_MS, function (err) {
    setStatus('brain serve is unreachable: ' + errText(err));
  });
})();
