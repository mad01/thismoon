'use strict';
// prs page — client-side render. The backend serves a static chrome-only
// shell plus the PR list as JSON at /api/prs; this script fetches it and
// builds the DOM with webkit's shared helpers. The page is read-only: acting
// on a PR happens on GitHub, one click away on the title.
// webkit.js loads before this file, so Webkit.el/escapeHtml are available.
(function () {
  var app = document.getElementById('app');

  // ── filter state ──
  // repo and author are exact-match dropdowns whose options come from the
  // facets in the API response; sort flips between newest and oldest. All
  // three go to the server as query params and persist in the URL
  // (?repo=&author=&sort=), so a filtered view survives a reload.
  var SORTS = ['newest', 'oldest'];
  var repo = '';
  var author = '';
  var sort = '';
  var lastData = null;

  (function readURL() {
    var p = new URLSearchParams(location.search);
    repo = p.get('repo') || '';
    author = p.get('author') || '';
    sort = SORTS.indexOf(p.get('sort')) >= 0 ? p.get('sort') : '';
  })();

  function syncURL() {
    var p = new URLSearchParams();
    if (repo) p.set('repo', repo);
    if (author) p.set('author', author);
    if (sort) p.set('sort', sort);
    var qs = p.toString();
    history.replaceState(null, '', qs ? '?' + qs : location.pathname);
  }

  function apiURL() {
    var p = new URLSearchParams();
    if (repo) p.set('repo', repo);
    if (author) p.set('author', author);
    if (sort) p.set('sort', sort);
    var qs = p.toString();
    return '/api/prs' + (qs ? '?' + qs : '');
  }

  // ── time helpers (mirror the sibling services' Local().Format) ──
  var DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  function fmtTime(iso) {
    if (!iso) return '';
    var d = new Date(iso);
    if (isNaN(d) || d.getFullYear() <= 1) return '';
    return DAYS[d.getDay()] + ' ' + MONTHS[d.getMonth()] + ' ' + d.getDate() +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function relTime(iso) {
    var d = new Date(iso), s = Math.max(0, (Date.now() - d.getTime()) / 1000);
    if (s < 60) return Math.floor(s) + 's ago';
    if (s < 3600) return Math.floor(s / 60) + 'm ago';
    if (s < 86400) return Math.floor(s / 3600) + 'h ago';
    return Math.floor(s / 86400) + 'd ago';
  }

  // timeEl renders when a PR was opened, carrying its ISO time in data-iso
  // so refreshTimes() can re-derive the label between full re-renders.
  function timeEl(iso) {
    var exact = fmtTime(iso);
    if (!exact) return null;
    return Webkit.el('span', { class: 'pr-time', 'data-iso': iso, title: 'opened ' + exact }, relTime(iso));
  }

  // reviewBadge maps the standing review decision → a wk-badge, or null when
  // no review stands yet.
  function reviewBadge(pr) {
    if (pr.review_decision === 'APPROVED') {
      return Webkit.el('wk-badge', { variant: 'ok' }, 'approved');
    }
    if (pr.review_decision === 'CHANGES_REQUESTED') {
      return Webkit.el('wk-badge', { variant: 'warn' }, 'changes requested');
    }
    return null;
  }

  // row renders one PR as a slim single line: age, repo, title (a link),
  // review/label badges, meta, and an explicit open-in-new-tab link.
  function row(pr) {
    var kids = [];
    var opened = timeEl(pr.created_at);
    if (opened) kids.push(opened);
    kids.push(Webkit.el('wk-badge', { variant: 'stat' }, pr.repo));
    kids.push(Webkit.el('div', { class: 'pr-title', title: pr.title },
      Webkit.el('a', { href: pr.url, target: '_blank', rel: 'noopener' }, pr.title)));
    var review = reviewBadge(pr);
    if (review) kids.push(review);
    (pr.labels || []).forEach(function (l) {
      kids.push(Webkit.el('wk-badge', { variant: 'outline' }, l));
    });
    kids.push(Webkit.el('span', { class: 'pr-meta' }, '#' + pr.number + ' · ' + pr.author));
    kids.push(Webkit.el('a', {
      class: 'pr-open', href: pr.url, target: '_blank', rel: 'noopener',
      'aria-label': 'open PR on GitHub'
    }, 'open ↗'));
    return Webkit.el('div', { class: 'pr-row' }, kids);
  }

  function errorCallout(e) {
    return Webkit.el('wk-callout', { variant: 'error', class: 'pr-error' },
      e.host + '/' + e.repo + ': ' + e.error);
  }

  // ── filter bar: one persistent subtree, re-appended as-is on render so
  // the controls survive a re-render. Options refresh from the facets.
  var repoSelect = selectEl('All repos', function (v) { repo = v; syncURL(); refresh(); });
  var authorSelect = selectEl('All authors', function (v) { author = v; syncURL(); refresh(); });
  var sortSelect = selectEl(null, function (v) { sort = v === 'oldest' ? 'oldest' : ''; syncURL(); refresh(); });
  fillOptions(sortSelect, ['newest', 'oldest'], null);

  var resetBtn = document.createElement('button');
  resetBtn.type = 'button';
  resetBtn.textContent = 'Reset';
  resetBtn.addEventListener('click', function () {
    repo = ''; author = ''; sort = '';
    repoSelect.value = ''; authorSelect.value = ''; sortSelect.value = 'newest';
    syncURL();
    refresh();
  });

  var filterBar = document.createElement('div');
  filterBar.className = 'filters';
  filterBar.appendChild(repoSelect);
  filterBar.appendChild(authorSelect);
  filterBar.appendChild(sortSelect);
  filterBar.appendChild(resetBtn);

  // selectEl builds a <select>; options arrive later via fillOptions since
  // the facets come from the API.
  function selectEl(allLabel, onChange) {
    var sel = document.createElement('select');
    sel.dataset.allLabel = allLabel || '';
    sel.addEventListener('change', function () { onChange(sel.value); });
    return sel;
  }

  // fillOptions rebuilds a select's options, keeping the current selection
  // when it still exists.
  function fillOptions(sel, values, current) {
    var want = current !== null ? current : sel.value;
    sel.innerHTML = '';
    if (sel.dataset.allLabel) {
      sel.appendChild(Webkit.el('option', { value: '' }, sel.dataset.allLabel));
    }
    values.forEach(function (v) {
      sel.appendChild(Webkit.el('option', { value: v }, v));
    });
    sel.value = values.indexOf(want) >= 0 || want === '' ? want : '';
  }

  function render(data) {
    lastData = data;
    var prs = data.prs || [];
    var status = data.status || {};
    var facets = data.facets || {};

    fillOptions(repoSelect, facets.repos || [], repo);
    fillOptions(authorSelect, facets.authors || [], author);
    sortSelect.value = sort || 'newest';

    var count = status.open_prs || 0;
    var nodes = [
      Webkit.el('wk-page-header', {}, [
        Webkit.el('wk-title', {}, 'Open PRs'),
        Webkit.el('wk-subtitle', {},
          count + (count === 1 ? ' open PR' : ' open PRs') +
          ' across ' + (status.repos || 0) + ' repos')
      ]),
      filterBar
    ];

    (status.errors || []).forEach(function (e) { nodes.push(errorCallout(e)); });

    if (!prs.length) {
      nodes.push(Webkit.el('div', { class: 'empty' }, 'No open PRs match.'));
    } else {
      nodes.push(Webkit.el('div', { class: 'pr-list' }, prs.map(row)));
    }

    var polled = fmtTime(status.polled_at);
    nodes.push(Webkit.el('div', { class: 'footer' },
      polled ? 'Last poll ' + polled : 'Not polled yet'));

    app.innerHTML = '';
    nodes.forEach(function (n) { app.appendChild(n); });
  }

  function renderError(err) {
    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'empty' },
      'Failed to load PRs: ' + ((err && err.message) || String(err))));
  }

  // refresh re-fetches with the current filters and re-renders in place.
  function refresh() {
    return fetch(apiURL())
      .then(function (r) { if (!r.ok) throw new Error('load failed (' + r.status + ')'); return r.json(); })
      .then(render)
      .catch(renderError);
  }

  // refreshTimes re-derives the relative labels in place, ticking faster
  // than the full refresh so "3m ago" never goes visibly stale.
  function refreshTimes() {
    app.querySelectorAll('[data-iso]').forEach(function (node) {
      node.textContent = relTime(node.dataset.iso);
    });
  }

  // refresh() honours the current filters, so drive the 60s live refresh
  // with it rather than Webkit.poll (whose URL is fixed at call time and
  // would clobber a filtered view). One immediate fetch, then every 60s.
  refresh();
  setInterval(refresh, 60000);
  setInterval(refreshTimes, 7000);
})();
