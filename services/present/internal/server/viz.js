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
  var DIAGRAM_LAYER_GAP = 56; // between layers, unless a label needs more

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
    var widest = measure(node.label || '', 'label');
    lines.forEach(function (l) { widest = Math.max(widest, measure(l, 'text')); });
    var w = Math.min(DIAGRAM_BOX.maxW, Math.max(DIAGRAM_BOX.minW, Math.ceil(widest) + 2 * DIAGRAM_BOX.padX));
    var h = 2 * DIAGRAM_BOX.padY + DIAGRAM_BOX.label + lines.length * DIAGRAM_BOX.text;
    return { width: w, height: h, lines: lines };
  }

  // diagramElk turns a spec into the graph ELK lays out: groups become
  // compound nodes holding their nodes and child groups (padded for the
  // group label), nodes carry the box size diagramBox gives them, and edges
  // sit at the root. Edge labels stay out of the graph: ELK gives a centre
  // label a layer of its own, which doubled the width of a chain, so the
  // renderer puts each label on its route (routeLabelPoint) instead.
  // Coordinates come back relative to the root, so the drawing needs no
  // offsetting.
  function diagramElk(spec, measure) {
    var gap = DIAGRAM_LAYER_GAP, lr = spec.direction !== 'TB';
    var edges = (spec.edges || []).map(function (e, i) {
      // Left to right, the widest label sets the gap between layers, so a
      // label on the run between two boxes never reaches into either box.
      // Top to bottom the runs are vertical and a label's width is beside
      // them, so the gap stays.
      if (e.label && lr) gap = Math.max(gap, Math.ceil(measure(e.label, 'text')) + 24);
      return { id: 'e' + i, sources: [e.from], targets: [e.to] };
    });
    // ELK spaces a set of siblings by their parent's options, not the
    // root's, so every group carries the same spacing as the root. Without
    // it the boxes inside a group sit ELK's default 20 apart and the
    // labels on the runs between them land on the boxes.
    var spacing = {
      'elk.layered.spacing.nodeNodeBetweenLayers': String(gap),
      'elk.layered.spacing.edgeNodeBetweenLayers': '24',
      'elk.spacing.nodeNode': '28',
      'elk.spacing.edgeNode': '24',
      'elk.spacing.edgeEdge': '14'
    };
    function spaced(options) {
      for (var k in spacing) options[k] = spacing[k];
      return options;
    }
    var byParent = Object.create(null);
    function child(parent, el) { (byParent[parent || ''] = byParent[parent || ''] || []).push(el); }
    (spec.groups || []).forEach(function (g) {
      child(g.group, { id: g.id, group: true, layoutOptions: spaced({ 'elk.padding': DIAGRAM_GROUP_PAD }) });
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
    return {
      id: 'root',
      layoutOptions: spaced({
        'elk.algorithm': 'layered',
        'elk.direction': spec.direction === 'TB' ? 'DOWN' : 'RIGHT',
        'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
        'elk.edgeRouting': 'ORTHOGONAL',
        'elk.json.edgeCoords': 'ROOT',
        'elk.json.shapeCoords': 'ROOT',
        'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX'
      }),
      children: attach(''),
      edges: edges
    };
  }

  // routeLabelPoint is where an edge's label sits: the middle of the
  // longest straight run of its route, so a label lands on the long
  // horizontal of an orthogonal route and not on a short jog. Null when
  // the edge has no route.
  function routeLabelPoint(edge) {
    var best = null, bestLen = -1;
    (edge.sections || []).forEach(function (sec) {
      var pts = [sec.startPoint].concat(sec.bendPoints || [], [sec.endPoint]);
      for (var i = 1; i < pts.length; i++) {
        var len = Math.abs(pts[i].x - pts[i - 1].x) + Math.abs(pts[i].y - pts[i - 1].y);
        if (len > bestLen) { bestLen = len; best = { x: (pts[i].x + pts[i - 1].x) / 2, y: (pts[i].y + pts[i - 1].y) / 2 }; }
      }
    });
    return best;
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
    var groups = Object.create(null), nodes = Object.create(null);
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
    var parent = Object.create(null);
    (spec.groups || []).forEach(function (g) { parent[g.id] = g.group || ''; });
    function lit(id, group) {
      if (step.focus.indexOf(id) >= 0) return true;
      for (var g = group || ''; g; g = parent[g]) { if (step.focus.indexOf(g) >= 0) return true; }
      return false;
    }
    var groups = Object.create(null), nodes = Object.create(null);
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

  // DIAGRAM_KIND_ROOM is the room a shape's decoration takes inside its
  // box: a person's figure a strip down the left, a store's cap a band
  // across the top. diagramGraph grows the box by it, so the text keeps
  // its padding, and diagramText moves the text past it.
  var DIAGRAM_KIND_ROOM = { person: { left: 14, top: 0 }, store: { left: 0, top: 6 } };
  // DIAGRAM_GROUP_LABEL is where a group's label sits from the group's top
  // left corner: x the left inset (kept on the right too), y the baseline,
  // inside the top padding diagramElk gives every group.
  var DIAGRAM_GROUP_LABEL = { x: 12, y: 20 };

  // diagramGraph is diagramElk's graph made ready to draw: every box grown
  // by its shape's room and every group at least as wide as its label
  // (drawn in upper case, so that is what measure gets).
  function diagramGraph(spec, measure) {
    var graph = diagramElk(spec, measure), kind = Object.create(null), label = Object.create(null);
    (spec.nodes || []).forEach(function (n) { kind[n.id] = n.kind; });
    (spec.groups || []).forEach(function (g) { label[g.id] = String(g.label || '').toUpperCase(); });
    function grow(children) {
      (children || []).forEach(function (el) {
        var room = DIAGRAM_KIND_ROOM[kind[el.id]];
        if (room && !el.group) { el.width += room.left; el.height += room.top; }
        if (el.group) {
          var w = Math.ceil(measure(label[el.id], 'group')) + 2 * DIAGRAM_GROUP_LABEL.x;
          el.layoutOptions['elk.nodeSize.constraints'] = 'MINIMUM_SIZE';
          el.layoutOptions['elk.nodeSize.minimum'] = '(' + w + ',0)';
        }
        grow(el.children);
      });
    }
    grow(graph.children);
    return graph;
  }

  // diagramText places a node's text in its drawn box of width w: the
  // centre line x, the label row's middle, and each text line's middle,
  // the rows diagramBox sized the box for, moved past the shape's room.
  function diagramText(node, lineCount, w) {
    var room = DIAGRAM_KIND_ROOM[node.kind] || { left: 0, top: 0 };
    var top = DIAGRAM_BOX.padY + room.top, lines = [];
    for (var i = 0; i < lineCount; i++) lines.push(top + DIAGRAM_BOX.label + (i + 0.5) * DIAGRAM_BOX.text);
    return { x: (w + room.left) / 2, label: top + DIAGRAM_BOX.label / 2, lines: lines };
  }

  root.PresentViz = {
    stepPlan: stepPlan, blockStepsAt: blockStepsAt, chartStepVisibility: chartStepVisibility, seriesMax: seriesMax,
    ribbonLayout: ribbonLayout,
    wrapLines: wrapLines, diagramBox: diagramBox, diagramElk: diagramElk, elkPath: elkPath, routeLabelPoint: routeLabelPoint,
    diagramShown: diagramShown, diagramFocus: diagramFocus, flowSpeed: flowSpeed,
    diagramGraph: diagramGraph, diagramText: diagramText, diagramGroupLabel: DIAGRAM_GROUP_LABEL
  };
})(typeof window !== 'undefined' ? window : globalThis);
