'use strict';
// present deck visuals: the pure, node-tested parts of the deck's in-block
// stepping and of the ribbon chart's layout. shell.html loads this file before
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

  root.PresentViz = {
    stepPlan: stepPlan, blockStepsAt: blockStepsAt, chartStepVisibility: chartStepVisibility, seriesMax: seriesMax,
    ribbonLayout: ribbonLayout
  };
})(typeof window !== 'undefined' ? window : globalThis);
