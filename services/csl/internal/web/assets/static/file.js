// File view page: renders one file section like a search match, with controls
// to widen the context stepwise or jump to the full file, and a copy-path
// action. Deep-linked as /file?repo=&file=&start=&end= — the URL is the whole
// state, there is no server-side record of the view. csl_show_file builds
// these links.

(function initFilePage() {
  const status = document.getElementById('status');
  const actions = document.getElementById('actions');
  const view = document.getElementById('view');
  if (!status || !view) return;

  const params = new URLSearchParams(location.search);
  const repo = params.get('repo') || '';
  const file = params.get('file') || '';
  // The highlighted section, fixed for the page's lifetime.
  const hlStart = parseInt(params.get('start'), 10) || 0;
  const hlEnd = parseInt(params.get('end'), 10) || hlStart;

  if (!repo || !file) {
    status.textContent = 'missing ?repo= and ?file= — open this page via csl_show_file or a search result';
    return;
  }
  document.title = 'csl · ' + file.split('/').pop();

  // The fetched window. Grows by STEP on "more context", or to the whole file.
  const STEP = 25;
  let cur = hlStart
    ? { start: Math.max(1, hlStart - 3), end: hlEnd + 3 }
    : { start: 0, end: 0 };

  function lineHtml(l) {
    const hit = hlStart && l.number >= hlStart && l.number <= hlEnd;
    return '<div class="line ' + (hit ? 'hit' : 'ctx') + '"><span class="ln">' + l.number +
      '</span><span class="code">' + Webkit.escapeHtml(l.text) + '</span></div>';
  }

  function renderActions(data) {
    const esc = Webkit.escapeHtml;
    const parts = [];
    if (data.localPath) {
      parts.push('<button class="mini-btn js-copy" data-copy="' + esc(data.localPath) + '" title="Copy local path">copy path</button>');
    }
    if (data.fileURL) {
      parts.push('<a class="mini-btn" href="' + esc(data.fileURL) + '" target="_blank" rel="noopener" title="Open on web">open web</a>');
    }
    const whole = cur.start === 0 && cur.end === 0;
    if (!whole) {
      parts.push('<button class="mini-btn" id="moreBtn">± ' + STEP + ' lines</button>');
      parts.push('<button class="mini-btn" id="fullBtn">full file</button>');
    }
    actions.innerHTML = parts.join('');
    bindCopyButtons(actions);
    const more = document.getElementById('moreBtn');
    if (more) more.addEventListener('click', () => {
      cur = { start: Math.max(1, cur.start - STEP), end: cur.end + STEP };
      load();
    });
    const full = document.getElementById('fullBtn');
    if (full) full.addEventListener('click', () => { cur = { start: 0, end: 0 }; load(); });
  }

  function statusText(data) {
    const n = data.lines.length;
    let range = n ? 'lines ' + data.lines[0].number + '–' + data.lines[n - 1].number : 'empty file';
    if (data.truncated) range += ' (first ' + n + ')';
    return data.repo + ' · ' + range + ' of ' + data.totalLines;
  }

  async function load() {
    status.textContent = 'loading…';
    const q = new URLSearchParams({ repo, file, start: cur.start, end: cur.end });
    try {
      const res = await fetch('/api/read?' + q.toString());
      const data = await res.json();
      if (!res.ok) {
        status.textContent = 'error';
        view.innerHTML = '<div class="line"><span class="code err">' + Webkit.escapeHtml(data.error || 'error') + '</span></div>';
        return;
      }
      status.textContent = statusText(data);
      view.innerHTML =
        '<div class="file-group"><div class="file-head">' +
        '<span class="file-path">' + Webkit.escapeHtml(data.path) + '</span></div>' +
        '<div class="lines">' + data.lines.map(lineHtml).join('') + '</div></div>';
      renderActions(data);
      // Scroll the highlighted section into view once the window extends
      // beyond it.
      const firstHit = view.querySelector('.line.hit');
      if (firstHit && (cur.start === 0 || cur.end - cur.start > hlEnd - hlStart + 10)) {
        firstHit.scrollIntoView({ block: 'center' });
      }
    } catch (err) {
      status.textContent = 'error: ' + err.message;
    }
  }

  load();
})();
