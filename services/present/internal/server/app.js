'use strict';
// present page view — client-side render. The backend serves a static shell
// (chrome only) plus JSON at /api/p/{id}; this script fetches the page data and
// builds the DOM. The Doc->HTML compile happens once at authoring time (Go), so
// `content` (the brief) and `deck` (the slide deck) are trusted authored HTML
// we mount as-is; the Cytoscape graph (plus its edge-flow animation), metric
// charts, references, and read-aloud are initialized client-side. The same
// shell serves both renditions: /p/{id} mounts the brief, /p/{id}/deck splits
// the deck into slides (renderDeck below). webkit.js loads before this file,
// so Webkit.el/escapeHtml/poll and the wk-* custom elements are available.
(function () {
  // ── Graph layout engines ──
  // The layout option objects live here rather than in the generated graph
  // script: the ELK ones take the container's aspect ratio at run time (the
  // adapter passes cy.width()/cy.height() as elk.aspectRatio), and the engine
  // control in setupGraphControls rebuilds them for any engine on any page.
  // The generated initGraph() calls presentGraphLayout(engine, direction)
  // with the engine the page was authored with and the direction the server
  // resolved ('LR' or 'TB'). 'elk' is ELK layered with wrapping, which folds
  // a long chain of layers into rows until the drawing approaches the
  // container's aspect ratio; the elk-<algorithm> engines pick another ELK
  // algorithm without wrapping. The order here is the cycle order of the
  // engine control.
  var GRAPH_ENGINES = ['dagre', 'elk', 'elk-layered', 'elk-mrtree', 'elk-stress', 'elk-radial', 'elk-force', 'cose'];

  function presentGraphLayout(engine, direction) {
    var dir = direction === 'LR' ? 'LR' : 'TB';
    var opts;
    if (engine === 'cose') {
      opts = { name: 'cose', padding: 30, nodeRepulsion: 8000 };
    } else if (engine === 'elk' || (engine || '').indexOf('elk-') === 0) {
      opts = elkLayout(engine, dir);
    } else {
      engine = 'dagre';
      // Gaps tuned for the 140x50 node boxes: nodeSep separates siblings
      // within a rank, rankSep separates ranks (where edges and labels run).
      opts = {
        name: 'dagre', rankDir: dir,
        nodeSep: dir === 'LR' ? 20 : 25, rankSep: dir === 'LR' ? 80 : 60, edgeSep: 10,
        padding: 30, nodeDimensionsIncludeLabels: true
      };
    }
    // Stamped so the shim and the engine control can read back what ran.
    opts.presentEngine = engine;
    opts.presentDirection = dir;
    return opts;
  }

  function elkLayout(engine, dir) {
    var algorithm = engine === 'elk' ? 'layered' : engine.slice(4);
    var elk = {
      algorithm: algorithm,
      'elk.direction': dir === 'LR' ? 'RIGHT' : 'DOWN',
      'elk.spacing.nodeNode': 25
    };
    if (algorithm === 'layered') {
      elk['elk.layered.spacing.nodeNodeBetweenLayers'] = dir === 'LR' ? 80 : 60;
      elk['elk.spacing.edgeNode'] = 20;
    }
    if (engine === 'elk') {
      elk['elk.layered.wrapping.strategy'] = 'MULTI_EDGE';
      elk['elk.layered.wrapping.additionalEdgeSpacing'] = 20;
    }
    if (algorithm === 'stress') elk['elk.stress.desiredEdgeLength'] = 200;
    if (algorithm === 'force') elk['elk.spacing.nodeNode'] = 60;
    return { name: 'elk', padding: 30, nodeDimensionsIncludeLabels: true, elk: elk };
  }
  window.presentGraphLayout = presentGraphLayout;

  // ── Graph style ──
  // The page's graph script carries data only (elements, engine, direction);
  // the Cytoscape style is built here at load, so a palette, box, or tone
  // change reaches every stored graph the next time it is opened, with no
  // rerender. getGraphColors reads the palette roles at call time and the
  // page rebuilds the instance on wk-themechange, which fires on the toggle,
  // on a family change made on the themes page in another tab, and on a
  // system appearance change. graph.go validates tones and module colors
  // against the same lists (graphTones, moduleColors).
  var GRAPH_TONES = ['neutral', 'green', 'red', 'blue', 'amber', 'purple'];
  var MODULE_COLORS = 4;
  function cssVar(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }
  // Every colour is a palette role read from the stylesheet at call time, so
  // the graph follows the family the reader picked (webkit emits each role
  // as a literal hex, which Cytoscape needs; a var() would not resolve).
  function role(name) {
    return cssVar('--' + name, '#808080');
  }
  function getGraphColors() {
    var tones = {};
    GRAPH_TONES.forEach(function (t) {
      tones[t] = { bg: role('tone-' + t + '-bg'), border: role('tone-' + t + '-border'), text: role('tone-' + t + '-text') };
    });
    var modBorder = [];
    for (var i = 1; i <= MODULE_COLORS; i++) modBorder.push(role('graph-module-' + i));
    return {
      bg: role('graph-bg'),
      centerBg: role('graph-center-bg'), centerBorder: role('graph-center-border'), centerText: role('graph-center-text'),
      leafBg: role('graph-leaf-bg'), leafBorder: role('graph-leaf-border'), leafText: role('graph-leaf-text'),
      modBorder: modBorder,
      edge: role('graph-edge'), edgeArrow: role('graph-edge-arrow'), hot: role('graph-hot'),
      registryBg: role('graph-registry-bg'), registryBorder: role('graph-registry-border'), registryText: role('graph-registry-text'),
      tones: tones
    };
  }
  // Elements come as the array graph.go emits or as {nodes, edges}.
  function elementList(elements) {
    if (Array.isArray(elements)) return elements;
    return [].concat((elements && elements.nodes) || [], (elements && elements.edges) || []);
  }
  function presentGraphStyle(elements) {
    var c = getGraphColors();
    var maxW = 0;
    elementList(elements).forEach(function (e) {
      var w = e && e.data && e.data.weight;
      if (typeof w === 'number' && w > maxW) maxW = w;
    });
    var style = [
      { selector: 'node', style: {
          'label': 'data(label)', 'text-wrap': 'wrap', 'text-max-width': '120px',
          'font-size': '12px', 'text-valign': 'center', 'text-halign': 'center',
          // The height follows the wrapped label so a long name grows the
          // box instead of spilling past it; the width stays fixed so the
          // layouts keep their even columns. Body plus padding is 144px wide.
          'width': '120px', 'height': 'label', 'padding': '12px', 'shape': 'roundrectangle',
          'background-color': c.leafBg, 'border-width': 2,
          'border-color': c.leafBorder, 'color': c.leafText
      }},
      { selector: 'node[type="center"]', style: {
          'background-color': c.centerBg, 'border-color': c.centerBorder,
          'border-width': 3, 'color': c.centerText, 'font-weight': 'bold'
      }},
      { selector: 'node[type="registry"]', style: {
          'background-color': c.registryBg, 'border-color': c.registryBorder,
          'border-style': 'dashed', 'color': c.registryText
      }}
    ];
    for (var i = 0; i < MODULE_COLORS; i++) {
      style.push({ selector: 'node[type="module"][color="' + i + '"]', style: { 'border-color': c.modBorder[i] } });
    }
    // A tone comes after the type selectors so it wins over their colors.
    GRAPH_TONES.forEach(function (t) {
      style.push({ selector: 'node[tone="' + t + '"]', style: {
        'background-color': c.tones[t].bg, 'border-color': c.tones[t].border, 'color': c.tones[t].text } });
    });
    style.push(
      { selector: 'edge', style: {
          'width': 2, 'line-color': c.edge, 'target-arrow-color': c.edgeArrow,
          'target-arrow-shape': 'triangle', 'curve-style': 'bezier',
          'arrow-scale': 0.8
      }},
      { selector: 'edge[type="publishes"]', style: { 'line-style': 'dashed' } },
      // startGraphFlow marches line-dash-offset over this pattern's period
      // (16), so keep the two in step.
      { selector: 'edge[flow]', style: { 'line-style': 'dashed', 'line-dash-pattern': [10, 6] } }
    );
    if (maxW > 0) {
      style.push(
        { selector: 'edge[weight]', style: { 'width': 'mapData(weight, 0, ' + maxW + ', 1.5, 7)' } },
        { selector: 'edge[hot]', style: { 'line-color': c.hot, 'target-arrow-color': c.hot } }
      );
    }
    style.push({ selector: 'edge[label]', style: {
        'label': 'data(label)', 'font-size': '10px', 'color': c.leafText,
        'text-background-color': c.bg, 'text-background-opacity': 0.8,
        'text-background-padding': '2px'
    }});
    return style;
  }
  window.presentGraphStyle = presentGraphStyle;

  // The engine the reader picked with the graph control; null means the
  // page's authored engine. It outlives a theme recolor, which rebuilds the
  // Cytoscape instance through the shim below.
  var engineOverride = null;
  // The layout options the page's graph script passed to cytoscape(), for
  // the engine and direction it was authored with. A page rendered before
  // the engines moved here carries a dagre or cose literal instead.
  var baseLayout = null;
  function layoutDirection(l) { return (l && (l.presentDirection || l.rankDir)) === 'LR' ? 'LR' : 'TB'; }
  function layoutEngine(l) { return (l && (l.presentEngine || l.name)) || 'dagre'; }
  function currentEngine() { return engineOverride || layoutEngine(baseLayout); }
  // The toolbar is built before the page's graph script runs, so the label
  // is set again once initGraph has passed its layout through the shim.
  function showEngine() {
    var btn = document.querySelector('.cy-engine');
    if (btn) btn.textContent = currentEngine();
  }

  // Capture the Cytoscape instance the injected graph script creates, give it
  // the current style, apply the reader's engine choice, and make the dagre
  // and elk layout registrations explicit/idempotent (same shim the old
  // template carried inline). The style is filled in when the script passes
  // none (data-only scripts) and replaced when the script's layout came from
  // presentGraphLayout: that script was rendered by an earlier template with
  // the style inline, and the current one supersedes it. A raw JS graph with
  // its own style and layout is left alone.
  (function () {
    var orig = window.cytoscape;
    if (!orig) return;
    if (typeof orig.use === 'function') {
      if (window.cytoscapeDagre) { try { orig.use(window.cytoscapeDagre); } catch (e) {} }
      if (window.cytoscapeElk) { try { orig.use(window.cytoscapeElk); } catch (e) {} }
    }
    window._cyInstance = null;
    var wrapped = function () {
      var opts = arguments[0];
      var isPage = !!(opts && opts.container);
      if (isPage && (!opts.style || (opts.layout && opts.layout.presentEngine))) {
        opts.style = presentGraphStyle(opts.elements);
      }
      if (isPage && opts.layout) {
        baseLayout = opts.layout;
        if (engineOverride) opts.layout = presentGraphLayout(engineOverride, layoutDirection(baseLayout));
      }
      var cy = orig.apply(this, arguments);
      if (isPage) window._cyInstance = cy;
      return cy;
    };
    for (var k in orig) if (orig.hasOwnProperty(k)) wrapped[k] = orig[k];
    window.cytoscape = wrapped;
  })();

  // webkit.css only neutralises CSS animations under Reduce Motion; the JS
  // loops here (graph flow, chart entry animation) must check it themselves.
  var reducedMotion = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);

  // ── Metric charts (Chart.js) ──
  // Each {t:"chart"} Doc block renders a .present-chart div carrying a JSON spec
  // in a <script type="application/json">. We read every block, build a Chart.js
  // instance into its canvas, and rebuild on theme change so colors track the
  // palette — mirroring how the Cytoscape graph recolors. Kinds group into
  // three families: cartesian (bar, line, area, sparkline, stacked-bar,
  // horizontal-bar, scatter), radial (doughnut), and sankey (needs the vendored
  // chartjs-chart-sankey plugin).
  var CHART_SERIES = 4;
  // Palette roles, read at call time like the graph's: series-1 to series-4,
  // the grid and text inks, and the surface for the gaps between stacked
  // segments and slices.
  function getChartColors() {
    var series = [];
    for (var i = 1; i <= CHART_SERIES; i++) series.push(role('series-' + i));
    return { series: series, grid: role('chart-grid'), text: role('chart-text'), label: role('chart-label'), surface: role('card-bg') };
  }
  // A stored spec names a series colour by its legacy name (terracotta, blue,
  // green, purple, the default family's first four) or by its role
  // (series-1 to series-4); both land on the same index in every family.
  var SERIES_NAMES = { terracotta: 0, blue: 1, green: 2, purple: 3, 'series-1': 0, 'series-2': 1, 'series-3': 2, 'series-4': 3 };
  function chartSeriesColor(colors, name, i) {
    var idx = (name && Object.prototype.hasOwnProperty.call(SERIES_NAMES, name)) ? SERIES_NAMES[name] : (i % colors.series.length);
    return colors.series[idx];
  }
  function axisTitle(colors, text) {
    return { display: !!text, text: text || '', color: colors.text, font: { size: 11 } };
  }
  function entryAnimation(animate) {
    return animate ? { duration: 700, easing: 'easeOutQuart' } : false;
  }
  function legendLabels(colors) {
    return { color: colors.text, boxWidth: 12, boxHeight: 12, font: { size: 11 } };
  }
  function chartTypeFor(kind) {
    if (kind === 'bar' || kind === 'stacked-bar' || kind === 'horizontal-bar') return 'bar';
    if (kind === 'scatter') return 'scatter';
    return 'line'; // line, area, sparkline
  }
  function cartesianOptions(kind, spec, colors, animate) {
    var spark = kind === 'sparkline';
    var multi = (spec.series || []).length > 1;
    var horizontal = kind === 'horizontal-bar';
    var stacked = kind === 'stacked-bar';
    var scatter = kind === 'scatter';
    var valueAxis = horizontal ? 'x' : 'y', categoryAxis = horizontal ? 'y' : 'x';
    var scales = {};
    scales[categoryAxis] = {
      display: !spark, stacked: stacked,
      grid: { color: colors.grid, display: !horizontal }, border: { display: false },
      ticks: { color: colors.text, font: { size: 11 } }
    };
    scales[valueAxis] = {
      display: !spark, stacked: stacked, beginAtZero: true,
      grid: { color: colors.grid }, border: { display: false },
      ticks: { color: colors.text, font: { size: 11 } },
      title: axisTitle(colors, spec.unit)
    };
    if (scatter) { scales.x.type = 'linear'; scales.x.title = axisTitle(colors, spec.xunit); }
    return {
      responsive: true, maintainAspectRatio: false, animation: entryAnimation(animate),
      indexAxis: horizontal ? 'y' : 'x',
      interaction: scatter ? { mode: 'nearest', intersect: true } : { mode: 'index', intersect: false },
      plugins: {
        legend: { display: multi && !spark, labels: legendLabels(colors) },
        title: { display: false },
        tooltip: { enabled: !spark }
      },
      scales: scales
    };
  }
  function cartesianDatasets(kind, spec, colors) {
    return (spec.series || []).map(function (s, i) {
      var col = chartSeriesColor(colors, s.color, i);
      var pts = s.points || [];
      var ds = { label: s.name || '', borderColor: col, backgroundColor: col };
      if (kind === 'scatter') {
        ds.data = pts.map(function (p) { return { x: parseFloat(p.x), y: p.y }; });
        ds.borderColor = colors.surface; ds.borderWidth = 1; ds.pointRadius = 5; ds.pointHoverRadius = 7;
        return ds;
      }
      ds.data = pts.map(function (p) { return p.y; });
      if (kind === 'area') { ds.fill = true; ds.backgroundColor = col + '33'; ds.tension = 0.3; ds.pointRadius = 2; ds.borderWidth = 2; }
      else if (kind === 'line') { ds.fill = false; ds.tension = 0.3; ds.pointRadius = 0; ds.pointHoverRadius = 5; ds.borderWidth = 2; }
      else if (kind === 'sparkline') { ds.fill = false; ds.pointRadius = 0; ds.borderWidth = 1.5; ds.tension = 0.3; }
      else if (kind === 'stacked-bar') { ds.borderColor = colors.surface; ds.borderWidth = 2; ds.borderRadius = 3; }
      else { ds.borderWidth = 0; ds.borderRadius = 3; } // bar, horizontal-bar
      return ds;
    });
  }
  function cartesianConfig(kind, spec, colors, animate) {
    var first = (spec.series || [])[0];
    var labels = ((first && first.points) || []).map(function (p) { return p.x || ''; });
    return {
      type: chartTypeFor(kind),
      data: { labels: labels, datasets: cartesianDatasets(kind, spec, colors) },
      options: cartesianOptions(kind, spec, colors, animate)
    };
  }
  // Doughnut: one series, one slice per point, slice colors in fixed palette order.
  function doughnutConfig(spec, colors, animate) {
    var first = (spec.series || [])[0] || {};
    var pts = first.points || [];
    return {
      type: 'doughnut',
      data: {
        labels: pts.map(function (p) { return p.x || ''; }),
        datasets: [{
          data: pts.map(function (p) { return p.y; }),
          backgroundColor: pts.map(function (_, i) { return colors.series[i % colors.series.length]; }),
          borderColor: colors.surface, borderWidth: 2, hoverOffset: 6
        }]
      },
      options: {
        responsive: true, maintainAspectRatio: false, cutout: '62%', animation: entryAnimation(animate),
        plugins: {
          legend: { position: 'right', labels: legendLabels(colors) },
          tooltip: { callbacks: { label: function (t) { return ' ' + t.label + ': ' + t.parsed + (spec.unit ? ' ' + spec.unit : ''); } } }
        }
      }
    };
  }
  // Sankey: flows {from, to, value}; each node takes the next palette color in
  // first-appearance order and every link fades from its source to its target.
  function sankeyConfig(spec, colors, animate) {
    var flows = (spec.flows || []).map(function (f) { return { from: f.from, to: f.to, flow: f.value }; });
    var nodeColor = {}, n = 0;
    flows.forEach(function (f) {
      [f.from, f.to].forEach(function (k) {
        if (!Object.prototype.hasOwnProperty.call(nodeColor, k)) { nodeColor[k] = colors.series[n % colors.series.length]; n++; }
      });
    });
    return {
      type: 'sankey',
      data: { datasets: [{
        label: spec.title || '', data: flows, colorMode: 'gradient', alpha: 0.55, nodeWidth: 10, borderWidth: 0,
        color: colors.label, font: { size: 11 },
        colorFrom: function (ctx) { return nodeColor[ctx.raw.from]; },
        colorTo: function (ctx) { return nodeColor[ctx.raw.to]; }
      }] },
      options: {
        responsive: true, maintainAspectRatio: false, animation: entryAnimation(animate),
        plugins: { legend: { display: false }, tooltip: { enabled: true } }
      }
    };
  }
  function registerSankey() {
    var sk = window['chartjs-chart-sankey'];
    if (!sk || !sk.SankeyController) return;
    try { Chart.register(sk.SankeyController, sk.Flow); } catch (e) {}
  }
  function sankeyAvailable() {
    try { return !!Chart.registry.getController('sankey'); } catch (e) { return false; }
  }
  function chartNote(block, text) {
    var n = block.querySelector('.present-chart-note');
    if (!n) { n = document.createElement('p'); n.className = 'present-chart-note'; block.appendChild(n); }
    n.textContent = text;
  }
  function initPresentCharts() {
    if (typeof Chart === 'undefined') return;
    registerSankey();
    document.querySelectorAll('.present-chart').forEach(function (block) {
      var specEl = block.querySelector('.chart-spec');
      var canvas = block.querySelector('canvas');
      if (!specEl || !canvas) return;
      var spec;
      try { spec = JSON.parse(specEl.textContent); } catch (e) { return; }
      if (block._chart) { block._chart.destroy(); block._chart = null; }
      var kind = spec.kind || 'bar';
      block.classList.toggle('is-sparkline', kind === 'sparkline');
      var colors = getChartColors();
      // Entry animation plays once per block; a theme recolor rebuilds silently.
      var animate = !reducedMotion && !block._built;
      block._built = true;
      var cfg;
      if (kind === 'doughnut') {
        cfg = doughnutConfig(spec, colors, animate);
      } else if (kind === 'sankey') {
        if (!sankeyAvailable()) {
          chartNote(block, 'Sankey needs the chartjs-chart-sankey asset: run make cache in services/present.');
          return;
        }
        cfg = sankeyConfig(spec, colors, animate);
      } else {
        cfg = cartesianConfig(kind, spec, colors, animate);
      }
      block._chart = new Chart(canvas, cfg);
    });
  }

  // ── Graph edge flow ── marches the dash pattern of every edge with data.flow
  // from source to target; speed follows data.weight. presentGraphStyle gives
  // those edges line-dash-pattern [10, 6], so the period here is 16. The
  // loop pauses while the graph is off screen or the tab is hidden, and under
  // Reduce Motion it draws one static frame.
  var flowStop = null;
  function startGraphFlow() {
    if (flowStop) { flowStop(); flowStop = null; }
    var cy = window._cyInstance, el = document.getElementById('cy-graph');
    if (!cy || !el) return;
    var edges = cy.edges('[flow]');
    if (!edges.length) return;
    var period = 16;
    var max = 0;
    edges.forEach(function (e) { max = Math.max(max, e.data('weight') || 0); });
    function speed(e) { // px per ms: 6 px/s for the lightest edge up to 20 px/s for the heaviest
      var share = max ? (e.data('weight') || 0) / max : 0.5;
      return 0.006 + 0.014 * share;
    }
    function frame(now) {
      cy.startBatch();
      edges.forEach(function (e) { e.style('line-dash-offset', period - ((now * speed(e)) % period)); });
      cy.endBatch();
    }
    if (reducedMotion) { frame(0); return; }
    var raf = 0, visible = true, stopped = false, io = null;
    function tick(now) { raf = 0; if (stopped) return; frame(now); if (visible && !document.hidden) raf = requestAnimationFrame(tick); }
    function kick() { if (!raf && !stopped && visible && !document.hidden) raf = requestAnimationFrame(tick); }
    if ('IntersectionObserver' in window) {
      // Entries batch when the graph crosses in and out between callbacks; the
      // last one is the current state, the first can be stale.
      io = new IntersectionObserver(function (entries) {
        visible = entries[entries.length - 1].isIntersecting;
        kick();
      }, { threshold: 0.05 });
      io.observe(el);
    }
    document.addEventListener('visibilitychange', kick);
    kick();
    flowStop = function () {
      stopped = true;
      if (raf) cancelAnimationFrame(raf);
      if (io) io.disconnect();
      document.removeEventListener('visibilitychange', kick);
    };
  }

  // ── Cytoscape graph controls ── engine/zoom/fit/fullscreen chrome around
  // #cy-graph. No-ops when the page has no graph. Removes any prior backdrop
  // first so a re-render does not stack duplicates. The engine button cycles
  // through GRAPH_ENGINES and re-runs the layout in place, so any page can be
  // compared across engines without re-authoring it.
  function setupGraphControls() {
    var el = document.getElementById('cy-graph');
    if (!el) return;
    document.querySelectorAll('.cy-backdrop').forEach(function (b) { b.remove(); });

    var wrapper = document.createElement('div');
    wrapper.className = 'cy-wrapper';
    el.parentNode.insertBefore(wrapper, el);
    wrapper.appendChild(el);

    var backdrop = document.createElement('div');
    backdrop.className = 'cy-backdrop';
    document.body.appendChild(backdrop);

    var svgFit = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M3 8V5a2 2 0 012-2h3"/><path d="M21 8V5a2 2 0 00-2-2h-3"/><path d="M3 16v3a2 2 0 002 2h3"/><path d="M21 16v3a2 2 0 01-2 2h-3"/><circle cx="12" cy="12" r="3"/></svg>';
    var svgExpand = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M15 3h6v6"/><path d="M9 21H3v-6"/><path d="M21 3l-7 7"/><path d="M3 21l7-7"/></svg>';
    var svgCollapse = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M4 14h6v6"/><path d="M20 10h-6V4"/><path d="M14 10l7-7"/><path d="M3 21l7-7"/></svg>';
    var svgClose = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6L6 18"/><path d="M6 6l12 12"/></svg>';

    var ctrls = document.createElement('div');
    ctrls.className = 'cy-controls';
    ctrls.innerHTML =
      '<button class="size-btn cy-close-btn" data-cy="close" title="Close">' + svgClose + '</button>' +
      '<button class="size-btn cy-engine" data-cy="engine" title="Cycle layout engine"></button>' +
      '<button class="size-btn" data-cy="in" title="Zoom in">+</button>' +
      '<button class="size-btn" data-cy="out" title="Zoom out">&minus;</button>' +
      '<button class="size-btn" data-cy="fit" title="Reset view">' + svgFit + '</button>' +
      '<button class="size-btn" data-cy="fs" title="Fullscreen">' + svgExpand + '</button>';
    wrapper.appendChild(ctrls);

    function cy() { return window._cyInstance; }
    function mid() { return { x: el.clientWidth / 2, y: el.clientHeight / 2 }; }

    showEngine();

    function runEngine(c, engine) {
      c.layout(presentGraphLayout(engine, layoutDirection(baseLayout))).run();
    }
    // An ELK layout is shaped by the container's aspect ratio, so a resize
    // re-runs it; dagre and cose keep their drawing and just refit.
    function refit(c, pad) {
      if (currentEngine().indexOf('elk') === 0) { runEngine(c, currentEngine()); return; }
      c.fit(undefined, pad);
      c.center();
    }

    function openPopup() {
      wrapper.classList.add('cy-fullscreen');
      backdrop.classList.add('active');
      ctrls.querySelector('[data-cy="fs"]').innerHTML = svgCollapse;
      ctrls.querySelector('[data-cy="fs"]').title = 'Exit fullscreen';
      var c = cy();
      if (c) setTimeout(function () { c.resize(); refit(c, 40); }, 120);
    }

    function closePopup() {
      wrapper.classList.remove('cy-fullscreen');
      backdrop.classList.remove('active');
      ctrls.querySelector('[data-cy="fs"]').innerHTML = svgExpand;
      ctrls.querySelector('[data-cy="fs"]').title = 'Fullscreen';
      var c = cy();
      if (c) setTimeout(function () { c.resize(); refit(c, 30); }, 80);
    }

    ctrls.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-cy]');
      if (!btn) return;
      var action = btn.getAttribute('data-cy'), c = cy();
      if (action === 'close') { closePopup(); return; }
      if (action === 'fs') {
        wrapper.classList.contains('cy-fullscreen') ? closePopup() : openPopup();
        return;
      }
      if (!c) return;
      if (action === 'engine') {
        var next = GRAPH_ENGINES[(GRAPH_ENGINES.indexOf(currentEngine()) + 1) % GRAPH_ENGINES.length];
        engineOverride = next;
        showEngine();
        runEngine(c, next);
        return;
      }
      if (action === 'in') c.zoom({ level: c.zoom() * 1.3, renderedPosition: mid() });
      else if (action === 'out') c.zoom({ level: c.zoom() / 1.3, renderedPosition: mid() });
      else if (action === 'fit') { c.fit(undefined, 30); c.center(); }
    });

    backdrop.addEventListener('click', closePopup);

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && wrapper.classList.contains('cy-fullscreen')) closePopup();
    });
  }

  // referencesSection builds the bottom-of-page references block from structured
  // data. Mirrors the old server-side referencesHTML: only http(s) links, title
  // + host/path. Webkit.el sets children via textContent, so it is XSS-safe.
  function referencesSection(refs) {
    var items = [];
    (refs || []).forEach(function (ref) {
      var u;
      try { u = new URL(ref.url); } catch (e) { return; }
      if (u.protocol !== 'http:' && u.protocol !== 'https:') return;
      items.push(Webkit.el('li', { class: 'refs-item' }, [
        Webkit.el('a', { href: ref.url, target: '_blank', rel: 'noopener' }, ref.title),
        Webkit.el('span', { class: 'refs-url' }, u.host + u.pathname)
      ]));
    });
    if (!items.length) return null;
    return Webkit.el('wk-section', { class: 'refs-section', id: 'references' }, [
      Webkit.el('wk-section-heading', {}, 'References'),
      Webkit.el('ul', { class: 'refs-list' }, items)
    ]);
  }

  var root = document.getElementById('root');

  // readAloud builds the page's read-aloud element, or returns null without
  // a speak URL (a shared instance, say). Any earlier element goes first, so
  // a re-render never leaves two. The element reads its targets on connect,
  // so callers create it once the content is mounted.
  function readAloud(data) {
    var old = document.querySelector('wk-read-aloud');
    if (old) old.remove();
    if (!data.speak_url) return null;
    var ra = document.createElement('wk-read-aloud');
    ra.setAttribute('targets', '.brief-summary, wk-section');
    ra.setAttribute('prepare', '');
    ra.setAttribute('name', data.title || 'present');
    ra.setAttribute('endpoint', data.speak_url);
    return ra;
  }

  // The graph is shared by the brief and the deck, and only the rendition
  // that placed a graph block has the container, so a page's graph script is
  // run only where #cy-graph exists (Cytoscape throws on a null container).
  function initGraphAndFlow() {
    if (typeof initGraph === 'function' && document.getElementById('cy-graph')) initGraph();
    showEngine();
    startGraphFlow();
  }

  function render(data) {
    document.title = data.title || 'present';
    var brief = document.createElement('div');
    brief.className = 'brief';
    brief.innerHTML = data.content || '';
    var refs = referencesSection(data.references || []);
    if (refs) brief.appendChild(refs);
    root.innerHTML = '';
    root.appendChild(brief);

    // The graph is delivered as the authored Cytoscape init script; executing it
    // (re)defines initGraph(). Drop any prior copy so a re-render stays clean.
    var prev = document.getElementById('present-graph-script');
    if (prev) prev.remove();
    if (data.graph) {
      var sc = document.createElement('script');
      sc.id = 'present-graph-script';
      sc.textContent = data.graph;
      document.body.appendChild(sc);
    }

    setupGraphControls();
    initGraphAndFlow();
    initPresentCharts();

    brief.querySelectorAll('a[href]:not([href^="#"])').forEach(function (a) {
      a.target = '_blank'; a.rel = 'noopener';
    });

    // Read-aloud reads its targets on connect, so it is created once the
    // content is mounted. With a speak URL the element registers the page's
    // text there (prepared mode) and shows its status bar where it sits, so
    // it goes right under the summary, or above the first section on a page
    // without one. No speak URL (a shared instance, say) means no element.
    var ra = readAloud(data);
    if (ra) {
      // before() rather than brief.insertBefore(): a legacy raw-HTML page can
      // wrap its sections, and insertBefore throws when the anchor is not a
      // direct child, which would blank the page.
      var summary = brief.querySelector('.brief-summary');
      var firstSection = brief.querySelector('wk-toc, wk-section');
      if (summary) summary.insertAdjacentElement('afterend', ra);
      else if (firstSection) firstSection.before(ra);
      else brief.appendChild(ra);
    }

    // Highlight code blocks / enhance prose in the freshly injected content.
    if (window.Webkit && typeof Webkit.enhanceProse === 'function') Webkit.enhanceProse(brief);
  }

  // showViewLink unhides the header link to the page's other rendition: the
  // brief offers Slides when the page has a deck, the deck offers Brief when
  // the page has one. Both anchors sit hidden in the shell's header.
  function showViewLink(elID, href) {
    var a = document.getElementById(elID);
    if (!a) return;
    a.href = href;
    a.removeAttribute('hidden');
  }

  // wireShare attaches the Share button and its modal. The button is hidden
  // unless the server reports sharing enabled (a shared instance is
  // configured and this is a local serve), so a shared instance never shows
  // it. Confirm posts to /p/{id}/share; the result shows the shared URL and
  // whether it expires. Re-sharing replaces the copy under the same URL.
  function wireShare(share) {
    var btn = document.getElementById('shareBtn');
    var modal = document.getElementById('share-modal');
    if (!btn || !modal || !share || !share.enabled) return;
    var eph = document.getElementById('share-ephemeral');
    var result = document.getElementById('share-result');
    var urlEl = document.getElementById('share-url');
    var status = document.getElementById('share-status');
    var confirm = document.getElementById('share-confirm');

    function show(info) {
      if (!info || !info.url) { result.setAttribute('hidden', ''); btn.textContent = 'Share'; return; }
      urlEl.textContent = info.url;
      status.textContent = info.ephemeral && info.expires_at
        ? 'Expires ' + new Date(info.expires_at).toLocaleDateString()
        : 'Kept until unshared';
      eph.checked = !!info.ephemeral;
      result.removeAttribute('hidden');
      btn.textContent = 'Shared';
    }
    function open() { modal.removeAttribute('hidden'); }
    function close() { modal.setAttribute('hidden', ''); }

    show(share);
    btn.removeAttribute('hidden');
    btn.onclick = open;
    document.getElementById('share-cancel').onclick = close;
    document.getElementById('share-x').onclick = close;
    modal.addEventListener('click', function (e) { if (e.target === modal) close(); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !modal.hasAttribute('hidden')) close();
    });
    document.getElementById('share-copy').onclick = function () {
      if (navigator.clipboard && urlEl.textContent) navigator.clipboard.writeText(urlEl.textContent);
    };
    confirm.onclick = function () {
      confirm.setAttribute('disabled', '');
      fetch('/p/' + encodeURIComponent(id) + '/share', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ ephemeral: eph.checked })
      })
        .then(function (r) {
          if (r.ok) return r.json();
          return r.text().then(function (t) { throw new Error(t || ('share failed (' + r.status + ')')); });
        })
        .then(show)
        .catch(function (err) {
          status.textContent = 'Share failed: ' + ((err && err.message) || err);
          result.removeAttribute('hidden');
        })
        .then(function () { confirm.removeAttribute('disabled'); });
    };
  }

  // Live update: reload once the page's version moves past the one this tab
  // rendered. A server with a change feed pushes the version over
  // server-sent events; otherwise, or when the stream cannot be trusted, the
  // tab polls the lightweight version endpoint (a bare int, which is valid
  // JSON) the way it always has.
  var POLL_MS = 1500;
  // The server resends the version every 25 seconds, so two missed
  // heartbeats mean a proxy is holding the stream back.
  var STREAM_SILENCE_MS = 60000;

  function watchVersion(id, known) {
    var base = '/p/' + encodeURIComponent(id);
    var poller = null;
    var stream = null;
    var watchdog = null;
    // gone is for good: the page was deleted or expired. leaving lasts while
    // the tab unloads or sits in the back-forward cache. Either one keeps the
    // browser's teardown of the stream, which fires the error handler, from
    // starting a pointless poll.
    var gone = false;
    var leaving = false;

    function onVersion(v) {
      if (v === known) return;
      leaving = true;
      closeStream();
      location.reload();
    }

    function poll() {
      closeStream();
      if (gone || leaving || poller) return;
      if (!(window.Webkit && typeof Webkit.poll === 'function')) return;
      poller = Webkit.poll(base + '/version', onVersion, POLL_MS);
    }

    function armWatchdog() {
      clearTimeout(watchdog);
      watchdog = setTimeout(poll, STREAM_SILENCE_MS);
    }

    function closeStream() {
      clearTimeout(watchdog);
      if (stream) {
        stream.close();
        stream = null;
      }
    }

    function openStream() {
      if (gone || leaving || poller || stream || document.hidden) return;
      stream = new EventSource(base + '/events');
      stream.addEventListener('version', function (e) {
        armWatchdog();
        onVersion(parseInt(e.data, 10));
      });
      // Nothing left to reload into.
      stream.addEventListener('gone', function () {
        gone = true;
        closeStream();
      });
      stream.onerror = function () {
        // CONNECTING means the browser is already retrying a dropped stream,
        // as it does across a rollout. CLOSED means the server refused it,
        // a 404 from a server without a change feed included.
        if (stream && stream.readyState === EventSource.CLOSED) poll();
      };
      armWatchdog();
    }

    if (!window.EventSource) {
      poll();
      return;
    }
    window.addEventListener('pagehide', function () {
      leaving = true;
      closeStream();
    });
    // A tab restored from the back-forward cache listens again; the reopened
    // stream's first event catches up on anything it missed.
    window.addEventListener('pageshow', function (e) {
      if (!e.persisted) return;
      leaving = false;
      openStream();
    });
    // A hidden tab lets its stream go, so it holds no connection; showing it
    // again reopens the stream, which catches up the same way.
    document.addEventListener('visibilitychange', function () {
      if (document.hidden) closeStream();
      else openStream();
    });
    openStream();
  }


  // ── Deck view ── the deck's compiled fragment holds the same markup the
  // brief does (title hero, an optional wk-toc, one wk-section per section),
  // so the slides come from its top-level nodes: everything before the first
  // section is the title slide, each section is one slide, the toc is dropped
  // (the counter and the keys replace it), and the references make the last
  // slide. Only the active slide is displayed; the graph is initialised the
  // first time its slide shows, because Cytoscape sizes itself from a
  // visible container.
  function makeSlide(nodes, cls) {
    var body = Webkit.el('div', { class: 'slide-body' });
    nodes.forEach(function (n) { body.appendChild(n); });
    var slide = Webkit.el('section', { class: 'slide ' + cls }, [body]);
    if (body.querySelector('#cy-graph, .present-chart')) slide.classList.add('has-viz');
    return slide;
  }

  // splitSlides returns the slides and the deck's chrome. The chrome is the
  // JSON island the renderer puts at the head of the fragment when the deck
  // sets a chrome field (logo, logo_position, progress, presenter, footer);
  // it is pulled out of the hero before the title slide is built, and an
  // absent island means every default.
  function splitSlides(html, refs) {
    var scratch = document.createElement('div');
    scratch.innerHTML = html || '';
    var hero = [], slides = [], seenSection = false, chrome = {};
    Array.prototype.slice.call(scratch.childNodes).forEach(function (n) {
      if (n.nodeType !== 1) {
        if (!seenSection && n.nodeType === 3 && n.textContent.trim()) hero.push(n);
        return;
      }
      var tag = n.tagName.toLowerCase();
      if (tag === 'script' && n.classList.contains('deck-chrome')) {
        try { chrome = JSON.parse(n.textContent) || {}; } catch (e) { chrome = {}; }
        return;
      }
      if (tag === 'wk-toc') { seenSection = true; return; }
      if (tag !== 'wk-section' && !seenSection) { hero.push(n); return; }
      seenSection = true;
      slides.push(makeSlide([n], 'slide-section'));
    });
    if (hero.length) slides.unshift(makeSlide(hero, 'slide-title'));
    if (refs) slides.push(makeSlide([refs], 'slide-refs'));
    if (!slides.length) slides.push(makeSlide([Webkit.el('p', {}, 'This deck has no slides yet.')], 'slide-title'));
    return { slides: slides, chrome: chrome };
  }

  // ── Deck chrome ── the view's furniture around the slides, built from the
  // chrome island: a strip along the bottom edge with the footer line on
  // the left in the text-3 role, the progress marker in the centre, and the
  // logo on the right (a bottom-left logo swaps sides with the footer; a
  // top corner logo is its own fixed element). Defaults are the embedded
  // logo and dots; the footer line is opt-in, shown only when the deck sets
  // presenter or footer. The strip sits outside every read-aloud and
  // fixation target, and inside it the dots are buttons and the logo an
  // image with an empty alt, which both walks skip anyway. The marker
  // tracks slides: one dot per slide, done ones filled in the primary role,
  // the current one ringed, the rest hollow in the border role, each a
  // button that jumps there. Past DOTS_MAX slides the dots give way to a
  // thin bar; progress "bar" asks for it at any length.
  var DOTS_MAX = 24;
  function deckChrome(chrome, title, count, goTo) {
    chrome = chrome || {};
    var position = chrome.logo_position || 'bottom-right';
    var logo = chrome.logo === 'none' ? null
      : Webkit.el('img', { class: 'deck-logo', src: chrome.logo || '/logo.png', alt: '' });
    var corner = logo && position.indexOf('top-') === 0 ? logo : null;
    if (corner) corner.classList.add('deck-logo-corner', 'deck-logo-' + position);
    var stripLogo = corner ? null : logo;

    var footer = null;
    if (chrome.presenter || chrome.footer) {
      var text = chrome.footer === 'none' ? '' : (chrome.footer || title || '');
      var line = [text, chrome.presenter || ''].filter(Boolean).join(' · ');
      if (line) footer = Webkit.el('div', { class: 'deck-footer' }, line);
    }

    var progress = chrome.progress || 'dots';
    if (progress === 'dots' && count > DOTS_MAX) progress = 'bar';
    var marker = null, dots = [], fill = null;
    if (progress === 'dots') {
      for (var i = 0; i < count; i++) {
        var label = 'slide ' + (i + 1) + ' of ' + count;
        dots.push(Webkit.el('button', { class: 'deck-dot', type: 'button', 'data-slide': String(i), title: label, 'aria-label': label }));
      }
      marker = Webkit.el('div', { class: 'deck-dots', role: 'group', 'aria-label': 'Slides' }, dots);
      marker.addEventListener('click', function (e) {
        var b = e.target.closest('.deck-dot');
        if (!b) return;
        goTo(parseInt(b.getAttribute('data-slide'), 10));
        // Focus would stay on the dot, where a later Space is the button's
        // own click and jumps back to it.
        b.blur();
      });
    } else if (progress === 'bar') {
      fill = Webkit.el('div', { class: 'deck-progress-fill' });
      marker = Webkit.el('div', { class: 'deck-progress', role: 'progressbar', 'aria-label': 'Deck progress', 'aria-valuemin': '1', 'aria-valuemax': String(count) }, [fill]);
    }

    var strip = null;
    if (footer || marker || stripLogo) {
      var swap = position === 'bottom-left';
      strip = Webkit.el('div', { class: 'deck-strip' }, [
        Webkit.el('div', { class: 'deck-strip-start' }, swap ? [stripLogo] : [footer]),
        Webkit.el('div', { class: 'deck-strip-centre' }, [marker]),
        Webkit.el('div', { class: 'deck-strip-end' }, swap ? [footer] : [stripLogo])
      ]);
    }
    return {
      strip: strip,
      corner: corner,
      update: function (i) {
        dots.forEach(function (d, j) {
          d.classList.toggle('done', j < i);
          d.classList.toggle('current', j === i);
          if (j === i) d.setAttribute('aria-current', 'true'); else d.removeAttribute('aria-current');
        });
        if (fill) {
          fill.style.width = ((i + 1) / count * 100) + '%';
          marker.setAttribute('aria-valuenow', String(i + 1));
        }
      }
    };
  }

  // slideFromHash reads the 1-based slide number a URL fragment names.
  function slideFromHash() {
    var m = (location.hash || '').match(/^#(\d+)$/);
    return m ? parseInt(m[1], 10) : 0;
  }

  // deckMemory keeps where a tab is in a deck (slide and whether it is
  // presenting) in sessionStorage, so the reload an update triggers, or a
  // reload by hand, comes back on the same slide in the same mode instead of
  // at the title slide with the chrome showing. sessionStorage is per tab,
  // so a fresh tab still starts at the beginning. The hash carries the slide
  // too and wins when present; the memory covers the mode, and the slide
  // when the tab reached the deck without a hash.
  function deckMemory(id) {
    var key = 'present-deck:' + id;
    function read() {
      try { return JSON.parse(sessionStorage.getItem(key) || '{}') || {}; } catch (e) { return {}; }
    }
    function write(patch) {
      var m = read();
      for (var k in patch) if (Object.prototype.hasOwnProperty.call(patch, k)) m[k] = patch[k];
      try { sessionStorage.setItem(key, JSON.stringify(m)); } catch (e) {}
    }
    return {
      slide: function () { var n = parseInt(read().slide, 10); return n > 0 ? n : 0; },
      presenting: function () { return read().presenting === true; },
      // seq is the last remote command this tab saw; -1 when it never polled,
      // which tells the poll this is a fresh tab and not one reloading.
      seq: function () { var n = read().seq; return typeof n === 'number' ? n : -1; },
      save: function (slide, presenting) { write({ slide: slide, presenting: presenting }); },
      saveSeq: function (seq) { write({ seq: seq }); }
    };
  }

  function renderDeck(data) {
    document.title = data.title || 'present';
    // The shell's header names the brief; the deck view relabels it in place
    // (wk-header renders once, so the attribute alone would not do).
    var headerTitle = document.querySelector('wk-header .topbar-title');
    if (headerTitle) headerTitle.textContent = 'Deck';
    var split = splitSlides(data.deck, referencesSection(data.references || []));
    var slides = split.slides;
    var deck = Webkit.el('div', { class: 'brief deck', id: 'deck' }, slides);
    var counter = Webkit.el('span', { class: 'deck-counter', 'aria-live': 'polite' });
    var presentBtn = Webkit.el('button', { class: 'btn btn-ghost', type: 'button', 'data-deck': 'present', title: 'Present (F): hide the chrome and fill the window' }, 'Present');
    var bar = Webkit.el('div', { class: 'deck-bar', role: 'toolbar', 'aria-label': 'Slides' }, [
      Webkit.el('button', { class: 'btn btn-ghost deck-arrow', type: 'button', 'data-deck': 'prev', title: 'Previous slide (Left)', 'aria-label': 'Previous slide' }, '‹'),
      counter,
      Webkit.el('button', { class: 'btn btn-ghost deck-arrow', type: 'button', 'data-deck': 'next', title: 'Next slide (Right or Space)', 'aria-label': 'Next slide' }, '›'),
      presentBtn
    ]);
    root.innerHTML = '';
    root.appendChild(deck);
    root.appendChild(bar);

    // The chrome strip goes after the bar; with a strip the bar moves up
    // above it, the deck leaves room for both, and the presenting slide
    // keeps its bottom padding clear of it (an html class drives that; it is not
    // named after the strip, or the strip rules would hit the html element).
    var chrome = deckChrome(split.chrome, data.title, slides.length, function (i) { show(i); });
    document.documentElement.classList.toggle('has-deck-strip', !!chrome.strip);
    if (chrome.strip) {
      root.appendChild(chrome.strip);
      deck.classList.add('has-strip');
      bar.classList.add('raised');
    }
    if (chrome.corner) root.appendChild(chrome.corner);

    // The graph script (re)defines initGraph(); it runs when its slide shows.
    var prev = document.getElementById('present-graph-script');
    if (prev) prev.remove();
    if (data.graph) {
      var sc = document.createElement('script');
      sc.id = 'present-graph-script';
      sc.textContent = data.graph;
      document.body.appendChild(sc);
    }
    initPresentCharts();
    deck.querySelectorAll('a[href]:not([href^="#"])').forEach(function (a) {
      a.target = '_blank'; a.rel = 'noopener';
    });
    if (window.Webkit && typeof Webkit.enhanceProse === 'function') Webkit.enhanceProse(deck);

    // Read-aloud sits on the title slide, under the summary when there is
    // one, and registers every slide's section the way the brief does, so
    // each slide carries its own play control while the status bar stays
    // on the first slide.
    var ra = readAloud(data);
    if (ra) {
      var titleBody = deck.querySelector('.slide-title .slide-body');
      var summary = titleBody && titleBody.querySelector('.brief-summary');
      if (summary) summary.insertAdjacentElement('afterend', ra);
      else if (titleBody) titleBody.appendChild(ra);
      else deck.appendChild(ra);
    }

    var current = -1;
    // graphReady says the graph is drawn for the current theme; a theme
    // change off screen clears it so the next showing redraws. The controls
    // wrap the container once and stay, whatever the theme does.
    var graphReady = false;
    var controlsReady = false;
    var presenting = false;
    var memory = deckMemory(data.id);
    function remember() { memory.save(current + 1, presenting); }

    function graphSlide(slide) { return !!slide.querySelector('#cy-graph'); }

    // refreshVisuals fits the graph and charts on the active slide to their
    // container, which changes when the slide first shows and when
    // presenting toggles the layout.
    function refreshVisuals() {
      var slide = slides[current];
      if (!slide) return;
      if (graphSlide(slide)) {
        if (!graphReady) {
          graphReady = true;
          if (!controlsReady) { controlsReady = true; setupGraphControls(); }
          initGraphAndFlow();
        } else if (window._cyInstance) {
          var c = window._cyInstance;
          setTimeout(function () { c.resize(); c.fit(undefined, 30); c.center(); }, 60);
        }
      }
      slide.querySelectorAll('.present-chart').forEach(function (b) { if (b._chart) b._chart.resize(); });
    }

    function show(i) {
      if (i < 0) i = 0;
      if (i > slides.length - 1) i = slides.length - 1;
      if (i === current) return;
      current = i;
      slides.forEach(function (sl, j) {
        sl.classList.toggle('active', j === i);
        if (j === i) sl.scrollTop = 0;
      });
      counter.textContent = (i + 1) + ' / ' + slides.length;
      chrome.update(i);
      if (slideFromHash() !== i + 1) history.replaceState(null, '', '#' + (i + 1));
      remember();
      refreshVisuals();
    }
    function next() { show(current + 1); }
    function previous() { show(current - 1); }

    // Presenting hides the chrome through a class on <html> and asks for
    // browser fullscreen on top when the browser allows it (a key or a click
    // does, a remote command does not; the class alone still fills the
    // window). Leaving fullscreen through the browser ends presenting too.
    // ownFullscreen is true once this document's own fullscreen request was
    // granted. A document loaded by a reload inherits the previous one's
    // fullscreen and then watches the browser leave it; that exit is not the
    // reader ending the presentation and must not end it here, or the reload
    // an update triggers would come back with the chrome showing.
    var ownFullscreen = false;
    function setPresenting(on) {
      if (presenting === on) return;
      presenting = on;
      document.documentElement.classList.toggle('presenting', on);
      presentBtn.textContent = on ? 'Exit' : 'Present';
      remember();
      var d = document.documentElement;
      if (on && d.requestFullscreen) {
        try {
          d.requestFullscreen().then(function () { ownFullscreen = true; }).catch(function () {});
        } catch (e) {}
      } else if (!on && document.fullscreenElement && document.exitFullscreen) {
        ownFullscreen = false;
        try { document.exitFullscreen().catch(function () {}); } catch (e) {}
      }
      setTimeout(refreshVisuals, 150);
    }
    document.addEventListener('fullscreenchange', function () {
      if (document.fullscreenElement || !ownFullscreen) return;
      ownFullscreen = false;
      if (presenting) setPresenting(false);
    });
    // F and P start presenting and, in fullscreen, end it. In between sits
    // the state a reload leaves behind: presenting without fullscreen,
    // because the request needed a gesture. A press there is the gesture,
    // so it asks for fullscreen and stays presenting; if the browser still
    // refuses, the press ends presenting as it would anywhere else.
    function pressPresent() {
      var d = document.documentElement;
      if (presenting && !document.fullscreenElement && d.requestFullscreen) {
        var req;
        try { req = d.requestFullscreen(); } catch (e) { setPresenting(false); return; }
        if (req && req.then) req.then(function () { ownFullscreen = true; }).catch(function () { setPresenting(false); });
        return;
      }
      setPresenting(!presenting);
    }

    bar.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-deck]');
      if (!btn) return;
      var action = btn.getAttribute('data-deck');
      if (action === 'prev') previous();
      else if (action === 'next') next();
      else if (action === 'present') setPresenting(!presenting);
    });

    // Keys: Right, Space, PageDown next; Left, PageUp, Backspace previous;
    // Home and End jump; F or P toggle presenting; Escape ends it. Modifier
    // combinations, typing in a field, an open modal, and the graph's own
    // fullscreen popup (which owns Escape) are left alone. Space on a focused
    // button is the button's click, not a page turn.
    function graphPopupOpen() { return !!document.querySelector('.cy-wrapper.cy-fullscreen'); }
    document.addEventListener('keydown', function (e) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      var t = e.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return;
      if (document.querySelector('wk-modal:not([hidden])')) return;
      // Space on a focused foreign button is that button's click, not a
      // page turn; the deck's own buttons (the bar, the progress dots) are
      // not foreign, so Space turns the page from them too.
      var onButton = t && (t.tagName === 'BUTTON' || t.tagName === 'A' || t.tagName === 'WK-BUTTON') &&
        !t.closest('.deck-bar, .deck-strip');
      switch (e.key) {
        case 'ArrowRight': case 'PageDown': e.preventDefault(); next(); break;
        case ' ': if (onButton) return; e.preventDefault(); next(); break;
        case 'ArrowLeft': case 'PageUp': case 'Backspace': e.preventDefault(); previous(); break;
        case 'Home': e.preventDefault(); show(0); break;
        case 'End': e.preventDefault(); show(slides.length - 1); break;
        case 'f': case 'F': case 'p': case 'P': e.preventDefault(); pressPresent(); break;
        case 'Escape': if (presenting && !graphPopupOpen()) setPresenting(false); break;
      }
    });
    window.addEventListener('hashchange', function () {
      var n = slideFromHash();
      if (n) show(n - 1);
    });

    // A theme change rebuilds the graph; that only works while its slide is
    // on screen, so off screen it is rebuilt on the slide's next showing.
    document.addEventListener('wk-themechange', function () {
      var slide = slides[current];
      if (slide && graphSlide(slide) && graphReady) initGraphAndFlow();
      else graphReady = false;
      initPresentCharts();
    });

    // Read the remembered mode before the first show() writes the memory.
    var resumePresenting = memory.presenting();
    show(Math.max(slideFromHash() || memory.slide(), 1) - 1);
    // Presenting comes back after a reload without browser fullscreen: the
    // request needs a gesture, so it is refused and caught, and the class
    // alone fills the window until the reader presses F, which asks again.
    if (resumePresenting) setPresenting(true);
    if (data.deck_control) watchDeckCommands(data.id, {
      start: function () { setPresenting(true); },
      stop: function () { setPresenting(false); },
      next: next,
      prev: previous,
      goto: function (n) { show(n - 1); }
    }, memory);
  }

  // watchDeckCommands follows the remote control: present_deck and `present
  // deck` append to the page's kept commands, and the view polls for
  // everything after the last sequence number it applied and runs them in
  // order, so two commands inside one poll interval both land. A fresh tab
  // (one that never saw a command) runs only the newest command and only
  // when it was sent moments ago, which is the agent opening the deck and
  // sending start back to back; older history is never replayed. The
  // sequence number is remembered with the slide, so a reload continues
  // from where the poll was. The poll pauses while the tab is hidden and
  // catches up when it shows again.
  var COMMAND_POLL_MS = 1000;
  var COMMAND_FRESH_MS = 10000;
  function watchDeckCommands(id, apply, memory) {
    var base = '/p/' + encodeURIComponent(id) + '/deck/command';
    var known = memory.seq();
    var timer = null;
    var stopped = false;
    function run(cmd) {
      var fn = apply[cmd.action];
      if (fn) fn(cmd.slide);
    }
    function onData(data) {
      if (!data || typeof data.seq !== 'number') return;
      var cmds = data.commands || [];
      if (known < 0) {
        var last = cmds.length ? cmds[cmds.length - 1] : null;
        if (last && Date.now() - Date.parse(last.at) < COMMAND_FRESH_MS) run(last);
        known = data.seq;
      } else {
        cmds.forEach(function (c) {
          if (typeof c.seq === 'number' && c.seq > known) { known = c.seq; run(c); }
        });
      }
      memory.saveSeq(known);
    }
    function tick() {
      fetch(base + '?after=' + known, { headers: { accept: 'application/json' } })
        .then(function (r) { if (!r.ok) throw new Error('request failed (' + r.status + ')'); return r.json(); })
        .then(function (data) { if (!stopped) onData(data); })
        .catch(function () {});
    }
    function start() {
      if (timer) return;
      stopped = false;
      tick();
      timer = setInterval(tick, COMMAND_POLL_MS);
    }
    function stop() {
      stopped = true;
      if (timer) { clearInterval(timer); timer = null; }
    }
    document.addEventListener('visibilitychange', function () {
      if (document.hidden) stop(); else start();
    });
    if (!document.hidden) start();
  }

  // pageId reads the page id from the URL; deckView says whether this is the
  // /p/{id}/deck rendition of it.
  function pageId() {
    var m = location.pathname.match(/^\/p\/([^/]+)/);
    return m ? m[1] : '';
  }
  var deckView = /^\/p\/[^/]+\/deck\/?$/.test(location.pathname);

  var id = pageId();
  if (!id) return;

  fetch('/api/p/' + encodeURIComponent(id), { headers: { accept: 'application/json' } })
    .then(function (r) { if (!r.ok) throw new Error('load failed (' + r.status + ')'); return r.json(); })
    .then(function (data) {
      var base = '/p/' + encodeURIComponent(id);
      // The server redirects a URL whose rendition is gone, but a replica
      // whose page cache trails an update can still serve the old view;
      // the page data is a direct read, so it is the last word.
      if (deckView && !data.has_deck && data.content) { location.replace(base); return; }
      if (!deckView && !data.content && data.has_deck) { location.replace(base + '/deck'); return; }
      if (deckView) {
        renderDeck(data);
        if (data.content) showViewLink('briefLink', base);
      } else {
        render(data);
        if (data.has_deck) showViewLink('deckLink', base + '/deck');
        // Recolor the graph + charts when the webkit theme toggle fires
        // (the deck view registers its own listener).
        document.addEventListener('wk-themechange', function () {
          initGraphAndFlow();
          initPresentCharts();
        });
      }
      wireShare(data.share);
      watchVersion(id, data.version);
    })
    .catch(function (err) {
      root.innerHTML = '';
      var msg = (err && err.message) || String(err);
      root.appendChild(Webkit.el('div', { class: 'brief' }, 'Failed to load presentation: ' + msg));
    });
})();
