'use strict';
// present index view — client-side render. The backend serves a static shell
// (chrome only) plus JSON at GET /api/pages; this script fetches the page list
// and builds the DOM. webkit.js loads before this file, so Webkit.el /
// Webkit.escapeHtml and the wk-* custom elements are available. It also owns
// the markdown import: a header button or a file dropped on the page goes to
// POST /api/import and the browser moves to the new page.
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

    children.push(Webkit.el('div', { class: 'header-row' }, [
      Webkit.el('wk-page-header', {}, [
        Webkit.el('wk-title', {}, 'Presentations'),
        Webkit.el('wk-subtitle', {}, total + ' page' + (total === 1 ? '' : 's'))
      ]),
      Webkit.el('wk-button', {
        id: 'import-button', variant: 'ghost', role: 'button', tabindex: '0',
        title: 'Import a markdown file as a new page (or drop one anywhere)'
      }, 'Import markdown')
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
    wireImportButton();
  }

  // toast shows a short message bottom-right, reusing the page's
  // <wk-toast-host> or adding one; webkit.css styles both.
  function toast(text, variant) {
    var host = document.querySelector('wk-toast-host');
    if (!host) {
      host = document.createElement('wk-toast-host');
      document.body.appendChild(host);
    }
    var el = Webkit.el('wk-toast', { variant: variant || 'err', role: 'alert' }, text);
    host.appendChild(el);
    setTimeout(function () { el.remove(); }, 8000);
  }

  // importFile reads a markdown file in the browser, posts it to
  // POST /api/import, and opens the page it became. The server converts and
  // stores it; errors come back as {"error": "..."} and land in a toast.
  var importing = false;

  // setBusy marks an import in flight. The button is looked up each time
  // because a delete re-renders the list and replaces it.
  function setBusy(on) {
    importing = on;
    var btn = document.getElementById('import-button');
    if (!btn) return;
    if (on) btn.setAttribute('aria-busy', 'true'); else btn.removeAttribute('aria-busy');
  }

  // A page restored from the back/forward cache comes back as it was left,
  // busy flag included, when the user returns from the imported page.
  window.addEventListener('pageshow', function (e) { if (e.persisted) setBusy(false); });

  function importFile(file) {
    if (!file || importing) return;
    if (!/\.(md|markdown|txt)$/i.test(file.name)) {
      toast('Import failed: ' + file.name + ' is not a markdown file (.md, .markdown, .txt)');
      return;
    }
    setBusy(true);
    file.text().then(function (markdown) {
      return fetch('/api/import', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: file.name, markdown: markdown })
      });
    }).then(function (resp) {
      return resp.json().catch(function () { return {}; }).then(function (data) {
        if (!resp.ok) throw new Error(data.error || ('import failed (' + resp.status + ')'));
        // Same-origin by construction: the server's url may name the backend
        // host when a proxy sits in front, so navigate by id.
        location.assign('/p/' + encodeURIComponent(data.id));
      });
    }).catch(function (err) {
      setBusy(false);
      toast('Import failed: ' + ((err && err.message) || String(err)));
    });
  }

  // wireImportButton makes the freshly rendered header button open the file
  // picker (the hidden #import-file input in the shell). wk-button is not a
  // <button>, so Enter and Space are handled by hand.
  function wireImportButton() {
    var btn = document.getElementById('import-button');
    var input = document.getElementById('import-file');
    if (!btn || !input) return;
    btn.addEventListener('click', function () { input.click(); });
    btn.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); input.click(); }
    });
  }

  // wireImportDrop wires the shell's file input and page-wide drag-and-drop
  // once; render() may run many times but the shell elements never change.
  function wireImportDrop() {
    var input = document.getElementById('import-file');
    if (!input) return;
    input.addEventListener('change', function () {
      importFile(input.files && input.files[0]);
      input.value = '';
    });
    function hasFiles(e) {
      var types = e.dataTransfer && e.dataTransfer.types;
      return !!types && Array.prototype.indexOf.call(types, 'Files') !== -1;
    }
    document.addEventListener('dragover', function (e) {
      if (!hasFiles(e)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'copy';
      document.body.classList.add('drop-active');
    });
    document.addEventListener('dragleave', function (e) {
      if (e.relatedTarget === null) document.body.classList.remove('drop-active');
    });
    document.addEventListener('drop', function (e) {
      if (!hasFiles(e)) return;
      e.preventDefault();
      document.body.classList.remove('drop-active');
      var files = e.dataTransfer.files;
      if (files.length > 1) toast('Importing the first file only (' + files[0].name + ')', 'ok');
      importFile(files[0]);
    });
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

  wireImportDrop();
  load();
})();
