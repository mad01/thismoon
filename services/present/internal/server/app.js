'use strict';
// present page view — client-side render. The backend serves a static shell
// (chrome only) plus JSON at /api/p/{id}; this script fetches the page data and
// builds the DOM. The Doc->HTML compile happens once at authoring time (Go), so
// `content` is trusted authored HTML we mount as-is; the Cytoscape graph, metric
// charts, references, and read-aloud are initialized client-side. webkit.js
// loads before this file, so Webkit.el/escapeHtml/poll and the wk-* custom
// elements are available.
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

  // ── Metric charts (Chart.js) ──
  // Each {t:"chart"} Doc block renders a .present-chart div carrying a JSON spec
  // in a <script type="application/json">. We read every block, build a Chart.js
  // instance into its canvas, and rebuild on theme change so colors track the
  // palette — mirroring how the Cytoscape graph recolors.
  function getChartColors() {
    var dark = document.documentElement.getAttribute('data-theme') === 'dark';
    return dark
      ? { series: ['#E8956A', '#7AAAE8', '#6BC48A', '#8B6BB0'], grid: '#4A453D', text: '#B8B2A7' }
      : { series: ['#C4704B', '#5B8EC4', '#4A9E6B', '#8B6BB0'], grid: '#D8D4CD', text: '#6B6459' };
  }
  function chartSeriesColor(colors, name, i) {
    var map = { terracotta: 0, blue: 1, green: 2, purple: 3 };
    var idx = (name && Object.prototype.hasOwnProperty.call(map, name)) ? map[name] : (i % colors.series.length);
    return colors.series[idx];
  }
  function buildChartOptions(kind, spec, colors) {
    var spark = kind === 'sparkline';
    var multi = (spec.series || []).length > 1;
    return {
      responsive: true, maintainAspectRatio: false, animation: false,
      plugins: {
        legend: { display: multi && !spark, labels: { color: colors.text, boxWidth: 12, font: { size: 11 } } },
        title: { display: false },
        tooltip: { enabled: !spark }
      },
      scales: {
        x: { display: !spark, grid: { color: colors.grid, drawBorder: false }, ticks: { color: colors.text, font: { size: 11 } } },
        y: { display: !spark, beginAtZero: true, grid: { color: colors.grid, drawBorder: false },
             ticks: { color: colors.text, font: { size: 11 } },
             title: { display: !!spec.unit, text: spec.unit || '', color: colors.text, font: { size: 11 } } }
      }
    };
  }
  function initPresentCharts() {
    if (typeof Chart === 'undefined') return;
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
      var series = spec.series || [];
      var labels = ((series[0] && series[0].points) || []).map(function (p) { return p.x || ''; });
      var datasets = series.map(function (s, i) {
        var col = chartSeriesColor(colors, s.color, i);
        var ds = { label: s.name || '', data: (s.points || []).map(function (p) { return p.y; }),
                   borderColor: col, backgroundColor: col };
        if (kind === 'area') { ds.fill = true; ds.backgroundColor = col + '33'; ds.tension = 0.3; ds.pointRadius = 2; ds.borderWidth = 2; }
        else if (kind === 'sparkline') { ds.fill = false; ds.pointRadius = 0; ds.borderWidth = 1.5; ds.tension = 0.3; }
        else { ds.borderWidth = 0; }
        return ds;
      });
      block._chart = new Chart(canvas, {
        type: kind === 'bar' ? 'bar' : 'line',
        data: { labels: labels, datasets: datasets },
        options: buildChartOptions(kind, spec, colors)
      });
    });
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
    if (typeof initGraph === 'function') initGraph();
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
        if (typeof initGraph === 'function') initGraph();
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
