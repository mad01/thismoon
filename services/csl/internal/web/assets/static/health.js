// Repo health page: renders /api/repo_health as a table, toggling between
// repos needing attention and the full fleet. The API is called once with
// all=true; the toggle filters client-side.

(function initHealthPage() {
  const status = document.getElementById('status');
  const results = document.getElementById('results');
  const toggle = document.getElementById('scopeToggle');
  if (!status || !results) return;

  let entries = [];
  let attention = 0;
  let scope = 'attention';

  // ACTIONS maps the API action to a short label and a wk-badge variant.
  const ACTIONS = {
    commit_or_stash: ['commit or stash', 'warn'],
    diverged: ['diverged', 'error'],
    push_recommended: ['push', 'warn'],
    pull_recommended: ['pull', 'warn'],
    no_upstream: ['no upstream', 'warn'],
    detached_head: ['detached HEAD', 'warn'],
    error: ['error', 'error'],
    ready: ['ready', 'ok'],
  };

  function changesCell(e) {
    if (!e.dirty) return '';
    const parts = [];
    if (e.modified_files) parts.push(e.modified_files + ' modified');
    if (e.untracked_files) parts.push(e.untracked_files + ' untracked');
    return parts.join(', ') || 'dirty';
  }

  function syncCell(e) {
    if (!e.has_upstream) return '';
    const parts = [];
    if (e.ahead) parts.push('↑' + e.ahead);
    if (e.behind) parts.push('↓' + e.behind);
    return parts.join(' ');
  }

  function row(e) {
    const esc = Webkit.escapeHtml;
    const [label, variant] = ACTIONS[e.action] || [e.action, 'warn'];
    const err = e.error ? '<div class="health-err">' + esc(e.error) + '</div>' : '';
    return '<tr>' +
      '<td><span class="repo-name">' + esc(e.name) + '</span>' + err + '</td>' +
      '<td>' + esc(e.branch || '') + '</td>' +
      '<td>' + esc(changesCell(e)) + '</td>' +
      '<td>' + esc(syncCell(e)) + '</td>' +
      '<td><wk-badge variant="' + variant + '">' + esc(label) + '</wk-badge></td>' +
      '<td><button class="mini-btn js-copy" data-copy="' + esc(e.path) + '" title="Copy local path">copy path</button></td>' +
      '</tr>';
  }

  function render() {
    const shown = scope === 'all' ? entries : entries.filter(e => e.action !== 'ready');
    status.textContent = attention + ' of ' + entries.length + ' repos need attention';
    if (!shown.length) {
      results.innerHTML = '<div class="health-empty">' +
        (scope === 'all' ? 'no repos found' : 'all clean — nothing needs attention') + '</div>';
      return;
    }
    results.innerHTML = '<wk-table><table>' +
      '<thead><tr><th>repo</th><th>branch</th><th>changes</th><th>ahead/behind</th><th>action</th><th></th></tr></thead>' +
      '<tbody>' + shown.map(row).join('') + '</tbody>' +
      '</table></wk-table>';
    bindCopyButtons(results);
  }

  if (toggle) {
    toggle.querySelectorAll('button').forEach(b => b.addEventListener('click', () => {
      toggle.querySelectorAll('button').forEach(x => x.classList.toggle('active', x === b));
      scope = b.dataset.scope;
      render();
    }));
  }

  fetch('/api/repo_health?all=true')
    .then(res => res.json().then(data => ({ ok: res.ok, data })))
    .then(({ ok, data }) => {
      if (!ok) throw new Error(data.error || 'request failed');
      entries = data.repos || [];
      attention = data.attention || 0;
      render();
    })
    .catch(err => { status.textContent = 'error: ' + err.message; });
})();
