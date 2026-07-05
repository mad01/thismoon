'use strict';
// deps page — client-side render. The backend serves a static shell (chrome
// only) plus the full enriched inventory as JSON at /api/deps; this script
// fetches it and builds the DOM with webkit's shared helpers. The shaping that
// used to live in Go (internal/server/render.go: buildView/toViewDep/ecoCount)
// lives here now. webkit.js loads before this file, so Webkit.el/escapeHtml/poll
// are available.
(function () {
  var app = document.getElementById('app');
  var host = document.getElementById('toast-host');

  // ── filter state ──
  // selected: repo path → true. When non-empty, the finding sections and the
  // inventory search show only the selected repos. Clicking a repo name
  // toggles it (multi-select); "Clear" empties the set. ecoSel is the same for
  // ecosystems (Go/npm chips). query live-filters the full inventory. lastData
  // is the most recent /api/deps payload so a filter toggle re-renders without
  // a refetch. All three persist in the URL (?repo=&eco=&q=) — repos by
  // basename for readable links, resolved back to paths on the first render.
  var selected = {};
  var ecoSel = {};
  var query = '';
  var pendingRepos = null; // basenames from the URL, resolved once data arrives
  var lastData = null;
  function selectedRepos() { return Object.keys(selected).filter(function (p) { return selected[p]; }); }
  function selectedEcos() { return Object.keys(ecoSel).filter(function (e) { return ecoSel[e]; }); }

  (function readURL() {
    var p = new URLSearchParams(location.search);
    var repos = (p.get('repo') || '').split(',').filter(Boolean);
    if (repos.length) pendingRepos = repos;
    (p.get('eco') || '').split(',').filter(Boolean).forEach(function (e) { ecoSel[e] = true; });
    query = p.get('q') || '';
  })();

  function syncURL() {
    var p = new URLSearchParams();
    var repos = selectedRepos().map(basename);
    if (repos.length) p.set('repo', repos.join(','));
    var ecos = selectedEcos();
    if (ecos.length) p.set('eco', ecos.join(','));
    if (query) p.set('q', query);
    var qs = p.toString();
    history.replaceState(null, '', qs ? '?' + qs : location.pathname);
  }

  // ── toast / fetch / button helpers (ported verbatim from the old inline
  //    script in index.html) ──
  function toast(msg, variant) {
    var t = document.createElement('wk-toast');
    t.setAttribute('variant', variant || 'ok');
    t.textContent = msg;
    host.appendChild(t);
    setTimeout(function () { t.remove(); }, 3200);
  }

  function post(url, body) {
    var opts = { method: 'POST' };
    if (body) { opts.headers = { 'Content-Type': 'application/json' }; opts.body = JSON.stringify(body); }
    return fetch(url, opts);
  }

  async function withButton(btn, label, fn) {
    var prev = btn.textContent;
    btn.disabled = true; btn.textContent = label;
    try { await fn(); }
    finally { btn.disabled = false; btn.textContent = prev; }
  }

  // ── time helpers (mirror render.go) ──
  var DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  // hasScan: a zero time.Time serializes as year 0001.
  function hasScan(iso) {
    if (!iso) return false;
    var d = new Date(iso);
    return !isNaN(d) && d.getFullYear() > 1;
  }

  // scannedText mirrors Go's Local().Format("Mon Jan 2 15:04").
  function scannedText(d) {
    return DAYS[d.getDay()] + ' ' + MONTHS[d.getMonth()] + ' ' + d.getDate() +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  var MIN = 60000, HR = 3600000, DAY = 86400000;
  // humanizeSince mirrors render.go's coarse "x ago" string.
  function humanizeSince(ms) {
    if (ms < MIN) return 'just now';
    if (ms < HR) return plural(Math.floor(ms / MIN), 'minute');
    if (ms < DAY) return plural(Math.floor(ms / HR), 'hour');
    return plural(Math.floor(ms / DAY), 'day');
  }
  function plural(n, unit) {
    if (n < 0) n = 0;
    return n + ' ' + unit + (n !== 1 ? 's' : '') + ' ago';
  }

  // staleAfter matches render.go (the serve loop's default daily interval).
  var STALE_AFTER = DAY;

  function basename(p) {
    var s = String(p || '');
    var i = s.lastIndexOf('/');
    return i >= 0 ? s.slice(i + 1) : s;
  }

  function depActive(d) {
    return (d.advisories || []).some(function (a) { return !a.resolved; });
  }

  // ── severity ranking ──
  // OSV severities on live data are plain labels (CRITICAL/HIGH/MODERATE/LOW,
  // often empty). Rank worst-first so the active section leads with what
  // matters; unknown/empty sorts last.
  var SEV_RANK = { CRITICAL: 0, HIGH: 1, MODERATE: 2, MEDIUM: 2, LOW: 3 };
  function sevRank(s) {
    var r = SEV_RANK[String(s || '').toUpperCase()];
    return r === undefined ? 4 : r;
  }
  function depSevRank(d) {
    var worst = 4;
    (d.advisories || []).forEach(function (a) {
      if (!a.resolved) worst = Math.min(worst, sevRank(a.severity));
    });
    return worst;
  }
  function sevClass(s) {
    var r = sevRank(s);
    return r <= 1 ? 'sev-high' : r === 2 ? 'sev-moderate' : '';
  }

  // ── inventory search ──
  // searchWrap (input + results) is one persistent subtree: render() re-appends
  // it as-is, and a keystroke only rebuilds the results container — never the
  // input itself, so typing survives re-renders without focus tricks.
  var searchInput = document.createElement('input');
  searchInput.type = 'search';
  searchInput.className = 'inv-search';
  searchInput.placeholder = 'Search 0 packages…';
  searchInput.value = query;
  searchInput.setAttribute('aria-label', 'Search packages');
  var searchResults = document.createElement('div');
  var searchWrap = document.createElement('div');
  searchWrap.className = 'inv-search-row';
  searchWrap.appendChild(searchInput);
  searchWrap.appendChild(searchResults);
  searchInput.addEventListener('input', function () {
    query = searchInput.value.trim();
    syncURL();
    if (lastData) renderSearch(lastData);
  });

  // renderSearch rebuilds only the results block under the (untouched) input.
  function renderSearch(data) {
    searchResults.innerHTML = '';
    if (!query) return;
    var groups = groupMatches(data.deps || [], query, currentFilter());
    var shown = groups.slice(0, SEARCH_CAP);
    searchResults.appendChild(Webkit.el('div', { class: 'sec-title inv-title' }, 'Matches · ' + groups.length));
    if (shown.length) {
      searchResults.appendChild(Webkit.el('div', { class: 'inv-list' }, shown.map(searchRow)));
    } else {
      searchResults.appendChild(Webkit.el('div', { class: 'note' }, 'No packages match.'));
    }
    if (groups.length > shown.length) {
      searchResults.appendChild(Webkit.el('div', { class: 'note' },
        (groups.length - shown.length) + ' more — narrow the search.'));
    }
  }

  // currentFilter builds the repo+ecosystem predicate shared by the finding
  // sections and the inventory search.
  function currentFilter() {
    var sel = selectedRepos();
    var ecos = selectedEcos();
    return function (d) {
      return (!sel.length || selected[d.repo]) && (!ecos.length || ecoSel[d.ecosystem]);
    };
  }

  var SEARCH_CAP = 50;

  // groupMatches folds matching inventory entries into one row per
  // ecosystem:name@version, collecting the pinning repos.
  function groupMatches(deps, q, inFilter) {
    var needle = q.toLowerCase();
    var by = {};
    var order = [];
    deps.forEach(function (d) {
      if (!inFilter(d)) return;
      if (d.name.toLowerCase().indexOf(needle) < 0) return;
      var k = d.ecosystem + ':' + d.name + '@' + d.version;
      if (!by[k]) {
        by[k] = { ecosystem: d.ecosystem, name: d.name, version: d.version, repoDirect: {}, repoOrder: [], flagged: false, graphOnly: true };
        order.push(k);
      }
      var g = by[k];
      // A monorepo holds many manifests, so the same repo recurs — dedupe,
      // keeping "(direct)" if any of its manifests pins the package directly.
      var rb = basename(d.repo);
      if (!(rb in g.repoDirect)) g.repoOrder.push(rb);
      g.repoDirect[rb] = g.repoDirect[rb] || !!d.direct;
      if ((d.advisories || []).length) g.flagged = true;
      if (d.imported !== false) g.graphOnly = false;
    });
    return order.map(function (k) {
      var g = by[k];
      g.repos = g.repoOrder.map(function (rb) { return rb + (g.repoDirect[rb] ? ' (direct)' : ''); });
      return g;
    }).sort(byName);
  }

  function searchRow(g) {
    var head = [
      Webkit.el('span', { class: 'flag-name' }, g.name + '@' + g.version),
      Webkit.el('span', { class: 'flag-repo' }, g.ecosystem + ' · ' + g.repos.join(', '))
    ];
    if (g.flagged) head.push(Webkit.el('wk-badge', { variant: g.graphOnly ? 'outline' : 'accent' },
      g.graphOnly ? 'flagged · not compiled in' : 'flagged'));
    return Webkit.el('div', { class: 'inv-row' }, head);
  }

  // groupResolved folds the flat resolved_fixed list (acknowledged advisories
  // that no longer appear in the scan — fixed by a version bump) into one entry
  // per package@version, collecting the advisory ids. These carry no repo
  // (the resolved set is version-keyed, repo-independent), so they aren't
  // filterable by repo.
  function groupResolved(items) {
    var by = {};
    var order = [];
    (items || []).forEach(function (it) {
      var k = it.ecosystem + ':' + it.name + '@' + it.version;
      if (!by[k]) { by[k] = { ecosystem: it.ecosystem, name: it.name, version: it.version, ids: [] }; order.push(k); }
      by[k].ids.push(it.id);
    });
    return order.map(function (k) { return by[k]; });
  }

  // buildView reduces the enriched inventory to what the page renders — a direct
  // port of render.go's buildView.
  function buildView(data) {
    var deps = data.deps || [];
    var v = {
      hasScan: hasScan(data.scanned_at),
      total: deps.length,
      activeCount: 0,
      resolvedOnly: 0,
      byEcosystem: [],
      repos: [],
      active: [],
      acknowledged: [],
      notCompiledIn: [],
      resolved: groupResolved(data.resolved_fixed)
    };
    if (v.hasScan) {
      var when = new Date(data.scanned_at);
      var age = Date.now() - when.getTime();
      v.scannedText = scannedText(when);
      v.agoText = humanizeSince(age);
      v.stale = age > STALE_AFTER;
    }

    var counts = {};
    var repoActive = {};
    var repoSeen = {};
    deps.forEach(function (d) {
      counts[d.ecosystem] = (counts[d.ecosystem] || 0) + 1;
      if (!(d.repo in repoSeen)) {
        repoSeen[d.repo] = true;
        v.repos.push({ name: basename(d.repo), path: d.repo, active: 0 });
      }
      if (!(d.advisories || []).length) return;
      // imported === false: a graph-only transitive (in the module graph but
      // not compiled into any binary). Not a real exposure — route it to the
      // informational "not compiled in" bucket, never active/acknowledged. An
      // undefined imported (older payload) fails open to imported.
      if (d.imported === false) {
        v.notCompiledIn.push(d);
        return;
      }
      if (depActive(d)) {
        v.activeCount++;
        repoActive[d.repo] = (repoActive[d.repo] || 0) + 1;
        v.active.push(d);
      } else {
        v.resolvedOnly++;
        v.acknowledged.push(d);
      }
    });
    v.repos.forEach(function (r) { r.active = repoActive[r.path] || 0; });
    Object.keys(counts).forEach(function (eco) {
      v.byEcosystem.push({ ecosystem: eco, count: counts[eco] });
    });

    v.byEcosystem.sort(function (a, b) { return a.ecosystem < b.ecosystem ? -1 : a.ecosystem > b.ecosystem ? 1 : 0; });
    v.repos.sort(byName);
    // Worst severity first so CRITICAL/HIGH lead the section; name breaks ties.
    v.active.sort(function (a, b) { return depSevRank(a) - depSevRank(b) || byName(a, b); });
    v.acknowledged.sort(byName);
    v.notCompiledIn.sort(byName);
    return v;
  }
  function byName(a, b) { return a.name < b.name ? -1 : a.name > b.name ? 1 : 0; }

  // ── DOM builders (mirror the old html/template markup) ──
  function pageHeader(v) {
    var sub = v.hasScan
      ? (v.activeCount + ' active · ' + v.total + ' total'
        + (v.resolved.length ? ' · ' + v.resolved.length + ' resolved' : '')
        + (v.resolvedOnly ? ' · ' + v.resolvedOnly + ' acknowledged' : '')
        + (v.notCompiledIn.length ? ' · ' + v.notCompiledIn.length + ' not compiled in' : ''))
      : 'no scan yet';
    return Webkit.el('wk-page-header', {}, [
      Webkit.el('wk-title', {}, 'Dependencies'),
      Webkit.el('wk-subtitle', {}, sub)
    ]);
  }

  function toolbar(v) {
    var kids = [Webkit.el('wk-button', { variant: 'accent', 'data-action': 'rescan-all' }, 'Rescan all')];
    if (v.hasScan) {
      kids.push(Webkit.el('span', { class: 'scanmeta' + (v.stale ? ' stale' : '') },
        'scanned ' + v.agoText + (v.stale ? ' · stale' : '')));
    }
    return Webkit.el('div', { class: 'toolbar' }, kids);
  }

  function repoChip(r) {
    var on = !!selected[r.path];
    var kids = [Webkit.el('button', {
      class: 'repo-name' + (on ? ' active' : ''),
      'data-action': 'filter-repo', 'data-repo': r.path,
      title: (on ? 'Remove ' + r.name + ' from filter' : 'Filter to ' + r.name)
    }, r.name), ' '];
    if (r.active) kids.push(Webkit.el('span', { class: 'n' }, String(r.active)), ' ');
    kids.push(Webkit.el('button', {
      class: 'btn btn-ghost', 'data-action': 'rescan-repo', 'data-repo': r.path, title: 'Rescan ' + r.name
    }, '↻'));
    return Webkit.el('span', { class: 'repo-chip' + (on ? ' sel' : '') }, kids);
  }

  // advisoryRow builds a finding's advisory line for the active section: id +
  // optional severity + optional fix + optional summary, with a Resolve action.
  function advisoryRow(a) {
    var line = [Webkit.el('span', { class: 'adv-id' }, a.id)];
    if (a.severity) line.push(' ', Webkit.el('span', { class: sevClass(a.severity) }, '[' + a.severity + ']'));
    if (a.fixed_version) line.push(' ', Webkit.el('span', { class: 'adv-fix' }, '→ fixed in ' + a.fixed_version));
    var left = [Webkit.el('div', {}, line)];
    if (a.summary) left.push(Webkit.el('div', {}, a.summary));

    var action = a.resolved
      ? Webkit.el('wk-badge', { variant: 'outline' }, 'resolved')
      : Webkit.el('button', { class: 'btn btn-ghost', 'data-action': 'resolve', 'data-key': a.key }, 'Resolve');

    return Webkit.el('div', { class: 'adv' + (a.resolved ? ' resolved' : '') }, [
      Webkit.el('div', {}, left),
      Webkit.el('div', { class: 'adv-actions' }, action)
    ]);
  }

  function activeCard(d) {
    return Webkit.el('wk-card', {}, [
      Webkit.el('div', { class: 'flag-head' }, [
        Webkit.el('wk-badge', { variant: 'accent' }, 'flagged'),
        Webkit.el('span', { class: 'flag-name' }, d.name + '@' + d.version),
        Webkit.el('span', { class: 'flag-repo' }, d.ecosystem + ' · ' + basename(d.repo) + (d.direct ? '' : ' · transitive'))
      ])
    ].concat((d.advisories || []).map(advisoryRow)));
  }

  function ackCard(d) {
    var advs = (d.advisories || []).map(function (a) {
      var line = [Webkit.el('span', { class: 'adv-id' }, a.id)];
      if (a.fixed_version) line.push(' ', Webkit.el('span', { class: 'adv-fix' }, '→ fixed in ' + a.fixed_version));
      return Webkit.el('div', { class: 'adv resolved' }, Webkit.el('div', {}, line));
    });
    return Webkit.el('wk-card', {}, [
      Webkit.el('div', { class: 'flag-head' }, [
        Webkit.el('wk-badge', { variant: 'outline' }, 'acknowledged'),
        Webkit.el('span', { class: 'flag-name' }, d.name + '@' + d.version),
        Webkit.el('span', { class: 'flag-repo' }, d.ecosystem + ' · ' + basename(d.repo))
      ])
    ].concat(advs));
  }

  // resolvedCard renders a fixed finding: the package@version that was
  // acknowledged and has since dropped out of the scan, with its advisory ids.
  function resolvedCard(r) {
    var advs = r.ids.map(function (id) {
      return Webkit.el('div', { class: 'adv resolved' }, Webkit.el('span', { class: 'adv-id' }, id));
    });
    return Webkit.el('wk-card', {}, [
      Webkit.el('div', { class: 'flag-head' }, [
        Webkit.el('wk-badge', { variant: 'ok' }, 'resolved'),
        Webkit.el('span', { class: 'flag-name' }, r.name + '@' + r.version),
        Webkit.el('span', { class: 'flag-repo' }, r.ecosystem + ' · updated past')
      ])
    ].concat(advs));
  }

  // notCompiledCard renders a graph-only finding: a flagged dependency that's in
  // the module graph but not imported into any binary, so it's not a real
  // exposure. Informational only — no Resolve action, dimmed badge.
  function notCompiledCard(d) {
    var advs = (d.advisories || []).map(function (a) {
      var line = [Webkit.el('span', { class: 'adv-id' }, a.id)];
      if (a.fixed_version) line.push(' ', Webkit.el('span', { class: 'adv-fix' }, '→ fixed in ' + a.fixed_version));
      return Webkit.el('div', { class: 'adv resolved' }, Webkit.el('div', {}, line));
    });
    return Webkit.el('wk-card', {}, [
      Webkit.el('div', { class: 'flag-head' }, [
        Webkit.el('wk-badge', { variant: 'outline' }, 'not compiled in'),
        Webkit.el('span', { class: 'flag-name' }, d.name + '@' + d.version),
        Webkit.el('span', { class: 'flag-repo' }, d.ecosystem + ' · ' + basename(d.repo) + ' · transitive')
      ])
    ].concat(advs));
  }

  function render(data) {
    lastData = data;
    // Repo basenames from the URL resolve to paths on the first render, once
    // the inventory tells us which paths exist.
    if (pendingRepos) {
      var wanted = {};
      pendingRepos.forEach(function (b) { wanted[b] = true; });
      (data.deps || []).forEach(function (d) {
        if (wanted[basename(d.repo)]) selected[d.repo] = true;
      });
      pendingRepos = null;
    }
    var v = buildView(data);
    var nodes = [pageHeader(v), toolbar(v)];
    nodes.push(Webkit.el('p', { class: 'hint' },
      'Supply-chain scan across the catalog-registered repos, checked against OSV.dev. Resolve a finding to acknowledge it — it returns if the version changes or a new advisory lands.'));

    if (!v.hasScan) {
      nodes.push(Webkit.el('div', { class: 'empty' }, [
        'No scan has run yet. Click ', Webkit.el('strong', {}, 'Rescan all'), ', or wait for the daily background scan.'
      ]));
    } else {
      nodes.push(Webkit.el('div', { class: 'counts' }, v.byEcosystem.map(function (e) {
        var attrs = { variant: 'filter', 'data-action': 'filter-eco', 'data-eco': e.ecosystem,
          title: (ecoSel[e.ecosystem] ? 'Remove ' + e.ecosystem + ' from filter' : 'Filter to ' + e.ecosystem) };
        if (ecoSel[e.ecosystem]) attrs.active = '';
        return Webkit.el('wk-badge', attrs, e.ecosystem + ' · ' + e.count);
      })));

      if (v.repos.length) {
        nodes.push(Webkit.el('div', { class: 'repos' }, v.repos.map(repoChip)));
      }

      // Filters compose: selected repos AND selected ecosystems narrow the
      // finding sections and the inventory search alike.
      var sel = selectedRepos();
      var ecos = selectedEcos();
      var inFilter = currentFilter();
      if (sel.length || ecos.length) {
        nodes.push(Webkit.el('div', { class: 'filterbar' }, [
          Webkit.el('span', {}, 'Showing ' + sel.map(basename).concat(ecos).join(', ')),
          Webkit.el('button', { class: 'btn btn-ghost', 'data-action': 'clear-filter' }, 'Clear filter')
        ]));
      }

      // Inventory search: the only view onto the full inventory — the finding
      // sections below cover just the flagged slice.
      searchInput.placeholder = 'Search ' + v.total + ' packages…';
      nodes.push(searchWrap);
      renderSearch(data);
      var activeList = v.active.filter(inFilter);
      var ackList = v.acknowledged.filter(inFilter);
      var notCompiledList = v.notCompiledIn.filter(inFilter);

      if (v.active.length) {
        nodes.push(Webkit.el('div', { class: 'sec-title' }, 'Active findings'));
        if (activeList.length) {
          nodes.push(Webkit.el('div', { class: 'flag-list' }, activeList.map(activeCard)));
        } else {
          nodes.push(Webkit.el('div', { class: 'note' }, 'No active findings for the selected repos.'));
        }
      } else {
        nodes.push(Webkit.el('div', { class: 'ok' },
          'No active findings — all ' + v.total + ' checked versions are clear of unresolved OSV advisories.'));
      }

      // Resolved: acknowledged findings that have since been fixed by a version
      // bump (gone from the scan). This is the section that matters — it records
      // actual remediation, so it's shown above Acknowledged and open by default.
      // No repo data on these, so they're not affected by the repo filter.
      if (v.resolved.length) {
        nodes.push(Webkit.el('details', { class: 'ack', open: '' }, [
          Webkit.el('summary', { class: 'sec-title ack-summary' },
            'Resolved · ' + v.resolved.length),
          Webkit.el('div', { class: 'flag-list ack-list' }, v.resolved.map(resolvedCard))
        ]));
      }

      if (sel.length ? ackList.length : v.acknowledged.length) {
        // Acknowledged findings are low-signal — collapsed by default, expand
        // to view. Native <details>; count stays visible in the summary.
        nodes.push(Webkit.el('details', { class: 'ack' }, [
          Webkit.el('summary', { class: 'sec-title ack-summary' },
            'Acknowledged · ' + ackList.length),
          Webkit.el('div', { class: 'flag-list ack-list' }, ackList.map(ackCard))
        ]));
      }

      // Not compiled in: flagged graph-only transitives (in the module graph but
      // not imported into any binary). These are not real exposures and can't be
      // fixed by a version bump in this repo, so they're separated out, dimmed,
      // and collapsed — informational, lowest signal of all.
      if (sel.length ? notCompiledList.length : v.notCompiledIn.length) {
        nodes.push(Webkit.el('details', { class: 'ack notcompiled' }, [
          Webkit.el('summary', { class: 'sec-title ack-summary' },
            'Not compiled in · ' + notCompiledList.length),
          Webkit.el('div', { class: 'note' },
            'Present in the module graph but not imported into any binary — not a real exposure, and not fixable by a version bump here. Shown for completeness.'),
          Webkit.el('div', { class: 'flag-list ack-list' }, notCompiledList.map(notCompiledCard))
        ]));
      }
    }

    // The wipe detaches searchInput (same node, value survives) — restore
    // focus + caret so typing across re-renders is seamless.
    var hadFocus = document.activeElement === searchInput;
    var selStart = hadFocus ? searchInput.selectionStart : 0;
    var selEnd = hadFocus ? searchInput.selectionEnd : 0;
    app.innerHTML = '';
    nodes.forEach(function (n) { app.appendChild(n); });
    if (hadFocus) {
      // Deferred: a synchronous focus() on a node re-attached within the same
      // event turn doesn't stick in Chrome.
      setTimeout(function () {
        searchInput.focus();
        try { searchInput.setSelectionRange(selStart, selEnd); } catch (e) { /* type=search quirk */ }
      }, 0);
    }
  }

  // refresh re-fetches the inventory and re-renders — replaces the old
  // location.reload() after a successful action.
  function refresh() {
    return fetch('/api/deps').then(function (r) {
      if (!r.ok) throw new Error('load failed (' + r.status + ')');
      return r.json();
    }).then(render);
  }

  // Action handling via event delegation on document, so it survives re-render.
  document.addEventListener('click', function (e) {
    var rescanAll = e.target.closest('[data-action="rescan-all"]');
    if (rescanAll) {
      withButton(rescanAll, 'Scanning…', async function () {
        var res = await post('/api/check');
        if (res.ok) { await refresh(); } else { toast('Scan failed: ' + (await res.text()), 'err'); }
      });
      return;
    }
    var rescan = e.target.closest('button[data-action="rescan-repo"]');
    if (rescan) {
      withButton(rescan, '…', async function () {
        var res = await post('/api/check?repo=' + encodeURIComponent(rescan.dataset.repo));
        if (res.ok) { await refresh(); } else { toast('Rescan failed: ' + (await res.text()), 'err'); }
      });
      return;
    }
    var filterRepo = e.target.closest('button[data-action="filter-repo"]');
    if (filterRepo) {
      var p = filterRepo.dataset.repo;
      if (selected[p]) delete selected[p]; else selected[p] = true;
      syncURL();
      if (lastData) render(lastData);
      return;
    }
    var filterEco = e.target.closest('[data-action="filter-eco"]');
    if (filterEco) {
      var eco = filterEco.dataset.eco;
      if (ecoSel[eco]) delete ecoSel[eco]; else ecoSel[eco] = true;
      syncURL();
      if (lastData) render(lastData);
      return;
    }
    var clearFilter = e.target.closest('[data-action="clear-filter"]');
    if (clearFilter) {
      selected = {};
      ecoSel = {};
      syncURL();
      if (lastData) render(lastData);
      return;
    }
    var resolve = e.target.closest('button[data-action="resolve"]');
    if (resolve) {
      withButton(resolve, '…', async function () {
        var res = await post('/api/resolve', { keys: [resolve.dataset.key] });
        if (res.ok) { await refresh(); } else { toast('Resolve failed: ' + (await res.text()), 'err'); }
      });
      return;
    }
  });

  // Webkit.poll fetches immediately and then every 60s, replacing the old
  // server-rendered page with no auto-reload.
  Webkit.poll('/api/deps', render, 60000, function (err) {
    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'empty' },
      'Failed to load dependencies: ' + ((err && err.message) || String(err))));
  });
})();
