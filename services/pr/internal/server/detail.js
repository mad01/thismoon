'use strict';
// pr detail — client-side render. The backend serves a static shell (chrome +
// the modals) and the PR detail as JSON at GET /api/pr/{host}/{owner}/{repo}/
// {number}; this script reads those coordinates from the URL, fetches the JSON,
// and builds the body with webkit's shared helpers. The markdown body and the
// unified diff arrive as server-rendered HTML (the heavy content transforms stay
// in Go) and are injected as-is. webkit.js loads before this file, so Webkit.el
// and Webkit.escapeHtml are available.
(function () {
  var appEl = document.getElementById('detail-app');

  // PR coordinates from /pr/{host}/{owner}/{repo}/{number}.
  var parts = location.pathname.split('/').filter(Boolean); // ['pr', host, owner, repo, number]
  var PR = {
    host: decodeURIComponent(parts[1] || ''),
    owner: decodeURIComponent(parts[2] || ''),
    repo: decodeURIComponent(parts[3] || ''),
    number: parseInt(parts[4], 10) || 0,
  };

  // humanAge mirrors the Go humanAge for an RFC3339 timestamp.
  function humanAge(iso) {
    var t = new Date(iso).getTime();
    if (isNaN(t)) return '';
    var ms = Date.now() - t;
    var min = ms / 60000;
    if (min < 1) return 'just now';
    if (min < 60) return Math.round(min) + 'm ago';
    var hr = ms / 3600000;
    if (hr < 24) return Math.round(hr) + 'h ago';
    var days = Math.round(hr / 24);
    if (days === 1) return '1 day ago';
    return days + ' days ago';
  }

  // checksSummary mirrors the Go buildChecksView summary text.
  function checksSummary(c) {
    if (!c) return '';
    if (c.failed > 0) return c.failed + '/' + c.total + ' failed';
    if (c.pending > 0) return c.pending + '/' + c.total + ' pending';
    if (c.passed === c.total && c.total > 0) return c.passed + '/' + c.total + ' passed';
    return '';
  }

  // --- actions ---

  function approve() {
    fetch('/api/approve', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(PR),
    }).then(function (r) {
      if (r.ok) load();
      else r.text().then(function (t) { alert('Approve failed: ' + t); });
    });
  }

  function merge() {
    fetch('/api/merge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host: PR.host, owner: PR.owner, repo: PR.repo, number: PR.number, method: 'squash' }),
    }).then(function (r) {
      if (r.ok) window.location.href = '/';
      else r.text().then(function (t) { alert('Merge failed: ' + t); });
    });
  }

  // --- modal system (the confirm + shortcuts modals are static in the shell) ---

  var modalCallback = null;

  function openModal(title, body, confirmLabel, confirmClass, cb) {
    document.getElementById('modal-title').textContent = title;
    document.getElementById('modal-body').textContent = body;
    var btn = document.getElementById('modal-confirm');
    btn.className = 'btn ' + (confirmClass || 'btn-primary');
    btn.innerHTML = Webkit.escapeHtml(confirmLabel) + ' <span class="kbd">↵</span>';
    modalCallback = cb;
    document.getElementById('confirm-modal').hidden = false;
    btn.focus();
  }

  function closeModal() {
    document.getElementById('confirm-modal').hidden = true;
    modalCallback = null;
  }

  function confirmModal() {
    var cb = modalCallback;
    closeModal();
    if (cb) cb();
  }

  function showMergeModal() {
    openModal('Merge PR', 'Squash and merge #' + PR.number + '?', 'Merge', 'btn-merge', merge);
  }

  function showApproveModal() {
    openModal('Approve PR', 'Approve #' + PR.number + '?', 'Approve', 'btn-approve', approve);
  }

  function shortcutsModal() { return document.getElementById('shortcuts-modal'); }
  function toggleShortcuts() {
    var m = shortcutsModal();
    if (m) m.hidden = !m.hidden;
  }
  // The static Close button in the shell uses onclick="toggleShortcuts()".
  window.toggleShortcuts = toggleShortcuts;

  function scrollToFile(name) {
    var headers = document.querySelectorAll('.diff-file-name');
    for (var i = 0; i < headers.length; i++) {
      if (headers[i].textContent === name) {
        headers[i].closest('.diff-file').scrollIntoView({ behavior: 'smooth', block: 'start' });
        break;
      }
    }
  }

  // --- keyboard shortcuts (ported from the old detail template) ---

  var chordKey = null;
  var chordTimer = null;

  document.addEventListener('keydown', function (e) {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;
    var confirmOpen = !document.getElementById('confirm-modal').hidden;
    var shortcutsOpen = !shortcutsModal().hidden;

    // g-chord: g then i -> go home
    if (chordKey === 'g' && e.key === 'i') {
      chordKey = null; clearTimeout(chordTimer);
      window.location.href = '/'; return;
    }
    if (e.key === 'g' && !e.metaKey && !e.ctrlKey && !confirmOpen && !shortcutsOpen) {
      chordKey = 'g';
      clearTimeout(chordTimer);
      chordTimer = setTimeout(function () { chordKey = null; }, 1000);
      return;
    }
    chordKey = null;

    if ((e.metaKey || e.ctrlKey) && e.key === '/') { e.preventDefault(); toggleShortcuts(); return; }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      if (confirmOpen) confirmModal();
      else showApproveModal();
      return;
    }
    if (e.key === 'Escape') {
      if (shortcutsOpen) { toggleShortcuts(); return; }
      if (confirmOpen) { closeModal(); return; }
      window.location.href = '/';
      return;
    }
    if (e.key === 'Enter' && confirmOpen) { e.preventDefault(); confirmModal(); return; }
  });

  // Wire the static confirm-modal buttons + backdrop dismissal.
  document.getElementById('modal-cancel').addEventListener('click', closeModal);
  document.getElementById('modal-confirm').addEventListener('click', confirmModal);
  document.getElementById('confirm-modal').addEventListener('click', function (e) {
    if (e.target === this) closeModal();
  });
  shortcutsModal().addEventListener('click', function (e) {
    if (e.target === this) toggleShortcuts();
  });

  // --- rendering ---

  function metaRow(d) {
    var children = [
      Webkit.el('span', {}, [Webkit.el('strong', {}, d.author), ' opened ' + humanAge(d.created_at)]),
      Webkit.el('span', {}, d.repo.name + ' on ' + d.repo.host),
      Webkit.el('span', {}, d.head_ref + ' → ' + d.base_ref),
      Webkit.el('span', { style: 'color:var(--pr-additions);' }, '+' + d.additions),
      Webkit.el('span', { style: 'color:var(--pr-deletions);' }, '−' + d.deletions),
      Webkit.el('span', {}, d.changed_files + ' files'),
    ];
    if (d.draft) children.push(Webkit.el('span', { style: 'color:var(--pr-draft);' }, 'Draft'));
    return Webkit.el('div', { class: 'pr-detail-meta' }, children);
  }

  function actionBar(htmlURL) {
    return Webkit.el('div', { class: 'pr-action-bar' }, [
      Webkit.el('button', { class: 'btn btn-approve', onclick: approve }, 'Approve'),
      Webkit.el('button', { class: 'btn btn-merge', onclick: showMergeModal }, 'Merge'),
      Webkit.el('a', { class: 'btn', href: htmlURL, target: '_blank' }, 'View on GitHub'),
      Webkit.el('a', { class: 'btn', href: htmlURL + '/files', target: '_blank' }, 'Comment on GitHub'),
      Webkit.el('span', {
        class: 'shortcut-hint',
        html: '<span class="kbd">⌘</span> <span class="kbd">↵</span> approve' +
          ' <span style="margin:0 0.5rem;">·</span> <span class="kbd">⌘</span> <span class="kbd">/</span> shortcuts' +
          ' <span style="margin:0 0.5rem;">·</span> <span class="kbd">esc</span> back',
      }),
    ]);
  }

  function render(d) {
    document.title = '#' + d.number + ' ' + d.title + ' · pr';

    var children = [
      Webkit.el('div', { class: 'pr-detail-header' }, [
        Webkit.el('h1', { class: 'pr-detail-title' }, '#' + d.number + ' ' + d.title),
        metaRow(d),
      ]),
      actionBar(d.html_url || ''),
    ];

    if (d.checks) {
      children.push(Webkit.el('div', { class: 'checks-bar' }, [
        Webkit.el('span', {}, 'Checks: ' + checksSummary(d.checks)),
      ]));
    }

    if (d.reviews && d.reviews.length) {
      children.push(Webkit.el('div', { class: 'pr-reviews' }, d.reviews.map(function (rv) {
        var row = [
          Webkit.el('span', { class: 'review-state ' + rv.state }, rv.state),
          Webkit.el('span', {}, [Webkit.el('strong', {}, rv.author), ' ' + humanAge(rv.submitted_at)]),
        ];
        if (rv.body) row.push(Webkit.el('span', { style: 'color:var(--text-2);' }, rv.body));
        return Webkit.el('div', { class: 'pr-review-item' }, row);
      })));
    }

    if (d.body_html) {
      children.push(Webkit.el('wk-card', { class: 'pr-body', html: d.body_html }));
    }

    if (d.files && d.files.length) {
      children.push(Webkit.el('div', { class: 'pr-files-summary' }, d.files.map(function (f) {
        return Webkit.el('span', {
          class: 'pr-file-chip',
          onclick: function () { scrollToFile(f.filename); },
        }, [
          f.filename + ' ',
          Webkit.el('span', { class: 'pr-file-stat', style: 'color:var(--pr-additions);' }, '+' + f.additions),
          Webkit.el('span', { class: 'pr-file-stat', style: 'color:var(--pr-deletions);' }, '-' + f.deletions),
        ]);
      })));
    }

    if (d.diff_html) {
      children.push(Webkit.el('div', { html: d.diff_html }));
    }

    appEl.innerHTML = '';
    children.forEach(function (c) { appEl.appendChild(c); });
  }

  function load() {
    fetch('/api/pr/' + PR.host + '/' + PR.owner + '/' + PR.repo + '/' + PR.number,
      { headers: { accept: 'application/json' } })
      .then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t || ('request failed (' + r.status + ')')); });
        return r.json();
      })
      .then(render)
      .catch(function (err) {
        appEl.innerHTML = '';
        appEl.appendChild(Webkit.el('div', { class: 'empty' },
          'Failed to load PR: ' + ((err && err.message) || String(err))));
      });
  }

  load();
})();
