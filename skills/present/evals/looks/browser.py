"""agent-browser driven from Python: one named headless session, JSON eval, screenshots.

Two headless traps shape this file: a smooth scroll never finishes, so every
scroll passes behavior 'instant'; and requestAnimationFrame slows to about
one frame a second while a Cytoscape canvas is in view, so each shot waits
for the page's render markers and then settles before it is taken.
"""

import json
import subprocess
import time

VIEWPORT = (1440, 900)
SETTLE_S = 1.5
READY_TIMEOUT_S = 25

# What has rendered: Chart.js and diagram blocks mark themselves _built, the
# graph container gets .ready, a diagram's entry animation holds svg.entering,
# and a finite running animation (a deck's view transition between slides or
# into presenting mode) means the frame is not final yet. A shot taken during
# the transition into presenting mode showed the title slide tiled 2x2.
READY_JS = r"""
((scope) => {
  const root = (scope && document.querySelector(scope)) || document;
  const q = s => [...root.querySelectorAll(s)];
  const charts = q('.present-chart'), diagrams = q('.present-diagram');
  const cy = root.querySelector('#cy-graph');
  const imgs = q('img');
  const text = document.body ? document.body.innerText : '';
  const markers = ['did not load', 'run make cache', 'Diagram could not be drawn',
                   'is not defined', '404 page not found'];
  return {
    title: document.title,
    height: document.documentElement.scrollHeight,
    theme: document.documentElement.dataset.theme || '',
    charts: charts.length, charts_built: charts.filter(c => c._built).length,
    diagrams: diagrams.length, diagrams_built: diagrams.filter(d => d._built).length,
    entering: q('svg.entering').length,
    animating: document.getAnimations().filter(a => a.playState === 'running'
      && a.effect && a.effect.getComputedTiming().endTime !== Infinity).length,
    graph: !!cy, graph_ready: cy ? cy.classList.contains('ready') : true,
    images: imgs.length, images_loaded: imgs.filter(i => i.complete && i.naturalWidth > 0).length,
    notes: q('.present-chart-note').map(n => n.textContent.trim()).slice(0, 5),
    error_text: markers.filter(m => text.includes(m)),
    fonts_loaded: !document.fonts || document.fonts.status === 'loaded',
    font: document.body ? (document.body.style.fontFamily || getComputedStyle(document.body).fontFamily) : '',
    font_key: localStorage.getItem('webkit-font'),
    slides: document.querySelectorAll('section.slide').length,
    active_slide: [...document.querySelectorAll('section.slide')]
      .findIndex(s => s.classList.contains('active')) + 1,
  };
})(SCOPE)
"""


FIGURE_SELECTOR = ".present-chart, .present-diagram, #cy-graph"

# Each figure scrolled to the middle of the viewport; returns how many there are.
FIGURE_JS = r"""
((n) => {
  const figs = [...document.querySelectorAll(SELECTOR)];
  if (n >= 0 && figs[n]) figs[n].scrollIntoView({behavior: 'instant', block: 'center'});
  return figs.length;
})(INDEX)
"""


def is_ready(state):
    """Images are left out: a remote image that never loads is a metric, not a wait."""
    return (state["charts_built"] >= state["charts"]
            and state["diagrams_built"] >= state["diagrams"]
            and state["entering"] == 0
            and state["animating"] == 0
            and state["graph_ready"]
            and state.get("fonts_loaded", True))


class BrowserError(RuntimeError):
    pass


class Browser:
    def __init__(self, session):
        self.session = session

    def run(self, *args, stdin=None, timeout=90):
        cmd = ["agent-browser", "--session", self.session, *args]
        out = subprocess.run(cmd, input=stdin, capture_output=True, text=True, timeout=timeout)
        if out.returncode != 0:
            raise BrowserError(f"{' '.join(args[:2])}: {(out.stderr or out.stdout).strip()[:400]}")
        return out.stdout

    def eval(self, js):
        data = json.loads(self.run("--json", "eval", "--stdin", stdin=js))
        if not data.get("success"):
            raise BrowserError(f"eval failed: {data.get('error')}")
        return data["data"]["result"]

    def start(self):
        self.run("set", "viewport", str(VIEWPORT[0]), str(VIEWPORT[1]))

    def open(self, url):
        self.run("open", url)

    def set_reader(self, theme, font=None):
        """Store the reader's theme and face the way the page header does; both apply
        on the next load. The palette family keys are removed, so each mode uses its
        default family; with no font the face key is removed, so the page's default shows."""
        face = (f"localStorage.setItem('webkit-font', {json.dumps(font)}); " if font
                else "localStorage.removeItem('webkit-font'); ")
        self.eval(f"localStorage.setItem('webkit-theme', {json.dumps(theme)}); "
                  "localStorage.removeItem('webkit-palette-light'); "
                  "localStorage.removeItem('webkit-palette-dark'); " + face + "true")

    def scroll_to_figure(self, index):
        """Scroll figure index (-1: none) to the viewport middle; return the figure count."""
        js = FIGURE_JS.replace("SELECTOR", json.dumps(FIGURE_SELECTOR)).replace("INDEX", str(int(index)))
        return self.eval(js)

    def wait_ready(self, scope=None):
        """Poll the render markers under scope (a selector; the document when None)
        until everything drew or the timeout; return the last state."""
        js = READY_JS.replace("SCOPE", json.dumps(scope))
        deadline = time.monotonic() + READY_TIMEOUT_S
        state = self.eval(js)
        while not is_ready(state) and time.monotonic() < deadline:
            time.sleep(0.5)
            state = self.eval(js)
        state["ready"] = is_ready(state)
        time.sleep(SETTLE_S)
        return state

    def scroll_to(self, y):
        self.eval(f"window.scrollTo({{top: {int(y)}, behavior: 'instant'}}); window.scrollY")

    def screenshot(self, path, full=False):
        args = ["screenshot", "--full", str(path)] if full else ["screenshot", str(path)]
        self.run(*args)

    def press(self, key):
        self.run("press", key)

    def close(self):
        try:
            self.run("close", timeout=30)
        except (BrowserError, subprocess.TimeoutExpired):
            pass
