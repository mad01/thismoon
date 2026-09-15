'use strict';
// present page view — client-side render. The backend serves a static shell
// (chrome only) plus JSON at /api/p/{id}; this script fetches the page data and
// builds the DOM. The Doc->HTML compile happens once at authoring time (Go), so
// `content` is trusted authored HTML we mount as-is; the Cytoscape graph (plus
// its edge-flow animation), metric charts, references, and read-aloud are
// initialized client-side. webkit.js loads before this file, so
// Webkit.el/escapeHtml/poll and the wk-* custom elements are available.
(function () {
  // Capture the Cytoscape instance the injected graph script creates, and make
  // the dagre layout registration explicit/idempotent (same shim the old
  // template carried inline).
  (function () {
    var orig = window.cytoscape;
    if (!orig) return;
    if (window.cytoscapeDagre && typeof orig.use === 'function') {
      try { orig.use(window.cytoscapeDagre); } catch (e) {}
    }
    window._cyInstance = null;
    var wrapped = function () {
      var cy = orig.apply(this, arguments);
      if (arguments[0] && arguments[0].container) window._cyInstance = cy;
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
  function cssVar(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }
  function getChartColors() {
    var dark = document.documentElement.getAttribute('data-theme') === 'dark';
    var c = dark
      ? { series: ['#E8956A', '#7AAAE8', '#6BC48A', '#8B6BB0'], grid: '#4A453D', text: '#B8B2A7', label: '#F5F3EF' }
      : { series: ['#C4704B', '#5B8EC4', '#4A9E6B', '#8B6BB0'], grid: '#D8D4CD', text: '#6B6459', label: '#252320' };
    // Surface color for the gaps between stacked segments and slices.
    c.surface = cssVar('--card-bg', dark ? '#252320' : '#FFFFFF');
    return c;
  }
  function chartSeriesColor(colors, name, i) {
    var map = { terracotta: 0, blue: 1, green: 2, purple: 3 };
    var idx = (name && Object.prototype.hasOwnProperty.call(map, name)) ? map[name] : (i % colors.series.length);
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
  // from source to target; speed follows data.weight. The graph template styles
  // those edges with line-dash-pattern [10, 6], so the period here is 16. The
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

  // ── Cytoscape graph controls ── zoom/fit/fullscreen chrome around #cy-graph.
  // No-ops when the page has no graph. Removes any prior backdrop first so a
  // re-render does not stack duplicates.
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
      '<button class="size-btn" data-cy="in" title="Zoom in">+</button>' +
      '<button class="size-btn" data-cy="out" title="Zoom out">&minus;</button>' +
      '<button class="size-btn" data-cy="fit" title="Reset view">' + svgFit + '</button>' +
      '<button class="size-btn" data-cy="fs" title="Fullscreen">' + svgExpand + '</button>';
    wrapper.appendChild(ctrls);

    function cy() { return window._cyInstance; }
    function mid() { return { x: el.clientWidth / 2, y: el.clientHeight / 2 }; }

    function openPopup() {
      wrapper.classList.add('cy-fullscreen');
      backdrop.classList.add('active');
      ctrls.querySelector('[data-cy="fs"]').innerHTML = svgCollapse;
      ctrls.querySelector('[data-cy="fs"]').title = 'Exit fullscreen';
      var c = cy();
      if (c) setTimeout(function () { c.resize(); c.fit(undefined, 40); c.center(); }, 120);
    }

    function closePopup() {
      wrapper.classList.remove('cy-fullscreen');
      backdrop.classList.remove('active');
      ctrls.querySelector('[data-cy="fs"]').innerHTML = svgExpand;
      ctrls.querySelector('[data-cy="fs"]').title = 'Fullscreen';
      var c = cy();
      if (c) setTimeout(function () { c.resize(); c.fit(undefined, 30); }, 80);
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

  function initGraphAndFlow() {
    if (typeof initGraph === 'function') initGraph();
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

    // Read-aloud reads its targets on connect; append it after the content so it
    // sees the sections. webkit.js already defined the element, so it upgrades.
    var oldRA = document.querySelector('wk-read-aloud');
    if (oldRA) oldRA.remove();
    var ra = document.createElement('wk-read-aloud');
    ra.setAttribute('targets', '.brief-summary, wk-section');
    root.appendChild(ra);

    // Highlight code blocks / enhance prose in the freshly injected content.
    if (window.Webkit && typeof Webkit.enhanceProse === 'function') Webkit.enhanceProse(brief);
  }

  function pageId() {
    var m = location.pathname.match(/^\/p\/([^/]+)/);
    return m ? m[1] : '';
  }

  var id = pageId();
  if (!id) return;

  fetch('/api/p/' + encodeURIComponent(id), { headers: { accept: 'application/json' } })
    .then(function (r) { if (!r.ok) throw new Error('load failed (' + r.status + ')'); return r.json(); })
    .then(function (data) {
      render(data);
      var known = data.version;
      // Recolor the graph + charts when the webkit theme toggle fires.
      document.addEventListener('wk-themechange', function () {
        initGraphAndFlow();
        initPresentCharts();
      });
      // Live update: poll the lightweight version endpoint (a bare int, which is
      // valid JSON) and reload when it changes. Replaces the old inline poller.
      if (window.Webkit && typeof Webkit.poll === 'function') {
        Webkit.poll('/p/' + encodeURIComponent(id) + '/version', function (v) {
          if (v !== known) location.reload();
        }, 1500);
      }
    })
    .catch(function (err) {
      root.innerHTML = '';
      var msg = (err && err.message) || String(err);
      root.appendChild(Webkit.el('div', { class: 'brief' }, 'Failed to load presentation: ' + msg));
    });
})();
