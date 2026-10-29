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
  // An exported page (present export) is one file with no server behind
  // it. Its <html> says so, and the code below then asks no server for
  // anything (renderExport at the end).
  var exported = document.documentElement.getAttribute('data-present-mode') === 'export';

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
  var colorCtx = null; // the one-pixel canvas colorLiteral paints on
  // colorLiteral turns any colour the browser can paint (a computed style
  // can come back as color(srgb ...)) into the rgb() string Cytoscape and
  // Chart.js parse, by painting one pixel with it.
  function colorLiteral(value) {
    if (!colorCtx) {
      var c = document.createElement('canvas');
      c.width = c.height = 1;
      colorCtx = c.getContext('2d', { willReadFrequently: true });
    }
    colorCtx.clearRect(0, 0, 1, 1);
    colorCtx.fillStyle = '#000';
    colorCtx.fillStyle = value;
    colorCtx.fillRect(0, 0, 1, 1);
    var d = colorCtx.getImageData(0, 0, 1, 1).data;
    return 'rgb(' + d[0] + ', ' + d[1] + ', ' + d[2] + ')';
  }
  // surfaceColor is the colour the shell names --surface at el: the page
  // (a slide paints no box of its own) or a card. It is read through a probe
  // child with its own transition off, never from a painted background:
  // webkit eases the body and panel backgrounds over 300 ms and fires
  // wk-themechange in the same tick, so right after a theme change the
  // painted colour is still the old theme's. Cytoscape and Chart.js parse
  // colours themselves, so the value is normalised to rgb().
  function surfaceColor(el) {
    if (!el || !el.appendChild) return role('bg');
    var probe = document.createElement('span');
    probe.style.cssText = 'position:absolute;width:0;height:0;transition:none;background:var(--surface, var(--bg))';
    el.appendChild(probe);
    var bg = getComputedStyle(probe).backgroundColor;
    el.removeChild(probe);
    return colorLiteral(bg || role('bg'));
  }
  function getGraphColors(container) {
    var tones = {};
    GRAPH_TONES.forEach(function (t) {
      tones[t] = { bg: role('tone-' + t + '-bg'), border: role('tone-' + t + '-border'), text: role('tone-' + t + '-text') };
    });
    var modBorder = [];
    for (var i = 1; i <= MODULE_COLORS; i++) modBorder.push(role('graph-module-' + i));
    return {
      bg: container ? surfaceColor(container) : role('graph-bg'),
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
  // presentGraphStyle styles the page graph; container, when given, is the
  // element the graph draws in, so edge labels are backed with the colour
  // behind it (its card, or the page) instead of a fixed role.
  function presentGraphStyle(elements, container) {
    var c = getGraphColors(container);
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
  // availableEngines is the engine control's cycle: every engine, less the
  // ELK ones when ELK or its adapter did not load. An exported page loads
  // them only for a graph authored with an ELK engine.
  function availableEngines() {
    var elk = !!(window.ELK && window.cytoscapeElk);
    return GRAPH_ENGINES.filter(function (e) { return elk || e.indexOf('elk') !== 0; });
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
        opts.style = presentGraphStyle(opts.elements, opts.container);
      }
      if (isPage && opts.layout) {
        baseLayout = opts.layout;
        if (engineOverride) opts.layout = presentGraphLayout(engineOverride, layoutDirection(baseLayout));
      }
      // The page's layout runs after the instance exists, so the ready
      // listener below sees it stop: a synchronous layout started by the
      // constructor would finish before any listener could be attached.
      var layout = isPage ? opts.layout : null;
      if (layout) delete opts.layout;
      var cy = orig.apply(this, arguments);
      if (isPage) {
        window._cyInstance = cy;
        // The container fades in once its first layout has stopped (the
        // shell's .cy-container.ready rule); a later rebuild (theme change)
        // keeps the class, so only the first showing fades. The timer covers
        // a layout that never reports stopping.
        var container = opts.container;
        if (!container.classList.contains('ready')) {
          var ready = function () { container.classList.add('ready'); };
          cy.one('layoutstop', ready);
          setTimeout(ready, 1500);
        }
        if (layout) cy.layout(layout).run();
      }
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
  // segments and slices, which is whatever is painted behind the block (a
  // chart sits straight on the page or slide unless it keeps its card).
  function getChartColors(block) {
    var series = [];
    for (var i = 1; i <= CHART_SERIES; i++) series.push(role('series-' + i));
    return { series: series, grid: role('chart-grid'), text: role('chart-text'), label: role('chart-label'), surface: surfaceColor(block) };
  }
  // A stored spec names a series colour by its legacy name (terracotta, blue,
  // green, purple, the default family's first four) or by its role
  // (series-1 to series-4); both land on the same index in every family.
  var SERIES_NAMES = { terracotta: 0, blue: 1, green: 2, purple: 3, 'series-1': 0, 'series-2': 1, 'series-3': 2, 'series-4': 3 };
  // seriesSlot is the palette slot (0 to 3) a series draws with: the slot
  // its named colour maps to, or its index in order.
  function seriesSlot(name, i) {
    return (name && Object.prototype.hasOwnProperty.call(SERIES_NAMES, name)) ? SERIES_NAMES[name] : (i % CHART_SERIES);
  }
  function chartSeriesColor(colors, name, i) {
    return colors.series[seriesSlot(name, i)];
  }
  function seriesName(series, i) {
    return (series[i] && series[i].name) || '';
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
  // chartStepAt is the step a stepped chart block stands at: the deck
  // writes data-step-at as it walks the steps, the brief never does, and
  // null means every series shows.
  function chartStepAt(block) {
    if (!block.hasAttribute('data-step-at')) return null;
    var n = parseInt(block.getAttribute('data-step-at'), 10);
    return isNaN(n) ? null : n;
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
  // needsAsset is the note a block shows when a library it draws with did
  // not load: a served page names the cache step, an exported one the CDN.
  function needsAsset(what, asset) {
    return exported
      ? what + ' needs the ' + asset + ' from cdn.jsdelivr.net, which did not load: open this page online.'
      : what + ' needs the ' + asset + ': run make cache in services/present.';
  }
  // cutStep applies a step change to an svg renderer with its transitions
  // off for a frame when the deck cuts (block._stepCut) or Reduce Motion
  // is on, so the swap does not fade; otherwise the stylesheet's
  // transition plays.
  function cutStep(block, svg, apply) {
    var cut = reducedMotion || block._stepCut;
    if (cut) svg.classList.add('cut');
    apply();
    if (!cut) return;
    svg.getBoundingClientRect(); // apply the change under cut before it lifts
    requestAnimationFrame(function () { svg.classList.remove('cut'); });
  }
  // buildChart builds (or rebuilds, on a theme change) one chart block. The
  // entry animation plays once per block, the first time it is built, and
  // opts.duration caps it: the brief plays Chart.js's 700 ms at mount, the
  // deck 400 ms on the first showing of the slide. A ribbon draws with d3
  // (buildRibbon), every other kind with Chart.js.
  function buildChart(block, opts) {
    var specEl = block.querySelector('.chart-spec');
    if (!specEl) return;
    var spec;
    try { spec = JSON.parse(specEl.textContent); } catch (e) { return; }
    if (spec.kind === 'ribbon') { buildRibbon(block, spec, opts); return; }
    var canvas = block.querySelector('canvas');
    if (!canvas || typeof Chart === 'undefined') return;
    if (block._chart) { block._chart.destroy(); block._chart = null; }
    var kind = spec.kind || 'bar';
    block.classList.toggle('is-sparkline', kind === 'sparkline');
    var colors = getChartColors(block);
    var animate = !reducedMotion && !block._built;
    block._built = true;
    var cfg;
    if (kind === 'doughnut') {
      cfg = doughnutConfig(spec, colors, animate);
    } else if (kind === 'sankey') {
      if (!sankeyAvailable()) {
        chartNote(block, needsAsset('Sankey', 'chartjs-chart-sankey asset'));
        return;
      }
      cfg = sankeyConfig(spec, colors, animate);
    } else {
      cfg = cartesianConfig(kind, spec, colors, animate);
    }
    if (animate && opts && opts.duration !== undefined && cfg.options) {
      cfg.options.animation = opts.duration > 0 ? { duration: opts.duration, easing: 'easeOutQuart' } : false;
    }
    if (block.hasAttribute('data-steps')) {
      applyStepConfig(cfg, spec, block, kind);
      registerStepHook(block, spec);
    }
    block._chart = new Chart(canvas, cfg);
  }
  // applyStepConfig shapes a stepped chart for the step the deck has walked
  // to (data-step-at; the brief sets none and gets the full chart): the
  // series of later steps start hidden, the value axis is pinned to the
  // full data, and the legend lists only the series shown and stops
  // toggling them, so a click cannot undo a step.
  function applyStepConfig(cfg, spec, block, kind) {
    var at = chartStepAt(block);
    if (at === null || !cfg.data || !cfg.data.datasets) return;
    var shown = PresentViz.chartStepVisibility(spec.series || [], at);
    cfg.data.datasets.forEach(function (ds, i) { if (shown[i] === false) ds.hidden = true; });
    var opts = cfg.options || {};
    if (opts.scales) {
      var axis = opts.indexAxis === 'y' ? 'x' : 'y';
      if (opts.scales[axis]) opts.scales[axis].suggestedMax = PresentViz.seriesMax(spec.series, kind === 'stacked-bar');
    }
    if (opts.plugins && opts.plugins.legend) {
      opts.plugins.legend.onClick = function () {};
      opts.plugins.legend.labels = opts.plugins.legend.labels || {};
      opts.plugins.legend.labels.filter = function (item) { return !item.hidden; };
    }
  }
  // registerStepHook gives the deck the hook that moves a stepped chart
  // between steps. It is registered on every build and reads block._chart
  // when called, because a theme change replaces the chart. Reduce Motion
  // and a deck that cuts (block._stepCut) swap the series without animating.
  function registerStepHook(block, spec) {
    block._presentStep = function (n) {
      var c = block._chart;
      if (!c) return;
      PresentViz.chartStepVisibility(spec.series || [], n).forEach(function (on, i) {
        if (c.data.datasets[i]) c.setDatasetVisibility(i, on);
      });
      c.update(reducedMotion || block._stepCut ? 'none' : undefined);
    };
  }
  // ── Ribbon charts (d3) ──
  // A ribbon chart draws one column per period with the series stacked in
  // it, largest on top unless the spec orders them as given, and a band
  // joining each series' segments in neighbouring columns, so a change of
  // rank shows as ribbons crossing. PresentViz.ribbonLayout does the
  // arithmetic; this part draws it into one svg. Every colour is a palette
  // variable in the markup, so a theme change needs no new colours, and all
  // motion (entry, steps, hover) is CSS opacity on classes: s-<i> names an
  // element's series and col-<k> the column it shows with.
  var RIBBON_STAGGER_MS = 60; // between columns in the entry animation
  var RIBBON_CHAR_PX = 6.5; // estimated width of one 11 px character
  var RIBBON_LABEL_MIN_PX = 16; // the shortest segment that carries its name
  var RIBBON_LEFT_MIN_PX = 40; // the value axis's margin before its labels widen it
  var SVG_NS = 'http://www.w3.org/2000/svg';

  // ribbonSeriesVar is a series' fill as a palette variable, series-1 to
  // series-4, so the markup needs no new colour on a theme change.
  function ribbonSeriesVar(s, i) {
    return 'var(--series-' + (seriesSlot(s && s.color, i) + 1) + ')';
  }
  // ribbonFadeMs is the opacity transition shell.html gives the chart's
  // parts, read from its --ribbon-fade so the duration lives in one place.
  function ribbonFadeMs(svg) {
    return parseFloat(getComputedStyle(svg).getPropertyValue('--ribbon-fade')) || 350;
  }
  // ribbonMargins leaves room for the value axis's widest tick label, the
  // unit above it, and the period labels under the columns.
  function ribbonMargins(spec, y) {
    var fmt = y.tickFormat(4), widest = 0;
    y.ticks(4).forEach(function (t) { widest = Math.max(widest, fmt(t).length); });
    return {
      top: spec.unit ? 22 : 8, right: 4, bottom: 22,
      left: Math.max(RIBBON_LEFT_MIN_PX, Math.ceil(widest * RIBBON_CHAR_PX) + 10)
    };
  }
  // ribbonElements are the parts of the chart that step and dim: segments,
  // ribbons, and the names inside segments. Each carries its layout datum.
  function ribbonElements(svg) {
    return Array.prototype.slice.call(svg.querySelectorAll('.seg, .rib, .seg-label'));
  }
  // ribbonGeometry places the layout in a w by h svg: the margins, the
  // scales (the value axis pinned to the full data so steps never rescale),
  // every segment in pixels, and the path of a ribbon between two columns.
  function ribbonGeometry(spec, layout, w, h) {
    var y = d3.scaleLinear().domain([0, layout.max || 1]).nice(4);
    var m = ribbonMargins(spec, y);
    var iw = Math.max(0, w - m.left - m.right), ih = Math.max(0, h - m.top - m.bottom);
    y.range([ih, 0]);
    var x = d3.scaleBand().domain(layout.periods).range([0, iw]).padding(0.5);
    var bw = x.bandwidth();
    // band is a segment's top and bottom in pixels, 1 px in from each edge,
    // so stacked segments (and the ribbons leaving them) keep a 2 px gap.
    function band(v0, v1) {
      var top = y(v1) + 1, bottom = y(v0) - 1;
      if (bottom < top) top = bottom = (top + bottom) / 2;
      return { top: top, bottom: bottom };
    }
    var segs = [];
    layout.columns.forEach(function (c, k) {
      c.segments.forEach(function (s) {
        var b = band(s.y0, s.y1);
        segs.push({ series: s.series, col: k, period: c.period, value: s.value, top: b.top, bottom: b.bottom });
      });
    });
    var area = d3.area().x(function (p) { return p.x; }).y0(function (p) { return p.y0; })
      .y1(function (p) { return p.y1; }).curve(d3.curveBumpX);
    function ribbonPath(r) {
      var a = band(r.a.y0, r.a.y1), b = band(r.b.y0, r.b.y1);
      return area([
        { x: x(layout.periods[r.col - 1]) + bw, y0: a.bottom, y1: a.top },
        { x: x(layout.periods[r.col]), y0: b.bottom, y1: b.top }
      ]);
    }
    return { m: m, iw: iw, ih: ih, x: x, y: y, bw: bw, segs: segs, ribbonPath: ribbonPath };
  }
  // ribbonTitles are the hover tooltips: a segment's value in its period,
  // and a ribbon's values at both ends.
  function ribbonTitles(spec, layout) {
    var series = spec.series || [], fmt = d3.format(','), unit = spec.unit ? ' ' + spec.unit : '';
    var valueAt = layout.columns.map(function (c) {
      var v = {};
      c.segments.forEach(function (s) { v[s.series] = s.value; });
      return v;
    });
    function name(i) { return seriesName(series, i); }
    return {
      seg: function (d) { return name(d.series) + ': ' + fmt(d.value) + unit + ' (' + d.period + ')'; },
      rib: function (r) {
        return name(r.series) + ': ' + fmt(valueAt[r.col - 1][r.series]) + ' to ' + fmt(valueAt[r.col][r.series]) +
          unit + ' (' + layout.periods[r.col - 1] + ' to ' + layout.periods[r.col] + ')';
      }
    };
  }
  // drawRibbon draws the chart into svg at w by h pixels, replacing what was
  // there: grid and axis, ribbons, columns, then labels. An element whose
  // column hidden(col) names starts with off, set before it enters the page
  // so its first paint plays no transition.
  function drawRibbon(svg, spec, layout, w, h, hidden) {
    var series = spec.series || [];
    var geo = ribbonGeometry(spec, layout, w, h), x = geo.x, bw = geo.bw;
    var titles = ribbonTitles(spec, layout);
    function name(d) { return seriesName(series, d.series); }
    function fill(d) { return 'fill: ' + ribbonSeriesVar(series[d.series], d.series); }
    function cls(kind) {
      return function (d) { return kind + ' s-' + d.series + ' col-' + d.col + (hidden(d.col) ? ' off' : ''); };
    }
    function fits(d) {
      return d.bottom - d.top >= RIBBON_LABEL_MIN_PX && name(d).length * RIBBON_CHAR_PX <= bw - 8;
    }
    function centre(period) { return x(period) + bw / 2; }

    var g = d3.create('svg:g').attr('transform', 'translate(' + geo.m.left + ',' + geo.m.top + ')');
    // The axis's own font attributes go, so its labels take the page font
    // from shell.html like every other label in the chart.
    g.append('g').attr('class', 'grid')
      .call(d3.axisLeft(geo.y).ticks(4).tickSize(-geo.iw).tickPadding(6))
      .attr('font-family', null).attr('font-size', null)
      .call(function (a) { a.select('.domain').remove(); });
    if (spec.unit) g.append('text').attr('class', 'unit').attr('x', -geo.m.left).attr('y', -10).text(spec.unit);
    g.append('g').attr('class', 'ribbons').selectAll('path').data(layout.ribbons).join('path')
      .attr('class', cls('rib')).attr('style', fill).attr('d', geo.ribbonPath)
      .append('title').text(titles.rib);
    g.append('g').attr('class', 'columns').selectAll('rect').data(geo.segs).join('rect')
      .attr('class', cls('seg')).attr('style', fill).attr('rx', 2)
      .attr('x', function (d) { return x(d.period); }).attr('width', bw)
      .attr('y', function (d) { return d.top; }).attr('height', function (d) { return d.bottom - d.top; })
      .append('title').text(titles.seg);
    g.append('g').attr('class', 'periods').selectAll('text').data(layout.periods).join('text')
      .attr('x', centre).attr('y', geo.ih + 16).attr('text-anchor', 'middle').text(function (p) { return p; });
    g.append('g').attr('class', 'seg-labels').selectAll('text').data(geo.segs.filter(fits)).join('text')
      .attr('class', cls('seg-label')).attr('text-anchor', 'middle').attr('dominant-baseline', 'central')
      .attr('x', function (d) { return centre(d.period); }).attr('y', function (d) { return (d.top + d.bottom) / 2; })
      .text(name);

    svg.setAttribute('viewBox', '0 0 ' + w + ' ' + h);
    while (svg.firstChild) svg.removeChild(svg.firstChild);
    svg.appendChild(g.node());
  }
  // ribbonStep shows the columns before step n and the ribbons that end in
  // them; a null n shows everything.
  function ribbonStep(svg, n) {
    if (!svg) return;
    ribbonElements(svg).forEach(function (el) {
      el.classList.toggle('off', n !== null && n !== undefined && el.__data__.col >= n);
    });
  }
  // ribbonHighlight dims every series but i, or clears the dimming when i
  // is null.
  function ribbonHighlight(svg, i) {
    if (!svg) return;
    ribbonElements(svg).forEach(function (el) {
      var own = i !== null && el.__data__.series === i;
      el.classList.toggle('dim', i !== null && !own);
      el.classList.toggle('hover', own);
    });
  }
  // ribbonEnter fades the drawn columns in from left to right. Everything
  // starts off; a frame later the columns the step allows lose it, each
  // delayed by its column index, and the delays clear once the last is in.
  // block._ribbonEntering holds until then, so a resize in that window
  // (the presenting toggle after a reload) redraws and enters again
  // instead of landing the chart in its final state.
  function ribbonEnter(block, svg, periodCount) {
    block._ribbonEntering = true;
    requestAnimationFrame(function () {
      // Flush style so the off state is computed and the change animates.
      svg.getBoundingClientRect();
      var at = chartStepAt(block), els = ribbonElements(svg);
      els.forEach(function (el) {
        var col = el.__data__.col;
        if (at !== null && col >= at) return;
        el.style.transitionDelay = (col * RIBBON_STAGGER_MS) + 'ms';
        el.classList.remove('off');
      });
      setTimeout(function () {
        block._ribbonEntering = false;
        els.forEach(function (el) { el.style.transitionDelay = ''; });
      }, ribbonFadeMs(svg) + RIBBON_STAGGER_MS * periodCount);
    });
  }
  // ribbonSvg is the block's svg, made on the first build in place of the
  // canvas and kept across rebuilds; hover on a segment or ribbon dims the
  // other series.
  function ribbonSvg(block, wrap) {
    if (block._ribbonSvg) return block._ribbonSvg;
    var svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('class', 'present-ribbon');
    svg.setAttribute('width', '100%');
    svg.setAttribute('height', '100%');
    svg.addEventListener('mouseover', function (e) {
      var t = e.target.closest ? e.target.closest('.seg, .rib') : null;
      if (t && t.__data__) ribbonHighlight(svg, t.__data__.series);
    });
    svg.addEventListener('mouseout', function () { ribbonHighlight(svg, null); });
    wrap.innerHTML = '';
    wrap.appendChild(svg);
    block._ribbonSvg = svg;
    return svg;
  }
  // ribbonLegend puts one key per series above the chart when there is more
  // than one. The name is drawn from data-name by CSS, so read-aloud, which
  // skips the svg, does not read the legend either.
  function ribbonLegend(block, wrap, series) {
    var old = block.querySelector('.present-ribbon-legend');
    if (old) old.remove();
    if (series.length < 2) return;
    var legend = document.createElement('div');
    legend.className = 'present-ribbon-legend';
    series.forEach(function (s, i) {
      var key = document.createElement('span');
      key.className = 'present-ribbon-key';
      key.setAttribute('data-name', (s && s.name) || '');
      var swatch = document.createElement('span');
      swatch.className = 'present-ribbon-swatch';
      swatch.setAttribute('style', 'background: ' + ribbonSeriesVar(s, i));
      key.appendChild(swatch);
      key.addEventListener('mouseenter', function () { ribbonHighlight(block._ribbonSvg, i); });
      key.addEventListener('mouseleave', function () { ribbonHighlight(block._ribbonSvg, null); });
      legend.appendChild(key);
    });
    wrap.parentNode.insertBefore(legend, wrap);
  }
  // buildRibbon builds a ribbon chart block, or redraws it in place on a
  // rebuild: the svg sized to its wrapper, the legend, the deck's step hook,
  // and a ResizeObserver that redraws, unanimated, when presenting changes
  // the wrapper's size. A wrapper with no size yet (a slide not showing)
  // draws when the observer first sees one.
  function buildRibbon(block, spec, opts) {
    if (typeof d3 === 'undefined') {
      chartNote(block, needsAsset('Ribbon', 'd3 asset'));
      return;
    }
    var wrap = block.querySelector('.present-chart-canvas');
    if (!wrap) return;
    var svg = ribbonSvg(block, wrap);
    var layout = PresentViz.ribbonLayout(spec.series || [], spec.order);
    ribbonLegend(block, wrap, spec.series || []);
    var animate = !reducedMotion && !block._built && !(opts && opts.duration === 0);
    block._built = true;
    block._presentStep = function (n) {
      cutStep(block, svg, function () { ribbonStep(svg, n); });
    };
    function draw(entering) {
      var w = wrap.clientWidth, h = wrap.clientHeight;
      if (w < 1 || h < 1) return false;
      block._ribbonSize = { w: w, h: h };
      var at = chartStepAt(block);
      drawRibbon(svg, spec, layout, w, h, function (col) { return entering || (at !== null && col >= at); });
      return true;
    }
    block._ribbonResize = function () {
      var s = block._ribbonSize;
      if (s && Math.abs(s.w - wrap.clientWidth) <= 1 && Math.abs(s.h - wrap.clientHeight) <= 1) return;
      if (block._ribbonEntering) { if (draw(true)) ribbonEnter(block, svg, layout.periods.length); return; }
      draw(false);
    };
    if (block._ribbonObserver) block._ribbonObserver.disconnect();
    if (typeof ResizeObserver === 'function') {
      block._ribbonObserver = new ResizeObserver(function () { block._ribbonResize(); });
      block._ribbonObserver.observe(wrap);
    }
    if (draw(animate) && animate) ribbonEnter(block, svg, layout.periods.length);
  }
  // initPresentCharts builds every chart block under root (the document by
  // default); with opts.builtOnly it rebuilds only the ones already built,
  // which is what a theme change wants in the deck, where a chart on a slide
  // not yet shown waits for its first showing. buildChart checks per kind
  // that its library loaded, so a ribbon builds without Chart.js.
  function initPresentCharts(root, opts) {
    if (typeof Chart !== 'undefined') registerSankey();
    (root || document).querySelectorAll('.present-chart').forEach(function (block) {
      if (opts && opts.builtOnly && !block._built) return;
      buildChart(block, opts);
    });
  }

  // ── Diagram blocks (elk + d3) ──
  // A diagram block is laid out once by ELK from its spec, every box sized
  // from its measured text, and drawn as one svg whose geometry never
  // changes after: steps, focus, the entry, and a theme change only move
  // classes and CSS variables, and a resize rescales the svg through its
  // width and height. Every drawn group, node, and edge carries
  // {kind, key, item} as __data__ for stepping, and a class naming it
  // (g-<id>, n-<id>, e-<k>) for the stylesheet and anyone inspecting it.
  var DIAGRAM_LABEL_FONT = '600 13px '; // a node's name
  var DIAGRAM_TEXT_FONT = '11.5px '; // a node's text lines and the edge labels
  var DIAGRAM_GROUP_FONT_PX = 11; // a group's label, drawn in upper case
  var DIAGRAM_GROUP_FONT = DIAGRAM_GROUP_FONT_PX + 'px ';
  var DIAGRAM_GROUP_TRACKING = 0.06; // the group label's letter spacing, in em
  var DIAGRAM_BOX_RX = 8; // a box's corner
  var DIAGRAM_GROUP_RX = 10; // a group's corner
  var DIAGRAM_HALO = { pad: 8, h: 16, rx: 3 }; // an edge label's halo around its text
  var DIAGRAM_MAX_SCALE = 1.25; // how far a small diagram grows to fill its width
  var DIAGRAM_MAX_H = 420; // the canvas height when the stylesheet sets none
  var DIAGRAM_STAGGER_MS = 40; // between node layers in the entry
  var DIAGRAM_PHASE_MS = 120; // between the entry's phases: groups, nodes, edges
  var DIAGRAM_DOT_R = 3.5; // a flow dot
  // Node centres closer than this along the flow share a layer in the
  // entry: less than any two layers sit apart (a box is at least 120 wide
  // or 38 high, plus 56 between layers).
  var DIAGRAM_LAYER_PX = 30;
  var DIAGRAM_CAP_RY = 6; // a store's cap
  var DIAGRAM_PIPE_INSET = 8; // a queue's end lines from its ends
  // A person's figure in its box's top left corner: inset from the edges,
  // the head's radius, the shoulders' radius (half the strip
  // PresentViz.diagramGraph adds to the box), and the gap between them.
  var DIAGRAM_FIGURE = { inset: 9, head: 5, shoulders: 7, gap: 2 };
  var DIAGRAM_ARROW = 8; // the arrowhead's length and width
  var diagramEngine = null; // one ELK for every diagram, made on first use
  var diagramCount = 0; // numbers each svg, so its marker id is unique on the page
  var measureCtx = null; // the canvas context diagramMeasure measures text on

  // diagramMeasure returns the measure(string, role) PresentViz sizes
  // boxes and labels with: the width of the string in the font its role
  // is drawn in (label: a node's name; group: a group's label with its
  // tracking; anything else: a text line or an edge label), in the block's
  // font family. The header's font control changes that family after the
  // layout, so the boxes keep the measured width until a reload.
  function diagramMeasure(block) {
    if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
    var family = getComputedStyle(block).fontFamily || 'sans-serif';
    return function (text, role) {
      text = String(text);
      measureCtx.font = (role === 'label' ? DIAGRAM_LABEL_FONT : role === 'group' ? DIAGRAM_GROUP_FONT : DIAGRAM_TEXT_FONT) + family;
      var w = measureCtx.measureText(text).width;
      return role === 'group' ? w + text.length * DIAGRAM_GROUP_FONT_PX * DIAGRAM_GROUP_TRACKING : w;
    };
  }
  // diagramLayout lays the spec out with ELK once the page's fonts have
  // loaded (a fallback font measures differently), resolving to ELK's
  // graph with every position filled in, relative to the root.
  function diagramLayout(block, spec) {
    var fonts = document.fonts && document.fonts.ready ? document.fonts.ready : Promise.resolve();
    return fonts.then(function () {
      if (!diagramEngine) diagramEngine = new ELK();
      var measure = diagramMeasure(block, spec);
      return diagramEngine.layout(PresentViz.diagramGraph(spec, measure)).then(function (layout) {
        return { graph: layout, measure: measure };
      });
    });
  }
  // diagramBoxes maps every group and node id to its laid-out box.
  function diagramBoxes(layout) {
    var boxes = Object.create(null);
    function walk(children) {
      (children || []).forEach(function (c) {
        boxes[c.id] = { x: c.x, y: c.y, width: c.width, height: c.height };
        walk(c.children);
      });
    }
    walk(layout.children);
    return boxes;
  }
  // diagramFlags returns, for an element's datum, whether it is off (not
  // shown at step at, or everything while the entry has not started), lit
  // by the step's focus, or dimmed by it.
  function diagramFlags(spec, at, entering) {
    var shown = PresentViz.diagramShown(spec, at), focus = PresentViz.diagramFocus(spec, at);
    return function (d) {
      var lit = !!(focus && focus[d.kind][d.key]);
      return { off: entering || !shown[d.kind][d.key], focus: lit, dim: !!focus && !lit };
    };
  }
  // diagramClass is an element's class list: its own classes, then the
  // step's flags, so the first paint already has them.
  function diagramClass(base, flags) {
    return function (d) {
      var f = flags(d);
      return base(d) + (f.off ? ' off' : '') + (f.focus ? ' focus' : '') + (f.dim ? ' dim' : '');
    };
  }
  // diagramToken is an id made safe for a class name: ids are any
  // non-empty string, and a space would split the class.
  function diagramToken(id) {
    return String(id).replace(/[^A-Za-z0-9_-]/g, '_');
  }
  function diagramTone(item) {
    return item.tone ? ' tone-' + item.tone : '';
  }
  // diagramDepth is how deep a group nests, so parents draw first and a
  // nested group sits on top of the one holding it.
  function diagramDepth(spec) {
    var parent = Object.create(null);
    (spec.groups || []).forEach(function (g) { parent[g.id] = g.group || ''; });
    return function (id) {
      var n = 0;
      for (var p = parent[id]; p; p = parent[p]) n++;
      return n;
    };
  }
  // diagramSvg makes a diagram's svg, detached: sized to the layout in
  // viewBox units, with the arrowhead every edge ends in.
  function diagramSvg(layout) {
    var id = 'present-diagram-' + (++diagramCount) + '-arrow';
    var svg = d3.create('svg').attr('class', 'present-diagram-svg')
      .attr('viewBox', '0 0 ' + layout.width + ' ' + layout.height);
    svg.append('defs').append('marker').attr('id', id).attr('viewBox', '0 0 10 10')
      .attr('refX', 10).attr('refY', 5).attr('markerUnits', 'userSpaceOnUse')
      .attr('markerWidth', DIAGRAM_ARROW).attr('markerHeight', DIAGRAM_ARROW).attr('orient', 'auto-start-reverse')
      .append('path').attr('class', 'dedge-arrow').attr('d', 'M0,0L10,5L0,10z');
    return svg.node();
  }
  function diagramRect(g, w, h, rx) {
    return g.append('rect').attr('class', 'dnode-shape').attr('width', w).attr('height', h).attr('rx', rx);
  }
  // DIAGRAM_SHAPES draws a node's shape by kind into its group, the box w
  // by h at the origin. external is the service box; the stylesheet
  // dashes it.
  var DIAGRAM_SHAPES = {
    service: function (g, w, h) { diagramRect(g, w, h, 8); },
    external: function (g, w, h) { diagramRect(g, w, h, 8); },
    store: function (g, w, h) {
      var rx = w / 2, ry = DIAGRAM_CAP_RY, arc = 'A' + rx + ',' + ry + ' 0 0 0 ';
      g.append('path').attr('class', 'dnode-shape')
        .attr('d', 'M0,' + ry + 'V' + (h - ry) + arc + w + ',' + (h - ry) + 'V' + ry + arc + '0,' + ry + 'Z');
      g.append('ellipse').attr('class', 'dnode-shape').attr('cx', rx).attr('cy', ry).attr('rx', rx).attr('ry', ry);
    },
    queue: function (g, w, h) {
      var r = h / 2, x = DIAGRAM_PIPE_INSET, dy = Math.sqrt(Math.max(0, r * r - (r - x) * (r - x)));
      diagramRect(g, w, h, r);
      g.append('path').attr('class', 'dnode-detail')
        .attr('d', 'M' + x + ',' + (r - dy) + 'V' + (r + dy) + 'M' + (w - x) + ',' + (r - dy) + 'V' + (r + dy));
    },
    person: function (g, w, h) {
      var f = DIAGRAM_FIGURE, cx = f.inset + f.shoulders, base = f.inset + 2 * f.head + f.gap + f.shoulders;
      diagramRect(g, w, h, 8);
      g.append('circle').attr('class', 'dnode-figure').attr('cx', cx).attr('cy', f.inset + f.head).attr('r', f.head);
      g.append('path').attr('class', 'dnode-figure')
        .attr('d', 'M' + f.inset + ',' + base + 'a' + f.shoulders + ',' + f.shoulders + ' 0 0 1 ' + 2 * f.shoulders + ',0z');
    }
  };
  // drawDiagramGroups draws the boundaries, parents before the groups
  // they hold: a tinted box with its label in the top padding.
  function drawDiagramGroups(layer, spec, boxes, flags) {
    var depth = diagramDepth(spec), at = PresentViz.diagramGroupLabel;
    var data = (spec.groups || []).map(function (g, i) {
      return { kind: 'groups', key: g.id, item: g, box: boxes[g.id], order: i };
    }).sort(function (a, b) { return depth(a.key) - depth(b.key) || a.order - b.order; });
    var g = layer.selectAll('g').data(data).join('g')
      .attr('class', diagramClass(function (d) { return 'dgroup g-' + diagramToken(d.key) + diagramTone(d.item); }, flags))
      .attr('transform', function (d) { return 'translate(' + d.box.x + ',' + d.box.y + ')'; });
    g.append('rect').attr('class', 'dgroup-box').attr('rx', DIAGRAM_GROUP_RX)
      .attr('width', function (d) { return d.box.width; }).attr('height', function (d) { return d.box.height; });
    g.append('text').attr('class', 'dgroup-label').attr('x', at.x).attr('y', at.y)
      .text(function (d) { return d.item.label; });
  }
  // drawDiagramNodes draws the boxes: the shape by kind, the name, the
  // text lines under it, and the text as a tooltip.
  function drawDiagramNodes(layer, spec, boxes, flags, measure) {
    var data = (spec.nodes || []).map(function (n) { return { kind: 'nodes', key: n.id, item: n, box: boxes[n.id] }; });
    layer.selectAll('g').data(data).join('g')
      .attr('class', diagramClass(function (d) {
        return 'dnode n-' + diagramToken(d.key) + ' kind-' + (d.item.kind || 'service') + diagramTone(d.item);
      }, flags))
      .attr('transform', function (d) { return 'translate(' + d.box.x + ',' + d.box.y + ')'; })
      .each(function (d) {
        var g = d3.select(this), node = d.item, w = d.box.width;
        (DIAGRAM_SHAPES[node.kind] || DIAGRAM_SHAPES.service)(g, w, d.box.height);
        var lines = PresentViz.diagramBox(node, measure).lines, at = PresentViz.diagramText(node, lines.length, w);
        g.append('text').attr('class', 'dnode-label').attr('x', at.x).attr('y', at.label).text(node.label);
        lines.forEach(function (line, i) {
          g.append('text').attr('class', 'dnode-text').attr('x', at.x).attr('y', at.lines[i]).text(line);
        });
        if (node.text) g.append('title').text(node.label + ': ' + node.text);
      });
  }
  // drawDiagramEdges draws the arrows along ELK's routes: the line, a dot
  // on a flow edge (none under Reduce Motion, where the dash alone says
  // flow), and the label on a halo in the surface colour.
  function drawDiagramEdges(layer, spec, layout, flags, arrowId, measure) {
    var routes = Object.create(null);
    (layout.edges || []).forEach(function (e) { routes[e.id] = e; });
    var data = (spec.edges || []).map(function (e, k) {
      return { kind: 'edges', key: k, item: e, route: routes['e' + k] || {} };
    });
    var g = layer.selectAll('g').data(data).join('g')
      .attr('class', diagramClass(function (d) { return 'dedge e-' + d.key + (d.item.flow ? ' flow' : ''); }, flags));
    g.append('path').attr('class', 'dedge-line').attr('marker-end', 'url(#' + arrowId + ')')
      .attr('d', function (d) { return PresentViz.elkPath(d.route); });
    if (!reducedMotion) {
      g.filter(function (d) { return d.item.flow && d.route.sections && d.route.sections.length; })
        .append('circle').attr('class', 'dflow').attr('r', DIAGRAM_DOT_R)
        .attr('cx', function (d) { return d.route.sections[0].startPoint.x; })
        .attr('cy', function (d) { return d.route.sections[0].startPoint.y; });
    }
    // The label sits on the longest straight run of the route (ELK never
    // saw it, so the chain stays as tight as its boxes), on a halo.
    g.each(function (d) {
      var p = d.item.label ? PresentViz.routeLabelPoint(d.route) : null;
      if (!p) return;
      var w = Math.ceil(measure(d.item.label, 'text')) + DIAGRAM_HALO.pad, h = DIAGRAM_HALO.h;
      var label = d3.select(this).append('g').attr('class', 'dedge-label')
        .attr('transform', 'translate(' + (p.x - w / 2) + ',' + (p.y - h / 2) + ')');
      label.append('rect').attr('width', w).attr('height', h).attr('rx', DIAGRAM_HALO.rx);
      label.append('text').attr('x', w / 2).attr('y', h / 2).text(d.item.label);
    });
  }
  // drawDiagram draws the whole diagram into its detached svg, groups
  // under edges under nodes, every element already carrying its step's
  // classes.
  function drawDiagram(svg, spec, result, flags) {
    var root = d3.select(svg), boxes = diagramBoxes(result.graph);
    drawDiagramGroups(root.append('g').attr('class', 'dgroups'), spec, boxes, flags);
    var arrowId = svg.querySelector('marker').id;
    drawDiagramEdges(root.append('g').attr('class', 'dedges'), spec, result.graph, flags, arrowId, result.measure);
    drawDiagramNodes(root.append('g').attr('class', 'dnodes'), spec, boxes, flags, result.measure);
  }
  // diagramElements are the parts that step, dim, and enter: every group,
  // node, and edge, each carrying its datum.
  function diagramElements(svg) {
    return Array.prototype.slice.call(svg.querySelectorAll('.dgroup, .dnode, .dedge'));
  }
  // diagramStep moves a drawn diagram to step n (null: everything shown,
  // nothing dimmed).
  function diagramStep(svg, spec, n) {
    var flags = diagramFlags(spec, n, false);
    diagramElements(svg).forEach(function (el) {
      var f = flags(el.__data__);
      el.classList.toggle('off', f.off);
      el.classList.toggle('focus', f.focus);
      el.classList.toggle('dim', f.dim);
    });
  }
  // diagramStepHook is the deck's hook for a diagram block. It reads the
  // svg when called, since a step can arrive before the layout is drawn
  // (the draw then reads data-step-at itself). Reduce Motion and a deck
  // that cuts swap the classes with the transitions off for a frame.
  function diagramStepHook(block, spec) {
    return function (n) {
      var svg = block._diagramSvg;
      if (svg) cutStep(block, svg, function () { diagramStep(svg, spec, n); });
    };
  }
  // diagramDelays times the entry: groups first, then the nodes layer by
  // layer along the flow (x for LR, y for TB), then each edge with its
  // source's layer. total is when the last edge has drawn in.
  function diagramDelays(spec, boxes, fade) {
    var lr = spec.direction !== 'TB';
    function along(id) { var b = boxes[id]; return lr ? b.x + b.width / 2 : b.y + b.height / 2; }
    var layerOf = Object.create(null), layer = -1, start = -Infinity;
    (spec.nodes || []).map(function (n) { return n.id; })
      .sort(function (a, b) { return along(a) - along(b); })
      .forEach(function (id) {
        if (along(id) - start > DIAGRAM_LAYER_PX) { layer++; start = along(id); }
        layerOf[id] = layer;
      });
    var nodes = DIAGRAM_PHASE_MS, edges = nodes + (layer + 1) * DIAGRAM_STAGGER_MS + DIAGRAM_PHASE_MS;
    return {
      at: function (d) {
        if (d.kind === 'groups') return 0;
        if (d.kind === 'nodes') return nodes + layerOf[d.key] * DIAGRAM_STAGGER_MS;
        return edges + layerOf[d.item.from] * DIAGRAM_STAGGER_MS;
      },
      total: edges + (layer + 1) * DIAGRAM_STAGGER_MS + fade
    };
  }
  // diagramFadeMs is the opacity transition shell.html gives the diagram's
  // parts, read from its --diagram-fade so the duration lives in one place;
  // an edge draws in over the same time.
  function diagramFadeMs(svg) {
    return parseFloat(getComputedStyle(svg).getPropertyValue('--diagram-fade')) || 350;
  }
  // diagramDrawIn draws an edge's line from its source to its tip: a dash
  // as long as the line, offset by its length and run to 0.
  function diagramDrawIn(line, delay, fade) {
    line.style.transition = 'stroke-dashoffset ' + fade + 'ms ease ' + delay + 'ms';
    line.style.strokeDashoffset = '0';
  }
  // diagramEnter fades the drawn diagram in. Everything starts off; a
  // frame later the elements the step shows lose it, each delayed by
  // diagramDelays, and the edges draw in. Once the last is in, the delays
  // and the draw-in dash clear (a flow edge gets its own dash back from
  // the stylesheet) and the flow dots, hidden by .entering, fade in.
  function diagramEnter(block, svg, spec, boxes) {
    var fade = diagramFadeMs(svg), delays = diagramDelays(spec, boxes, fade);
    svg.classList.add('entering');
    requestAnimationFrame(function () {
      var flags = diagramFlags(spec, chartStepAt(block), false), els = diagramElements(svg);
      var shown = els.filter(function (el) { return !flags(el.__data__).off; });
      var lines = svg.querySelectorAll('.dedge-line');
      shown.forEach(function (el) {
        var line = el.__data__.kind === 'edges' && el.querySelector('.dedge-line');
        if (!line) return;
        var len = line.getTotalLength();
        line.style.strokeDasharray = len + ' ' + len;
        line.style.strokeDashoffset = String(len);
      });
      svg.getBoundingClientRect(); // flush, so the off state and the dash animate
      shown.forEach(function (el) {
        var d = el.__data__, delay = delays.at(d);
        el.style.transitionDelay = delay + 'ms';
        el.classList.remove('off');
        if (d.kind === 'edges') diagramDrawIn(el.querySelector('.dedge-line'), delay, fade);
      });
      setTimeout(function () {
        els.forEach(function (el) { el.style.transitionDelay = ''; });
        Array.prototype.forEach.call(lines, function (l) {
          l.style.transition = ''; l.style.strokeDasharray = ''; l.style.strokeDashoffset = '';
        });
        svg.classList.remove('entering');
      }, delays.total);
    });
  }
  // visibleLoop calls frame(now) every animation frame while el is on
  // screen and the tab is showing, pausing otherwise the way
  // startGraphFlow does, and returns the function that stops it for good.
  function visibleLoop(el, frame) {
    var raf = 0, visible = true, stopped = false, io = null;
    function kick() { if (!raf && !stopped && visible && !document.hidden) raf = requestAnimationFrame(tick); }
    function tick(now) { raf = 0; if (stopped) return; frame(now); kick(); }
    if ('IntersectionObserver' in window) {
      // Entries batch; the last one is the current state.
      io = new IntersectionObserver(function (entries) {
        visible = entries[entries.length - 1].isIntersecting;
        kick();
      }, { threshold: 0.05 });
      io.observe(el);
    }
    document.addEventListener('visibilitychange', kick);
    kick();
    return function () {
      stopped = true;
      if (raf) cancelAnimationFrame(raf);
      if (io) io.disconnect();
      document.removeEventListener('visibilitychange', kick);
    };
  }
  // diagramFlow moves one dot along each flow edge, in viewBox units, at
  // PresentViz.flowSpeed for its weight against the heaviest flow edge.
  // A rebuild stops the block's previous loop; Reduce Motion drew no dots.
  function diagramFlow(block, svg, spec) {
    if (block._flowStop) { block._flowStop(); block._flowStop = null; }
    var max = 0, runs = [];
    (spec.edges || []).forEach(function (e) { if (e.flow) max = Math.max(max, Number(e.weight) || 0); });
    svg.querySelectorAll('.dedge.flow').forEach(function (g) {
      var dot = g.querySelector('.dflow'), line = g.querySelector('.dedge-line');
      var length = dot ? line.getTotalLength() : 0;
      if (length > 0) runs.push({ g: g, dot: dot, line: line, length: length, speed: PresentViz.flowSpeed(g.__data__.item.weight, max) });
    });
    if (!runs.length) return;
    block._flowStop = visibleLoop(svg, function (now) {
      runs.forEach(function (r) {
        // An edge a step hides or dims shows no dot, so it need not move.
        if (r.g.classList.contains('off') || r.g.classList.contains('dim')) return;
        var p = r.line.getPointAtLength((now * r.speed) % r.length);
        r.dot.setAttribute('cx', p.x);
        r.dot.setAttribute('cy', p.y);
      });
    });
  }
  // diagramMaxHeight is the canvas's height cap in pixels, which the
  // stylesheet sets per context (page, slide, presenting, solo).
  function diagramMaxHeight(wrap) {
    var v = parseFloat(getComputedStyle(wrap).maxHeight);
    return isNaN(v) ? DIAGRAM_MAX_H : v;
  }
  // diagramFitter returns fit(), which scales the svg to its wrapper: as
  // wide as the wrapper allows, no taller than its cap, and at most
  // DIAGRAM_MAX_SCALE, so a three-box diagram does not fill a slide. It
  // only sets the svg's size, so nothing redraws or replays.
  function diagramFitter(wrap, svg, layout) {
    var last = null;
    return function () {
      var w = wrap.clientWidth, maxH = diagramMaxHeight(wrap);
      if (w < 1 || (last && Math.abs(last.w - w) <= 1 && Math.abs(last.h - maxH) <= 1)) return;
      last = { w: w, h: maxH };
      var s = Math.min(w / layout.width, maxH / layout.height, DIAGRAM_MAX_SCALE);
      svg.setAttribute('width', String(Math.floor(layout.width * s)));
      svg.setAttribute('height', String(Math.floor(layout.height * s)));
    };
  }
  // mountDiagram puts a laid-out diagram on the page the first time its
  // wrapper has a width (a slide not showing has none): drawn with its
  // step's classes before it enters the page, so the first paint plays no
  // transition, then sized, entered, and set moving. After that a change
  // of width (the observer) or of the height cap (the deck's refresh)
  // only refits it.
  function mountDiagram(block, spec, result, wrap, animate) {
    var fit = null;
    function attempt() {
      if (fit) { fit(); return; }
      if (wrap.clientWidth < 1) return;
      var svg = diagramSvg(result.graph);
      drawDiagram(svg, spec, result, diagramFlags(spec, chartStepAt(block), animate));
      fit = diagramFitter(wrap, svg, result.graph);
      fit();
      wrap.appendChild(svg);
      block._diagramSvg = svg;
      if (animate) diagramEnter(block, svg, spec, diagramBoxes(result.graph));
      diagramFlow(block, svg, spec);
    }
    block._diagramFit = attempt;
    if (block._diagramObserver) block._diagramObserver.disconnect();
    if (typeof ResizeObserver === 'function') {
      // A frame later, since fitting the svg resizes the wrapper the
      // observer watches, which the browser reports as a loop otherwise.
      block._diagramObserver = new ResizeObserver(function () { requestAnimationFrame(attempt); });
      block._diagramObserver.observe(wrap);
    }
    attempt();
  }
  // buildDiagram builds a diagram block: its step hook at once, then the
  // ELK layout, kept on the block, and the drawing. A later call (a theme
  // change, the deck's refresh) only refits: every colour is a CSS
  // variable and the geometry does not change, so ELK never runs twice.
  // The entry plays once, on the brief's mount or the deck's first
  // showing, and never under Reduce Motion or a deck that cuts
  // (opts.duration 0).
  function buildDiagram(block, opts) {
    if (typeof d3 === 'undefined' || typeof ELK === 'undefined') {
      chartNote(block, needsAsset('Diagram', 'elk and d3 assets'));
      return;
    }
    if (block._built) { if (block._diagramFit) block._diagramFit(); return; }
    var specEl = block.querySelector('.diagram-spec'), wrap = block.querySelector('.present-diagram-canvas');
    if (!specEl || !wrap) return;
    var spec;
    try { spec = JSON.parse(specEl.textContent); } catch (e) { return; }
    block._built = true;
    var animate = !reducedMotion && !(opts && opts.duration === 0);
    block._presentStep = diagramStepHook(block, spec);
    diagramLayout(block, spec).then(function (result) {
      block._diagramLayout = result.graph;
      mountDiagram(block, spec, result, wrap, animate);
    }).catch(function (err) {
      chartNote(block, 'Diagram could not be drawn: ' + ((err && err.message) || err));
    });
  }
  // initPresentDiagrams builds every diagram block under root (the
  // document by default); with opts.builtOnly it only refits the ones
  // already built, which is all a theme change in the deck needs, since
  // a diagram's colours are CSS variables.
  function initPresentDiagrams(root, opts) {
    (root || document).querySelectorAll('.present-diagram').forEach(function (block) {
      if (opts && opts.builtOnly && !block._built) return;
      buildDiagram(block, opts);
    });
  }

  // ── Live terminal blocks (xterm.js) ──
  // A terminal block shows a terminal session serve owns, named by the page
  // and the block's data-session, over a WebSocket on
  // /api/p/{id}/terminal/{session}. The session outlives the tab: every page
  // update reloads it (watchVersion), so the tab attaches again and serve
  // replays the kept scrollback before the live stream. Two blocks naming
  // one session show the same process, each over its own socket, and the
  // last resize wins. A block opens its xterm once its canvas has a width (a
  // slide not showing has none) and the mono font has loaded, and attaches
  // the first time it is seen: scrolled into view in the brief, its slide's
  // first showing in the deck, or Run. The switch lives in serve (present
  // terminal on); with it off, or on a server without the route (an older
  // binary, a shared instance), the canvas names the command instead.
  var TERMINAL_ROWS = 24; // the rows when the block names none
  var TERMINAL_FONT_PX = 13; // the size when the stylesheet sets no --terminal-font-size
  var TERMINAL_LINE_GUESS = 1.3; // a cell's height per font px, until xterm has measured one
  var TERMINAL_SCROLLBACK = 5000;
  var TERMINAL_RETRY_MS = 500; // the first reconnect delay, doubled per try
  var TERMINAL_RETRY_MAX_MS = 8000;
  var TERMINAL_RETRIES = 10;
  var TERMINAL_OFF_POLL_MS = 3000; // how often a block showing the off note asks the switch again
  var TERMINAL_FONT_WAIT_MS = 2000; // how long a terminal waits for the mono font
  var terminals = []; // every upgraded block's controller
  var terminalLeaving = false; // the tab is unloading, so a closing socket is no drop
  var terminalFonts = null; // the one wait for the mono font, made on first use

  function terminalVar(el, name) {
    return getComputedStyle(el).getPropertyValue(name).trim();
  }
  function terminalFontSize(ctl) {
    return parseFloat(terminalVar(ctl.canvas, '--terminal-font-size')) || TERMINAL_FONT_PX;
  }
  // terminalFills says the stylesheet gives the canvas its height (a slide
  // whose only block is the terminal sets --terminal-fill) and the block
  // names no rows, so the rows follow the height instead of setting it.
  function terminalFills(ctl) {
    return !ctl.rowsNamed && terminalVar(ctl.canvas, '--terminal-fill') === '1';
  }
  // terminalFontsReady resolves once the page's mono face has loaded, or
  // after a short wait when it does not (no fonts asset). xterm measures
  // its cell when it opens, and a fallback face would leave the columns
  // off until a reload.
  function terminalFontsReady(family) {
    if (!terminalFonts) {
      var load = document.fonts && document.fonts.load
        ? document.fonts.load(TERMINAL_FONT_PX + 'px ' + family).catch(function () {})
        : Promise.resolve();
      terminalFonts = Promise.race([load, new Promise(function (done) { setTimeout(done, TERMINAL_FONT_WAIT_MS); })]);
    }
    return terminalFonts;
  }
  // hexColor is colorLiteral as #rrggbb, the form xterm's theme parser
  // always takes.
  function hexColor(value) {
    var c = colorLiteral(value).match(/\d+/g);
    return '#' + c.slice(0, 3).map(function (n) { return ('0' + Number(n).toString(16)).slice(-2); }).join('');
  }
  function darkColor(hex) {
    var n = parseInt(hex.slice(1), 16);
    return (0.2126 * (n >> 16) + 0.7152 * ((n >> 8) & 255) + 0.0722 * (n & 255)) / 255 < 0.5;
  }
  // terminalTheme maps the palette onto xterm's colours, read at call time
  // so a theme change only sets them again. The frame is the code block's
  // (--code-bg and --code-text, which webkit scopes to the block). The ANSI
  // colours swap ends with the mode. On a light page the semantic roles
  // (red, green, ...) read near 3:1, so the normal colours take the tone
  // text roles (about 8:1) and the bright ones the semantic roles. On a
  // dark page the tone text is washed out (above 11:1) and the semantic
  // roles read near 5:1, so the two trade places. Black is the strongest
  // ink on a light page and a dim one on a dark page. On a light page
  // white and bright white are the page itself (--bg, the paper the block
  // sits on), so black text on a white background stays legible, and
  // bright black is the lightest text grey, which keeps it apart from
  // bright white. Every colour resolves through a probe in the block,
  // since a role can be scoped or a color-mix().
  function terminalTheme(block) {
    var probe = document.createElement('span');
    probe.style.cssText = 'position:absolute;width:0;height:0;transition:none';
    block.appendChild(probe);
    function color(expr) {
      probe.style.color = '';
      probe.style.color = expr;
      return hexColor(getComputedStyle(probe).color);
    }
    var bg = color('var(--code-bg)'), fg = color('var(--code-text)'), dark = darkColor(bg);
    function tone(t) { return 'var(--tone-' + t + '-text)'; }
    function role(r) { return 'var(--' + r + ')'; }
    // ansi returns the [normal, bright] pair for one hue from its tone and
    // its semantic role.
    function ansi(t, r) {
      return dark ? [color(role(r)), color(tone(t))] : [color(tone(t)), color(role(r))];
    }
    function mix(a, b) { return 'color-mix(in srgb, ' + a + ' 50%, ' + b + ')'; }
    var red = ansi('red', 'red'), green = ansi('green', 'green'), yellow = ansi('amber', dark ? 'yellow' : 'amber');
    var blue = ansi('blue', 'blue'), magenta = ansi('purple', 'purple');
    var cyan = dark
      ? [color(mix(role('blue'), role('green'))), color(mix(tone('blue'), tone('green')))]
      : [color(mix(tone('blue'), tone('green'))), color(mix(role('blue'), role('green')))];
    var theme = {
      background: bg, foreground: fg, cursor: fg, cursorAccent: bg,
      // xterm draws an opaque selection colour at 30 % over the text.
      selectionBackground: color(role('primary')),
      black: color(role(dark ? 'border-hover' : 'text-1')), brightBlack: color(role('text-3')),
      red: red[0], brightRed: red[1], green: green[0], brightGreen: green[1],
      yellow: yellow[0], brightYellow: yellow[1], blue: blue[0], brightBlue: blue[1],
      magenta: magenta[0], brightMagenta: magenta[1], cyan: cyan[0], brightCyan: cyan[1],
      white: color(role(dark ? 'text-2' : 'bg')), brightWhite: color(role(dark ? 'text-1' : 'bg'))
    };
    block.removeChild(probe);
    return theme;
  }
  // terminalStatus shows the block's state in its header (connecting,
  // live, exited (code N), off, disconnected, killed); data-state carries
  // it for the stylesheet.
  function terminalStatus(ctl, state, text) {
    ctl.state = state;
    ctl.block.setAttribute('data-state', state);
    if (ctl.status) ctl.status.textContent = text || state;
  }
  // terminalNote lays a message over the canvas (the switch is off, the
  // asset is missing), or takes it away when given nothing. The message is
  // one span in the centring box, so its words and code flow as a line.
  function terminalNote(ctl, parts) {
    var n = ctl.canvas.querySelector('.present-terminal-note');
    if (!parts) { if (n) n.remove(); return; }
    if (!n) { n = Webkit.el('div', { class: 'present-terminal-note' }); ctl.canvas.appendChild(n); }
    n.textContent = '';
    n.appendChild(Webkit.el('span', {}, [].concat(parts)));
  }
  function terminalOffNote() {
    return ['Live terminal is off. Run ', Webkit.el('code', {}, 'present terminal on'), ', then press Run.'];
  }
  // terminalSwitch asks serve whether the live terminal is on: 'on', 'off'
  // (switched off, or a 404 from a server without the route), or 'error'
  // (serve unreachable, which a retry may get past).
  function terminalSwitch() {
    return fetch('/api/terminal', { headers: { accept: 'application/json' } })
      .then(function (r) {
        if (r.status === 404) return 'off';
        if (!r.ok) return 'error';
        return r.json().then(function (d) { return d && d.enabled ? 'on' : 'off'; });
      })
      .catch(function () { return 'error'; });
  }
  function terminalPath(ctl) {
    return '/api/p/' + encodeURIComponent(id) + '/terminal/' + encodeURIComponent(ctl.session);
  }
  function terminalSend(ctl, data) {
    if (ctl.ws && ctl.ws.readyState === WebSocket.OPEN) ctl.ws.send(data);
  }
  // terminalResize tells serve the terminal's size. A block not showing
  // sends nothing: its size is not the one on screen.
  function terminalResize(ctl) {
    if (!ctl.term || !ctl.shown) return;
    terminalSend(ctl, JSON.stringify({ t: 'resize', cols: ctl.term.cols, rows: ctl.term.rows }));
  }
  // terminalHeight is the canvas height that holds the block's rows: the
  // cell height xterm measured (the private field the fit addon reads as
  // well) times the rows, plus the padding above and below the terminal
  // element (--terminal-pad-y, which the stylesheet sets that padding
  // from). Before xterm opens it has measured nothing, so the cell is the
  // guess; the placeholder takes its height from here too, which keeps the
  // page from jumping when xterm opens.
  function terminalHeight(ctl) {
    var core = ctl.term && ctl.term._core, dims = core && core._renderService && core._renderService.dimensions;
    var cell = dims && dims.css && dims.css.cell && dims.css.cell.height;
    if (!cell) cell = Math.ceil(terminalFontSize(ctl) * TERMINAL_LINE_GUESS);
    var pad = parseFloat(terminalVar(ctl.canvas, '--terminal-pad-y')) || 0;
    return Math.ceil(ctl.rows * cell + 2 * pad);
  }
  // terminalFit sizes the canvas and the terminal to the context: the font
  // size the stylesheet names (larger while presenting), the canvas height
  // from the rows unless the block fills (terminalFills), and the columns
  // from the width. Coming back into view it sends its size again, since
  // another view of the same session may have resized it meanwhile.
  function terminalFit(ctl) {
    var term = ctl.term, canvas = ctl.canvas;
    if (!term) return;
    if (canvas.clientWidth < 1) { ctl.shown = false; return; }
    var size = terminalFontSize(ctl);
    if (term.options.fontSize !== size) term.options.fontSize = size;
    canvas.style.height = terminalFills(ctl) ? '' : terminalHeight(ctl) + 'px';
    ctl.fit.fit();
    if (!ctl.shown) { ctl.shown = true; terminalResize(ctl); }
  }
  // terminalMount opens the block's xterm once the canvas has a width and
  // the font has loaded, then fits it and attaches when that is already
  // wanted. A later call only refits. The placeholder height is set again
  // first, since the font size may have changed since the upgrade (a deck
  // that comes back presenting after a reload).
  function terminalMount(ctl) {
    if (ctl.term) { terminalFit(ctl); return; }
    if (ctl.inert || ctl.opening || ctl.canvas.clientWidth < 1) return;
    ctl.opening = true;
    if (!terminalFills(ctl)) ctl.canvas.style.height = terminalHeight(ctl) + 'px';
    var family = cssVar('--mono', 'monospace');
    terminalFontsReady(family).then(function () {
      var term = new Terminal({
        cursorBlink: true, scrollback: TERMINAL_SCROLLBACK, fontFamily: family,
        fontSize: terminalFontSize(ctl), theme: terminalTheme(ctl.block)
      });
      ctl.fit = new FitAddon.FitAddon();
      term.loadAddon(ctl.fit);
      term.open(ctl.canvas);
      ctl.term = term;
      term.onData(function (d) { terminalSend(ctl, ctl.encoder.encode(d)); });
      // Binary input (some mouse reports) is one byte per character.
      term.onBinary(function (d) {
        var b = new Uint8Array(d.length);
        for (var i = 0; i < d.length; i++) b[i] = d.charCodeAt(i) & 255;
        terminalSend(ctl, b);
      });
      term.onResize(function () { terminalResize(ctl); });
      terminalFit(ctl);
      if (ctl.wanted) terminalAttach(ctl);
    });
  }
  // terminalAttach connects the block to its session once serve says the
  // live terminal is on. Off, the canvas says how to turn it on and Run
  // asks again. A block held by Kill stays down until Run, so a later
  // showing does not start the session again, and an inert block (a shared
  // instance, no xterm asset) never asks at all.
  function terminalAttach(ctl) {
    if (ctl.held || ctl.inert) return;
    ctl.wanted = true;
    if (!ctl.term) { terminalMount(ctl); return; }
    if (ctl.ws || ctl.checking || ctl.exited || terminalLeaving) return;
    clearTimeout(ctl.retry);
    ctl.checking = true;
    terminalStatus(ctl, 'connecting');
    terminalSwitch().then(function (sw) {
      ctl.checking = false;
      if (ctl.held) return;
      if (sw === 'off') { terminalStatus(ctl, 'off'); terminalNote(ctl, terminalOffNote()); terminalPollOff(ctl); return; }
      if (sw === 'error') { terminalRetry(ctl); return; }
      terminalNote(ctl, null);
      terminalConnect(ctl);
    });
  }
  // terminalPollOff keeps a block that found the switch off asking, so
  // `present terminal on` in another window starts it without a reload or
  // a Run press, the way its first showing would have. The asks are quiet
  // (no status change) and pause while the tab is hidden or the block is
  // off screen (a slide not showing has no width), so only the block the
  // room sees starts its session. A block whose session has ended waits
  // for Run instead: the switch coming back is no reason to run its
  // command again.
  function terminalPollOff(ctl) {
    clearTimeout(ctl.retry);
    ctl.retry = setTimeout(function () {
      if (ctl.state !== 'off' || ctl.exited || ctl.held || ctl.ws) return;
      if (document.hidden || ctl.canvas.clientWidth < 1 || terminalUnrevealed(ctl)) { terminalPollOff(ctl); return; }
      terminalSwitch().then(function (sw) {
        if (ctl.state !== 'off' || ctl.exited || ctl.held || ctl.ws) return;
        if (sw === 'on') terminalAttach(ctl); else terminalPollOff(ctl);
      });
    }, TERMINAL_OFF_POLL_MS);
  }
  // terminalConnect opens the socket. The query carries the size only:
  // serve reads what the session runs (its command, directory and prefill)
  // from the stored page's block of that name, never from the browser,
  // since a script injected into the page body could otherwise open the
  // socket with any command. The block's data-cmd, data-cwd and
  // data-prefill are there for the reader.
  function terminalConnect(ctl) {
    var q = 'cols=' + ctl.term.cols + '&rows=' + ctl.term.rows;
    var ws;
    try {
      ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + terminalPath(ctl) + '?' + q);
    } catch (e) { terminalRetry(ctl); return; }
    ws.binaryType = 'arraybuffer';
    ctl.ws = ws;
    // Kill pressed while this socket was connecting waits for it
    // (terminalKill), since serve may be starting the session already.
    ws.onopen = function () {
      if (ctl.ws === ws && ctl.held) ws.send(JSON.stringify({ t: 'kill' }));
    };
    ws.onmessage = function (e) { terminalMessage(ctl, e.data); };
    // A socket this block let go of (Run on an exited session, Kill) is no
    // longer ctl.ws, so its close starts nothing. A held block whose socket
    // closes before the exit frame (it never opened, or dropped after the
    // kill went out) ends the session through the DELETE route instead.
    ws.onclose = function (e) {
      if (ctl.ws !== ws) return;
      ctl.ws = null;
      if (ctl.exited || terminalLeaving) return;
      if (ctl.held) { terminalDeleteHeld(ctl); return; }
      terminalClosed(ctl, e.code, e.reason);
    };
  }
  // terminalClosed reads why a socket closed without an exit frame. None
  // of 1000 (the session ended, its exit frame lost), 1008 (serve refused
  // the session, a bad cwd in its block say, with a reason meant for the
  // reader), and 1001 (serve is closing) comes right by retrying, which
  // would only ask again and again, so the block waits for Run. 1013
  // (the socket fell behind and serve dropped it) and any other drop
  // reconnect, and the replay fills in what was missed.
  function terminalClosed(ctl, code, reason) {
    if (code === 1000) {
      terminalExited(ctl, 'ended');
    } else if (code === 1008) {
      terminalStatus(ctl, 'disconnected', 'refused');
      terminalNote(ctl, reason || 'serve refused this terminal.');
    } else if (code === 1001) {
      terminalStatus(ctl, 'disconnected', 'server closing');
    } else {
      terminalRetry(ctl);
    }
  }
  // terminalMessage takes one frame: PTY bytes in a binary frame, control
  // JSON in a text frame. hello comes before the replay, which holds the
  // whole kept scrollback, so the screen starts over rather than repeat
  // what an earlier socket already showed.
  function terminalMessage(ctl, data) {
    if (typeof data !== 'string') { ctl.term.write(new Uint8Array(data)); return; }
    var msg;
    try { msg = JSON.parse(data); } catch (e) { return; }
    // Valid JSON need not be an object (null, a number), and reading .t off
    // null throws.
    if (!msg || typeof msg !== 'object') return;
    if (msg.t === 'hello') {
      ctl.term.reset();
      ctl.tries = 0;
      // A held block is open only to send the kill, so it never says live.
      if (!ctl.held) terminalStatus(ctl, 'live');
      terminalResize(ctl);
    } else if (msg.t === 'exit') {
      terminalExited(ctl, 'exited (code ' + msg.code + ')');
    }
  }
  // terminalExited marks the session ended. `present terminal off` ends
  // every session the same way a program exiting does, so the block asks
  // the switch once and, when it is off, shows the off note in place of
  // the exit code. A Run pressed meanwhile keeps its own state.
  function terminalExited(ctl, label) {
    ctl.exited = true;
    terminalStatus(ctl, 'exited', label);
    terminalSwitch().then(function (sw) {
      if (sw !== 'off' || !ctl.exited || ctl.ws) return;
      terminalStatus(ctl, 'off');
      terminalNote(ctl, terminalOffNote());
    });
  }
  // terminalRetry reconnects after a dropped socket or an unreachable
  // serve: 0.5 s doubling to 8 s, ten tries, then it stays disconnected
  // until Run. Every try asks the switch again, so turning it off ends the
  // retries with the off note.
  function terminalRetry(ctl) {
    terminalStatus(ctl, 'disconnected');
    if (ctl.tries >= TERMINAL_RETRIES) return;
    var delay = Math.min(TERMINAL_RETRY_MS * Math.pow(2, ctl.tries), TERMINAL_RETRY_MAX_MS);
    ctl.tries++;
    clearTimeout(ctl.retry);
    ctl.retry = setTimeout(function () { terminalAttach(ctl); }, delay);
  }
  function terminalLetGo(ctl) {
    var ws = ctl.ws;
    ctl.ws = null;
    clearTimeout(ctl.retry);
    if (ws) { try { ws.close(); } catch (e) {} }
  }
  function terminalDelete(ctl) {
    return fetch(terminalPath(ctl), { method: 'DELETE' }).catch(function () {});
  }
  // terminalDeleteHeld ends a held block's session through the DELETE
  // route and says killed, unless Run has taken the block back meanwhile
  // or the session has exited (its exit code says more).
  function terminalDeleteHeld(ctl) {
    terminalDelete(ctl).then(function () {
      if (ctl.held && !ctl.exited) terminalStatus(ctl, 'killed');
    });
  }
  // The hold Kill sets outlives the tab's reloads: Kill removes the
  // session from serve, so the next attach would start the command again
  // with no click, and every page update reloads the tab (watchVersion).
  // sessionStorage keeps it per tab, keyed by the page and the session,
  // until Run; every block naming the session shares it, as they share
  // the process.
  function terminalHoldKey(session) {
    return 'present-terminal-held:' + id + ':' + session;
  }
  function terminalHeldBefore(session) {
    try { return sessionStorage.getItem(terminalHoldKey(session)) === '1'; } catch (e) { return false; }
  }
  function terminalHold(ctl, on) {
    terminals.forEach(function (c) { if (c.session === ctl.session) c.held = on; });
    try {
      if (on) sessionStorage.setItem(terminalHoldKey(ctl.session), '1');
      else sessionStorage.removeItem(terminalHoldKey(ctl.session));
    } catch (e) {}
  }
  // terminalRun is the Run button. A block not connected attaches (asking
  // the switch again); an exited session starts over, removed first, since
  // attaching would only replay its end; a live one takes the focus.
  function terminalRun(ctl) {
    ctl.tries = 0;
    terminalHold(ctl, false);
    if (ctl.exited) {
      terminalLetGo(ctl);
      ctl.exited = false;
      terminalDelete(ctl).then(function () {
        if (ctl.term) ctl.term.reset();
        terminalAttach(ctl);
      });
    } else {
      terminalAttach(ctl);
    }
    if (ctl.term) ctl.term.focus();
  }
  // terminalKill is the Kill button: over the socket while connected
  // (serve answers with the exit frame), otherwise through the DELETE
  // route, which also clears an exited session from serve's list. A socket
  // still connecting is kept: serve may be starting the session, and one
  // closed now could leave the process running after a DELETE that came
  // first and found nothing. Its onopen sends the kill, and its onclose
  // falls back to the DELETE.
  function terminalKill(ctl) {
    terminalHold(ctl, true);
    var ws = ctl.ws;
    if (ws && !ctl.exited && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ t: 'kill' }));
      return;
    }
    if (ws && !ctl.exited && ws.readyState === WebSocket.CONNECTING) return;
    terminalLetGo(ctl);
    terminalDeleteHeld(ctl);
  }
  // terminalUnrevealed says the block sits in a deck fragment not shown
  // yet. visibility:hidden keeps its box, so it reads as on screen, but
  // nobody can see it, Run and Kill included; the deck's steps call
  // terminalShow once the fragment shows.
  function terminalUnrevealed(ctl) {
    return !!ctl.block.closest('.fragment:not(.shown)');
  }
  // terminalShow is what the deck calls when the block's slide shows, and
  // its fragment: it attaches on the first showing it can be seen in and
  // refits to the slide's size and font.
  function terminalShow(ctl) {
    terminalMount(ctl);
    if (!ctl.wanted && !terminalUnrevealed(ctl)) terminalAttach(ctl);
  }
  // terminalObserve watches a live block: a resize refits the terminal to
  // its canvas, and the first time the block is seen it attaches. The
  // observer stays while the block is in an unrevealed fragment; a block
  // the deck attached meanwhile only ends the watch.
  function terminalObserve(ctl) {
    if (typeof ResizeObserver === 'function') {
      // A frame later, since the fit sets the height of the canvas the
      // observer watches, which the browser reports as a loop otherwise.
      new ResizeObserver(function () { requestAnimationFrame(function () { terminalMount(ctl); }); }).observe(ctl.canvas);
    }
    if (!('IntersectionObserver' in window)) { if (!terminalUnrevealed(ctl)) terminalAttach(ctl); return; }
    var io = new IntersectionObserver(function (entries) {
      if (!entries[entries.length - 1].isIntersecting || terminalUnrevealed(ctl)) return;
      io.disconnect();
      if (!ctl.wanted) terminalAttach(ctl);
    });
    io.observe(ctl.block);
  }
  // upgradeTerminal makes one terminal block live: its controller, Run and
  // Kill, and the observers. The canvas gets the height its rows need at
  // once (terminalHeight, with the cell guessed), so the page does not
  // jump when xterm opens, and a placeholder holds the same frame. A block
  // whose session this tab killed starts held, saying killed, until Run.
  // A block that can never run is inert: on a shared instance (the shell
  // marks its html data-present-mode="shared"), which has no terminal
  // routes, and in an exported page (data-present-mode="export"), which
  // has no server, it says so, hides Run and Kill, and never asks a server
  // anything; without the xterm asset it says so and only Kill works.
  function upgradeTerminal(block) {
    var canvas = block.querySelector('.present-terminal-canvas');
    if (block._terminal || !canvas) return;
    var rows = parseInt(block.getAttribute('data-rows'), 10);
    var mode = document.documentElement.getAttribute('data-present-mode');
    var shared = mode === 'shared' || mode === 'export';
    var missing = typeof Terminal !== 'function' || !window.FitAddon;
    var session = block.getAttribute('data-session') || '';
    var ctl = {
      block: block, canvas: canvas, status: block.querySelector('.present-terminal-status'),
      session: session, rowsNamed: rows > 0, rows: rows > 0 ? rows : TERMINAL_ROWS,
      term: null, fit: null, ws: null, encoder: new TextEncoder(), state: '', wanted: false,
      exited: false, held: !shared && terminalHeldBefore(session),
      checking: false, opening: false, shown: false, tries: 0, retry: 0, inert: shared || missing
    };
    block._terminal = ctl;
    terminals.push(ctl);
    if (!terminalFills(ctl)) canvas.style.height = terminalHeight(ctl) + 'px';
    var run = block.querySelector('.present-terminal-run'), kill = block.querySelector('.present-terminal-kill');
    if (shared) {
      [run, kill].forEach(function (b) { if (b) b.hidden = true; });
      terminalStatus(ctl, 'unavailable');
      terminalNote(ctl, mode === 'export'
        ? 'Live terminal is not available in an exported page.'
        : 'Live terminal is not available on a shared instance.');
      return;
    }
    if (ctl.held) terminalStatus(ctl, 'killed');
    if (kill) kill.addEventListener('click', function () { terminalKill(ctl); });
    if (missing) {
      terminalNote(ctl, 'Live terminal needs the xterm asset: run make cache in services/present.');
      return;
    }
    if (run) run.addEventListener('click', function () { terminalRun(ctl); });
    terminalObserve(ctl);
    terminalMount(ctl);
  }
  function initPresentTerminals(root) {
    (root || document).querySelectorAll('.present-terminal').forEach(upgradeTerminal);
  }
  // One set of page listeners serves every terminal. A theme change sets
  // the colours again. A mousedown outside a focused terminal blurs it, so
  // the keys go back to the page (the deck's keys yield to the textarea
  // xterm types into). Unloading marks the closing sockets as expected, so
  // none reconnects, and a page back from the back-forward cache attaches
  // again.
  document.addEventListener('wk-themechange', function () {
    terminals.forEach(function (ctl) { if (ctl.term) ctl.term.options.theme = terminalTheme(ctl.block); });
  });
  document.addEventListener('mousedown', function (e) {
    terminals.forEach(function (ctl) {
      if (ctl.term && ctl.block.contains(document.activeElement) && !ctl.block.contains(e.target)) ctl.term.blur();
    });
  });
  window.addEventListener('pagehide', function () { terminalLeaving = true; });
  window.addEventListener('pageshow', function (e) {
    if (!e.persisted) return;
    terminalLeaving = false;
    terminals.forEach(function (ctl) {
      if (ctl.wanted && !ctl.exited) { ctl.tries = 0; terminalAttach(ctl); }
    });
  });

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
    function frame(now) {
      cy.startBatch();
      edges.forEach(function (e) {
        e.style('line-dash-offset', period - ((now * PresentViz.flowSpeed(e.data('weight'), max)) % period));
      });
      cy.endBatch();
    }
    if (reducedMotion) { frame(0); return; }
    flowStop = visibleLoop(el, frame);
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

    // The popup paints a card over the page, so the edge labels' backing
    // follows the surface the graph sits on in and out of it.
    function restyleLabels(c) {
      c.style().selector('edge[label]').style('text-background-color', surfaceColor(el)).update();
    }

    function openPopup() {
      wrapper.classList.add('cy-fullscreen');
      backdrop.classList.add('active');
      ctrls.querySelector('[data-cy="fs"]').innerHTML = svgCollapse;
      ctrls.querySelector('[data-cy="fs"]').title = 'Exit fullscreen';
      var c = cy();
      if (c) { restyleLabels(c); setTimeout(function () { c.resize(); refit(c, 40); }, 120); }
    }

    function closePopup() {
      wrapper.classList.remove('cy-fullscreen');
      backdrop.classList.remove('active');
      ctrls.querySelector('[data-cy="fs"]').innerHTML = svgExpand;
      ctrls.querySelector('[data-cy="fs"]').title = 'Fullscreen';
      var c = cy();
      if (c) { restyleLabels(c); setTimeout(function () { c.resize(); refit(c, 30); }, 80); }
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
        var engines = availableEngines();
        var next = engines[(engines.indexOf(currentEngine()) + 1) % engines.length];
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
  // Without Cytoscape the block says so instead, and the rest of the page
  // still builds.
  function initGraphAndFlow() {
    var el = document.getElementById('cy-graph');
    if (el && typeof cytoscape !== 'function') {
      chartNote(el.parentNode, needsAsset('Graph', 'cytoscape asset'));
      return;
    }
    if (typeof initGraph === 'function' && el) initGraph();
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
    enhanceBrief(brief, data);
  }

  // enhanceBrief brings a mounted brief to life: the graph script and its
  // controls, charts, diagrams, terminals, link targets, read-aloud, and
  // code highlighting. An exported page runs it on the body it carries.
  function enhanceBrief(brief, data) {
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
    initPresentDiagrams();
    initPresentTerminals(brief);

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

  // markChangedSections flags the brief's sections that changed since this
  // browser last opened the page. It keeps the last version seen per page in
  // localStorage, asks the diff route what changed since then, and adds
  // section-changed to those sections until the next load. A server without
  // history (a shared instance) answers 404, and nothing is marked; a first
  // visit only records the version.
  function markChangedSections(id, version) {
    var key = 'present-seen:' + id;
    var seen = 0;
    try { seen = parseInt(localStorage.getItem(key) || '0', 10) || 0; } catch (e) {}
    try { localStorage.setItem(key, String(version)); } catch (e) {}
    if (!seen || seen >= version) return;
    fetch('/api/p/' + encodeURIComponent(id) + '/diff?since=' + seen, { headers: { accept: 'application/json' } })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (!d) return;
        var sections = root.querySelectorAll('wk-section');
        PresentViz.changedSectionTargets(d.content).forEach(function (t) {
          var el = t.id !== undefined ? document.getElementById(t.id) : sections[t.index];
          if (el && el.tagName === 'WK-SECTION') el.classList.add('section-changed');
        });
      })
      .catch(function () {});
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
    // A visual (the graph, a chart, an image, a diagram) sizes the slide by the
    // viewport instead of zooming the body while presenting. A terminal does
    // too: a zoomed xterm would also place its selection off the pointer.
    if (body.querySelector('#cy-graph, .present-chart, wk-figure, .present-diagram')) slide.classList.add('has-viz');
    if (body.querySelector('.present-terminal')) slide.classList.add('has-viz');
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
      var slide = makeSlide([n], 'slide-section');
      decorateSlide(slide, n);
      slides.push(slide);
    });
    if (hero.length) slides.unshift(makeSlide(hero, 'slide-title'));
    if (refs) slides.push(makeSlide([refs], 'slide-refs'));
    if (!slides.length) slides.push(makeSlide([Webkit.el('p', {}, 'This deck has no slides yet.')], 'slide-title'));
    return { slides: slides, chrome: chrome };
  }

  // sectionBlocks is a section's top-level blocks: every element child but
  // the heading and the notes template.
  function sectionBlocks(section) {
    return Array.prototype.slice.call(section.children).filter(function (c) {
      var t = c.tagName.toLowerCase();
      return t !== 'wk-section-heading' && !(t === 'template' && c.classList.contains('deck-notes'));
    });
  }

  // decorateSlide carries a section's layout onto its slide: the
  // data-layout, data-tone (with its --slide-accent variable), and
  // data-reveal attributes the renderer put on the wk-section; a solo class
  // when the only block is a stat, a quote, or a diagram, which is the
  // big-number, quote, or picture slide with no field to set; the speaker notes template, read from
  // its inert content; and the slide's steps, one per Next: on a reveal
  // slide every item of a top-level list and every other top-level block
  // is a fragment, and a stepped chart adds its own steps on any slide.
  function decorateSlide(slide, section) {
    ['data-layout', 'data-tone', 'data-reveal'].forEach(function (a) {
      if (section.hasAttribute(a)) slide.setAttribute(a, section.getAttribute(a));
    });
    var accent = section.style.getPropertyValue('--slide-accent');
    if (accent) slide.style.setProperty('--slide-accent', accent);
    var blocks = sectionBlocks(section);
    if (blocks.length === 1) {
      var tag = blocks[0].tagName.toLowerCase();
      if (tag === 'wk-stat') slide.classList.add('solo', 'solo-stat');
      else if (tag === 'blockquote') slide.classList.add('solo', 'solo-quote');
      else if (blocks[0].classList.contains('present-diagram')) slide.classList.add('solo', 'solo-diagram');
      else if (blocks[0].classList.contains('present-terminal')) slide.classList.add('solo', 'solo-terminal');
    }
    var notes = null;
    Array.prototype.slice.call(section.children).forEach(function (c) {
      if (c.tagName === 'TEMPLATE' && c.classList.contains('deck-notes')) notes = c.content;
    });
    slide._notes = notes;
    // Steps: on a reveal slide each item of a top-level list and every
    // other top-level block is one fragment. A stepped block (data-steps: a
    // chart with steps) adds its own steps right after the fragment that
    // holds it, a chart inside a columns block included, and a slide
    // without reveal still walks its stepped blocks. PresentViz.stepPlan
    // orders the entries; the slide keeps them as _steps.
    var reveal = section.hasAttribute('data-reveal');
    var items = [];
    blocks.forEach(function (b) {
      var lis = (b.tagName === 'UL' || b.tagName === 'OL')
        ? Array.prototype.slice.call(b.children).filter(function (c) { return c.tagName === 'LI'; })
        : [];
      if (reveal && lis.length) { lis.forEach(function (li) { items.push({ el: li, fragment: true }); }); return; }
      var own = stepCount(b);
      var children = own ? [] : Array.prototype.slice.call(b.querySelectorAll('[data-steps]')).map(function (el) {
        return { el: el, steps: stepCount(el) };
      });
      items.push({ el: b, fragment: reveal, steps: own, children: children });
    });
    var plan = PresentViz.stepPlan(items), fragments = 0;
    plan.forEach(function (entry) {
      if (entry.kind !== 'fragment') return;
      fragments++;
      entry.el.classList.add('fragment');
      entry.el.setAttribute('data-step', String(fragments));
    });
    if (plan.length) slide._steps = plan;
  }
  function stepCount(el) {
    return parseInt(el.getAttribute('data-steps'), 10) || 0;
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
  // deckLogo is the logo a deck shows when its chrome names none: the repo
  // logo serve answers at /logo.png. An exported page has no such path, so
  // renderExport clears it and the deck shows none.
  var deckLogo = '/logo.png';
  function deckChrome(chrome, title, count, goTo) {
    chrome = chrome || {};
    var position = chrome.logo_position || 'bottom-right';
    var logoSrc = chrome.logo === 'none' ? '' : (chrome.logo || deckLogo);
    var logo = logoSrc ? Webkit.el('img', { class: 'deck-logo', src: logoSrc, alt: '' }) : null;
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
    // Speaker notes: a drawer outside every slide (so a slide transition
    // never includes it), toggled by the N key and, when any slide has
    // notes, a bar button. It shows the current slide's notes, read from
    // the template the renderer put in the section.
    var hasNotes = slides.some(function (sl) { return !!sl._notes; });
    var notesBtn = hasNotes
      ? Webkit.el('button', { class: 'btn btn-ghost', type: 'button', 'data-deck': 'notes', title: 'Speaker notes (N)', 'aria-pressed': 'false' }, 'Notes')
      : null;
    var bar = Webkit.el('div', { class: 'deck-bar', role: 'toolbar', 'aria-label': 'Slides' }, [
      Webkit.el('button', { class: 'btn btn-ghost deck-arrow', type: 'button', 'data-deck': 'prev', title: 'Previous slide (Left)', 'aria-label': 'Previous slide' }, '‹'),
      counter,
      Webkit.el('button', { class: 'btn btn-ghost deck-arrow', type: 'button', 'data-deck': 'next', title: 'Next slide (Right or Space)', 'aria-label': 'Next slide' }, '›'),
      notesBtn,
      presentBtn
    ]);
    var drawer = Webkit.el('aside', { class: 'deck-notes-drawer', hidden: '', 'aria-label': 'Speaker notes' }, [
      Webkit.el('div', { class: 'deck-notes-title' }, 'Notes'),
      Webkit.el('div', { class: 'deck-notes-body' })
    ]);
    // The transition between slides: fade unless the island says slide or
    // none. The html attribute drives the shell's keyframes.
    var transition = split.chrome.transition === 'slide' || split.chrome.transition === 'none' ? split.chrome.transition : 'fade';
    document.documentElement.setAttribute('data-transition', transition);
    root.innerHTML = '';
    root.appendChild(deck);
    root.appendChild(bar);
    root.appendChild(drawer);

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
    // Charts are built the first time their slide shows (refreshVisuals),
    // so the draw-in plays where it can be seen.
    deck.querySelectorAll('a[href]:not([href^="#"])').forEach(function (a) {
      a.target = '_blank'; a.rel = 'noopener';
    });
    if (window.Webkit && typeof Webkit.enhanceProse === 'function') Webkit.enhanceProse(deck);
    // Terminals get their controller now and open and attach on their
    // slide's first showing (refreshVisuals), when the canvas has a size.
    initPresentTerminals(deck);

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
    // pending is the slide a transition is moving to, and the steps it will
    // show, set before the View Transitions API defers apply(). Everything
    // that asks "where are we" reads it first, so two remote commands in one
    // poll (next, next) land two slides on, and a step pressed mid-crossfade
    // counts on the slide being entered.
    var pending = null;
    function at() { return pending ? pending.i : current; }
    function shownSteps() { return pending ? pending.steps : step; }
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
      if (typeof Chart !== 'undefined') registerSankey();
      // The draw-in follows the deck's transition: 400 ms, or none for a
      // deck that cuts (Reduce Motion already turns it off in buildChart).
      slide.querySelectorAll('.present-chart').forEach(function (b) {
        if (!b._built) buildChart(b, { duration: transition === 'none' ? 0 : 400 });
        else if (b._chart) b._chart.resize();
        else if (b._ribbonResize) b._ribbonResize();
      });
      // A diagram draws in the same way; built, it refits to the height cap
      // presenting sets, which no resize observer sees.
      slide.querySelectorAll('.present-diagram').forEach(function (b) {
        if (!b._built) buildDiagram(b, { duration: transition === 'none' ? 0 : 400 });
        else if (b._diagramFit) b._diagramFit();
      });
      // A terminal attaches on its slide's first showing and refits to the
      // size and font presenting sets.
      slide.querySelectorAll('.present-terminal').forEach(function (b) {
        if (b._terminal) terminalShow(b._terminal);
      });
    }

    // Steps: how many of the current slide's steps (reveal fragments and
    // a stepped chart's steps) are taken. Next takes one more before
    // moving on, Prev takes one back before moving back, and a jump (goto,
    // a dot, the hash, Home and End) lands with every step taken. The hash
    // and the dots track slides only.
    var step = 0;
    function stepTotal(slide) { return slide && slide._steps ? slide._steps.length : 0; }
    // applySteps lands the slide on step n: the first n fragments shown and
    // every stepped block at the highest step among its entries below n
    // (step 0 before its first one). A terminal in a fragment attaches when
    // the fragment shows, not when its slide does (terminalShow skips it
    // while hidden), so it starts where Run and Kill can be seen.
    function applySteps(slide, n) {
      step = n;
      if (!slide || !slide._steps) return;
      slide._steps.forEach(function (entry, k) {
        if (entry.kind === 'fragment') entry.el.classList.toggle('shown', k < n);
      });
      PresentViz.blockStepsAt(slide._steps, n).forEach(function (r) { setBlockStep(r.el, r.n); });
      slide.querySelectorAll('.fragment.shown .present-terminal, .present-terminal.fragment.shown').forEach(function (b) {
        if (b._terminal) terminalShow(b._terminal);
      });
    }
    // setBlockStep moves a stepped block to step m: the attribute the chart
    // bootstrap reads when the block builds later, the renderer's hook when
    // it is already built, and the caption of that step.
    function setBlockStep(el, m) {
      el.setAttribute('data-step-at', String(m));
      el._stepCut = transition === 'none';
      if (typeof el._presentStep === 'function') el._presentStep(m);
      var list = el.querySelector('.present-steps');
      if (!list) return;
      Array.prototype.slice.call(list.children).forEach(function (li, i) { li.classList.toggle('current', i + 1 === m); });
    }
    function updateCounter() {
      var total = stepTotal(slides[current]);
      counter.textContent = (current + 1) + ' / ' + slides.length + (total ? ' · ' + step + '/' + total : '');
    }

    var notesOpen = false;
    function renderNotes() {
      var body = drawer.querySelector('.deck-notes-body');
      body.innerHTML = '';
      var n = slides[current] && slides[current]._notes;
      if (n) body.appendChild(n.cloneNode(true));
      else body.appendChild(Webkit.el('p', { class: 'deck-notes-empty' }, 'No notes for this slide.'));
      // The drawer sits outside the deck, so the deck's link pass never
      // reaches it; a link in the notes must not navigate the presenting tab.
      body.querySelectorAll('a[href]:not([href^="#"])').forEach(function (a) {
        a.target = '_blank'; a.rel = 'noopener';
      });
    }
    function toggleNotes(on) {
      if (!hasNotes) return;
      notesOpen = on === undefined ? !notesOpen : on;
      drawer.hidden = !notesOpen;
      if (notesBtn) notesBtn.setAttribute('aria-pressed', String(notesOpen));
      if (notesOpen) renderNotes();
    }

    // show moves to slide i. stepsMode says how a slide with steps lands: 'none'
    // (arriving by Next, nothing shown yet) or 'all' (any other way). The
    // change itself is apply(); with the transition none, Reduce Motion on,
    // no View Transitions API, or on the first showing it runs at once,
    // otherwise the browser crossfades the old and new slide snapshots
    // (the active slide carries the view-transition-name; the chrome, the
    // bar, and the drawer sit outside it and never move). The direction
    // classes pick the keyframes for the slide transition, and deck-vt
    // turns the fallback entry keyframe off while the API runs. Visuals
    // refresh once the transition has finished, so the graph's first
    // layout and a chart's draw-in start after the crossfade.
    function show(i, stepsMode) {
      if (i < 0) i = 0;
      if (i > slides.length - 1) i = slides.length - 1;
      var from = at();
      if (i === from) return;
      var html = document.documentElement;
      var first = current < 0;
      html.classList.toggle('deck-next', i > from);
      html.classList.toggle('deck-prev', !first && i < from);
      // target is this call's own record: a step pressed before apply runs
      // bumps its steps, and a later show() replaces pending without
      // touching it, so each deferred apply lands what was asked of it.
      var target = { i: i, steps: stepsMode === 'none' ? 0 : stepTotal(slides[i]) };
      pending = target;
      function apply() {
        current = i;
        if (pending === target) pending = null;
        slides.forEach(function (sl, j) {
          sl.classList.toggle('active', j === i);
          if (j === i) sl.scrollTop = 0;
        });
        applySteps(slides[i], target.steps);
        updateCounter();
        chrome.update(i);
        if (notesOpen) renderNotes();
        if (slideFromHash() !== i + 1) history.replaceState(null, '', '#' + (i + 1));
        remember();
      }
      var animate = !first && transition !== 'none' && !reducedMotion && typeof document.startViewTransition === 'function';
      html.classList.toggle('deck-vt', animate);
      if (!animate) { apply(); refreshVisuals(); return; }
      var vt = document.startViewTransition(apply);
      // A transition skipped by a quicker next one rejects ready with an
      // AbortError nobody needs to hear about; finished settles either way.
      vt.ready.catch(function () {});
      vt.finished.then(refreshVisuals, refreshVisuals);
    }
    // A step on a slide still being entered is recorded on the pending
    // target and lands with it; on the current slide it shows at once.
    function stepTo(n) {
      if (pending) { pending.steps = n; return; }
      applySteps(slides[current], n);
      updateCounter();
    }
    function next() {
      var i = at(), slide = slides[i], shown = shownSteps();
      if (slide && slide._steps && shown < slide._steps.length) { stepTo(shown + 1); return; }
      show(i + 1, 'none');
    }
    function previous() {
      var i = at(), slide = slides[i], shown = shownSteps();
      if (slide && slide._steps && shown > 0) { stepTo(shown - 1); return; }
      show(i - 1, 'all');
    }

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
    // In browser fullscreen the browser takes Escape for itself and leaves
    // fullscreen, which ends presenting, so a deck holding a terminal locks
    // the key with the Keyboard Lock API: Escape then reaches the page, a
    // focused terminal keeps it, and the key handler below ends presenting
    // otherwise. Holding Escape still leaves fullscreen. Only Chromium has
    // the API (Safari and Firefox have no Keyboard Lock), and only in a
    // secure context: over https or on localhost. A page on a plain http
    // host has no navigator.keyboard, so there the guard does nothing and
    // Escape leaves fullscreen as it would without a terminal. Leaving
    // fullscreen releases the lock; ending presenting unlocks it as well.
    var hasTerminal = !!deck.querySelector('.present-terminal');
    function enteredFullscreen() {
      ownFullscreen = true;
      if (hasTerminal && navigator.keyboard && navigator.keyboard.lock) navigator.keyboard.lock(['Escape']).catch(function () {});
    }
    function setPresenting(on) {
      if (presenting === on) return;
      presenting = on;
      document.documentElement.classList.toggle('presenting', on);
      presentBtn.textContent = on ? 'Exit' : 'Present';
      remember();
      if (!on && navigator.keyboard && navigator.keyboard.unlock) navigator.keyboard.unlock();
      var d = document.documentElement;
      if (on && d.requestFullscreen) {
        try {
          d.requestFullscreen().then(enteredFullscreen).catch(function () {});
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
        if (req && req.then) req.then(enteredFullscreen).catch(function () { setPresenting(false); });
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
      else if (action === 'notes') toggleNotes();
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
      // Space on a focused button is that button's click, not a page turn:
      // Prev goes back and Present presents. The progress dots are the one
      // exception, so a dot reached by Tab turns the page rather than
      // re-jumping to itself (Enter still activates it).
      var onButton = t && (t.tagName === 'BUTTON' || t.tagName === 'A' || t.tagName === 'WK-BUTTON') &&
        !t.closest('.deck-strip');
      switch (e.key) {
        case 'ArrowRight': case 'PageDown': e.preventDefault(); next(); break;
        case ' ': if (onButton) return; e.preventDefault(); next(); break;
        case 'ArrowLeft': case 'PageUp': case 'Backspace': e.preventDefault(); previous(); break;
        case 'Home': e.preventDefault(); show(0); break;
        case 'End': e.preventDefault(); show(slides.length - 1); break;
        case 'f': case 'F': case 'p': case 'P': e.preventDefault(); pressPresent(); break;
        case 'n': case 'N': e.preventDefault(); toggleNotes(); break;
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
      initPresentCharts(document, { builtOnly: true });
      initPresentDiagrams(document, { builtOnly: true });
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

  // followTheme recolours the brief's graph, charts, and diagrams when the
  // webkit theme toggle fires (the deck view registers its own listener).
  function followTheme() {
    document.addEventListener('wk-themechange', function () {
      initGraphAndFlow();
      initPresentCharts();
      initPresentDiagrams();
    });
  }

  // renderExport mounts an exported page from what the file carries: the
  // page data in #present-export and the body already in #root, which a
  // reader with scripts off sees as it is. The brief keeps that body and
  // gets its graph, charts, and the rest. The deck is split from it, less
  // the references block, which the deck makes its last slide. Nothing
  // here asks a server: no version poll, share, change marks, deck remote,
  // or terminal session.
  function renderExport() {
    var island = document.getElementById('present-export');
    var brief = root.querySelector('.brief');
    var data;
    try { data = JSON.parse(island.textContent); } catch (e) { return; }
    if (!brief) return;
    if (data.view === 'deck') {
      var refs = brief.querySelector('.refs-section');
      if (refs) refs.remove();
      data.deck = brief.innerHTML;
      deckLogo = '';
      renderDeck(data);
      return;
    }
    document.title = data.title || 'present';
    enhanceBrief(brief, data);
    followTheme();
  }
  if (exported) { renderExport(); return; }

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
        markChangedSections(id, data.version);
        if (data.has_deck) showViewLink('deckLink', base + '/deck');
        followTheme();
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
