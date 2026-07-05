'use strict';
// present index view — client-side render. The backend serves a static shell
// (chrome only) plus JSON at GET /api/pages; this script fetches the page list
// and builds the DOM. webkit.js loads before this file, so Webkit.el /
// Webkit.escapeHtml and the wk-* custom elements are available.
(function () {
  var app = document.getElementById('app');

  // pagerLink builds a Newer/Older control: a real anchor when the target page
  // exists, otherwise a hidden spacer that preserves the flex layout. Pager
  // links stay full-navigation anchors to /?page=&size= — the shell reloads and
  // this script re-reads the new query, matching the old server-rendered behavior.
  function pagerLink(label, has, targetPage, size) {
    if (has) {
      return Webkit.el('a', { class: 'wk-button', href: '/?page=' + targetPage + '&size=' + size }, label);
    }
    return Webkit.el('span', { class: 'wk-button pager-spacer' }, label);
  }

  function pageCard(p) {
    var title = p.title || '(untitled)';
    var sub = Webkit.el('div', { class: 'row-sub' }, [
      Webkit.el('span', { class: 'row-id' }, p.id),
      document.createTextNode(' · v' + p.version + ' · updated ' + (p.updated_at || ''))
    ]);
    var main = Webkit.el('div', { class: 'row-main' }, [
      Webkit.el('div', { class: 'row-title' }, title),
      sub
    ]);
    var del = Webkit.el('button', {
      class: 'row-delete', type: 'button', title: 'Delete',
      'aria-label': 'Delete ' + title, 'data-id': p.id, 'data-title': title
    }, '✕');
    return Webkit.el('a', { class: 'wk-card', href: '/p/' + p.id }, [main, del]);
  }

  function render(data) {
    var total = data.total || 0;
    var children = [];

    children.push(Webkit.el('wk-page-header', {}, [
      Webkit.el('wk-title', {}, 'Presentations'),
      Webkit.el('wk-subtitle', {}, total + ' page' + (total === 1 ? '' : 's'))
    ]));

    var pages = data.pages || [];
    if (!pages.length) {
      children.push(Webkit.el('div', { class: 'empty' }, 'No presentations yet.'));
    }

    var grid = Webkit.el('div', { class: 'page-grid' }, pages.map(pageCard));
    children.push(grid);

    if ((data.total_pages || 1) > 1) {
      children.push(Webkit.el('nav', { class: 'pager' }, [
        pagerLink('← Newer', data.has_prev, data.prev_page, data.size),
        Webkit.el('span', { class: 'pager-status' }, 'Page ' + data.page + ' of ' + data.total_pages),
        pagerLink('Older →', data.has_next, data.next_page, data.size)
      ]));
    }

    app.innerHTML = '';
    children.forEach(function (c) { app.appendChild(c); });
    wireDelete();
  }

  // wireDelete attaches the delete-confirm modal to the freshly rendered list.
  // Mirrors the old inline index script: open on .row-delete click, confirm →
  // DELETE /p/{id}, then re-fetch + re-render (instead of location.reload()).
  // Esc / backdrop / cancel close.
  function wireDelete() {
    var modal = document.getElementById('delete-modal');
    var titleEl = document.getElementById('delete-title');
    var pendingID = null;

    function open(id, title) {
      pendingID = id;
      titleEl.textContent = title;
      modal.removeAttribute('hidden');
    }
    function close() {
      pendingID = null;
      modal.setAttribute('hidden', '');
    }

    app.querySelectorAll('.row-delete').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        open(btn.dataset.id, btn.dataset.title);
      });
    });

    document.getElementById('delete-cancel').onclick = close;
    document.getElementById('delete-x').onclick = close;
    modal.onclick = function (e) { if (e.target === modal) close(); };
    document.onkeydown = function (e) {
      if (e.key === 'Escape' && !modal.hasAttribute('hidden')) close();
    };

    document.getElementById('delete-confirm').onclick = function () {
      if (!pendingID) return;
      var id = pendingID;
      fetch('/p/' + id, { method: 'DELETE' }).then(function (resp) {
        if (resp.ok || resp.status === 404) { close(); load(); return; }
        close();
        alert('Delete failed: ' + resp.status);
      }).catch(function (err) {
        close();
        alert('Delete failed: ' + err);
      });
    };
  }

  function load() {
    fetch('/api/pages' + location.search, { headers: { accept: 'application/json' } })
      .then(function (r) { if (!r.ok) throw new Error('load failed (' + r.status + ')'); return r.json(); })
      .then(render)
      .catch(function (err) {
        var msg = (err && err.message) || String(err);
        app.innerHTML = '';
        app.appendChild(Webkit.el('div', { class: 'empty' }, 'Failed to load presentations: ' + msg));
      });
  }

  load();
})();
