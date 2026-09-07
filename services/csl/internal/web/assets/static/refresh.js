// Index refresh page: renders /api/refresh_status as a table with per-repo
// and refresh-all triggers. Polls fast while a refresh runs, slowly otherwise,
// so a kicked refresh shows its outcome without a manual reload.

(function initRefreshPage() {
  const status = document.getElementById('status');
  const results = document.getElementById('results');
  const refreshAll = document.getElementById('refreshAll');
  if (!status || !results) return;

  let pollTimer = null;
  // pageState persists the paged-table expansion across polls so a re-render
  // does not collapse the reader back to the first page.
  const pageState = { shown: TABLE_PAGE_SIZE };

  // STATUSES maps the pull outcome to a short label and a wk-badge variant.
  const STATUSES = {
    ok: ['up to date', 'ok'],
    updated: ['updated', 'ok'],
    dirty: ['dirty — skipped', 'warn'],
    fail: ['failed', 'error'],
    detach: ['detached HEAD', 'warn'],
    skip: ['non-default branch', 'warn'],
    noremote: ['no remote', 'warn'],
    notgit: ['not a git repo', 'warn'],
  };

  // ago is the shared timeAgo helper (common.js), aliased for local call sites.
  const ago = timeAgo;

  function untilLabel(iso) {
    const secs = Math.floor((Date.parse(iso) - Date.now()) / 1000);
    if (secs <= 0) return 'soon';
    if (secs < 60) return 'in ' + secs + 's';
    return 'in ' + Math.ceil(secs / 60) + 'm';
  }

  function summaryLine(d) {
    if (d.running) return 'refresh running…';
    const parts = [];
    parts.push(d.enabled
      ? 'background refresh every ' + d.interval_minutes + 'm'
      : 'background refresh disabled (manual only)');
    if (d.last_run) parts.push('last run ' + ago(d.last_run));
    if (d.enabled && d.next_run) parts.push('next ' + untilLabel(d.next_run));
    if (d.last_error) parts.push('⚠ ' + d.last_error);
    if (d.error) parts.push('⚠ ' + d.error);
    return parts.join(' · ');
  }

  function row(e) {
    const esc = Webkit.escapeHtml;
    const [label, variant] = STATUSES[e.status] || [null, null];
    const badge = label
      ? '<wk-badge variant="' + variant + '">' + esc(label) + '</wk-badge>'
      : '<span class="refresh-none">not yet refreshed</span>';
    return '<tr>' +
      '<td><span class="repo-name">' + esc(e.name) + '</span></td>' +
      '<td>' + badge + '</td>' +
      '<td>' + esc(e.message || '') + '</td>' +
      '<td>' + esc(ago(e.refreshed_at)) + '</td>' +
      '<td>' + esc(e.indexed_at ? ago(e.indexed_at) : 'never') + '</td>' +
      '<td><button class="mini-btn js-refresh" data-repo="' + esc(e.name) + '" title="Pull and reindex this repo now">refresh</button></td>' +
      '</tr>';
  }

  function render(d) {
    status.textContent = summaryLine(d);
    refreshAll.disabled = d.running;
    const repos = d.repos || [];
    if (!repos.length) {
      results.innerHTML = '<div class="health-empty">no repos found</div>';
      return;
    }
    renderPagedTable(results, repos, {
      state: pageState,
      headHtml: '<thead><tr><th>repo</th><th>last refresh</th><th>detail</th><th>refreshed</th><th>indexed</th><th></th></tr></thead>',
      rowFn: row,
      onRender: root => {
        root.querySelectorAll('.js-refresh').forEach(btn => {
          btn.disabled = d.running;
          btn.addEventListener('click', () => kick(btn.dataset.repo));
        });
      },
    });
  }

  function schedule(delay) {
    clearTimeout(pollTimer);
    pollTimer = setTimeout(load, delay);
  }

  function load() {
    fetch('/api/refresh_status')
      .then(res => res.json().then(data => ({ ok: res.ok, data })))
      .then(({ ok, data }) => {
        if (!ok) throw new Error(data.error || 'request failed');
        render(data);
        schedule(data.running ? 3000 : 60000);
      })
      .catch(err => {
        status.textContent = 'error: ' + err.message;
        schedule(15000);
      });
  }

  function kick(repo) {
    const url = repo ? '/api/refresh?repo=' + encodeURIComponent(repo) : '/api/refresh';
    fetch(url, { method: 'POST' })
      .then(res => res.json().then(data => ({ ok: res.ok, data })))
      .then(({ ok, data }) => {
        if (!ok) throw new Error(data.error || 'request failed');
        status.textContent = repo ? 'refreshing ' + repo + '…' : 'refresh running…';
        schedule(1000);
      })
      .catch(err => { status.textContent = 'error: ' + err.message; });
  }

  refreshAll.addEventListener('click', () => kick(''));
  load();
})();
