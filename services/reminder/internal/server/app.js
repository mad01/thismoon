'use strict';
// reminder list — client-side render. The backend serves a static chrome-only
// shell plus the reminder list as JSON at /api/reminders; this script fetches it
// and builds the DOM with webkit's shared helpers. The presentation logic that
// used to live in Go (internal/server/render.go: toView/badge/DueText) lives
// here now. webkit.js loads before this file, so Webkit.el/escapeHtml/poll are
// available.
(function () {
  var app = document.getElementById('app');

  // ── presentation helpers (ported from render.go) ──

  var WD = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MO = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  // dueText mirrors render.go's r.Due.Local().Format("Mon Jan 2 15:04") — the
  // due time in the browser's local zone.
  function dueText(iso) {
    var d = new Date(iso);
    if (isNaN(d)) return iso || '';
    return WD[d.getDay()] + ' ' + MO[d.getMonth()] + ' ' + d.getDate() +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  // badge mirrors render.go's badge(): effective state -> wk-badge variant+label.
  function badge(r) {
    if (r.overdue) return { variant: 'accent', label: 'overdue' };
    if (r.status === 'pending') return { variant: 'stat', label: 'pending' };
    return { variant: 'outline', label: r.status };
  }

  // ── rendering ──

  function card(r) {
    var b = badge(r);

    var mainKids = [Webkit.el('div', { class: 'rem-title' }, r.title)];
    if (r.body) mainKids.push(Webkit.el('div', { class: 'rem-sub' }, r.body));
    mainKids.push(Webkit.el('div', { class: 'rem-sub' },
      dueText(r.due) + (r.repeat ? ' · repeats ' + r.repeat : '')));

    var actions = [Webkit.el('wk-badge', { variant: b.variant }, b.label)];
    actions.push(Webkit.el('button',
      { class: 'btn btn-ghost', 'data-action': 'test', 'data-id': r.id,
        title: 'Send this notification now to check it works (does not change the reminder)' }, 'Test'));
    if (r.status === 'pending') {
      actions.push(Webkit.el('button',
        { class: 'btn btn-ghost', 'data-action': 'cancel', 'data-id': r.id }, 'Cancel'));
    }
    actions.push(Webkit.el('button',
      { class: 'btn btn-danger', 'data-action': 'delete', 'data-id': r.id, 'data-title': r.title },
      'Delete'));

    return Webkit.el('wk-card', { class: 'rem' }, [
      Webkit.el('div', { class: 'rem-main' }, mainKids),
      Webkit.el('div', { class: 'rem-actions' }, actions)
    ]);
  }

  function render(data) {
    var reminders = (data && data.reminders) || [];
    var pending = reminders.filter(function (r) { return r.status === 'pending'; }).length;

    app.innerHTML = '';
    app.appendChild(Webkit.el('wk-page-header', {}, [
      Webkit.el('wk-title', {}, 'Reminders'),
      Webkit.el('wk-subtitle', {},
        pending + ' pending' + (reminders.length ? ' · ' + reminders.length + ' total' : ''))
    ]));
    app.appendChild(Webkit.el('p', { class: 'hint' }, [
      'Add reminders from the CLI (',
      Webkit.el('code', {}, 'reminder add'),
      ') or by asking Claude. Cancel or delete them here.'
    ]));

    if (!reminders.length) {
      app.appendChild(Webkit.el('div', { class: 'empty' }, 'No reminders yet. Add one above.'));
      return;
    }
    app.appendChild(Webkit.el('div', { class: 'rem-list' }, reminders.map(card)));
  }

  function renderError(err) {
    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'empty' },
      'Failed to load reminders: ' + ((err && err.message) || String(err))));
  }

  // refresh re-fetches and re-renders after a mutation, replacing the old
  // location.reload() calls so the list updates in place.
  function refresh() {
    fetch('/api/reminders')
      .then(function (r) { if (!r.ok) throw new Error('load failed (' + r.status + ')'); return r.json(); })
      .then(render)
      .catch(renderError);
  }

  // ── interactions (ported from index.html) ──

  var host = document.getElementById('toast-host');
  function toast(msg, variant) {
    var t = document.createElement('wk-toast');
    t.setAttribute('variant', variant || 'ok');
    t.textContent = msg;
    host.appendChild(t);
    setTimeout(function () { t.remove(); }, 2800);
  }

  // Cancel (soft) — direct.
  document.addEventListener('click', async function (e) {
    var btn = e.target.closest('button[data-action="cancel"]');
    if (!btn) return;
    var res = await fetch('/api/reminders/' + btn.dataset.id + '/cancel', { method: 'POST' });
    if (res.ok) { refresh(); }
    else { toast('Cancel failed: ' + (await res.text()), 'err'); }
  });

  // Test — fire the notification now without changing the reminder.
  document.addEventListener('click', async function (e) {
    var btn = e.target.closest('button[data-action="test"]');
    if (!btn) return;
    var res = await fetch('/api/reminders/' + btn.dataset.id + '/test', { method: 'POST' });
    if (res.ok) { toast('Test notification sent', 'ok'); }
    else { toast('Test failed: ' + (await res.text()), 'err'); }
  });

  // Delete (hard) — confirm via modal.
  var modal = document.getElementById('delete-modal');
  var titleEl = document.getElementById('delete-title');
  var pendingID = null;
  function openModal(id, title) { pendingID = id; titleEl.textContent = title; modal.removeAttribute('hidden'); }
  function closeModal() { pendingID = null; modal.setAttribute('hidden', ''); }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('button[data-action="delete"]');
    if (!btn) return;
    openModal(btn.dataset.id, btn.dataset.title);
  });
  document.getElementById('delete-cancel').addEventListener('click', closeModal);
  document.getElementById('delete-x').addEventListener('click', closeModal);
  modal.addEventListener('click', function (e) { if (e.target === modal) closeModal(); });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && !modal.hasAttribute('hidden')) closeModal();
  });
  document.getElementById('delete-confirm').addEventListener('click', async function () {
    if (!pendingID) return;
    var id = pendingID;
    var res = await fetch('/api/reminders/' + id, { method: 'DELETE' });
    closeModal();
    if (res.ok || res.status === 404) { refresh(); return; }
    toast('Delete failed: ' + res.status, 'err');
  });

  // Webkit.poll fetches immediately and then every 60s, keeping the list live;
  // mutations call refresh() for an instant update in between polls.
  Webkit.poll('/api/reminders', render, 60000, renderError);
})();
