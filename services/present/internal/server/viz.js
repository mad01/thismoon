'use strict';
// present deck visuals: the pure, node-tested parts of the deck's in-block
// stepping, the ribbon chart's layout, and the diagram block's ELK graph,
// stepping, and routes. shell.html loads this file before
// app.js, which reads the helpers from window.PresentViz. Nothing here touches
// the DOM at load time.
(function (root) {
  // blockEntries returns one block entry per in-block step of el.
  function blockEntries(el, steps) {
    var out = [];
    for (var n = 1; n <= (steps || 0); n++) {
      out.push({ kind: 'block', el: el, n: n });
    }
    return out;
  }

  // stepPlan flattens a slide's top-level items into its ordered steps: an
  // item's fragment entry first, then its own block steps, then each nested
  // child's block steps in order.
  function stepPlan(items) {
    var plan = [];
    (items || []).forEach(function (item) {
      if (item.fragment) {
        plan.push({ kind: 'fragment', el: item.el });
      }
      plan = plan.concat(blockEntries(item.el, item.steps));
      (item.children || []).forEach(function (child) {
        plan = plan.concat(blockEntries(child.el, child.steps));
      });
    });
    return plan;
  }

  // blockStepsAt reduces a slide's plan at step n to one entry per stepped
  // block: the highest of its step numbers among the entries below n, or 0
  // before its first one.
  function blockStepsAt(plan, n) {
    var out = [];
    (plan || []).forEach(function (entry, k) {
      if (entry.kind !== 'block') return;
      var m = k < n ? entry.n : 0;
      for (var i = 0; i < out.length; i++) {
        if (out[i].el === entry.el) { if (m > out[i].n) out[i].n = m; return; }
      }
      out.push({ el: entry.el, n: m });
    });
    return out;
  }

  // seriesMax is the value a stepped chart's axis is pinned to: the largest
  // positive point, or the largest per-category sum of the positive points
  // when the bars stack (Chart.js stacks negatives on their own), so a
  // series shown at a later step does not rescale the ones already there.
  function seriesMax(series, stacked) {
    var sums = [], max = 0;
    (series || []).forEach(function (s) {
      (s.points || []).forEach(function (p, i) {
        var y = Number(p.y) || 0;
        if (y <= 0) return;
        if (stacked) { sums[i] = (sums[i] || 0) + y; y = sums[i]; }
        if (y > max) max = y;
      });
    });
    return max;
  }

  // isStep reports whether v is a usable 1-based step number.
  function isStep(v) {
    return Number.isInteger(v) && v > 0;
  }

  // chartStepVisibility says which series are drawn at stepAt. A null or
  // undefined stepAt means not stepping, so every series shows; a series
  // without a valid step always shows.
  function chartStepVisibility(series, stepAt) {
    var stepping = stepAt !== null && stepAt !== undefined;
    return (series || []).map(function (s) {
      var step = s ? s.step : undefined;
      return !stepping || !isStep(step) || step <= stepAt;
    });
  }

  // ribbonLayout lays a ribbon chart out in value units. periods are every
  // distinct x across the series in order of first appearance (the same
  // walk the renderer's validation takes, so step k is period k). Each
  // column holds the segments of the series with a positive value in that
  // period, ordered top-down by rank (largest first, ties as given) or, with
  // order "given", in series order; y0 and y1 are a segment's bottom and top
  // measured up from the baseline, so a column is bottom-aligned. A ribbon
  // joins a series' segments in two adjacent columns; its col is the index
  // of the column it ends at, so it shows with that column's step. max is
  // the largest column total, the value axis the renderer pins to.
  function ribbonLayout(series, order) {
    var periods = [], values = [];
    (series || []).forEach(function (s, i) {
      values[i] = [];
      ((s && s.points) || []).forEach(function (p) {
        var x = p.x === undefined || p.x === null ? '' : String(p.x);
        var k = periods.indexOf(x);
        if (k < 0) { k = periods.length; periods.push(x); }
        values[i][k] = Number(p.y) || 0;
      });
    });
    var columns = periods.map(function (period, k) {
      var segments = [];
      values.forEach(function (v, i) {
        if (v[k] > 0) segments.push({ series: i, value: v[k] });
      });
      if (order !== 'given') segments.sort(function (a, b) { return b.value - a.value; });
      var cum = 0;
      for (var j = segments.length - 1; j >= 0; j--) {
        segments[j].y0 = cum;
        cum += segments[j].value;
        segments[j].y1 = cum;
      }
      return { period: period, total: cum, segments: segments };
    });
    var ribbons = [], max = 0;
    columns.forEach(function (col, k) {
      if (col.total > max) max = col.total;
      if (k === 0) return;
      col.segments.forEach(function (to) {
        columns[k - 1].segments.forEach(function (from) {
          if (from.series !== to.series) return;
          ribbons.push({ series: to.series, col: k, a: { y0: from.y0, y1: from.y1 }, b: { y0: to.y0, y1: to.y1 } });
        });
      });
    });
    return { periods: periods, columns: columns, ribbons: ribbons, max: max };
  }

  // ── Diagram helpers ──
  // The diagram block's pure side: the ELK graph built from a spec, which
  // elements a step shows and lights up, an ELK edge route as an SVG path,
  // and the flow speed. app.js measures text, runs ELK, and draws.
  var DIAGRAM_BOX = { minW: 120, maxW: 240, padX: 14, padY: 10, label: 18, text: 15, lineChars: 28 };
  var DIAGRAM_GROUP_PAD = '[top=36,left=16,bottom=16,right=16]';

  // wrapLines breaks text into at most two lines of about max characters,
  // on spaces; a long last line is cut with an ellipsis.
  function wrapLines(text, max) {
    var words = String(text || '').split(/\s+/).filter(Boolean), lines = [], line = '';
    words.forEach(function (w) {
      var next = line ? line + ' ' + w : w;
      if (next.length <= max || !line) { line = next; return; }
      lines.push(line);
      line = w;
    });
    if (line) lines.push(line);
    if (lines.length > 2) { lines = lines.slice(0, 2); lines[1] = lines[1].slice(0, Math.max(0, max - 1)) + '\u2026'; }
    return lines;
  }

  // diagramBox sizes a node from its label and text lines: as wide as the
  // widest line plus padding, between minW and maxW, and one label line
  // plus one text line per wrapped line high. measure(string) returns the
  // string's width in pixels.
  function diagramBox(node, measure) {
    var lines = wrapLines(node.text, DIAGRAM_BOX.lineChars);
    var widest = measure(node.label || '');
    lines.forEach(function (l) { widest = Math.max(widest, measure(l)); });
    var w = Math.min(DIAGRAM_BOX.maxW, Math.max(DIAGRAM_BOX.minW, Math.ceil(widest) + 2 * DIAGRAM_BOX.padX));
    var h = 2 * DIAGRAM_BOX.padY + DIAGRAM_BOX.label + lines.length * DIAGRAM_BOX.text;
    return { width: w, height: h, lines: lines };
  }

  // diagramElk turns a spec into the graph ELK lays out: groups become
  // compound nodes holding their nodes and child groups (padded for the
  // group label), nodes carry the box size diagramBox gives them, and edges
  // sit at the root with their label sized, so ELK places it. Coordinates
  // come back relative to the root, so the drawing needs no offsetting.
  function diagramElk(spec, measure) {
    var byParent = {};
    function child(parent, el) { (byParent[parent || ''] = byParent[parent || ''] || []).push(el); }
    (spec.groups || []).forEach(function (g) {
      child(g.group, { id: g.id, group: true, layoutOptions: { 'elk.padding': DIAGRAM_GROUP_PAD } });
    });
    (spec.nodes || []).forEach(function (n) {
      var box = diagramBox(n, measure);
      child(n.group, { id: n.id, width: box.width, height: box.height });
    });
    function attach(parentId) {
      return (byParent[parentId] || []).map(function (el) {
        if (el.group) el.children = attach(el.id);
        return el;
      });
    }
    var edges = (spec.edges || []).map(function (e, i) {
      var edge = { id: 'e' + i, sources: [e.from], targets: [e.to] };
      if (e.label) edge.labels = [{ text: e.label, width: Math.ceil(measure(e.label)) + 8, height: 16 }];
      return edge;
    });
    return {
      id: 'root',
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': spec.direction === 'TB' ? 'DOWN' : 'RIGHT',
        'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
        'elk.edgeRouting': 'ORTHOGONAL',
        'elk.json.edgeCoords': 'ROOT',
        'elk.json.shapeCoords': 'ROOT',
        'elk.layered.spacing.nodeNodeBetweenLayers': '56',
        'elk.layered.spacing.edgeNodeBetweenLayers': '24',
        'elk.spacing.nodeNode': '28',
        'elk.spacing.edgeNode': '24',
        'elk.spacing.edgeEdge': '14',
        'elk.edgeLabels.inline': 'true',
        'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX'
      },
      children: attach(''),
      edges: edges
    };
  }

  // elkPath turns an ELK edge's sections (start, bend points, end) into one
  // SVG path, section after section.
  function elkPath(edge) {
    var d = '';
    (edge.sections || []).forEach(function (sec) {
      d += 'M' + sec.startPoint.x + ',' + sec.startPoint.y;
      (sec.bendPoints || []).forEach(function (p) { d += 'L' + p.x + ',' + p.y; });
      d += 'L' + sec.endPoint.x + ',' + sec.endPoint.y;
    });
    return d;
  }

  // diagramShown says which groups, nodes, and edges a diagram shows at
  // stepAt (null: everything). An element shows from its step on; step 0
  // or none means from the start.
  function diagramShown(spec, stepAt) {
    function on(step) { return stepAt === null || stepAt === undefined || !isStep(step) || step <= stepAt; }
    var groups = {}, nodes = {};
    (spec.groups || []).forEach(function (g) { groups[g.id] = on(g.step); });
    (spec.nodes || []).forEach(function (n) { nodes[n.id] = on(n.step); });
    return { groups: groups, nodes: nodes, edges: (spec.edges || []).map(function (e) { return on(e.step); }) };
  }

  // diagramFocus says which groups, nodes, and edges light up at stepAt:
  // the step's focus ids, everything inside a focused group, and every
  // edge touching a focused node. Null means the step singles nothing out
  // (or there is no stepping), so nothing dims.
  function diagramFocus(spec, stepAt) {
    var step = isStep(stepAt) ? (spec.steps || [])[stepAt - 1] : null;
    if (!step || !step.focus || !step.focus.length) return null;
    var parent = {};
    (spec.groups || []).forEach(function (g) { parent[g.id] = g.group || ''; });
    function lit(id, group) {
      if (step.focus.indexOf(id) >= 0) return true;
      for (var g = group || ''; g; g = parent[g]) { if (step.focus.indexOf(g) >= 0) return true; }
      return false;
    }
    var groups = {}, nodes = {};
    (spec.groups || []).forEach(function (g) { groups[g.id] = lit(g.id, g.group); });
    (spec.nodes || []).forEach(function (n) { nodes[n.id] = lit(n.id, n.group); });
    return { groups: groups, nodes: nodes, edges: (spec.edges || []).map(function (e) { return !!(nodes[e.from] || nodes[e.to]); }) };
  }

  // flowSpeed is a flow edge's dot speed in px per ms: 6 px/s for the
  // lightest edge up to 20 px/s for the heaviest, the graph's scale; with
  // no weights every edge runs at the middle.
  function flowSpeed(weight, max) {
    var share = max ? (Number(weight) || 0) / max : 0.5;
    return 0.006 + 0.014 * share;
  }

  root.PresentViz = {
    stepPlan: stepPlan, blockStepsAt: blockStepsAt, chartStepVisibility: chartStepVisibility, seriesMax: seriesMax,
    ribbonLayout: ribbonLayout,
    wrapLines: wrapLines, diagramBox: diagramBox, diagramElk: diagramElk, elkPath: elkPath,
    diagramShown: diagramShown, diagramFocus: diagramFocus, flowSpeed: flowSpeed
  };
})(typeof window !== 'undefined' ? window : globalThis);
