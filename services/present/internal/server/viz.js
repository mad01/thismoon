'use strict';
// present deck visuals: the pure, node-tested parts of the deck's in-block
// stepping (and later the D3 renderers). shell.html loads this file before
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

  root.PresentViz = { stepPlan: stepPlan, chartStepVisibility: chartStepVisibility };
})(typeof window !== 'undefined' ? window : globalThis);
