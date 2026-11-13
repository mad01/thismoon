"""Page probes: measurements taken on the rendered page in the browser.

A computed font size reads the same at any scale, so the probes measure what
was drawn: the bounding box of each piece of text, in CSS px at the 1440x900
viewport. Text height is the box height (the font's ascent plus descent), not
the font size; `font_px` beside it is the computed size times the drawn scale.

Brief, in the light theme:
- diagram labels: every text in each diagram svg (node names, node text lines,
  edge labels, group labels); smallest and median box height, the smallest
  drawn font size
- label collisions: label boxes that overlap a label box of another node, edge
  or group, or that a node box cuts (an edge or group label over a node, a
  node's own text outside its box); an edge label's box is its halo
- node ellipsis: node texts that end in an ellipsis
- figure in the first 900px: the top of the first chart, diagram, graph, stat
  or image above y = 900 of the page
- characters per line: each wrapped line of body prose (p), the last line of
  each paragraph left out; the median line
- line charts: whether the drawn line is smoothed (tension above 0 or monotone
  interpolation), read from the Chart.js instance on the block

Deck, every slide in presenting mode: the smallest text box on the slide as a
share of the slide (canvas) width, from html text, svg text and Chart.js axis
and legend text (canvas text measured with the chart's own font); whether the
slide scrolls; the diagram label probes above, with shares of canvas width.

Not measured: Cytoscape graph labels (drawn on a canvas with no font record
the page exposes), and ribbon charts (d3, no Chart.js instance).
"""

import json
import statistics
from pathlib import Path

from common import read_json

PROBE_VERSION = 1
PROBE_THEME = "light"

PROBE_JS = r"""
((MODE) => {
  const TOL = 2;
  const round = (v, d = 1) => v == null || !isFinite(v) ? null : Math.round(v * 10 ** d) / 10 ** d;
  const median = a => {
    if (!a.length) return null;
    const s = [...a].sort((x, y) => x - y), m = s.length >> 1;
    return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
  };
  const least = a => a.length ? Math.min(...a) : null;
  const shown = el => !el || !el.checkVisibility
    || el.checkVisibility({opacityProperty: true, visibilityProperty: true});
  const box = el => el.getBoundingClientRect();
  const union = rs => rs.reduce((u, r) => ({left: Math.min(u.left, r.left), top: Math.min(u.top, r.top),
    right: Math.max(u.right, r.right), bottom: Math.max(u.bottom, r.bottom)}),
    {left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity});
  const overlap = (a, b) => Math.min(a.right, b.right) - Math.max(a.left, b.left) > TOL
    && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > TOL;
  const within = (a, b) => a.left >= b.left - TOL && a.right <= b.right + TOL
    && a.top >= b.top - TOL && a.bottom <= b.bottom + TOL;
  const measureCtx = document.createElement('canvas').getContext('2d');
  const fontBoxPx = font => {
    measureCtx.font = font;
    const m = measureCtx.measureText('Hg');
    return m.fontBoundingBoxAscent + m.fontBoundingBoxDescent;
  };

  // --- diagrams -------------------------------------------------------------
  function diagramLabels(svg) {
    const out = [];
    svg.querySelectorAll('text').forEach(t => {
      const text = t.textContent.trim();
      if (!text || !shown(t)) return;
      const r = box(t);
      if (r.width < 0.5 || r.height < 0.5) return;
      const ctm = t.getScreenCTM();
      const scale = ctm ? Math.hypot(ctm.a, ctm.b) : 1;
      const edge = t.closest('.dedge-label'), node = t.closest('.dnode'), group = t.closest('.dgroup');
      const kind = node ? (t.classList.contains('dnode-text') ? 'node-text' : 'node')
        : edge ? 'edge' : group ? 'group' : 'other';
      out.push({text, kind, owner: node || edge || group || t, r, area: edge ? box(edge) : r,
                font: parseFloat(getComputedStyle(t).fontSize) * scale});
    });
    return out;
  }
  function diagramProbe(block, canvasW) {
    const svg = block.querySelector('svg');
    if (!svg) return {drawn: false};
    const labels = diagramLabels(svg);
    const nodes = [...svg.querySelectorAll('.dnode')].filter(shown).map(g => {
      const shapes = [...g.querySelectorAll('.dnode-shape')];
      return {g, r: union((shapes.length ? shapes : [g]).map(box))};
    });
    const why = new Map();
    const flag = (l, reason) => { if (!why.has(l)) why.set(l, reason); };
    labels.forEach((a, i) => {
      labels.slice(i + 1).forEach(b => {
        if (a.owner !== b.owner && overlap(a.area, b.area)) {
          flag(a, 'overlaps "' + b.text + '"'); flag(b, 'overlaps "' + a.text + '"');
        }
      });
      nodes.forEach(n => {
        if (a.owner === n.g) { if (!within(a.r, n.r)) flag(a, 'outside its node box'); }
        else if (overlap(a.area, n.r)) flag(a, 'cut by a node box');
      });
    });
    const heights = labels.map(l => l.r.height), fonts = labels.map(l => l.font);
    const flagged = [...why.entries()];
    const viewBox = svg.viewBox && svg.viewBox.baseVal;
    const out = {
      drawn: true, labels: labels.length,
      label_px: heights.map(h => round(h)).sort((x, y) => x - y),
      label_px_min: round(least(heights)), label_px_median: round(median(heights)),
      label_font_px_min: round(least(fonts)),
      smallest_kind: labels.length ? labels[heights.indexOf(least(heights))].kind : null,
      collisions: flagged.length,
      overlapping: flagged.filter(([, w]) => w.startsWith('overlaps')).length,
      cut: flagged.filter(([, w]) => !w.startsWith('overlaps')).length,
      node_ellipsis: labels.filter(l => (l.kind === 'node' || l.kind === 'node-text')
        && /(…|\.\.\.)$/.test(l.text)).length,
      scale: viewBox && viewBox.width ? round(box(svg).width / viewBox.width, 3) : null,
      examples: flagged.slice(0, 4).map(([l, w]) => ({text: l.text.slice(0, 40), kind: l.kind, why: w})),
    };
    if (canvasW) {
      out.label_share_min = round(least(heights) / canvasW, 4);
      out.label_share_median = round(median(heights) / canvasW, 4);
    }
    return out;
  }

  // --- charts ---------------------------------------------------------------
  function chartFonts(c) {
    const fonts = [];
    Object.values(c.scales || {}).forEach(s => {
      if (s.options.display === false) return;
      if (s.options.ticks && s.options.ticks.display !== false) fonts.push(s.options.ticks.font);
      if (s.options.title && s.options.title.display) fonts.push(s.options.title.font);
    });
    const legend = c.options.plugins && c.options.plugins.legend;
    if (legend && legend.display) fonts.push(legend.labels.font);
    const toFont = window.Chart && Chart.helpers && Chart.helpers.toFont;
    return fonts.filter(Boolean).map(f => toFont ? toFont(f) : {size: f.size, string: f.size + 'px ' + f.family});
  }
  function chartProbe(block) {
    let spec = {};
    try { spec = JSON.parse(block.querySelector('.chart-spec').textContent); } catch (e) {}
    const out = {kind: spec.kind || 'bar'};
    const c = block._chart;
    if (!c) {
      out.reachable = false;
      out.why = out.kind === 'ribbon' ? 'ribbon draws with d3' : 'no Chart.js instance on the block';
      return out;
    }
    out.reachable = true;
    const scale = c.canvas.clientWidth ? box(c.canvas).width / c.canvas.clientWidth : 1;
    const fonts = chartFonts(c);
    out.text_font_px_min = round(least(fonts.map(f => f.size)) * scale);
    out.text_px_min = round(least(fonts.map(f => fontBoxPx(f.string))) * scale);
    const lines = c.data.datasets.map((d, k) => c.getDatasetMeta(k)).filter(m => m.type === 'line' && m.dataset);
    if (lines.length) {
      const opts = lines.map(m => m.dataset.options);
      out.line = true;
      out.smoothed = opts.some(o => o.cubicInterpolationMode === 'monotone' || (o.tension || 0) > 0);
      out.tension = Math.max(...opts.map(o => o.tension || 0));
      out.interpolation = [...new Set(opts.map(o => o.cubicInterpolationMode))];
      out.stepped = opts.some(o => !!o.stepped);
      const valueAxis = c.scales[c.options.indexAxis === 'y' ? 'x' : 'y'];
      const values = c.data.datasets.flatMap(d => d.data.map(p => typeof p === 'number' ? p : p && p.y))
        .filter(v => typeof v === 'number');
      if (valueAxis) out.value_axis_min = valueAxis.min;
      out.data_min = least(values);
    }
    return out;
  }

  // --- brief ----------------------------------------------------------------
  const FIGURES = '.present-chart, .present-diagram, #cy-graph, wk-stat, wk-figure';
  function figureKind(el) {
    if (el.id === 'cy-graph') return 'graph';
    if (el.classList.contains('present-chart')) return 'chart';
    if (el.classList.contains('present-diagram')) return 'diagram';
    return el.tagName.toLowerCase() === 'wk-stat' ? 'stat' : 'image';
  }
  function firstFigure() {
    let best = null;
    document.querySelectorAll(FIGURES).forEach(el => {
      if (!shown(el)) return;
      const top = box(el).top + window.scrollY;
      if (!best || top < best.top) best = {top, kind: figureKind(el)};
    });
    return best;
  }
  // The characters on each line of p, by the vertical middle of each
  // character's box: a character whose middle sits below the first
  // character of the current line starts the next line.
  function linesOf(p, range) {
    const lines = [];
    let current = '', lineBottom = null;
    const walker = document.createTreeWalker(p, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      for (let i = 0; i < node.data.length; i++) {
        range.setStart(node, i); range.setEnd(node, i + 1);
        const rects = range.getClientRects();
        if (!rects.length) continue;
        const r = rects[rects.length - 1];
        if (lineBottom !== null && (r.top + r.bottom) / 2 > lineBottom) {
          lines.push(current); current = ''; lineBottom = null;
        }
        if (lineBottom === null) lineBottom = r.bottom;
        current += node.data[i];
      }
    }
    if (current) lines.push(current);
    return lines.map(l => l.replace(/\s+/g, ' ').trim().length).filter(n => n > 0);
  }
  function prose(root) {
    const skip = '.present-chart, .present-diagram, table, wk-callout, wk-stat, wk-figure, details:not([open])';
    const ps = [...root.querySelectorAll('p')].filter(p => shown(p) && !p.closest(skip));
    const range = document.createRange(), full = [], widths = [];
    ps.forEach(p => {
      const lines = linesOf(p, range);
      if (lines.length > 1) full.push(...lines.slice(0, -1));
      widths.push(box(p).width);
    });
    return {paragraphs: ps.length, lines_measured: full.length,
            chars_per_line_median: round(median(full), 0), chars_per_line_max: full.length ? Math.max(...full) : null,
            measure_px_median: round(median(widths), 0),
            font_px: ps.length ? parseFloat(getComputedStyle(ps[0]).fontSize) : null};
  }
  function briefProbe() {
    const root = document.querySelector('.brief') || document.body;
    const first = firstFigure();
    return {
      diagrams: [...root.querySelectorAll('.present-diagram')].map((b, i) => ({index: i + 1, ...diagramProbe(b, null)})),
      charts: [...root.querySelectorAll('.present-chart')].map((b, i) => ({index: i + 1, ...chartProbe(b)})),
      figure_first_top_px: first ? round(first.top, 0) : null,
      figure_first_kind: first ? first.kind : null,
      figure_in_first_900: !!first && first.top < 900,
      prose: prose(root),
      page_height_px: document.documentElement.scrollHeight,
    };
  }

  // --- slide in presenting mode ---------------------------------------------
  function slideTexts(s) {
    const texts = [], range = document.createRange();
    const walker = document.createTreeWalker(s, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const el = n.parentElement;
      if (!n.data.trim() || !el || el.closest('svg, script, style, template') || !shown(el)) continue;
      range.selectNodeContents(n);
      const hs = [...range.getClientRects()].filter(r => r.width > 0.5 && r.height > 0.5).map(r => r.height);
      if (hs.length) texts.push({px: Math.min(...hs), from: 'html', text: n.data.trim().slice(0, 40)});
    }
    s.querySelectorAll('svg text').forEach(t => {
      if (!t.textContent.trim() || !shown(t)) return;
      const h = box(t).height;
      if (h > 0.5) texts.push({px: h, from: 'svg', text: t.textContent.trim().slice(0, 40)});
    });
    return texts;
  }
  function slideProbe() {
    const s = document.querySelector('section.slide.active');
    if (!s) return {error: 'no active slide'};
    const W = s.clientWidth || window.innerWidth;
    const texts = slideTexts(s);
    const charts = [...s.querySelectorAll('.present-chart')].filter(shown).map(chartProbe);
    charts.forEach(c => { if (c.text_px_min) texts.push({px: c.text_px_min, from: 'chart', text: c.kind + ' chart axis or legend'}); });
    const smallest = texts.reduce((a, b) => (!a || b.px < a.px) ? b : a, null);
    return {
      canvas_px: W,
      scrolls: s.scrollHeight > s.clientHeight + 1 || s.scrollWidth > s.clientWidth + 1,
      overflow_px: Math.max(0, s.scrollHeight - s.clientHeight),
      text_min_px: smallest ? round(smallest.px) : null,
      text_min_share: smallest ? round(smallest.px / W, 4) : null,
      text_min_from: smallest ? smallest.from : null,
      text_min_sample: smallest ? smallest.text : null,
      diagrams: [...s.querySelectorAll('.present-diagram')].filter(shown).map(b => diagramProbe(b, W)),
      charts,
    };
  }

  return MODE === 'slide' ? slideProbe() : briefProbe();
})(MODE_ARG)
"""


def run_probe(browser, mode):
    """Run the probe script in the page: mode 'brief' (whole page) or 'slide' (the active slide)."""
    return browser.eval(PROBE_JS.replace("MODE_ARG", json.dumps(mode)))


def probe_path(page_dir):
    return Path(page_dir) / "probes.json"


# --- summaries ----------------------------------------------------------------

def _least(values):
    values = [v for v in values if v is not None]
    return min(values) if values else None


def _median(values):
    values = [v for v in values if v is not None]
    return round(statistics.median(values), 1) if values else None


def _diagram_summary(diagrams, prefix):
    drawn = [d for d in diagrams if d.get("drawn")]
    if not drawn:
        return {f"{prefix}_diagrams": 0}
    out = {
        f"{prefix}_diagrams": len(drawn),
        f"{prefix}_label_px_min": _least(d.get("label_px_min") for d in drawn),
        f"{prefix}_label_px_median": _median(px for d in drawn for px in d.get("label_px", [])),
        f"{prefix}_label_font_px_min": _least(d.get("label_font_px_min") for d in drawn),
        f"{prefix}_label_collisions": sum(d.get("collisions", 0) for d in drawn),
        f"{prefix}_node_ellipsis": sum(d.get("node_ellipsis", 0) for d in drawn),
    }
    if prefix == "slide":
        out["slide_label_share_min"] = _least(d.get("label_share_min") for d in drawn)
    return out


def _line_summary(charts, prefix):
    reachable = [c for c in charts if c.get("reachable")]
    lines = [c for c in reachable if c.get("line")]
    return {f"{prefix}_line_charts": len(lines),
            f"{prefix}_line_charts_smoothed": sum(1 for c in lines if c.get("smoothed")),
            f"{prefix}_charts_unreachable": len(charts) - len(reachable)}


def summarize(brief, slides):
    """The flat values the report shows, from the brief probe and the per-slide probes."""
    out = {}
    if brief:
        prose = brief.get("prose") or {}
        out.update(_diagram_summary(brief.get("diagrams") or [], "brief"))
        out.update(figure_in_first_900=brief.get("figure_in_first_900"),
                   figure_first_top_px=brief.get("figure_first_top_px"),
                   figure_first_kind=brief.get("figure_first_kind"),
                   chars_per_line_median=prose.get("chars_per_line_median"),
                   prose_lines=prose.get("lines_measured"))
        out.update(_line_summary(brief.get("charts") or [], "brief"))
    if slides:
        out.update(_diagram_summary([d for s in slides for d in s.get("diagrams") or []], "slide"))
        smallest = min((s for s in slides if s.get("text_min_share") is not None),
                       key=lambda s: s["text_min_share"], default=None)
        out.update(slides_probed=len(slides),
                   slide_text_share_min=smallest["text_min_share"] if smallest else None,
                   slide_text_px_min=smallest["text_min_px"] if smallest else None,
                   slide_text_min_at=(f"slide {smallest['slide']}, {smallest['text_min_from']}: "
                                      f"{smallest['text_min_sample']}") if smallest else "",
                   slides_scrolling=[s["slide"] for s in slides if s.get("scrolls")])
        out.update(_line_summary([c for s in slides for c in s.get("charts") or []], "slide"))
    return out


def page_record(present, font_requested, font_used, brief, slides):
    return {"probe_version": PROBE_VERSION, "theme": PROBE_THEME, "present": present,
            "font": {"requested": font_requested, "used": font_used},
            "brief": brief, "slides": slides, "summary": summarize(brief, slides)}


# --- tables ---------------------------------------------------------------------

def _cell(value, digits=None):
    if value is None or value == "":
        return ""
    if isinstance(value, bool):
        return "yes" if value else "**no**"
    if isinstance(value, float):
        return f"{value:.{digits}f}" if digits is not None else f"{value:g}"
    return str(value)


def _lines_cell(s, prefix):
    lines, unread = s.get(f"{prefix}_line_charts"), s.get(f"{prefix}_charts_unreachable")
    if not lines:
        return f"none ({unread} unread)" if unread else "none"
    return f"{s[f'{prefix}_line_charts_smoothed']}/{lines} smoothed"


def _row(cells):
    return "| " + " | ".join(cells) + " |"


def brief_line(row):
    s = row["probes"]["summary"]
    first = _cell(s.get("figure_in_first_900"))
    if s.get("figure_first_top_px") is not None:
        first += f" ({s['figure_first_kind']} at {s['figure_first_top_px']})"
    return _row([row["case"], row["model"], _cell(s.get("brief_diagrams")),
                 _cell(s.get("brief_label_px_min")), _cell(s.get("brief_label_px_median")),
                 _cell(s.get("brief_label_font_px_min")), _cell(s.get("brief_label_collisions")),
                 _cell(s.get("brief_node_ellipsis")), first, _cell(s.get("chars_per_line_median")),
                 _lines_cell(s, "brief")])


def deck_line(row):
    s = row["probes"]["summary"]
    scroll = s.get("slides_scrolling") or []
    return _row([row["case"], row["model"], _cell(s.get("slides_probed")),
                 _cell(s.get("slide_text_share_min"), 4), _cell(s.get("slide_text_px_min")),
                 s.get("slide_text_min_at", ""), _cell(s.get("slide_diagrams")),
                 _cell(s.get("slide_label_px_min")), _cell(s.get("slide_label_share_min"), 4),
                 _cell(s.get("slide_label_collisions")), _cell(s.get("slide_node_ellipsis")),
                 ", ".join(map(str, scroll)) or "none", _lines_cell(s, "slide")])


BRIEF_HEAD = ["case", "model", "diagrams", "label px min", "label px median", "label font px min",
              "label collisions", "node ellipsis", "figure in first 900px", "chars/line", "line charts"]
DECK_HEAD = ["case", "model", "slides", "text share min", "text px min", "smallest text", "diagrams",
             "label px min", "label share min", "label collisions", "node ellipsis", "slides scrolling",
             "line charts"]


def probe_table(rows):
    """Markdown lines for the pages that have probes.json, briefs and decks apart; empty when none has."""
    probed = [r for r in rows if (r.get("probes") or {}).get("summary") is not None]
    briefs = [r for r in probed if "figure_in_first_900" in r["probes"]["summary"]]
    decks = [r for r in probed if "slides_probed" in r["probes"]["summary"]]
    lines = []
    if briefs:
        lines += ["Brief:", "", _row(BRIEF_HEAD), "|" + "---|" * len(BRIEF_HEAD), *map(brief_line, briefs)]
    if decks:
        lines += ["", "Deck, presenting mode, every slide:", "", _row(DECK_HEAD), "|" + "---|" * len(DECK_HEAD),
                  *map(deck_line, decks)]
    return lines


def read_probes(page_dir):
    return read_json(probe_path(page_dir))
