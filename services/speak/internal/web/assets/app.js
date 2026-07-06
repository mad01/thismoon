'use strict';
// speak — client-side render. The backend serves a static chrome-only shell
// (internal/web/assets/shell.html) plus the markdown renderer at POST /read,
// which returns {name, content} JSON: `content` is the goldmark-rendered HTML
// split into <section class="doc-section"> blocks (compiled once in Go, mounted
// here as-is). This script builds the page header + upload form, and on submit
// fetches /read and mounts the rendered sections, then (re)creates the
// <wk-read-aloud> element so it scans the freshly mounted sections. webkit.js
// loads before this file, so Webkit.el and the wk-* custom elements exist.
//
// Three input paths — file picker, drag-and-drop, paste — all funnel through
// submitMarkdown(name, text), which is also what the recent-docs chips replay.
// Recents live in localStorage only; the server stays storage-free and /read
// stays the single render path.
(function () {
  var app = document.getElementById('app');
  if (!app) return;

  var RECENT_KEY = 'speak-recent';
  var RECENT_MAX = 5;
  // Per-doc size cap so five recents never brush the localStorage quota.
  var RECENT_DOC_MAX = 256 * 1024;

  // pageHeader is static chrome; it never changes after first paint.
  function pageHeader() {
    return Webkit.el('wk-page-header', {}, [
      Webkit.el('wk-title', {}, 'speak'),
      Webkit.el('wk-subtitle', {}, 'Upload, drop, or paste markdown and listen to it. ' +
        'Each section gets its own play button; the sentence being read is highlighted.')
    ]);
  }

  var fileInput = Webkit.el('input', {
    type: 'file',
    name: 'doc',
    accept: '.md,.markdown,.txt,text/markdown,text/plain',
    required: 'true'
  });
  var form = Webkit.el('form', { class: 'upload-form' }, [
    fileInput,
    Webkit.el('button', { type: 'submit' }, 'Read it')
  ]);

  // Empty-state hint doubles as the drop-zone affordance; hidden once a doc
  // is mounted.
  var hint = Webkit.el('div', { class: 'drop-hint' },
    'Drop a markdown file anywhere, paste markdown (⌘V), or choose a file above. ' +
    'Space plays or pauses; j/k jump between sections.');

  // Recent docs replayed from localStorage; row hidden when there are none.
  var recentRow = Webkit.el('div', { class: 'recent-row' });
  recentRow.style.display = 'none';

  // Engine-down callout. The <wk-read-aloud> reachability probe hits GET / on
  // its endpoint — same-origin here, i.e. this very page — so play buttons
  // appear even when the TTS engine behind the proxy is dead and every play
  // click would fail silently. /enginez pings the actual upstream.
  var engineCallout = Webkit.el('wk-callout', { variant: 'warn' }, [
    'Read-aloud is unavailable — the TTS engine is not responding, so the play ' +
    'buttons won’t work. Try ',
    Webkit.el('code', {}, 't-man restart speak-tts'),
    '.'
  ]);
  engineCallout.style.display = 'none';

  // doc holds the rendered sections; errBox surfaces a failed read. Both live
  // for the page lifetime and get their contents swapped on each upload.
  var docName = Webkit.el('div', { class: 'doc-name' }, '');
  var doc = Webkit.el('div', { class: 'doc' }, [docName]);
  var errBox = Webkit.el('div', { class: 'upload-error' });
  errBox.style.display = 'none';

  var dropOverlay = Webkit.el('div', { class: 'drop-overlay' }, 'Drop markdown to read');

  app.appendChild(pageHeader());
  app.appendChild(form);
  app.appendChild(recentRow);
  app.appendChild(engineCallout);
  app.appendChild(errBox);
  app.appendChild(hint);
  app.appendChild(doc);
  document.body.appendChild(dropOverlay);

  // ── Recents (localStorage) ──

  function loadRecent() {
    try {
      var v = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
      return Array.isArray(v) ? v : [];
    } catch (e) { return []; }
  }

  function saveRecent(name, md) {
    if (!md || md.length > RECENT_DOC_MAX) return;
    var items = loadRecent().filter(function (it) { return it && it.name !== name; });
    items.unshift({ name: name, md: md, ts: Date.now() });
    try {
      localStorage.setItem(RECENT_KEY, JSON.stringify(items.slice(0, RECENT_MAX)));
    } catch (e) { /* quota or private mode — recents are best-effort */ }
    renderRecent();
  }

  function renderRecent() {
    var items = loadRecent();
    recentRow.innerHTML = '';
    if (!items.length) { recentRow.style.display = 'none'; return; }
    recentRow.style.display = '';
    recentRow.appendChild(Webkit.el('span', { class: 'recent-label' }, 'Recent:'));
    items.forEach(function (it) {
      var chip = Webkit.el('button', { type: 'button', class: 'recent-chip', title: it.name }, it.name);
      chip.addEventListener('click', function () { submitMarkdown(it.name, it.md); });
      recentRow.appendChild(chip);
    });
  }

  // ── Engine reachability ──

  function checkEngine() {
    fetch('/enginez').then(function (r) {
      engineCallout.style.display = r.ok ? 'none' : '';
    }).catch(function () { engineCallout.style.display = ''; });
  }

  // ── Render + mount ──

  // mount swaps in the rendered sections and (re)creates <wk-read-aloud> so it
  // scans the new .doc-section targets — the element reads its targets once on
  // connect, so a fresh upload needs a fresh element (same pattern present uses).
  function mount(name, contentHtml) {
    errBox.style.display = 'none';
    errBox.textContent = '';
    hint.style.display = 'none';

    doc.innerHTML = '';
    docName.textContent = name || '';
    doc.appendChild(docName);
    // content is our own goldmark output (raw HTML is escaped by goldmark with
    // unsafe off), mounted as-is — the same trust boundary the old server-side
    // {{CONTENT}} substitution had.
    var holder = document.createElement('div');
    holder.innerHTML = contentHtml || '';
    while (holder.firstChild) doc.appendChild(holder.firstChild);

    var oldRA = document.querySelector('wk-read-aloud');
    if (oldRA) oldRA.remove();
    // Same-origin endpoint (""): this page is served by the process that proxies
    // /v1/audio/speech to the TTS engine.
    var ra = document.createElement('wk-read-aloud');
    ra.setAttribute('targets', '.doc-section');
    ra.setAttribute('endpoint', '');
    document.body.appendChild(ra);
  }

  function showError(msg) {
    errBox.textContent = msg;
    errBox.style.display = '';
  }

  // The one path to the server: raw markdown in, rendered sections mounted,
  // doc remembered. FormData keeps /read's multipart contract unchanged.
  function submitMarkdown(name, text) {
    var body = new FormData();
    body.append('doc', new Blob([text], { type: 'text/markdown' }), name);
    fetch('/read', { method: 'POST', body: body, headers: { accept: 'application/json' } })
      .then(function (r) {
        if (!r.ok) {
          return r.text().then(function (t) {
            throw new Error(t.trim() || ('read failed (' + r.status + ')'));
          });
        }
        return r.json();
      })
      .then(function (d) {
        mount(d.name, d.content);
        saveRecent(d.name, text);
        checkEngine();
      })
      .catch(function (err) { showError((err && err.message) || String(err)); });
  }

  // Pasted text has no filename; borrow the first heading when there is one.
  function nameFromMarkdown(text) {
    var m = /^#{1,2}\s+(.+)$/m.exec(text);
    if (!m) return 'pasted.md';
    return m[1].trim().slice(0, 48) + '.md';
  }

  // ── Input paths ──

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    var file = fileInput.files && fileInput.files[0];
    if (!file) return;
    file.text()
      .then(function (text) { submitMarkdown(file.name, text); })
      .catch(function (err) { showError('read file: ' + err); });
  });

  // Page-wide drag-and-drop. dragenter/dragleave fire on every child, so track
  // depth instead of toggling on each event.
  var dragDepth = 0;
  window.addEventListener('dragenter', function (e) {
    e.preventDefault();
    dragDepth++;
    dropOverlay.classList.add('visible');
  });
  window.addEventListener('dragover', function (e) { e.preventDefault(); });
  window.addEventListener('dragleave', function () {
    if (--dragDepth <= 0) { dragDepth = 0; dropOverlay.classList.remove('visible'); }
  });
  window.addEventListener('drop', function (e) {
    e.preventDefault();
    dragDepth = 0;
    dropOverlay.classList.remove('visible');
    var file = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
    if (file) {
      file.text()
        .then(function (text) { submitMarkdown(file.name, text); })
        .catch(function (err) { showError('read file: ' + err); });
      return;
    }
    var text = e.dataTransfer && e.dataTransfer.getData('text/plain');
    if (text) submitMarkdown(nameFromMarkdown(text), text);
  });

  // Paste anywhere outside an input reads the clipboard as a doc.
  document.addEventListener('paste', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    var text = e.clipboardData && e.clipboardData.getData('text/plain');
    if (!text || !text.trim()) return;
    e.preventDefault();
    submitMarkdown(nameFromMarkdown(text), text);
  });

  // ── Keyboard shortcuts ──

  // The play/pause button is the first .wk-ra-btn <wk-read-aloud> injects into
  // a section (the restart button sits after it).
  function playBtnOf(section) {
    return section.querySelector('.wk-ra-btn');
  }

  document.addEventListener('keydown', function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return;

    if (e.key === ' ') {
      // Toggle whatever is playing (section or selection float button), else
      // start the first section. preventDefault stops the page-scroll default.
      var active = document.querySelector('.wk-ra-btn.active');
      var first = document.querySelector('.doc-section');
      var btn = active || (first && playBtnOf(first));
      if (!btn) return;
      e.preventDefault();
      btn.click();
      return;
    }

    if (e.key === 'j' || e.key === 'k') {
      var sections = Array.prototype.slice.call(document.querySelectorAll('.doc-section'));
      if (!sections.length) return;
      var cur = -1;
      for (var i = 0; i < sections.length; i++) {
        if (sections[i].querySelector('.wk-ra-btn.active')) { cur = i; break; }
      }
      var next = e.key === 'j'
        ? Math.min(cur + 1, sections.length - 1)
        : Math.max(cur - 1, 0);
      if (next === cur) return;
      var target = playBtnOf(sections[next]);
      if (!target) return;
      e.preventDefault();
      target.click(); // startSession ends any current session itself
      sections[next].scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  });

  renderRecent();
  checkEngine();
})();
