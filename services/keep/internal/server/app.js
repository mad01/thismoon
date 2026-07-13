'use strict';
// keep page — client-side render. The backend serves a static chrome-only shell
// plus the assertion list as JSON at /api/assertions; this script fetches it and
// builds the DOM with webkit's shared helpers. The page is read-only: writing,
// checking, and retracting an assertion all go through the MCP or the CLI.
// webkit.js loads before this file, so Webkit.el/escapeHtml/poll are available.
(function () {
  var app = document.getElementById('app');

  // ── filter state ──
  // subject is a substring/prefix box; kind and status are exact dropdowns.
  // kind and status go to the server as query params (the store filters on
  // them); subject is applied client-side too so typing narrows without a
  // round-trip. All three persist in the URL (?subject=&kind=&status=).
  var KINDS = ['code-behavior', 'dead-end', 'preference', 'decision', 'machine-state', 'open-thread'];
  var STATUSES = ['fresh', 'stale', 'retracted'];
  var subject = '';
  var kind = '';
  var status = '';
  var lastData = null;

  (function readURL() {
    var p = new URLSearchParams(location.search);
    subject = p.get('subject') || '';
    kind = KINDS.indexOf(p.get('kind')) >= 0 ? p.get('kind') : '';
    status = STATUSES.indexOf(p.get('status')) >= 0 ? p.get('status') : '';
  })();

  function syncURL() {
    var p = new URLSearchParams();
    if (subject) p.set('subject', subject);
    if (kind) p.set('kind', kind);
    if (status) p.set('status', status);
    var qs = p.toString();
    history.replaceState(null, '', qs ? '?' + qs : location.pathname);
  }

  // apiURL builds /api/assertions with the server-side filters (kind, status);
  // subject is sent too so the store's prefix match trims the payload.
  function apiURL() {
    var p = new URLSearchParams();
    if (subject) p.set('subject', subject);
    if (kind) p.set('kind', kind);
    if (status) p.set('status', status);
    var qs = p.toString();
    return '/api/assertions' + (qs ? '?' + qs : '');
  }

  // ── time helpers (mirror the sibling services' Local().Format) ──
  var DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  // checkedText renders checked_at in the browser's local zone, or "never
  // checked" when the assertion has not been checked since it was recorded.
  function checkedText(iso) {
    if (!iso) return 'never checked';
    var d = new Date(iso);
    if (isNaN(d) || d.getFullYear() <= 1) return 'never checked';
    return 'checked ' + DAYS[d.getDay()] + ' ' + MONTHS[d.getMonth()] + ' ' + d.getDate() +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  // statusBadge maps status → a distinct wk-badge variant + label.
  function statusBadge(a) {
    if (a.status === 'fresh') return { variant: 'ok', label: 'fresh' };
    if (a.status === 'stale') return { variant: 'warn', label: 'stale' };
    if (a.status === 'retracted') return { variant: 'outline', label: 'retracted' };
    return { variant: 'stat', label: a.status };
  }

  // ── rendering ──

  function card(a) {
    var sb = statusBadge(a);
    var pins = (a.pins || []).length;

    var head = [
      Webkit.el('wk-badge', { variant: sb.variant }, sb.label),
      Webkit.el('wk-badge', { variant: 'stat' }, a.confidence)
    ];

    var kids = [
      Webkit.el('div', { class: 'a-head' }, head),
      Webkit.el('div', { class: 'a-stmt' }, a.statement),
      Webkit.el('div', { class: 'a-meta' },
        a.subject + ' · ' + a.kind + ' · ' + pins + (pins === 1 ? ' pin' : ' pins') +
        ' · ' + checkedText(a.checked_at))
    ];

    // Secondary muted lines explaining a non-fresh state, when present.
    if (a.status === 'stale' && a.stale_reason) {
      kids.push(Webkit.el('div', { class: 'a-note' }, 'stale: ' + a.stale_reason));
    }
    if (a.status === 'retracted' && a.retract_note) {
      kids.push(Webkit.el('div', { class: 'a-note' }, 'retracted: ' + a.retract_note));
    }

    return Webkit.el('wk-card', {}, kids);
  }

  // filterBar is one persistent subtree: render() re-appends it as-is so the
  // controls (and any in-progress typing) survive a re-render.
  var subjectInput = document.createElement('input');
  subjectInput.type = 'search';
  subjectInput.placeholder = 'Filter by subject…';
  subjectInput.value = subject;
  subjectInput.setAttribute('aria-label', 'Filter by subject');
  subjectInput.addEventListener('input', function () {
    subject = subjectInput.value.trim();
    syncURL();
    refresh();
  });

  var kindSelect = selectEl('All kinds', KINDS, kind, function (v) {
    kind = v; syncURL(); refresh();
  });
  var statusSelect = selectEl('All statuses', STATUSES, status, function (v) {
    status = v; syncURL(); refresh();
  });

  var filterBar = document.createElement('div');
  filterBar.className = 'filters';
  filterBar.appendChild(subjectInput);
  filterBar.appendChild(kindSelect);
  filterBar.appendChild(statusSelect);

  // selectEl builds a <select> with an empty "all" option plus one per value.
  function selectEl(allLabel, values, current, onChange) {
    var sel = document.createElement('select');
    sel.appendChild(Webkit.el('option', { value: '' }, allLabel));
    values.forEach(function (v) {
      var opt = Webkit.el('option', { value: v }, v);
      if (v === current) opt.setAttribute('selected', '');
      sel.appendChild(opt);
    });
    sel.value = current;
    sel.addEventListener('change', function () { onChange(sel.value); });
    return sel;
  }

  function render(data) {
    lastData = data;
    var assertions = (data && data.assertions) || [];

    var nodes = [
      Webkit.el('wk-page-header', {}, [
        Webkit.el('wk-title', {}, 'Assertions'),
        Webkit.el('wk-subtitle', {}, assertions.length + (assertions.length === 1 ? ' assertion' : ' assertions'))
      ]),
      Webkit.el('p', { class: 'hint' }, [
        'Recorded by agent sessions via the ',
        Webkit.el('code', {}, 'keep'),
        ' MCP or CLI, each pinned to a line range that ',
        Webkit.el('code', {}, 'keep check'),
        ' re-verifies. This view is read-only.'
      ]),
      filterBar
    ];

    var hadFocus = document.activeElement === subjectInput;
    var selStart = hadFocus ? subjectInput.selectionStart : 0;
    var selEnd = hadFocus ? subjectInput.selectionEnd : 0;

    if (!assertions.length) {
      nodes.push(Webkit.el('div', { class: 'empty' }, 'No assertions match.'));
    } else {
      nodes.push(Webkit.el('div', { class: 'a-list' }, assertions.map(card)));
    }

    app.innerHTML = '';
    nodes.forEach(function (n) { app.appendChild(n); });

    if (hadFocus) {
      // Deferred: a synchronous focus() on a node re-attached within the same
      // event turn doesn't stick in Chrome.
      setTimeout(function () {
        subjectInput.focus();
        try { subjectInput.setSelectionRange(selStart, selEnd); } catch (e) { /* type=search quirk */ }
      }, 0);
    }
  }

  function renderError(err) {
    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'empty' },
      'Failed to load assertions: ' + ((err && err.message) || String(err))));
  }

  // refresh re-fetches with the current filters and re-renders in place.
  function refresh() {
    return fetch(apiURL())
      .then(function (r) { if (!r.ok) throw new Error('load failed (' + r.status + ')'); return r.json(); })
      .then(render)
      .catch(renderError);
  }

  // refresh() honours the current filters, so drive the 60s live refresh with
  // it rather than Webkit.poll (whose URL is fixed at call time and would clobber
  // a filtered view). One immediate fetch, then every 60s.
  refresh();
  setInterval(refresh, 60000);
})();
