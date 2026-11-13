'use strict';
// present deck visuals: the pure, node-tested parts of the deck's in-block
// stepping, the ribbon chart's layout, the chart options that follow the
// data, the diagram block's forms, ELK graph, fit, stepping, and routes,
// and the brief's changed-section marker. shell.html loads this file before
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
  // The diagram block's pure side: the forms a diagram is laid out in, the
  // ELK graph of one form, which form a fit draws and at what scale, which
  // elements a step shows and lights up, an ELK edge route as an SVG path,
  // and the flow speed. app.js measures text, runs ELK, and draws.
  //
  // A form is one way to lay a diagram out: a direction and a density. The
  // floor is the smallest scale its text may draw at, and the aim the
  // scale a fit reaches for when it has the room.
  var DIAGRAM_BOX = { minW: 120, padX: 14, padY: 10, label: 18, text: 15 };
  // DIAGRAM_DENSITY is how much a form shows: its widest box, how many
  // lines an edge label may take, and whether the boxes keep their text.
  // compact trades width for height; names leaves the box text out (it
  // stays in the box's hover title) and is for slides only.
  var DIAGRAM_DENSITY = {
    full: { maxW: 240, edgeLines: 1, text: true },
    compact: { maxW: 190, edgeLines: 2, text: true },
    names: { maxW: 240, edgeLines: 2, text: false }
  };
  // DIAGRAM_HALO is an edge label's box around its text: padX across the
  // widest line, line per line, and padY across the top and bottom.
  var DIAGRAM_HALO = { padX: 8, line: 14, padY: 2 };
  var DIAGRAM_LABEL_SPLIT_PX = 84; // an edge label wider than this takes two lines where the density allows
  var DIAGRAM_GROUP_PAD = '[top=36,left=16,bottom=16,right=16]';
  // ELK spaces a set of siblings by their parent's options, not the
  // root's, so every group carries the same spacing as the root. Without
  // it the boxes inside a group sit ELK's default 20 apart. The labels are
  // in the layout, so the gap between layers no longer grows for them.
  var DIAGRAM_SPACING = {
    'elk.layered.spacing.nodeNodeBetweenLayers': '32',
    'elk.layered.spacing.edgeNodeBetweenLayers': '12',
    'elk.spacing.nodeNode': '28',
    'elk.spacing.edgeEdge': '14',
    'elk.spacing.edgeNode': '24'
  };
  var DIAGRAM_BREAK_AFTER = '/._-:'; // where a word wider than a line may break: after a path or name separator
  var DIAGRAM_CLOSING = ',.;:)]'; // marks that hang into the box padding rather than start a line
  var DIAGRAM_HANG = 2; // how many closing marks may hang
  // The sizes a fit works with. Text draws at these sizes at scale 1: a
  // box name at DIAGRAM_NAME_PX, box text and edge labels at
  // DIAGRAM_TEXT_PX.
  var DIAGRAM_NAME_PX = 13;
  var DIAGRAM_TEXT_PX = 11.5;
  var DIAGRAM_AIM_PX = 13; // the brief aims for every text at this size
  var DIAGRAM_TALL_REM = 45; // a brief drawing taller than this shrinks toward it, not below the aim
  var DIAGRAM_SLIDE_FLOOR = 0.011; // the smallest slide text, as a share of the canvas width
  var DIAGRAM_NAMES_GAIN = 1.15; // how much larger a names form must draw to replace one with box text
  var DIAGRAM_RATIO_EPS = 0.005; // ratios closer than this count as equal, and the earlier form wins

  // closingAt says whether the character at i of s is a closing mark.
  function closingAt(s, i) {
    var c = s.charAt(i);
    return c !== '' && DIAGRAM_CLOSING.indexOf(c) >= 0;
  }

  // hangingMarks is how many closing marks end s and may hang, never all of s.
  function hangingMarks(s) {
    var n = 0;
    while (n < DIAGRAM_HANG && n < s.length - 1 && closingAt(s, s.length - 1 - n)) n++;
    return n;
  }

  // wordCut is where a word too wide for a line breaks: after the longest
  // prefix that sits on a line, moved back to just after a separator in
  // that prefix's second half when it has one. What is left never starts
  // with a closing mark.
  function wordCut(word, sits) {
    var cut = word.length - 1;
    while (cut > 1 && !sits(word.slice(0, cut))) cut--;
    for (var i = cut; i > cut / 2; i--) {
      if (DIAGRAM_BREAK_AFTER.indexOf(word.charAt(i - 1)) >= 0 && !closingAt(word, i)) return i;
    }
    while (cut > 1 && closingAt(word, cut)) cut--;
    return cut;
  }

  // wrapText breaks text into lines no wider than maxWidth as measure(s,
  // role) reads them. Nothing is cut and nothing is added: the lines hold
  // every character of the text. A word wider than a line breaks after a
  // separator (a path, a dotted name), else after the last character that
  // fits, and one or two closing marks may hang past the width into the
  // box padding, so none starts a line.
  function wrapText(text, maxWidth, measure, role) {
    function fits(s) { return measure(s, role) <= maxWidth; }
    function sits(s) {
      var hang = hangingMarks(s);
      return fits(s) || (hang > 0 && fits(s.slice(0, s.length - hang)));
    }
    var lines = [], line = '';
    String(text || '').split(/\s+/).filter(Boolean).forEach(function (word) {
      var next = line ? line + ' ' + word : word;
      if (sits(next)) { line = next; return; }
      if (line) lines.push(line);
      line = word;
      while (line.length > 1 && !sits(line)) {
        var cut = wordCut(line, sits);
        lines.push(line.slice(0, cut));
        line = line.slice(cut);
      }
    });
    if (line) lines.push(line);
    return lines;
  }

  // diagramDensity is a density by name, full for an unknown one.
  function diagramDensity(name) {
    return Object.prototype.hasOwnProperty.call(DIAGRAM_DENSITY, name) ? DIAGRAM_DENSITY[name] : DIAGRAM_DENSITY.full;
  }

  // diagramBox sizes a node for a density: its name, and its text unless
  // the density leaves it out, wrapped inside the density's widest box;
  // the box as wide as its widest line plus padding (between minW and the
  // widest box) and one row per line high. measure(string, role) returns
  // the string's width in pixels.
  function diagramBox(node, measure, density) {
    var d = diagramDensity(density), inner = d.maxW - 2 * DIAGRAM_BOX.padX;
    var label = wrapText(node.label, inner, measure, 'label');
    var lines = d.text ? wrapText(node.text, inner, measure, 'text') : [];
    var widest = 0;
    label.forEach(function (l) { widest = Math.max(widest, measure(l, 'label')); });
    lines.forEach(function (l) { widest = Math.max(widest, measure(l, 'text')); });
    var w = Math.min(d.maxW, Math.max(DIAGRAM_BOX.minW, Math.ceil(widest) + 2 * DIAGRAM_BOX.padX));
    var h = 2 * DIAGRAM_BOX.padY + Math.max(1, label.length) * DIAGRAM_BOX.label + lines.length * DIAGRAM_BOX.text;
    return { width: w, height: h, label: label, lines: lines };
  }

  // edgeLabelBox is an edge label's lines and the halo around them: one
  // line, or, where the density allows two and the label is wider than
  // DIAGRAM_LABEL_SPLIT_PX, two lines split at the space that leaves the
  // shorter longest line.
  function edgeLabelBox(label, measure, density) {
    var text = String(label), lines = [text], widest = measure(text, 'text');
    if (diagramDensity(density).edgeLines > 1 && widest > DIAGRAM_LABEL_SPLIT_PX) {
      var words = text.split(' ');
      for (var i = 1; i < words.length; i++) {
        var a = words.slice(0, i).join(' '), b = words.slice(i).join(' ');
        var longest = Math.max(measure(a, 'text'), measure(b, 'text'));
        if (longest < widest) { widest = longest; lines = [a, b]; }
      }
    }
    return { lines: lines, w: Math.ceil(widest) + DIAGRAM_HALO.padX, h: lines.length * DIAGRAM_HALO.line + DIAGRAM_HALO.padY };
  }

  // diagramSpaced adds the spacing set to a graph's or a group's options.
  function diagramSpaced(options) {
    for (var k in DIAGRAM_SPACING) options[k] = DIAGRAM_SPACING[k];
    return options;
  }

  // diagramElk builds the ELK graph of one form ({direction, density}).
  // Groups become compound nodes holding their nodes and child groups,
  // padded for the group label and at least as wide as it. A node carries
  // the box diagramBox gives it, grown by its shape's room. Edges sit at
  // the root, and a labelled edge carries its label as an inline ELK
  // label, so ELK gives the label room on the edge's own route, where it
  // can land on no box, group label or other label. Coordinates come back
  // relative to the root. Returns the graph, each labelled edge's label box
  // by edge index, and each node's box by id.
  function diagramElk(spec, measure, form) {
    var density = form && form.density, labels = [], boxes = Object.create(null);
    var direction = form && form.direction ? form.direction : spec.direction;
    var edges = (spec.edges || []).map(function (e, i) {
      var el = { id: 'e' + i, sources: [e.from], targets: [e.to] };
      if (!e.label) return el;
      var box = edgeLabelBox(e.label, measure, density);
      labels[i] = box;
      el.labels = [{ id: 'l' + i, text: String(e.label), width: box.w, height: box.h,
        layoutOptions: { 'elk.edgeLabels.inline': 'true' } }];
      return el;
    });
    var byParent = Object.create(null);
    function child(parent, el) { (byParent[parent || ''] = byParent[parent || ''] || []).push(el); }
    (spec.groups || []).forEach(function (g) {
      var w = Math.ceil(measure(String(g.label || ''), 'group')) + 2 * DIAGRAM_GROUP_LABEL.x;
      child(g.group, { id: g.id, group: true, layoutOptions: diagramSpaced({
        'elk.padding': DIAGRAM_GROUP_PAD,
        'elk.nodeSize.constraints': 'MINIMUM_SIZE',
        'elk.nodeSize.minimum': '(' + w + ',0)'
      }) });
    });
    (spec.nodes || []).forEach(function (n) {
      var box = diagramBox(n, measure, density), room = diagramKindRoom(n.kind);
      boxes[n.id] = box;
      child(n.group, { id: n.id, width: box.width + room.left, height: box.height + room.top });
    });
    function attach(parentId) {
      return (byParent[parentId] || []).map(function (el) {
        if (el.group) el.children = attach(el.id);
        return el;
      });
    }
    var graph = {
      id: 'root',
      layoutOptions: diagramSpaced({
        'elk.algorithm': 'layered',
        'elk.direction': direction === 'TB' ? 'DOWN' : 'RIGHT',
        'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
        'elk.edgeRouting': 'ORTHOGONAL',
        'elk.json.edgeCoords': 'ROOT',
        'elk.json.shapeCoords': 'ROOT',
        'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX'
      }),
      children: attach(''),
      edges: edges
    };
    return { graph: graph, labels: labels, boxes: boxes };
  }

  // diagramLabelSpots is where ELK put each edge's label, by edge index:
  // the label's top left corner, relative to the root.
  function diagramLabelSpots(layout) {
    var spots = [];
    ((layout && layout.edges) || []).forEach(function (e) {
      var k = /^e(\d+)$/.exec(e.id), label = e.labels && e.labels[0];
      if (k && label) spots[parseInt(k[1], 10)] = { x: label.x, y: label.y };
    });
    return spots;
  }

  // diagramLadder is the forms a diagram is laid out in, in the order a
  // fit prefers them, each with its group: the authored direction full
  // then compact, the other direction the same, and on a slide the two
  // names forms last.
  function diagramLadder(direction, onSlide) {
    var a = direction === 'TB' ? 'TB' : 'LR', b = a === 'TB' ? 'LR' : 'TB';
    var forms = [
      { direction: a, density: 'full', group: 0 }, { direction: a, density: 'compact', group: 0 },
      { direction: b, density: 'full', group: 1 }, { direction: b, density: 'compact', group: 1 }
    ];
    if (onSlide) forms.push({ direction: a, density: 'names', group: 2 }, { direction: b, density: 'names', group: 2 });
    return forms;
  }

  // largestRatio is the index of the largest ratio among the indexes keep
  // passes, the earliest of near ties, or -1 when keep passes none.
  function largestRatio(ratios, keep) {
    var best = -1;
    ratios.forEach(function (r, i) {
      if (keep(i) && (best < 0 || r > ratios[best] + DIAGRAM_RATIO_EPS)) best = i;
    });
    return best;
  }

  // planInGroup is the form a group gives a fit: its first form at the
  // aim, else its largest when that reaches the floor, else -1.
  function planInGroup(forms, ratios, lim, group) {
    function member(i) { return forms[i].group === group; }
    for (var i = 0; i < forms.length; i++) {
      if (member(i) && ratios[i] >= lim.aim) return i;
    }
    var best = largestRatio(ratios, member);
    return best >= 0 && ratios[best] >= lim.floor ? best : -1;
  }

  // planUnderFloor is the form drawn when no group reaches the floor: the
  // largest form that keeps the box text, unless a names form draws
  // DIAGRAM_NAMES_GAIN times as large.
  function planUnderFloor(forms, ratios) {
    var any = largestRatio(ratios, function () { return true; });
    var keep = largestRatio(ratios, function (i) { return forms[i].density !== 'names'; });
    return keep >= 0 && ratios[any] < DIAGRAM_NAMES_GAIN * ratios[keep] ? keep : any;
  }

  // planScale is the scale a picked form draws at: its ratio capped at the
  // largest scale, and in the brief a drawing taller than room.soft shrunk
  // toward it but not below the aim. Under the floor the brief draws at
  // the floor (and scrolls) and a slide at the ratio.
  function planScale(form, ratio, under, room, lim) {
    if (under) return room.h ? ratio : lim.floor;
    var scale = Math.min(ratio, lim.max);
    if (!room.h && room.soft && form.height * scale > room.soft) {
      scale = Math.max(Math.min(ratio, lim.aim), room.soft / form.height);
    }
    return scale;
  }

  // diagramPlan picks the form a fit draws and its scale. forms: one
  // {width, height, group, density} per laid-out form, in ladder order; a
  // form whose layout failed is left out, and the pick is among the rest.
  // room: {w, h, soft}, h null where the page scrolls (the brief). lim:
  // {floor, aim, max} as scales. A form's ratio is the largest scale it
  // fits at. The groups go in order, and the first that gives a form wins
  // (planInGroup); when none does, planUnderFloor picks. So the brief keeps
  // the author's direction while either of its densities reaches the
  // floor, and a slide keeps the box text while any form with it does.
  function diagramPlan(forms, room, lim) {
    var ratios = forms.map(function (f) {
      return Math.min(room.w / f.width, room.h ? room.h / f.height : Infinity);
    });
    var pick = -1;
    for (var i = 0; i < forms.length && pick < 0; i++) {
      if (i === 0 || forms[i].group !== forms[i - 1].group) pick = planInGroup(forms, ratios, lim, forms[i].group);
    }
    if (pick < 0) pick = planUnderFloor(forms, ratios);
    var under = ratios[pick] < lim.floor;
    return { index: pick, scale: planScale(forms[pick], ratios[pick], under, room, lim), underFloor: under, scroll: under && !room.h };
  }

  // diagramLimits turns where a diagram is fitted into the room and the
  // scales diagramPlan takes. fit: {onSlide, wrapWidth, maxHeight,
  // wrapFontPx, rootPx, canvasWidth}. The brief: no height limit, the
  // floor at scale 1, the aim at every text DIAGRAM_AIM_PX, and box names
  // no larger than the wrapper's font (never under the aim); a drawing
  // taller than DIAGRAM_TALL_REM shrinks toward it. A slide: the canvas's
  // height cap, the floor and the aim at the smallest text
  // DIAGRAM_SLIDE_FLOOR of the canvas width, and box names no larger than
  // the wrapper's font, which the slide sets in its fitted unit (2.2u).
  function diagramLimits(fit) {
    if (!fit.onSlide) {
      var aim = DIAGRAM_AIM_PX / DIAGRAM_TEXT_PX;
      return {
        room: { w: fit.wrapWidth, h: null, soft: DIAGRAM_TALL_REM * fit.rootPx },
        lim: { floor: 1, aim: aim, max: Math.max(aim, fit.wrapFontPx / DIAGRAM_NAME_PX) }
      };
    }
    var floor = DIAGRAM_SLIDE_FLOOR * fit.canvasWidth / DIAGRAM_TEXT_PX;
    return {
      room: { w: fit.wrapWidth, h: fit.maxHeight, soft: null },
      lim: { floor: floor, aim: floor, max: Math.max(floor, fit.wrapFontPx / DIAGRAM_NAME_PX) }
    };
  }

  // ── Slide fit ──
  // SLIDE_FIT is the range of a slide's fit, the scale on its unit: from
  // max down to min in steps of step.
  var SLIDE_FIT = { max: 1, min: 0.7, step: 0.05 };
  // fitSteps finds the largest fit at which a slide holds its content.
  // measure(fit) applies the fit and says whether the slide still
  // overflows; it is tried at max, then one step lower each time, and the
  // first fit that holds wins. A slide that overflows at min reports it.
  function fitSteps(measure) {
    var passes = Math.round((SLIDE_FIT.max - SLIDE_FIT.min) / SLIDE_FIT.step);
    for (var k = 0; k <= passes; k++) {
      var fit = Math.round((SLIDE_FIT.max - k * SLIDE_FIT.step) * 100) / 100;
      if (!measure(fit)) return { fit: fit, overflow: false };
    }
    return { fit: SLIDE_FIT.min, overflow: true };
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
  // (or there is no stepping, or the diagram stands at rest after a jump
  // onto its slide), so nothing dims.
  function diagramFocus(spec, stepAt, rest) {
    var step = !rest && isStep(stepAt) ? (spec.steps || [])[stepAt - 1] : null;
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
  // across the top. diagramElk grows the box by it, so the text keeps
  // its padding, and diagramText moves the text past it.
  var DIAGRAM_KIND_ROOM = { person: { left: 14, top: 0 }, store: { left: 0, top: 6 } };
  var DIAGRAM_NO_ROOM = { left: 0, top: 0 };
  function diagramKindRoom(kind) {
    return Object.prototype.hasOwnProperty.call(DIAGRAM_KIND_ROOM, kind) ? DIAGRAM_KIND_ROOM[kind] : DIAGRAM_NO_ROOM;
  }
  // DIAGRAM_GROUP_LABEL is where a group's label sits from the group's top
  // left corner: x the left inset (kept on the right too), y the baseline,
  // inside the top padding diagramElk gives every group.
  var DIAGRAM_GROUP_LABEL = { x: 12, y: 20 };

  // diagramText places a node's lines in its drawn box of width w: the
  // centre line x, then the middle of each name line and of each text
  // line, in the rows diagramBox sized the box for (box is its result),
  // moved past the shape's room.
  function diagramText(node, box, w) {
    var room = diagramKindRoom(node.kind), top = DIAGRAM_BOX.padY + room.top;
    var label = box.label.map(function (_, i) { return top + (i + 0.5) * DIAGRAM_BOX.label; });
    top += Math.max(1, box.label.length) * DIAGRAM_BOX.label;
    var lines = box.lines.map(function (_, i) { return top + (i + 0.5) * DIAGRAM_BOX.text; });
    return { x: (w + room.left) / 2, label: label, lines: lines };
  }

  // edgeLabelRows is the middle of each line of an edge label, from the
  // top of its halo.
  function edgeLabelRows(box) {
    return box.lines.map(function (_, i) { return DIAGRAM_HALO.padY / 2 + (i + 0.5) * DIAGRAM_HALO.line; });
  }

  // ── Chart helpers ──
  var CHART_LINE_KINDS = { line: true, area: true, sparkline: true };
  var CHART_ZERO_KINDS = { bar: true, 'stacked-bar': true, 'horizontal-bar': true, area: true };
  var CHART_FITTED_KINDS = { line: true, sparkline: true, scatter: true };
  var CHART_GRACE = '5%'; // the air a fitted value axis keeps around the data
  var CHART_TICK = { share: 0.75, min: 11, max: 15 }; // ticks and axis titles against the root size
  var CHART_LEGEND_SHARE = 0.8125; // the legend and sankey labels against the root size
  var CHART_SLIDE = { tick: 1.5, legend: 1.7 }; // ticks and the legend on a slide, in the slide's unit
  var ROOT_PX = 16; // the browser's root font size, when none is read

  // chartLine is how a chart drawn with lines joins its points: straight,
  // since a curve draws values nobody measured. Nothing for other kinds.
  function chartLine(kind) {
    return CHART_LINE_KINDS[kind] === true ? { tension: 0 } : {};
  }

  // chartValueAxis is the value axis's range: bars and areas start at
  // zero, since their length or area is the value; a line, a sparkline,
  // and a scatter fit the data's range with a little air, so values
  // between 0.84 and 0.97 do not draw as a flat line. Null for a kind
  // with no value axis.
  function chartValueAxis(kind) {
    if (CHART_ZERO_KINDS[kind] === true) return { beginAtZero: true };
    if (CHART_FITTED_KINDS[kind] === true) return { beginAtZero: false, grace: CHART_GRACE };
    return null;
  }

  // chartFontSizes is the size of chart text in px for a root font size:
  // ticks and axis titles at CHART_TICK's share of it, kept between its
  // bounds, and the legend and sankey labels at CHART_LEGEND_SHARE. On a
  // slide (slide: the slide's unit in px, {u0, u, floor}, as the
  // stylesheet resolves it) they follow the canvas as small text does:
  // CHART_SLIDE's share of the fitted unit plus the reader's bump (the root
  // size over 16), never under the floor (1.1 u0 when none is given).
  function chartFontSizes(rootPx, slide) {
    var root = Number(rootPx) || ROOT_PX;
    if (slide) {
      var bump = root - ROOT_PX, floor = slide.floor || DIAGRAM_SLIDE_FLOOR * 100 * slide.u0;
      return {
        tick: Math.max(floor, CHART_SLIDE.tick * slide.u + bump),
        legend: Math.max(floor, CHART_SLIDE.legend * slide.u + bump)
      };
    }
    return {
      tick: Math.min(CHART_TICK.max, Math.max(CHART_TICK.min, CHART_TICK.share * root)),
      legend: CHART_LEGEND_SHARE * root
    };
  }

  // chartFontsChanged says whether a chart built with the fonts was (the
  // {tick, legend} pair of {family, size} app.js's chartFonts gives) would
  // draw its text otherwise with the fonts now: another face, or a size
  // more than CHART_FONT_EPSILON px off, which a lower slide fit, the
  // presenting unit, or the reader's size gives. A chart with no fonts on
  // record counts as changed.
  var CHART_FONT_EPSILON = 0.01;
  function chartFontsChanged(was, now) {
    if (!was || !now) return true;
    return ['tick', 'legend'].some(function (role) {
      var a = was[role] || {}, b = now[role] || {};
      return a.family !== b.family || !(Math.abs(a.size - b.size) <= CHART_FONT_EPSILON);
    });
  }

  // ── The reader's face and size ──
  // READING_SAMPLE is the text app.js measures to tell whether text
  // measures otherwise than before: capitals, lower case with ascenders and
  // descenders, and figures, so a new face moves it whatever its widths.
  var READING_SAMPLE = 'Hamburgefonstiv QWMjpy 0123456789';
  // readingChanged says whether the reading probe's box (app.js
  // watchReading) moved between two reads, was and now as {width, height}
  // in px. READING_EPSILON absorbs float noise only: a new face or size
  // moves the box by far more. No earlier read counts as changed.
  var READING_EPSILON = 0.01;
  function readingChanged(was, now) {
    if (!was) return true;
    return Math.abs(was.width - now.width) > READING_EPSILON || Math.abs(was.height - now.height) > READING_EPSILON;
  }
  // settle returns a function that calls fn once, ms after the last call
  // of a burst: three presses on the size control, or a face and then its
  // file loading, call fn once.
  function settle(fn, ms) {
    var timer = null;
    return function () {
      if (timer !== null) clearTimeout(timer);
      timer = setTimeout(function () { timer = null; fn(); }, ms);
    };
  }

  // seriesExtent is the range a stepped chart's fitted value axis is
  // pinned to: the smallest and largest point of every series, so a series
  // shown at a later step does not move the axis under the ones already
  // there. Null without a point.
  function seriesExtent(series) {
    var min = Infinity, max = -Infinity;
    (series || []).forEach(function (s) {
      ((s && s.points) || []).forEach(function (p) {
        var y = Number(p.y);
        if (isNaN(y)) return;
        if (y < min) min = y;
        if (y > max) max = y;
      });
    });
    return min > max ? null : { min: min, max: max };
  }

  // changedSectionTargets turns the content half of a diff answer
  // (GET /api/p/{id}/diff) into the sections the brief marks: changed and
  // added ones, by element id, or by position for an index:N name. Removed
  // sections are gone from the page and are not marked.
  function changedSectionTargets(diff) {
    var names = ((diff && diff.changed) || []).concat((diff && diff.added) || []);
    return names.map(function (name) {
      var m = /^index:(\d+)$/.exec(name);
      return m ? { index: parseInt(m[1], 10) } : { id: name };
    });
  }

  root.PresentViz = {
    changedSectionTargets: changedSectionTargets,
    stepPlan: stepPlan, blockStepsAt: blockStepsAt, chartStepVisibility: chartStepVisibility, seriesMax: seriesMax,
    seriesExtent: seriesExtent, ribbonLayout: ribbonLayout,
    chartLine: chartLine, chartValueAxis: chartValueAxis, chartFontSizes: chartFontSizes, chartFontsChanged: chartFontsChanged,
    readingSample: READING_SAMPLE, readingChanged: readingChanged, settle: settle,
    wrapText: wrapText, diagramBox: diagramBox, edgeLabelBox: edgeLabelBox, edgeLabelRows: edgeLabelRows,
    diagramElk: diagramElk, diagramLabelSpots: diagramLabelSpots, elkPath: elkPath,
    diagramLadder: diagramLadder, diagramPlan: diagramPlan, diagramLimits: diagramLimits, fitSteps: fitSteps,
    diagramShown: diagramShown, diagramFocus: diagramFocus, flowSpeed: flowSpeed,
    diagramText: diagramText, diagramGroupLabel: DIAGRAM_GROUP_LABEL
  };
})(typeof window !== 'undefined' ? window : globalThis);
