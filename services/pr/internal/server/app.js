'use strict';
// pr dashboard — client-side render. The backend serves a static shell (chrome
// only) plus the raw cached snapshot as JSON at /api/prs ([]RepoPRs); this
// script fetches it and builds the DOM with webkit's shared helpers. The view
// shaping that used to live in Go (internal/server/views.go buildListView /
// buildPRView / buildFilters) lives here now. webkit.js loads before this file,
// so Webkit.el / escapeHtml / poll are available.
(function () {
  var app = document.getElementById('app');
  var POLL_MS = 300000; // matches the old setTimeout(location.reload, 300000)

  // --- view shaping (ported from views.go) ---

  // humanAge mirrors views.go humanAge for an RFC3339 timestamp.
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

  // reviewDecisionToState mirrors views.go reviewDecisionToState.
  function reviewDecisionToState(d) {
    switch (d) {
      case 'APPROVED': return 'approved';
      case 'CHANGES_REQUESTED': return 'changes_requested';
      case 'REVIEW_REQUIRED': return 'review_required';
      default: return '';
    }
  }

  // buildPRView mirrors views.go buildPRView. Labels carry no color server-side
  // (Go sets only Name), so color stays empty — matching current behaviour.
  function buildPRView(pr, repo) {
    return {
      number: pr.number,
      title: pr.title,
      author: pr.author,
      age: humanAge(pr.created_at),
      htmlURL: pr.html_url,
      detailURL: '/pr/' + repo.Host + '/' + repo.Owner + '/' + repo.Name + '/' + pr.number,
      draft: !!pr.draft,
      additions: pr.additions || 0,
      deletions: pr.deletions || 0,
      labels: (pr.labels || []).map(function (l) { return { name: l, color: '' }; }),
      reviewState: '',
      headRef: pr.head_ref || '',
      baseRef: pr.base_ref || '',
      host: repo.Host,
      owner: repo.Owner,
      repo: repo.Name,
      ticketID: pr.ticket_id || '',
      createdAt: pr.created_at,
    };
  }

  // buildFilters mirrors views.go buildFilters: ticket/branch tags that recur.
  function buildFilters(prs) {
    var ticketCounts = {};
    var branchCounts = {};
    var branchHasTicket = {};
    prs.forEach(function (pr) {
      if (pr.ticketID) {
        ticketCounts[pr.ticketID] = (ticketCounts[pr.ticketID] || 0) + 1;
        branchHasTicket[pr.headRef] = true;
      }
      if (pr.headRef) {
        branchCounts[pr.headRef] = (branchCounts[pr.headRef] || 0) + 1;
      }
    });
    var filters = [];
    Object.keys(ticketCounts).forEach(function (k) {
      if (ticketCounts[k] >= 2) filters.push({ key: k, kind: 'ticket', count: ticketCounts[k] });
    });
    Object.keys(branchCounts).forEach(function (k) {
      if (branchCounts[k] >= 2 && !branchHasTicket[k]) {
        filters.push({ key: k, kind: 'branch', count: branchCounts[k] });
      }
    });
    filters.sort(function (a, b) {
      if (a.count !== b.count) return b.count - a.count;
      return a.key < b.key ? -1 : a.key > b.key ? 1 : 0;
    });
    return filters;
  }

  // hhmmss formats the current local time as Go's "15:04:05".
  function pad(n) { return n < 10 ? '0' + n : '' + n; }
  function hhmmss(d) { return pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds()); }

  // buildListView mirrors views.go buildListView over the /api/prs snapshot.
  function buildListView(data) {
    var allPRs = [];
    var errors = [];
    (data || []).forEach(function (rp) {
      var repo = rp.Repo || {};
      var htmlURL = 'https://' + repo.Host + '/' + repo.Owner + '/' + repo.Name;
      if (rp.Error) {
        errors.push({ name: repo.Owner + '/' + repo.Name, host: repo.Host, htmlURL: htmlURL, error: rp.Error });
      }
      var decisions = rp.ReviewDecisions || {};
      (rp.PRs || []).forEach(function (pr) {
        if (pr.draft || pr.state !== 'open') return;
        var pv = buildPRView(pr, repo);
        var d = decisions[String(pr.number)];
        if (d) pv.reviewState = reviewDecisionToState(d);
        allPRs.push(pv);
      });
    });
    allPRs.sort(function (a, b) {
      return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime();
    });
    return {
      prs: allPRs,
      errors: errors,
      filters: buildFilters(allPRs),
      totalPRs: allPRs.length,
      generated: hhmmss(new Date()),
    };
  }

  // --- actions ---

  function refresh() {
    fetch('/api/refresh', { method: 'POST' }).then(function () { tick(); });
  }

  function approve(btn) {
    var d = btn.dataset;
    fetch('/api/approve', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host: d.host, owner: d.owner, repo: d.repo, number: parseInt(d.number, 10) }),
    }).then(function (r) {
      if (r.ok) { tick(); }
      else { r.text().then(function (t) { alert('Approve failed: ' + t); }); }
    });
  }

  // --- filtering (operates on the live DOM, like the old inline script) ---

  var activeFilter = null;
  var activeKind = null;

  function applyFilter() {
    document.querySelectorAll('.filter-tag').forEach(function (t) {
      t.classList.toggle('active', activeFilter !== null && t.dataset.filter === activeFilter);
    });
    var clear = document.getElementById('filter-clear');
    if (clear) clear.style.display = activeFilter !== null ? '' : 'none';
    document.querySelectorAll('.pr-card').forEach(function (card) {
      if (activeFilter === null) { card.style.display = ''; return; }
      var match;
      if (activeKind === 'review') match = card.dataset.review === activeFilter;
      else if (activeKind === 'ticket') match = card.dataset.ticket === activeFilter;
      else match = card.dataset.branch === activeFilter;
      card.style.display = match ? '' : 'none';
    });
  }

  function setFilter(key, kind) {
    activeFilter = key;
    activeKind = kind;
    applyFilter();
  }

  function clearFilter() {
    activeFilter = null;
    activeKind = null;
    applyFilter();
  }

  function toggleFilter(el) {
    if (el.classList.contains('active')) { clearFilter(); return; }
    setFilter(el.dataset.filter, el.dataset.kind);
  }

  // --- keyboard navigation ---

  var focusIdx = -1;

  function visibleCards() {
    return Array.from(document.querySelectorAll('.pr-card')).filter(function (c) { return c.style.display !== 'none'; });
  }

  function focusCard(idx) {
    var cards = visibleCards();
    document.querySelectorAll('.pr-card.focused').forEach(function (c) { c.classList.remove('focused'); });
    if (idx < 0) idx = 0;
    if (idx >= cards.length) idx = cards.length - 1;
    focusIdx = idx;
    if (cards[focusIdx]) {
      cards[focusIdx].classList.add('focused');
      cards[focusIdx].scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }

  function openFocused() {
    var cards = visibleCards();
    if (focusIdx >= 0 && focusIdx < cards.length) {
      var link = cards[focusIdx].querySelector('.pr-title');
      if (link) window.location.href = link.href;
    }
  }

  function approveFocused() {
    var cards = visibleCards();
    if (focusIdx >= 0 && focusIdx < cards.length) {
      var btn = cards[focusIdx].querySelector('.btn-approve');
      if (btn) btn.click();
    }
  }

  function shortcutsModal() { return document.getElementById('shortcuts-modal'); }
  function toggleShortcuts() {
    var m = shortcutsModal();
    if (m) m.hidden = !m.hidden;
  }
  // The static Close button in shell.html uses onclick="toggleShortcuts()".
  window.toggleShortcuts = toggleShortcuts;

  var chordKey = null;
  var chordTimer = null;

  document.addEventListener('keydown', function (e) {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;
    var m = shortcutsModal();
    var shortcutsOpen = m && !m.hidden;

    // g-chord: g then i -> scroll to top / reset focus
    if (chordKey === 'g' && e.key === 'i') {
      chordKey = null; clearTimeout(chordTimer);
      clearFilter(); focusIdx = -1;
      document.querySelectorAll('.pr-card.focused').forEach(function (c) { c.classList.remove('focused'); });
      window.scrollTo({ top: 0, behavior: 'smooth' });
      return;
    }
    if (e.key === 'g' && !e.metaKey && !e.ctrlKey && !shortcutsOpen) {
      chordKey = 'g';
      clearTimeout(chordTimer);
      chordTimer = setTimeout(function () { chordKey = null; }, 1000);
      return;
    }
    chordKey = null;

    if ((e.metaKey || e.ctrlKey) && e.key === '/') { e.preventDefault(); toggleShortcuts(); return; }
    if (e.key === 'Escape') {
      if (shortcutsOpen) { toggleShortcuts(); return; }
      if (activeFilter) { clearFilter(); return; }
      return;
    }
    if (shortcutsOpen) return;

    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); approveFocused(); return; }
    if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'r') { e.preventDefault(); refresh(); return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); focusCard(focusIdx + 1); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); focusCard(focusIdx - 1); return; }
    if (e.key === 'Enter' && focusIdx >= 0) { e.preventDefault(); openFocused(); return; }
  });

  (function wireModalBackdrop() {
    var m = shortcutsModal();
    if (m) m.addEventListener('click', function (e) { if (e.target === this) toggleShortcuts(); });
  })();

  // --- rendering ---

  function prCard(pr) {
    var titleRow = [
      Webkit.el('span', { class: 'pr-number' }, '#' + pr.number),
      Webkit.el('a', { class: 'pr-title', href: pr.detailURL }, pr.title),
    ];
    if (pr.draft) titleRow.push(Webkit.el('span', { class: 'pr-draft-badge' }, 'Draft'));
    if (pr.reviewState === 'approved') titleRow.push(Webkit.el('span', { class: 'pr-review-badge approved' }, 'Approved'));
    if (pr.reviewState === 'changes_requested') titleRow.push(Webkit.el('span', { class: 'pr-review-badge changes-requested' }, 'Changes Requested'));
    if (pr.reviewState === 'review_required') titleRow.push(Webkit.el('span', { class: 'pr-review-badge review-required' }, 'Review Required'));
    if (pr.ticketID) {
      titleRow.push(Webkit.el('span', {
        class: 'pr-ticket',
        onclick: function (e) { e.stopPropagation(); setFilter(pr.ticketID, 'ticket'); },
      }, pr.ticketID));
    }

    var meta = [
      Webkit.el('span', { class: 'pr-repo-tag' }, pr.owner + '/' + pr.repo),
      Webkit.el('span', {}, [
        Webkit.el('span', { class: 'pr-author' }, pr.author),
        ' opened ' + pr.age,
      ]),
    ];
    if (pr.additions || pr.deletions) {
      meta.push(Webkit.el('span', { class: 'pr-stats' }, [
        Webkit.el('span', { class: 'pr-stat-add' }, '+' + pr.additions),
        Webkit.el('span', { class: 'pr-stat-del' }, '-' + pr.deletions),
      ]));
    }
    meta.push(Webkit.el('span', {}, pr.headRef + ' → ' + pr.baseRef));
    if (pr.labels.length) {
      meta.push(Webkit.el('span', { class: 'pr-labels' }, pr.labels.map(function (l) {
        return Webkit.el('span', {
          class: 'pr-label',
          style: 'background: #' + l.color + '20; color: #' + l.color + '; border-color: #' + l.color + '40;',
        }, l.name);
      })));
    }

    return Webkit.el('wk-card', {
      class: 'pr-card',
      'data-ticket': pr.ticketID,
      'data-branch': pr.headRef,
      'data-review': pr.reviewState,
    }, [
      Webkit.el('div', { class: 'pr-row' }, [
        Webkit.el('div', { class: 'pr-info' }, [
          Webkit.el('div', { class: 'pr-title-row' }, titleRow),
          Webkit.el('div', { class: 'pr-meta' }, meta),
        ]),
        Webkit.el('div', { class: 'pr-card-actions' }, [
          Webkit.el('a', { class: 'btn btn-sm', href: pr.detailURL }, 'Diff'),
          Webkit.el('button', {
            class: 'btn btn-sm btn-approve',
            'data-host': pr.host, 'data-owner': pr.owner, 'data-repo': pr.repo, 'data-number': pr.number,
            onclick: function (e) { approve(e.currentTarget); },
          }, '✓'),
          Webkit.el('a', { class: 'btn btn-sm', href: pr.htmlURL, target: '_blank' }, 'GitHub'),
        ]),
      ]),
    ]);
  }

  function render(data) {
    var view = buildListView(data);

    var header = Webkit.el('div', { class: 'pr-header' }, [
      Webkit.el('h2', {}, 'Open Pull Requests'),
      Webkit.el('div', { class: 'pr-actions-bar' }, [
        Webkit.el('span', { class: 'pr-count' }, view.totalPRs + ' open'),
        Webkit.el('button', { class: 'btn btn-sm', onclick: refresh }, 'Refresh'),
        Webkit.el('button', { class: 'btn btn-sm', title: 'Keyboard shortcuts (⌘/)', onclick: toggleShortcuts }, '?'),
      ]),
    ]);

    var children = [header];

    if (view.totalPRs === 0) {
      children.push(Webkit.el('div', { class: 'empty' }, 'No open pull requests across configured repos.'));
    }

    var filterTags = [Webkit.el('span', { class: 'filter-label' }, 'Filter:')];
    filterTags.push(Webkit.el('span', {
      class: 'filter-tag review-state approved', 'data-filter': 'approved', 'data-kind': 'review',
      onclick: function (e) { toggleFilter(e.currentTarget); },
    }, 'Approved'));
    filterTags.push(Webkit.el('span', {
      class: 'filter-tag review-state review-required', 'data-filter': 'review_required', 'data-kind': 'review',
      onclick: function (e) { toggleFilter(e.currentTarget); },
    }, 'Review Required'));
    view.filters.forEach(function (f) {
      filterTags.push(Webkit.el('span', {
        class: 'filter-tag ' + f.kind, 'data-filter': f.key, 'data-kind': f.kind,
        onclick: function (e) { toggleFilter(e.currentTarget); },
      }, [f.key + ' ', Webkit.el('span', { class: 'filter-count' }, String(f.count))]));
    });
    filterTags.push(Webkit.el('span', {
      class: 'filter-clear', id: 'filter-clear', onclick: clearFilter, style: 'display:none',
    }, '✕ clear'));
    children.push(Webkit.el('div', { class: 'filter-bar' }, filterTags));

    children.push(Webkit.el('div', { class: 'pr-list' }, view.prs.map(prCard)));

    if (view.errors.length) {
      children.push(Webkit.el('div', { style: 'margin-top: 1.5rem;' }, view.errors.map(function (er) {
        return Webkit.el('wk-callout', { variant: 'error' }, [
          Webkit.el('a', { href: er.htmlURL, target: '_blank', style: 'color: inherit; font-weight: 500;' }, er.name),
          ': ' + er.error,
        ]);
      })));
    }

    children.push(Webkit.el('div', { class: 'generated' }, 'Updated ' + view.generated));

    app.innerHTML = '';
    children.forEach(function (c) { app.appendChild(c); });

    // Re-apply any active filter to the freshly built cards; focus resets.
    focusIdx = -1;
    applyFilter();
  }

  var handle = Webkit.poll('/api/prs', render, POLL_MS, function (err) {
    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'empty' },
      'Failed to load PRs: ' + ((err && err.message) || String(err))));
  });

  // tick() forces an immediate re-fetch after an action (approve/refresh)
  // instead of the old location.reload().
  function tick() {
    fetch('/api/prs', { headers: { accept: 'application/json' } })
      .then(function (r) { return r.json(); })
      .then(render)
      .catch(function () {});
  }
})();
