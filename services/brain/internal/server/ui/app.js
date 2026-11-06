'use strict';
// The brain.this UI (docs/serve.md). shell.html loads webkit.js, the graph
// bundle (window.BrainSigma) and graph.js (window.BrainGraph) first; this
// IIFE builds the page into #app: one toolbar row (view, search, filters,
// review), the sigma graph under it, and a popup that shows a note or the
// review queues over the dimmed graph.
//
// Every decision about what to show (filters, colours, sizes, captions,
// depth, the physics settings, where a new node starts) is a pure helper in
// graph.js. This file owns the DOM, the fetches, sigma and the d3-force
// simulation that moves the nodes.
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
  var SEARCH_DEBOUNCE_MS = 300;
  // A literal search answers every matching file up to this many; a hybrid
  // one ranks this many chunks.
  var SEARCH_LIMIT = { literal: 500, hybrid: 50 };
  var DIM = 0.12;
  // In the global graph a link between two communities is drawn this
  // faint, so the space between the islands reads as empty.
  var CROSS_EDGE_OPACITY = 0.3;
  // A hover neighbourhood this small shows every neighbour's full title.
  var FOCUS_LABELS_MAX = 40;
  // Camera zoom bounds, shared by sigma's settings and the wheel handler.
  // One graph unit is one pixel at ratio 1.
  var MIN_CAMERA_RATIO = 0.1;
  var MAX_CAMERA_RATIO = 20;
  // A caption shows once its circle is this many pixels in radius.
  var CAPTION_MIN_RADIUS_PX = 22;
  // The global graph names its edges only when zoomed in past this ratio,
  // as a vault links far more densely than a typical Neo4j result. A local
  // graph and the hovered node's edges always show their names.
  var EDGE_LABEL_RATIO = 0.5;
  // The full title under a hovered node is this many pixels high.
  var PILL_PX = 12;
  // A hover from rest draws once the cursor has stayed on a node this long,
  // so a cursor passing over nodes does not dim the graph. Leaving a node
  // clears the hover after HOVER_LINGER_MS, so crossing the gap between two
  // nodes does not un-dim and re-dim it. The dimming fades in and out over
  // HOVER_FADE_MS.
  var HOVER_INTENT_MS = 80;
  var HOVER_LINGER_MS = 150;
  var HOVER_FADE_MS = 180;
  // A click waits this long for a second one, so a double click opens the
  // local graph instead of the popup.
  var DOUBLE_CLICK_MS = 250;
  // A press on a node becomes a drag once the pointer moved this far from
  // where it went down; short of that, its release is a click.
  var DRAG_START_PX = 5;
  // A live frame runs up to this many ticks while the graph settles on its
  // own, within FRAME_BUDGET_MS; a drag runs one per frame.
  var SETTLE_TICKS = 3;
  var FRAME_BUDGET_MS = 7;
  // The warm run after a filter or search change is shorter than the
  // first one, so the change shows at once and the rest settles live.
  var RESTYLE_WARM_MS = 120;
  var FIT_MARGIN = 0.06;
  var FIT_MS = 350;
  // Fitting a small graph stops at this ratio, so a local graph of a few
  // notes opens at about their natural size instead of filling the screen.
  var FIT_MIN_RATIO = 0.7;

  // ── state ──
  // stateKey is the last /api/state key; a new one refetches everything.
  // global is the graphology graph of every node, fetched with the topics
  // flag globalFlag; view is what sigma shows (global, or local around
  // centerId). prev keeps the last drawn positions per topics flag, so a
  // refetch starts from them. match is the node ids the search found, or
  // null with no search. globalIds is the set of nodes the cached global
  // layout covers, the ones DEFAULT_FILTERS shows; layoutHeat is the alpha
  // that layout still needs before it rests. It is set when the graph
  // loads, kept while a filter, a search or a local graph runs the
  // simulation over fewer nodes, and cleared once all of globalIds rest.
  // community maps every node of the global graph to its community and
  // homes to its island ([x, y, radius]); both hold across polls, so a
  // refetch keeps every island where it is (groupIslands).
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
    radii: [],
    visible: {},
    prev: { 0: {}, 1: {} },
    view: 'global',
    centerId: null,
    centerRef: '',
    depth: 1,
    colorBy: 'tier',
    filters: { tier: BG.DEFAULT_FILTERS.tier, tag: BG.DEFAULT_FILTERS.tag, status: BG.DEFAULT_FILTERS.status },
    orphans: BG.DEFAULT_FILTERS.orphans,
    dangling: BG.DEFAULT_FILTERS.dangling,
    byDegree: true,
    topics: false,
    hovered: null,
    hoverTimer: null,
    pointer: null,
    focus: null,
    focusCrowded: false,
    fade: { from: 0, to: 0, start: 0, value: 0 },
    drag: null,
    press: null,
    lastClick: null,
    selected: null,
    lastNode: null,
    noteRef: '',
    noteSeq: 0,
    popup: '',
    gen: 0,
    layoutNote: '',
    layoutDirty: false,
    layoutHeat: 0,
    globalIds: {},
    community: null,
    homes: null,
    searchMode: 'literal',
    query: '',
    match: null,
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

  // ── URL state: ?vault=&color=&note=&view=local&center=&depth=&topics=1 ──
  // note is the note the popup shows; center is the local graph's note, and
  // falls back to note, as links from before the popup wrote it.
  (function readURL() {
    var p = new URLSearchParams(location.search);
    S.vault = p.get('vault') || '';
    if (BG.COLOR_MODES.indexOf(p.get('color')) >= 0) S.colorBy = p.get('color');
    S.noteRef = p.get('note') || '';
    S.centerRef = p.get('center') || S.noteRef;
    S.view = p.get('view') === 'local' && S.centerRef ? 'local' : 'global';
    S.depth = BG.clampDepth(p.get('depth') || 1);
    S.topics = p.get('topics') === '1';
  })();

  function syncURL() {
    var p = new URLSearchParams();
    if (S.vault) p.set('vault', S.vault);
    if (S.colorBy !== 'tier') p.set('color', S.colorBy);
    if (S.noteRef) p.set('note', S.noteRef);
    if (S.view === 'local' && S.centerId) {
      p.set('view', 'local');
      p.set('center', centerRefOf(S.centerId));
    }
    if (S.depth !== 1) p.set('depth', String(S.depth));
    if (S.topics) p.set('topics', '1');
    var qs = p.toString();
    history.replaceState(null, '', qs ? '?' + qs : location.pathname);
  }

  function centerRefOf(id) {
    var n = nodeOf(id);
    return (n && BG.noteRef(n)) || id;
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
    restyleShown();
    renderLegend();
  });

  // ── page skeleton ──
  // One toolbar row: the view and depth, the search, the Filters button and
  // Review. The filters sit in a popover under their button, so the row
  // fits a laptop screen.
  var toolbar = el('div', { class: 'toolbar' });
  var viewGroup = el('div', { class: 'tb-group' });
  var searchGroup = el('div', { class: 'tb-group search' });
  var filterCount = el('span', { class: 'count', hidden: '' });
  var filtersBtn = el('button', {
    class: 'tb-btn filters-btn', type: 'button', popovertarget: 'filters',
    title: 'Vault, colour, tier, tag and status, and the graph toggles',
  }, ['Filters', filterCount]);
  var filterPanel = el('div', { id: 'filters', class: 'filters', popover: 'auto', role: 'group', 'aria-label': 'Filters' });
  var reviewCount = el('span', { class: 'count', hidden: '' });
  var reviewBtn = el('button', {
    class: 'tb-btn review tb-push', type: 'button', title: 'Promotions, the stale queue and check findings',
    onclick: function () { openReview(); },
  }, ['Review', reviewCount]);
  toolbar.append(viewGroup, searchGroup, filtersBtn, filterPanel, reviewBtn);

  // The popover opens under its button: beforetoggle places it before it
  // shows, and once open it moves left if it would leave the window.
  var POPOVER_GAP_PX = 6;
  filterPanel.addEventListener('beforetoggle', function (e) { if (e.newState === 'open') placeFilters(); });
  filterPanel.addEventListener('toggle', function (e) { if (e.newState === 'open') keepFiltersInView(); });
  window.addEventListener('resize', function () {
    if (!filterPanel.matches(':popover-open')) return;
    placeFilters();
    keepFiltersInView();
  });

  function placeFilters() {
    var b = filtersBtn.getBoundingClientRect();
    filterPanel.style.left = Math.round(b.left) + 'px';
    filterPanel.style.top = Math.round(b.bottom + POPOVER_GAP_PX) + 'px';
  }

  function keepFiltersInView() {
    var r = filterPanel.getBoundingClientRect();
    var over = r.right - (window.innerWidth - POPOVER_GAP_PX);
    if (over > 0) filterPanel.style.left = Math.max(POPOVER_GAP_PX, Math.round(r.left - over)) + 'px';
  }

  var graphBox = el('div', { id: 'graph' });
  var sigmaBox = el('div', { class: 'sigma', tabindex: '-1' });
  var statusEl = el('div', { class: 'overlay status' });
  var legendEl = el('div', { class: 'overlay legend' });
  var emptyEl = el('div', { class: 'empty', hidden: '' });
  var zoomEl = el('div', { class: 'overlay zoom' }, [
    el('button', { title: 'Zoom in', 'aria-label': 'Zoom in', onclick: function () { camera('in'); } }, '+'),
    el('button', { title: 'Zoom out', 'aria-label': 'Zoom out', onclick: function () { camera('out'); } }, '−'),
    el('button', { title: 'Fit the graph', 'aria-label': 'Fit the graph', onclick: function () { camera('fit'); } }, '▢'),
  ]);
  graphBox.append(sigmaBox, statusEl, legendEl, zoomEl, emptyEl);
  app.append(toolbar, graphBox);

  var pop = el('dialog', { class: 'pop', 'aria-labelledby': 'pop-title' });
  var popScroll = el('div', { class: 'pop-scroll', tabindex: '-1' });
  pop.appendChild(popScroll);
  document.body.appendChild(pop);

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
      class: 'row multi',
      type: 'button',
      title: opts.ref || '',
      onclick: function () { if (opts.ref) openNote(opts.ref); },
    }, lines);
  }

  function basename(path) {
    var s = String(path || '');
    return s.slice(s.lastIndexOf('/') + 1).replace(/\.md$/, '');
  }

  // ── graph: build, styles, reducers, sigma ──

  function ctx() {
    return { vaults: S.vaultNames, today: S.today, community: S.community };
  }

  function nodeOf(id) {
    var g = S.global;
    return g && g.hasNode(id) ? g.getNodeAttribute(id, 'n') : null;
  }

  function shownGraph() {
    return S.sigma ? S.sigma.getGraph() : null;
  }

  function buildGraph(data, positions) {
    var g = new B.Graph({ multi: true, type: 'directed', allowSelfLoops: false });
    data.nodes.forEach(function (n) {
      var p = positions[n.id] || [0, 0];
      g.addNode(n.id, { x: p[0], y: p[1], title: BG.nodeLabel(n), n: n });
    });
    data.edges.forEach(function (e) {
      if (e.source === e.target || !g.hasNode(e.source) || !g.hasNode(e.target)) return;
      var key = e.source + '>' + e.target + '#' + e.kind;
      if (g.hasEdge(key)) return;
      var cross = !!S.community && S.community[e.source] !== S.community[e.target];
      g.addDirectedEdgeWithKey(key, e.source, e.target, {
        kind: e.kind, resolved: e.resolved, s: e.source, t: e.target, cross: cross,
        label: BG.edgeCaption(e.kind), size: 1, opacity: e.resolved ? 1 : 0.6,
      });
    });
    restyle(g);
    return g;
  }

  // restyle writes what a node and an edge look like into their
  // attributes, which sigma's styles read: radius, fill and border, the
  // caption cut to fit its circle and the caption's colour. It runs when
  // the colour mode, the size toggle or the theme changes, so the reducers
  // that run on every frame only handle hover, search and visibility.
  //
  // It writes the attribute objects in place, which emits no graph event,
  // and the caller refreshes sigma in full. sigma answers a bulk update of
  // colours with an in-place repaint that throws when a scheduled full
  // refresh has not been processed yet.
  function restyle(g) {
    var c = ctx();
    if (S.data) S.radii = BG.radiiOf(S.data.nodes, S.maxDeg, S.byDegree);
    g.forEachNode(function (id, a) {
      var role = BG.colorRole(a.n, S.colorBy, c);
      var r = BG.nodeRadius(a.n, S.maxDeg, S.byDegree);
      var fs = BG.captionSize(r);
      a.size = r;
      a.color = color(role);
      a.borderColor = color(BG.borderRole(role));
      a.labelColor = BG.captionColor(a.color);
      a.labelSize = fs;
      a.caption = BG.caption(a.title, r, fs);
    });
    var text = color('edge-text');
    var bg = color('graph-bg');
    g.forEachEdge(function (k, a) {
      a.color = color(BG.edgeRole(a.kind));
      a.labelColor = text;
      a.labelBackground = bg;
    });
  }

  var HIDDEN_NODE = { visibility: 'hidden', labelVisibility: 'hidden' };
  var HIDDEN_EDGE = { visibility: 'hidden', labelVisibility: 'hidden' };
  var NO_EDGE_LABEL = { labelVisibility: 'hidden' };
  var CROSS_EDGE = { opacity: CROSS_EDGE_OPACITY };
  var CROSS_EDGE_NO_LABEL = { opacity: CROSS_EDGE_OPACITY, labelVisibility: 'hidden' };
  var SHOW_CAPTION = { labelVisibility: 'visible' };
  var NO_CAPTION = { labelVisibility: 'hidden' };
  var PLAIN = {};

  function cameraRatio() {
    return S.sigma ? S.sigma.getCamera().ratio : 1;
  }

  // edgeLabelsShown says whether the edges outside a hover show their
  // names: always in a local graph, in the global one only zoomed in. The
  // reducers first run inside sigma's constructor, before S.sigma is set,
  // at the starting ratio of 1.
  function edgeLabelsShown() {
    return S.view === 'local' || cameraRatio() <= EDGE_LABEL_RATIO;
  }

  // nodeReducer runs on every node on every frame the graph moves, so it
  // returns shared objects where it can and reads only S. It decides every
  // caption itself, from the circle's size on screen: with autoRescale off,
  // sigma's label grid only covers the nodes that sit inside the viewport
  // at ratio 1, so it would drop captions at the edge of a large graph.
  function nodeReducer(key, data, attrs) {
    if (!S.visible[key]) return HIDDEN_NODE;
    var ratio = cameraRatio();
    var big = BG.captionShown(attrs.size, ratio, CAPTION_MIN_RADIUS_PX);
    if (!S.focus) return big ? SHOW_CAPTION : NO_CAPTION;
    if (S.focus[key] === undefined) {
      return { opacity: BG.dimOpacity(S.fade.value, DIM), labelVisibility: 'hidden', zIndex: 0 };
    }
    var out = { zIndex: key === S.hovered ? 3 : 1, labelVisibility: big ? 'visible' : 'hidden' };
    // The hovered node, and its neighbours when there are few of them and
    // their captions are too small to read, show their full title in a
    // pill under the circle.
    if (key === S.hovered || (!S.focusCrowded && !big)) {
      out.label = attrs.title;
      out.labelPosition = 'below';
      out.labelVisibility = 'visible';
      out.labelSize = PILL_PX * ratio;
      out.labelColor = color('label-pill-text');
      out.labelBackgroundColor = color('label-pill');
      out.labelBackgroundPadding = 3;
    }
    return out;
  }

  function edgeReducer(key, data, attrs) {
    if (!S.visible[attrs.s] || !S.visible[attrs.t]) return HIDDEN_EDGE;
    if (!S.focus) {
      if (attrs.cross && S.view === 'global') return edgeLabelsShown() ? CROSS_EDGE : CROSS_EDGE_NO_LABEL;
      return edgeLabelsShown() ? PLAIN : NO_EDGE_LABEL;
    }
    if (attrs.s === S.hovered || attrs.t === S.hovered) return { size: 2, zIndex: 1, depth: 'topEdges' };
    return { opacity: BG.dimOpacity(S.fade.value, DIM / 2), labelVisibility: 'hidden' };
  }

  // The sigma 4 primitives: a circle with a darker ring of its own hue and
  // the caption over it, as Neo4j Browser draws a node; straight edges
  // with a plain arrowhead that ends on the target's ring, parallel edges
  // fanned out as arcs.
  function primitives() {
    return {
      nodes: {
        shapes: [B.sdfCircle()],
        variables: { borderColor: { type: 'color', default: '#9AA1AC' } },
        layers: [B.layerBorder({
          borders: [
            { size: 0.08, mode: 'relative', color: { attribute: 'borderColor' } },
            { size: 0, fill: true, color: { attribute: 'color' } },
          ],
        })],
        label: {
          font: { family: 'system-ui, -apple-system, "Segoe UI", Helvetica, Arial, sans-serif', size: 10, weight: '500' },
          margin: 4,
          // Captions are sized in graph units, like the circles they sit in.
          zoomToLabelSizeRatioFunction: function (ratio) { return ratio; },
        },
      },
      edges: {
        paths: [B.pathLine(), B.pathCurved(), B.pathLoop()],
        extremities: [B.extremityArrow({ lengthRatio: 7, widthRatio: 5.5 })],
        layers: [B.layerPlain()],
        defaultHead: 'arrow',
        label: { fontSizeMode: 'scaled', margin: 2, keepUpright: true },
      },
    };
  }

  function styles() {
    return {
      nodes: [
        {
          x: { attribute: 'x' },
          y: { attribute: 'y' },
          size: { attribute: 'size', defaultValue: 25 },
          color: { attribute: 'color', defaultValue: '#A5ABB6' },
          label: { attribute: 'caption' },
          labelColor: { attribute: 'labelColor', defaultValue: '#FFFFFF' },
          labelSize: { attribute: 'labelSize', defaultValue: 8.5 },
          labelPosition: 'over',
          labelVisibility: 'auto',
          visibility: 'visible',
          cursor: 'pointer',
          depth: function (a, st) { return st.isHovered || st.isDragged ? 'topNodes' : 'nodes'; },
          backdropVisibility: 'hidden',
          backdropArea: 'node',
          backdropPadding: 8,
          backdropShadowBlur: 0,
          backdropShadowColor: 'rgba(0,0,0,0)',
        },
        { whenState: 'isSelected', then: { backdropVisibility: 'visible', backdropColor: function () { return color('halo-selected'); } } },
        { whenState: 'isHovered', then: { backdropVisibility: 'visible', backdropColor: function () { return color('halo-hover'); } } },
      ],
      edges: [
        {
          path: 'straight',
          parallelPath: 'curved',
          selfLoopPath: 'loop',
          parallelSpread: 0.35,
          head: 'arrow',
          size: { attribute: 'size', defaultValue: 1 },
          color: { attribute: 'color', defaultValue: '#A5ABB6' },
          opacity: { attribute: 'opacity', defaultValue: 1 },
          label: { attribute: 'label' },
          labelColor: { attribute: 'labelColor', defaultValue: '#5D626C' },
          labelSize: 7,
          labelPosition: 'over',
          labelBackgroundColor: { attribute: 'labelBackground', defaultValue: '#F9FCFF' },
          labelBackgroundPadding: 1.5,
          visibility: 'visible',
          depth: 'edges',
        },
      ],
    };
  }

  function ensureSigma(graph) {
    if (S.sigma) {
      clearHover();
      S.drag = null;
      S.press = null;
      S.sigma.setGraph(graph);
      markSelected();
      return;
    }
    S.sigma = new B.Sigma(graph, sigmaBox, {
      primitives: primitives(),
      styles: styles(),
      customNodeState: { isSelected: false },
      settings: {
        // One graph unit is one pixel at camera ratio 1, nodes and captions
        // are sized in those units, and the frame never rescales to the
        // graph's extent, so a moving layout does not drag the camera.
        itemSizesReference: 'positions',
        zoomToSizeRatioFunction: function (ratio) { return ratio; },
        autoRescale: false,
        stagePadding: 0,
        renderEdgeLabels: true,
        edgeLabelAnchors: 'nodeLabels',
        labelRenderedSizeThreshold: CAPTION_MIN_RADIUS_PX,
        minEdgeThickness: 1,
        allowInvalidContainer: true,
        minCameraRatio: MIN_CAMERA_RATIO,
        maxCameraRatio: MAX_CAMERA_RATIO,
        enableCameraRotation: false,
        enableCameraMouseRotation: false,
        doubleClickTimeout: DOUBLE_CLICK_MS,
        // sigma drops a click after three move events; the app decides
        // click or drag by distance instead (pressClick), so sigma reports
        // every release.
        draggedEventsTolerance: Infinity,
        enableNodeDrag: true,
        dragPositionToAttributes: dragTo,
      },
      nodeReducer: nodeReducer,
      edgeReducer: edgeReducer,
    });
    markSelected();
    // pointer follows sigma's hover even mid-drag; the drawn hover waits
    // for the drop.
    S.sigma.on('enterNode', function (e) {
      S.pointer = e.node;
      phys.pin(e.node);
      if (!S.drag) enterHover(e.node);
    });
    S.sigma.on('leaveNode', function (e) {
      S.pointer = null;
      if (!S.drag || S.drag.node !== e.node) phys.unpin(e.node);
      if (!S.drag) lingerHover();
    });
    S.sigma.on('downNode', function (e) { S.press = { node: e.node, moved: false }; });
    S.sigma.on('downStage', function () { S.press = null; });
    // sigma reports a release as a click, or as a double click when its own
    // counter says so; both go to pressClick, which decides by itself.
    S.sigma.on('clickNode', function (e) { pressClick(e.node); });
    S.sigma.on('doubleClickNode', function (e) {
      e.preventSigmaDefault();
      pressClick(e.node);
    });
    S.sigma.on('nodeDragStart', function (e) { startDrag(e.node); });
    S.sigma.on('nodeDragEnd', function (e) { endDrag(e.node); });
    // The reducers read the camera ratio: a pill under a hovered node keeps
    // its pixel size, captions show once their circle is big enough, and
    // edge names come and go at EDGE_LABEL_RATIO. A zoom reruns them only
    // when one of those changes.
    var labelled = edgeLabelsShown();
    var lastRatio = cameraRatio();
    S.sigma.getCamera().on('updated', function (st) {
      var now = edgeLabelsShown();
      if (S.focus || now !== labelled) S.sigma.scheduleRefresh();
      else if (BG.captionsFlip(S.radii, CAPTION_MIN_RADIUS_PX, lastRatio, st.ratio)) refreshNodes();
      labelled = now;
      lastRatio = st.ratio;
    });
    // Capture runs before sigma's own wheel listener on its canvas, and
    // stopPropagation keeps the event from reaching it.
    sigmaBox.addEventListener('wheel', wheelZoom, { passive: false, capture: true });
    new ResizeObserver(function () {
      if (S.sigma) S.sigma.scheduleRefresh();
    }).observe(sigmaBox);
  }

  function refreshGraph() {
    if (S.sigma) S.sigma.refresh();
  }

  // refreshNodes reruns the node reducer alone, for a change that touches
  // no edge. It keeps sigma's indexation: skipping it throws when a full
  // refresh is still pending.
  function refreshNodes() {
    var g = shownGraph();
    if (g) S.sigma.refresh({ partialGraph: { nodes: g.nodes() }, schedule: true });
  }

  // markSelected gives the open note's node, and the local graph's centre,
  // the selected halo.
  function markSelected() {
    if (!S.sigma) return;
    var g = S.sigma.getGraph();
    var want = {};
    if (S.selected) want[S.selected] = true;
    if (S.view === 'local' && S.centerId) want[S.centerId] = true;
    var on = [];
    var off = [];
    g.forEachNode(function (id) {
      var now = S.sigma.getNodeState(id).isSelected;
      if (want[id] && !now) on.push(id);
      else if (!want[id] && now) off.push(id);
    });
    if (on.length) S.sigma.setNodesState(on, { isSelected: true });
    if (off.length) S.sigma.setNodesState(off, { isSelected: false });
  }

  function setSelected(id) {
    S.selected = id || null;
    if (id) S.lastNode = id;
    markSelected();
  }

  // ── physics ──
  // phys runs one d3-force simulation over the nodes the shown graph has
  // visible. d3's own timer is stopped: frame() ticks it once a frame and
  // writes every position into the graph in one update, so sigma refreshes
  // once per frame. When the simulation is cold and no fade runs, no frame
  // is scheduled, so an idle page draws nothing. On the global graph each
  // node is held on its island (S.homes, BG.islandForce) and a link
  // between communities barely pulls; a local graph keeps the plain
  // forces, so its few notes spread out around the centre.
  var phys = {
    sim: null,
    graph: null,
    index: {},
    P: null,
    cooled: true,
    pinned: null,
    // full says the simulation lays out the whole global layout: it runs
    // on the global graph over every node of S.globalIds, with no search.
    full: false,

    // start replaces the simulation with one over ids of g, at alpha, and
    // runs it for up to warmMs before returning, so the first frame after
    // it is already settled. A full one starts at least as hot as the
    // global layout still needs (S.layoutHeat).
    start: function (g, ids, alpha, warmMs) {
      this.retire();
      var islands = g === S.global && !!S.homes;
      var P = BG.simParams(islands);
      var shown = {};
      var nodes = ids.map(function (id) {
        shown[id] = true;
        var a = g.getNodeAttributes(id);
        return { id: id, x: a.x, y: a.y, r: a.size, home: islands ? S.homes[id] || null : null };
      });
      var edges = [];
      g.forEachEdge(function (k, a, s, t) { edges.push({ source: s, target: t }); });
      var full = g === S.global && !S.match && BG.coversAll(shown, S.globalIds);
      if (full) alpha = Math.max(alpha, S.layoutHeat);
      var links = BG.simLinks(edges, shown);
      var pull = BG.linkStrengths(links, P, islands ? S.community : null);
      var sim = B.forceSimulation(nodes).stop();
      sim.force('link', B.forceLink(links)
        .id(function (d) { return d.id; })
        .distance(function (l) { return l.source.r + l.target.r + P.linkGap; })
        .strength(function (l, i) { return pull[i]; }))
        .force('charge', B.forceManyBody().strength(P.charge).distanceMax(P.chargeDistanceMax))
        .force('collide', B.forceCollide(function (d) { return d.r + P.collidePad; }).iterations(P.collideIterations));
      if (islands) {
        sim.force('island', BG.islandForce(P));
      } else {
        sim.force('x', B.forceX(0).strength(P.gravity)).force('y', B.forceY(0).strength(P.gravity));
      }
      sim.velocityDecay(P.velocityDecay)
        .alphaMin(P.alphaMin)
        .alphaDecay(P.alphaDecay)
        .alpha(alpha);
      var index = {};
      nodes.forEach(function (d) { index[d.id] = d; });
      this.sim = sim;
      this.graph = g;
      this.index = index;
      this.P = P;
      this.full = full;
      this.cooled = false;
      this.calm = false;
      this.pinned = null;
      if (S.pointer) this.pin(S.pointer);
      this.warm(Math.min(warmMs, P.warmMs));
      wake();
    },

    // retire runs before a full simulation of the current global graph is
    // replaced: one that came to rest finishes as it would have, one still
    // moving leaves its alpha in S.layoutHeat for the next full run.
    retire: function () {
      if (!this.sim || !this.full || this.graph !== S.global) return;
      if (this.hot()) {
        S.layoutHeat = this.sim.alpha();
      } else if (!this.cooled) {
        this.cooled = true;
        cooled();
      }
    },

    stop: function () {
      this.sim = null;
      this.graph = null;
      this.index = {};
      this.pinned = null;
      this.full = false;
      this.calm = false;
    },

    hot: function () {
      return !!this.sim && this.sim.alpha() >= this.sim.alphaMin();
    },

    // warm ticks synchronously for up to ms or warmTicks, then writes once.
    warm: function (ms) {
      if (!this.hot() || ms <= 0) return 0;
      var t0 = performance.now();
      var n = 0;
      while (n < this.P.warmTicks && this.hot() && performance.now() - t0 < ms) {
        this.sim.tick();
        n++;
      }
      this.write();
      return n;
    },

    // step runs one live frame: one tick while a drag holds the graph hot
    // or calmed, more while it settles alone, within the frame budget.
    step: function () {
      var t0 = performance.now();
      var max = this.sim.alphaTarget() > 0 || this.calm ? 1 : SETTLE_TICKS;
      for (var i = 0; i < max && this.hot(); i++) {
        this.sim.tick();
        if (performance.now() - t0 > FRAME_BUDGET_MS) break;
      }
      this.write();
    },

    write: function () {
      var idx = this.index;
      this.graph.updateEachNodeAttributes(function (id, a) {
        var d = idx[id];
        if (d) {
          a.x = d.x;
          a.y = d.y;
        }
        return a;
      }, { attributes: ['x', 'y'] });
    },

    // soothe slows the graph down while the user moves a node: more
    // friction and one tick a frame, until the graph rests again.
    soothe: function (on) {
      if (!this.sim || this.calm === on) return;
      this.calm = on;
      this.sim.velocityDecay(on ? this.P.calmVelocityDecay : this.P.velocityDecay);
    },

    reheat: function (alpha) {
      if (!this.sim) return;
      if (this.sim.alpha() < alpha) this.sim.alpha(alpha);
      this.cooled = false;
      wake();
    },

    // pin holds the hovered node still, so it does not run from the cursor
    // while the graph moves.
    pin: function (id) {
      var d = this.index[id];
      if (!d || (S.drag && S.drag.node !== id)) return;
      this.unpin(this.pinned);
      d.fx = d.x;
      d.fy = d.y;
      this.pinned = id;
    },

    unpin: function (id) {
      var d = id && this.index[id];
      if (d) {
        d.fx = null;
        d.fy = null;
      }
      if (this.pinned === id) this.pinned = null;
    },
  };

  // ── the frame loop ──
  var raf = 0;
  function wake() {
    if (!raf) raf = requestAnimationFrame(frame);
  }

  function frame(now) {
    raf = 0;
    var busy = false;
    if (phys.hot()) {
      phys.step();
      busy = true;
    } else if (phys.sim && !phys.cooled) {
      phys.cooled = true;
      phys.soothe(false);
      cooled();
    }
    if (fadeStep(now)) busy = true;
    if (clickStep(now)) busy = true;
    if (busy) wake();
  }

  // cooled runs once the simulation comes to rest. Only a full simulation
  // (phys.full) has laid out the global layout: then a layout moved by a
  // drag, a filter or search that ran, or new nodes is saved, so it holds
  // across reloads. A local graph, or one a filter or search reduced, is
  // never posted; its changes wait for the next full run.
  function cooled() {
    if (!phys.full || !S.data) return;
    S.layoutHeat = 0;
    if (!S.layoutDirty) return;
    S.layoutDirty = false;
    saveLayout(S.data.key, positionsOf(S.global), S.gen);
  }

  // ── hover ──

  function clearHover() {
    clearTimeout(S.hoverTimer);
    S.hoverTimer = null;
    S.hovered = null;
    S.pointer = null;
    S.focus = null;
    S.focusCrowded = false;
    S.fade = { from: 0, to: 0, start: 0, value: 0 };
  }

  function fadeTo(target) {
    S.fade = { from: S.fade.value, to: target, start: performance.now(), value: S.fade.value };
    wake();
  }

  // fadeStep moves the dimming one frame on; it reports whether it is
  // still running. A fade out that ends drops the focus.
  function fadeStep(now) {
    var f = S.fade;
    if (f.value === f.to) return false;
    f.value = BG.tweenValue(f.from, f.to, f.start, now, HOVER_FADE_MS);
    if (f.value === f.to && f.to === 0 && !S.hovered) {
      S.focus = null;
      S.focusCrowded = false;
      refreshGraph();
      return false;
    }
    // Only opacity changes between the first and the last frame of a fade.
    var g = shownGraph();
    if (g) S.sigma.refresh({ partialGraph: { nodes: g.nodes(), edges: g.edges() }, schedule: true });
    return f.value !== f.to;
  }

  function setHover(id) {
    clearTimeout(S.hoverTimer);
    S.hoverTimer = null;
    if (id === S.hovered) return;
    S.hovered = id;
    if (id) {
      S.focus = BG.depthSearch(S.adj, id, 1);
      S.focusCrowded = Object.keys(S.focus).length > FOCUS_LABELS_MAX;
      fadeTo(1);
    } else {
      fadeTo(0);
    }
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
  // sigma starts a drag on the first move after a press on a node and
  // moves the node through dragTo, which pins it in the simulation where
  // the cursor is. One rule tells a drag from a click: the press becomes a
  // drag once the pointer has moved DRAG_START_PX from where it went down.
  // Then the graph heats up, so the neighbours follow on their links, and
  // the release is no click. A press that stays within it moves nothing
  // and its release is a click (pressClick). The drop lets the node go, so
  // it springs back among its neighbours, as in Obsidian.
  function startDrag(id) {
    var a = shownGraph().getNodeAttributes(id);
    var press = S.press && S.press.node === id ? S.press : { node: id, moved: false };
    S.press = press;
    S.drag = { node: id, x: a.x, y: a.y, press: press };
    clearTimeout(S.hoverTimer);
  }

  function dragTo(p, id) {
    var d = phys.index[id];
    if (d) {
      d.fx = p.x;
      d.fy = p.y;
    }
    var drag = S.drag;
    if (drag && !drag.press.moved) {
      // sigma offsets the node by the pointer's travel, in graph units; one
      // graph unit is one pixel at camera ratio 1.
      var px = Math.hypot(p.x - drag.x, p.y - drag.y) / S.sigma.getCamera().ratio;
      if (px >= DRAG_START_PX) {
        drag.press.moved = true;
        // A drag also drops a waiting click, and a double click across it.
        S.lastClick = null;
        if (phys.sim) {
          phys.soothe(true);
          phys.sim.alphaTarget(phys.P.dragAlphaTarget);
          phys.reheat(phys.P.dragAlphaTarget);
        }
      }
    }
    return { x: p.x, y: p.y };
  }

  function endDrag(id) {
    var drag = S.drag;
    S.drag = null;
    if (phys.sim) phys.sim.alphaTarget(0);
    if (drag && drag.press.moved) {
      phys.unpin(id);
      if (phys.graph === S.global) S.layoutDirty = true;
      phys.reheat(phys.P ? phys.P.alphaMin * 2 : 0);
    }
    if (S.pointer) setHover(S.pointer);
    else lingerHover();
  }

  // pressClick handles the release of a press on a node. It is a click
  // when the press started on that node and never became a drag. A second
  // click on the same node within DOUBLE_CLICK_MS, timed from the clicks
  // themselves, opens the local graph; a lone click waits in S.lastClick
  // and acts once that window has passed (clickStep). sigma's own double
  // click is not used: it counts releases with a reset timer, so it counts
  // a drag's release as a first click, and a late timer makes two clicks
  // seconds apart a double click.
  function pressClick(id) {
    var press = S.press;
    S.press = null;
    if (!press || press.node !== id || press.moved) return;
    var now = performance.now();
    var last = S.lastClick;
    if (last && last.node === id && now - last.at <= DOUBLE_CLICK_MS) {
      S.lastClick = null;
      showLocal(id);
      return;
    }
    S.lastClick = { node: id, at: now };
    wake();
  }

  // clickStep runs on every frame while a lone click waits, and acts on it
  // once DOUBLE_CLICK_MS has passed; it reports whether one still waits.
  // It runs from the frame loop, not a timer: in headless Chrome, page
  // timers can stall for seconds while animation frames keep running.
  function clickStep(now) {
    var c = S.lastClick;
    if (!c) return false;
    if (now - c.at < DOUBLE_CLICK_MS) return true;
    S.lastClick = null;
    clickNode(c.node);
    return false;
  }

  function clickNode(id) {
    var n = nodeOf(id);
    if (!n) return;
    var kind = BG.nodeKind(n);
    if (kind === 'note') { openNote(BG.noteRef(n)); return; }
    if (kind === 'tag') {
      S.filters.tag = S.filters.tag === n.title ? '' : n.title;
      renderToolbar();
      applyVisibility(true);
      return;
    }
    toast('No note matches [[' + BG.nodeLabel(n) + ']] yet', 'warn');
  }

  // ── camera ──

  function camera(how) {
    if (!S.sigma) return;
    var cam = S.sigma.getCamera();
    if (how === 'in') cam.animatedZoom({ duration: 200 });
    else if (how === 'out') cam.animatedUnzoom({ duration: 200 });
    else fitCamera(true);
  }

  // fitCamera frames the visible nodes. With autoRescale off one graph
  // unit is one pixel at ratio 1, so the ratio is the visible box over the
  // viewport.
  function fitCamera(animate) {
    if (!S.sigma) return;
    var g = S.sigma.getGraph();
    var circles = [];
    g.forEachNode(function (id, a) { if (S.visible[id]) circles.push({ x: a.x, y: a.y, r: a.size }); });
    var b = BG.boundsOf(circles);
    var cam = S.sigma.getCamera();
    if (!b) { cam.setState({ x: 0.5, y: 0.5, ratio: 1, angle: 0 }); return; }
    var dim = S.sigma.getDimensions();
    if (!dim.width || !dim.height) return;
    var ratio = BG.fitRatio(b, dim, FIT_MARGIN, FIT_MIN_RATIO, MAX_CAMERA_RATIO);
    var c = S.sigma.graphToFramedGraph({ x: (b.minX + b.maxX) / 2, y: (b.minY + b.maxY) / 2 });
    var state = { x: c.x, y: c.y, ratio: ratio, angle: 0 };
    if (animate) cam.animate(state, { duration: FIT_MS, easing: 'quadraticInOut' });
    else cam.setState(state);
  }

  // ── visibility: filters and search ──

  function filterSpec() {
    return {
      vault: S.vault, tier: S.filters.tier, tag: S.filters.tag, status: S.filters.status,
      orphans: S.orphans, dangling: S.dangling,
    };
  }

  function computeVisible() {
    S.visible = BG.restrictTo(BG.visibleIds(S.data.nodes, S.data.edges, filterSpec()), S.match);
  }

  // shownIds lists the nodes of g the simulation moves: the visible ones.
  function shownIds(g) {
    var out = [];
    g.forEachNode(function (id) { if (S.visible[id]) out.push(id); });
    return out;
  }

  // applyVisibility redraws after a filter or the search changed: the
  // simulation runs over the nodes still shown, warm for a moment so the
  // change shows at once, then live; the camera frames the result. A run on
  // the global graph moves its nodes, so the layout is posted again once a
  // full run rests (cooled).
  function applyVisibility(fit) {
    if (!S.data) return;
    computeVisible();
    renderFilterCount();
    var g = shownGraph();
    if (g) {
      if (g === S.global) S.layoutDirty = true;
      phys.start(g, shownIds(g), phys.P ? phys.P.reheatAlpha : 0.3, RESTYLE_WARM_MS);
      refreshGraph();
      if (fit) fitCamera(true);
    }
    renderStatus();
  }

  function positionsOf(g) {
    var out = {};
    g.forEachNode(function (id, a) { out[id] = [a.x, a.y]; });
    return out;
  }

  // ── layout ──

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

  // loadGraph follows the cache-miss steps of docs/serve.md: positions the
  // server returned or the page last drew, new nodes placed near their
  // neighbours, d3-force warm before the first draw, then live, and the
  // POST once it comes to rest.
  async function loadGraph() {
    var gen = ++S.gen;
    if (S.global) S.prev[S.globalFlag] = positionsOf(S.global);
    var data = await getJSON('/api/graph' + query({ topics: S.topics ? 1 : 0 }));
    if (gen !== S.gen) return;
    // The old global layout finishes first: posted if it just came to
    // rest, its alpha kept in S.layoutHeat if it was still moving.
    phys.retire();
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
    S.globalIds = BG.visibleIds(data.nodes, data.edges, BG.DEFAULT_FILTERS);
    groupIslands(data, known);
    var g = buildGraph(data, BG.placeFresh(fresh, S.adj, known, S.homes));
    var cached = !!(data.layout && data.layout.cached);
    S.global = g;
    S.globalFlag = flag;
    S.layoutDirty = !(cached && !fresh.length);
    S.layoutNote = cached && !fresh.length ? 'layout cached' : fresh.length ? 'placed ' + fresh.length + ' new' : '';
    S.selected = S.noteRef ? S.refs[S.noteRef] || null : null;
    computeVisible();
    renderToolbar();
    renderLegend();
    showEmpty(data.nodes.length ? null : S.vaults.length ? 'No notes in the configured vaults yet.' : null);
    var first = !S.sigma;
    var center = S.view === 'local' ? S.centerId || resolveRef(S.centerRef) : null;
    var alpha = BG.startAlpha(data.nodes.length, fresh.length, cached);
    // A cached layout rests; positions the page last drew may not have.
    S.layoutHeat = cached ? alpha : Math.max(alpha, S.layoutHeat);
    if (center && g.hasNode(center)) {
      // The local graph starts from the global positions, so the global
      // layout warms first; the rest of its run waits in S.layoutHeat.
      phys.start(g, shownIds(g), alpha, BG.simParams().warmMs);
      showLocal(center, !first);
    } else {
      showGlobal(!first, alpha);
    }
  }

  // groupIslands gives every node of data its community and its island
  // (S.community, S.homes) before the graph is built (docs/serve.md,
  // Layout). The communities come from the first of: the last load's, so
  // a poll moves nothing and a new note joins its neighbours; the islands
  // the known positions show, so a reload keeps them even when Louvain
  // would now split the graph another way; Louvain over the notes the
  // default filters show. Tags and dangling targets join the community
  // most of their neighbours are in. Islands with placed members stay
  // where those members are.
  function groupIslands(data, known) {
    var P = BG.simParams(true);
    var byId = {};
    data.nodes.forEach(function (n) { byId[n.id] = n; });
    function radiusOf(id) { return BG.nodeRadius(byId[id], S.maxDeg, S.byDegree); }
    var notes = data.nodes.filter(function (n) {
      return S.globalIds[n.id] && BG.nodeKind(n) === 'note';
    }).map(function (n) { return n.id; });
    var set = {};
    notes.forEach(function (id) { set[id] = true; });
    var previous = S.community;
    if (!previous) {
      var placed = {};
      notes.forEach(function (id) { if (known[id]) placed[id] = known[id]; });
      if (Object.keys(placed).length) previous = BG.islandsFromLayout(placed, radiusOf, P);
    }
    performance.mark('brain-communities-start');
    var community = BG.communities(notes, BG.simLinks(data.edges, set), louvain, previous);
    performance.clearMeasures('brain-communities');
    performance.measure('brain-communities', 'brain-communities-start');
    var rest = data.nodes.filter(function (n) { return community[n.id] === undefined; }).map(function (n) { return n.id; });
    BG.joinCommunities(rest, S.adj, community).forEach(function (id) { community[id] = BG.LOOSE; });
    S.community = community;
    S.homes = BG.islandHomes(community, radiusOf, known, P, S.globalIds);
  }

  // louvain answers each of ids a community label over links: the
  // bundle's Louvain with a fixed seed, so one graph splits one way.
  function louvain(ids, links) {
    var g = new B.Graph({ type: 'undirected' });
    ids.forEach(function (id) { g.addNode(id); });
    links.forEach(function (l) { if (!g.hasEdge(l.source, l.target)) g.addEdge(l.source, l.target); });
    if (!g.size) return {};
    return B.louvain(g, { rng: BG.seededRandom(BG.COMMUNITY_SEED) });
  }

  function resolveRef(ref) {
    if (!ref) return null;
    if (S.refs[ref]) return S.refs[ref];
    return S.global && S.global.hasNode(ref) ? ref : null;
  }

  // showGlobal draws every node. keepCamera holds the view on a refetch;
  // alpha is how hot the layout starts (0 when the cached layout stands).
  // Coming back from a local graph restarts the global simulation, which
  // picks up the run S.layoutHeat says the layout still needs.
  function showGlobal(keepCamera, alpha) {
    S.view = 'global';
    S.centerId = null;
    if (!S.global) return;
    var g = S.global;
    if (phys.graph !== g || alpha) phys.start(g, shownIds(g), alpha || 0, BG.simParams().warmMs);
    ensureSigma(g);
    if (keepCamera !== true) fitCamera(false);
    syncURL();
    renderToolbar();
    renderStatus();
  }

  // showLocal draws the nodes within depth hops of id as their own graph,
  // starting from their global positions, warmed so it opens settled. A
  // local layout is never posted: the cache holds the global one.
  function showLocal(id, keepCamera) {
    if (!S.global || !S.global.hasNode(id)) return;
    S.view = 'local';
    S.centerId = id;
    S.lastNode = id;
    var dist = BG.depthSearch(S.adj, id, S.depth);
    var lg = new B.Graph({ multi: true, type: 'directed', allowSelfLoops: false });
    Object.keys(dist).forEach(function (nid) {
      if (S.global.hasNode(nid)) lg.addNode(nid, Object.assign({}, S.global.getNodeAttributes(nid)));
    });
    S.global.forEachEdge(function (key, a, s, t) {
      if (lg.hasNode(s) && lg.hasNode(t)) lg.addDirectedEdgeWithKey(key, s, t, Object.assign({}, a));
    });
    phys.start(lg, shownIds(lg), 0.8, BG.simParams().warmMs);
    ensureSigma(lg);
    if (keepCamera !== true) fitCamera(false);
    syncURL();
    renderToolbar();
    renderStatus();
  }

  function renderStatus() {
    if (!S.data) { setStatus(''); return; }
    var g = shownGraph();
    var inView = {};
    if (g) g.forEachNode(function (id) { if (S.visible[id]) inView[id] = true; });
    var shown = BG.countNotes(S.data.nodes, inView);
    var all = {};
    S.data.nodes.forEach(function (n) { all[n.id] = true; });
    var notes = BG.countNotes(S.data.nodes, all);
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
    var s = el('select', { 'aria-label': label, title: label, onchange: function () { onChange(s.value); } },
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
    renderViewGroup();
    renderFilters();
  }

  function renderViewGroup() {
    viewGroup.innerHTML = '';
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
    viewGroup.append(
      seg([{ value: 'global', label: 'Global' }, { value: 'local', label: 'Local' }], S.view, function (v) {
        var around = S.centerId || S.selected || S.lastNode;
        if (v === 'global') showGlobal();
        else if (around) showLocal(around);
        else toast('Click a note first, then show its local graph', 'warn');
      }),
      el('label', { class: S.view === 'local' ? '' : 'off', title: 'Hops around the note in the local graph' }, ['Depth', depth, depthOut]),
    );
  }

  // renderFilters fills the Filters popover: the vault and colour, the
  // tier, tag and status filters, then the graph toggles.
  function renderFilters() {
    filterPanel.innerHTML = '';
    var fac = S.data ? BG.facets(S.data.nodes) : { tiers: [], tags: [], statuses: [] };
    var total = S.vaults.reduce(function (s, v) { return s + (v.notes || 0); }, 0);
    var vaults = [{ value: '', label: 'All vaults (' + total + ')' }].concat(S.vaults.map(function (v) {
      var marks = [v.shared ? 'shared' : '', v.default ? 'default' : ''].filter(Boolean).join(', ');
      return { value: v.name, label: v.name + ' (' + (v.notes || 0) + (marks ? ', ' + marks : '') + ')' };
    }));
    var colors = BG.COLOR_MODES.map(function (m) { return { value: m, label: m }; });
    function row(label, control) { return [el('span', {}, label), control]; }
    var grid = el('div', { class: 'f-grid' }, [].concat(
      row('Vault', select('Vault', S.vault, vaults, pickVault)),
      row('Color', select('Color by', S.colorBy, colors, function (v) {
        S.colorBy = v;
        syncURL();
        restyleShown();
        renderLegend();
      })),
      row('Tier', select('Tier', S.filters.tier, anyOf('any tier', fac.tiers), filterSetter('tier'))),
      row('Tag', select('Tag', S.filters.tag, anyOf('any tag', withCurrent(fac.tags, S.filters.tag)), filterSetter('tag'))),
      row('Status', select('Status', S.filters.status, anyOf('any status', fac.statuses), filterSetter('status'))),
    ));
    var toggles = el('div', { class: 'f-toggles' }, [
      check('Orphans', S.orphans, function (on) { S.orphans = on; applyVisibility(true); }, 'Notes with no links'),
      check('Dangling', S.dangling, function (on) {
        S.dangling = on;
        applyVisibility(true);
        renderLegend();
      }, 'Link targets no note matches'),
      check('Size by links', S.byDegree, function (on) {
        S.byDegree = on;
        restyleShown();
        applyVisibility(false);
      }),
      check('Topics', S.topics, function (on) {
        S.topics = on;
        syncURL();
        loadGraph().catch(showLoadError);
      }, 'A node per tag, linked to its notes'),
    ]);
    filterPanel.append(grid, toggles);
    renderFilterCount();
  }

  // renderFilterCount shows on the Filters button how many filters are set
  // away from their defaults.
  function renderFilterCount() {
    var n = BG.activeFilters(filterSpec());
    filterCount.textContent = String(n);
    if (n) filterCount.removeAttribute('hidden');
    else filterCount.setAttribute('hidden', '');
  }

  // restyleShown restyles the global graph and, in a local view, the
  // local copy sigma draws.
  function restyleShown() {
    if (S.global) restyle(S.global);
    var g = shownGraph();
    if (g && g !== S.global) restyle(g);
    refreshGraph();
  }

  function withCurrent(values, current) {
    return current && values.indexOf(current) < 0 ? values.concat([current]) : values;
  }

  function filterSetter(name) {
    return function (v) {
      S.filters[name] = v;
      applyVisibility(true);
    };
  }

  function pickVault(name) {
    S.vault = name;
    syncURL();
    applyVisibility(true);
    if (S.query.trim()) runSearch();
  }

  function renderLegend() {
    legendEl.innerHTML = '';
    if (!S.data || !S.data.nodes.length) return;
    var shows = Object.assign(ctx(), { topics: S.topics, dangling: S.dangling });
    BG.legend(S.colorBy, S.data.nodes, shows).forEach(function (e) {
      if (!e.role) { legendEl.appendChild(el('span', {}, e.label)); return; }
      var swatch = 'background:' + color(e.role) + ';border-color:' + color(BG.borderRole(e.role));
      legendEl.appendChild(el('span', {}, [el('i', { style: swatch }), e.label]));
    });
    var kinds = {};
    S.data.edges.forEach(function (e) { kinds[e.kind] = true; });
    BG.EDGE_KINDS.filter(function (k) { return kinds[k] && BG.edgeRole(k) !== 'edge'; }).forEach(function (k) {
      legendEl.appendChild(el('span', {}, [el('i', { class: 'line', style: 'background:' + color(BG.edgeRole(k)) }), k]));
    });
  }

  // ── search: reduces the graph to the notes it finds ──

  var searchTimer = null;
  var searchCount = el('span', { class: 'search-n', 'aria-live': 'polite' });
  var searchInput = el('input', {
    type: 'search', placeholder: 'Search notes', 'aria-label': 'Search notes',
    oninput: function () {
      S.query = searchInput.value;
      clearTimeout(searchTimer);
      searchTimer = setTimeout(runSearch, SEARCH_DEBOUNCE_MS);
    },
    onkeydown: function (e) {
      if (e.key === 'Enter') { clearTimeout(searchTimer); runSearch(); }
      if (e.key === 'Escape') {
        e.preventDefault();
        clearTimeout(searchTimer);
        searchInput.value = '';
        S.query = '';
        runSearch();
      }
    },
  });

  // setSearchCount shows the search's result line; a long one is cut to
  // fit the toolbar row and reads in full on hover.
  function setSearchCount(text) {
    searchCount.textContent = text;
    searchCount.title = text;
  }

  function renderSearch() {
    searchGroup.innerHTML = '';
    searchGroup.append(
      searchInput,
      seg([{ value: 'literal', label: 'Literal' }, { value: 'hybrid', label: 'Hybrid' }], S.searchMode, function (m) {
        S.searchMode = m;
        renderSearch();
        if (S.query.trim()) runSearch();
      }),
      searchCount,
    );
  }

  var searchSeq = 0;
  async function runSearch() {
    var q = S.query.trim();
    var seq = ++searchSeq;
    if (!q) {
      setSearchCount('');
      if (S.match) {
        S.match = null;
        applyVisibility(true);
      }
      return;
    }
    if (!S.data) return;
    var url = '/api/search' + query({ q: q, mode: S.searchMode, vault: S.vault, limit: SEARCH_LIMIT[S.searchMode] });
    var res;
    try {
      res = await getJSON(url);
    } catch (err) {
      if (seq !== searchSeq) return;
      setSearchCount(err.code === 'semantic_unavailable'
        ? 'Hybrid needs a semantic index; Literal still works'
        : 'Search failed: ' + errText(err));
      return;
    }
    if (seq !== searchSeq) return;
    S.match = BG.searchHits(res, S.refs);
    var warn = (res.warnings || []).length ? ' (' + res.warnings.join('; ') + ')' : '';
    applyVisibility(true);
    var inView = 0;
    S.match.forEach(function (id) { if (S.visible[id]) inView++; });
    setSearchCount((inView === 1 ? '1 match' : inView + ' matches') + warn);
  }

  // ── review: the queues behind one button ──

  function renderReviewCount() {
    var n = BG.reviewCount(S.queues);
    reviewCount.textContent = String(n);
    if (n) reviewCount.removeAttribute('hidden');
    else reviewCount.setAttribute('hidden', '');
  }

  function openReview() {
    S.popup = 'review';
    S.noteSeq++;
    S.noteRef = '';
    setSelected(null);
    syncURL();
    renderReview();
    openPopup();
  }

  // renderReview draws the review popup; keepScroll holds the reader's
  // place when the poll redraws it.
  function renderReview(keepScroll) {
    var q = S.queues;
    function count(v) { return v && !v.error ? ' ' + v.count : ''; }
    var kids = [
      popHead('Review', ''),
      seg([
        { value: 'promotions', label: 'Promotions' + count(q.promotions) },
        { value: 'stale', label: 'Stale' + count(q.stale) },
        { value: 'check', label: 'Check' + count(q.check) },
      ], S.queue, function (v) { S.queue = v; renderReview(); }),
    ];
    var cur = q[S.queue];
    if (!cur) kids.push(el('div', { class: 'hint' }, 'Loading…'));
    else if (cur.error) kids.push(el('div', { class: 'hint' }, 'Could not load: ' + errText(cur.error)));
    else kids.push(({ promotions: promotionsView, stale: staleView, check: checkView })[S.queue](cur.data));
    setPopContent(kids, keepScroll);
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

  // loadQueues keeps, per queue, count (what its tab shows) and actionable
  // (what the Review button adds up): every check finding shows in the
  // Check tab, but only its errors count on the button.
  async function loadQueues() {
    function settle(name, url, count, actionable, shape) {
      return getJSON(url).then(function (data) {
        var d = shape ? shape(data) : data;
        S.queues[name] = { data: d, count: count(d), actionable: actionable(d) };
      }, function (err) { S.queues[name] = { error: err }; });
    }
    function promotions(d) { return (d.promotions || []).length; }
    function stale(d) { return d.total || 0; }
    await Promise.all([
      settle('promotions', '/api/promotions', promotions, promotions),
      settle('stale', '/api/stale', stale, stale),
      settle('check', '/api/check', function (d) { return (d.errors || 0) + (d.warnings || 0); },
        function (d) { return d.errors || 0; }, mergeChecks),
    ]);
    renderReviewCount();
    if (S.popup === 'review' && pop.open) renderReview(true);
  }

  // ── the popup ──
  // A modal dialog: the browser keeps focus inside it and the graph inert
  // behind it. Esc, the close button and a click on the dimmed backdrop
  // close it, and focus goes back to the graph. The browser closes it on
  // Esc by itself, and Chrome can do so without a cancel event (on a page
  // with no user activation yet, such as a ?note= link), so the cleanup
  // runs from the close event, which every way of closing fires.

  pop.addEventListener('close', popupClosed);
  // A backdrop click closes the popup only when the press began on the
  // backdrop too: a text selection dragged out past the panel's edge ends
  // with a click on the dialog outside its box.
  var backdropPress = false;
  pop.addEventListener('pointerdown', function (e) { backdropPress = onBackdrop(e); });
  pop.addEventListener('click', function (e) {
    var pressed = backdropPress;
    backdropPress = false;
    if (pressed && onBackdrop(e)) closePopup();
  });

  function onBackdrop(e) {
    if (e.target !== pop) return false;
    var r = pop.getBoundingClientRect();
    return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
  }

  function openPopup() {
    // The graph goes inert behind the dialog and sees no pointer leave, so
    // a hover drawn on the clicked node ends here; its selected halo stays.
    if (S.hovered || S.focus || S.hoverTimer) {
      phys.unpin(phys.pinned);
      clearHover();
      refreshGraph();
    }
    if (!pop.open) pop.showModal();
    popScroll.scrollTop = 0;
    popScroll.focus({ preventScroll: true });
  }

  function closePopup() {
    if (pop.open) pop.close();
    popupClosed();
  }

  // popupClosed drops what the popup showed: the note in the URL, the
  // selected halo and any note still loading. It runs from closePopup and
  // again from the close event after it, so a second run does nothing,
  // and a popup opened again before that event arrives stays as it is.
  function popupClosed() {
    if (pop.open || !S.popup) return;
    S.noteSeq++;
    S.popup = '';
    S.noteRef = '';
    setSelected(null);
    syncURL();
    sigmaBox.focus({ preventScroll: true });
  }

  // setPopContent replaces what the popup shows. keepScroll holds the
  // scroll position, and focus inside the panel, across the redraw.
  function setPopContent(kids, keepScroll) {
    var top = popScroll.scrollTop;
    var focused = keepScroll && popScroll.contains(document.activeElement);
    popScroll.innerHTML = '';
    popScroll.append.apply(popScroll, kids.filter(Boolean));
    popScroll.scrollTop = keepScroll ? top : 0;
    if (focused && !popScroll.contains(document.activeElement)) popScroll.focus({ preventScroll: true });
  }

  function popHead(title, sub) {
    return el('div', {}, [
      el('div', { class: 'pop-head' }, [
        el('h2', { class: 'pop-title', id: 'pop-title' }, title),
        el('button', { class: 'btn-x', type: 'button', 'aria-label': 'Close', title: 'Close (Esc)', onclick: closePopup }, '✕'),
      ]),
      sub ? el('div', { class: 'note-ref' }, sub) : null,
    ].filter(Boolean));
  }

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

  popScroll.addEventListener('click', function (e) {
    var a = e.target.closest('a.wikilink[data-ref]');
    if (a) { e.preventDefault(); openNote(a.dataset.ref); return; }
    var tag = e.target.closest('wk-badge[data-tag]');
    if (tag) {
      S.filters.tag = tag.dataset.tag;
      closePopup();
      renderToolbar();
      applyVisibility(true);
    }
  });

  // openNote shows ref in the popup. quiet is the poll's call after the
  // vault changed: when the popup already shows ref it reads the note
  // again in place, with no Loading state and the reader's scroll and
  // focus kept.
  async function openNote(ref, quiet) {
    if (!ref) return;
    var refresh = !!quiet && pop.open && S.popup === 'note' && S.noteRef === ref;
    var seq = ++S.noteSeq;
    S.popup = 'note';
    S.noteRef = ref;
    setSelected(S.refs[ref] || null);
    syncURL();
    if (!refresh) {
      setPopContent([popHead(ref, ''), el('div', { class: 'hint' }, 'Loading…')]);
      openPopup();
    }
    var got;
    try {
      got = await Promise.all([
        getJSON('/api/note' + query({ ref: ref })),
        getJSON('/api/backlinks' + query({ ref: ref })).catch(function () { return null; }),
      ]);
    } catch (err) {
      if (seq !== S.noteSeq) return;
      setPopContent([popHead(ref, ''), el('wk-callout', { variant: 'error' }, err.status === 404 ? 'No note ' + ref + '.' : 'Could not read ' + ref + ': ' + errText(err))]);
      return;
    }
    if (seq !== S.noteSeq) return;
    var n = got[0];
    setSelected(S.refs[n.ref] || S.refs[n.vault + ':' + n.path] || S.selected);
    renderNote(n, got[1], refresh);
  }

  function renderNote(n, back, keepScroll) {
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
      actions.push(el('wk-button', { variant: 'ghost', onclick: function () { closePopup(); showLocal(id); } }, 'Local graph'));
    }
    var kids = [popHead(fm.title || basename(n.path), n.vault + ':' + n.path), el('div', { class: 'note-meta' }, meta)];
    if (actions.length) kids.push(el('div', { class: 'note-actions' }, actions));
    if (n.banner) kids.push(el('wk-callout', { variant: 'warn' }, n.banner));
    kids.push(el('div', { class: 'wk-prose note-body', html: md.render(n.body || '', { links: BG.linkIndex(n.links) }) }));
    kids.push(frontmatterView(fm));
    if (back) kids.push(backlinksView(back));
    setPopContent(kids, keepScroll);
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

  // titleFor uses the graph's title for a note when the graph has it.
  function titleFor(vault, path) {
    var id = S.refs[vault + ':' + path];
    var n = id ? nodeOf(id) : null;
    return (n && n.title) || basename(path);
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

  async function loadVaults() {
    var res = await getJSON('/api/vaults');
    S.vaults = res.vaults || [];
    S.vaultNames = S.vaults.map(function (v) { return v.name; });
    if (S.vault && S.vaultNames.indexOf(S.vault) < 0) S.vault = '';
    renderFilters();
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

  async function loadAll() {
    await loadVaults();
    var work = [loadQueues()];
    if (S.vaults.length) work.push(loadGraph());
    await Promise.all(work);
    if (S.query.trim()) runSearch();
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
  renderToolbar();
  renderReviewCount();
  Webkit.poll('/api/state', onState, POLL_MS, function (err) {
    setStatus('brain serve is unreachable: ' + errText(err));
  });
})();
