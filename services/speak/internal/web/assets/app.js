'use strict';
// speak — client-side render. The backend serves a static chrome-only shell
// (internal/web/assets/shell.html) plus the markdown renderer at POST /read,
// which returns {name, content, doc} JSON: `content` is the goldmark-rendered
// HTML split into <section class="doc-section"> blocks (compiled once in Go,
// mounted here as-is) and `doc` is the audio status of the parts serve
// synthesizes in the background. This script builds the page header + upload
// form, and on submit fetches /read and mounts the rendered sections, then
// (re)creates the <wk-read-aloud> element so it scans the freshly mounted
// sections. webkit.js loads before this file, so Webkit.el and the wk-* custom
// elements exist.
//
// Three input paths — file picker, drag-and-drop, paste — all funnel through
// submitMarkdown(name, text), which is also what the recent-docs chips replay.
// Recents live in localStorage only: the server keeps an audio cache on disk
// and recent docs in memory, never the markdown, and /read stays the single
// render path.
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
        'Each section gets its own play button; the passage being read is highlighted. ' +
        'Audio is prepared in the background and can be downloaded once ready.')
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

  // Engine health callout. The <wk-read-aloud> reachability probe hits GET /
  // on its endpoint — same-origin here, i.e. this very page — so play buttons
  // appear even when the TTS engine behind the proxy cannot speak. /enginez
  // reports the health a real test synthesis recorded, with the reason;
  // the callout says what is wrong and what to try.
  var engineCallout = Webkit.el('wk-callout', { variant: 'warn' }, '');
  engineCallout.style.display = 'none';

  // doc holds the rendered sections; errBox surfaces a failed read. Both live
  // for the page lifetime and get their contents swapped on each upload.
  var docName = Webkit.el('div', { class: 'doc-name' }, '');
  // Page-level audio line: parts ready, Prepare all, whole-page download.
  var audioBar = Webkit.el('div', { class: 'doc-audio' });
  audioBar.style.display = 'none';
  var doc = Webkit.el('div', { class: 'doc' }, [docName, audioBar]);
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
    fetch('/enginez', { cache: 'no-store' })
      .then(function (r) { return r.json(); })
      .then(renderEngine)
      .catch(function () {
        renderEngine({ status: 'down', kind: 'network', reason: 'speak itself is not responding' });
      });
  }

  // What to try next, by failure kind. Only the local engine has a known fix
  // command; remote providers point at speak doctor.
  function engineHint(state) {
    if (state.provider === 'local' && state.kind === 'network') {
      return ['Try ', Webkit.el('code', {}, 't-man restart speak-tts'), '.'];
    }
    if (state.kind === 'quota') return ['It usually clears on its own; try again shortly.'];
    return ['Run ', Webkit.el('code', {}, 'speak doctor'), ' for details.'];
  }

  function renderEngine(state) {
    if (!state || state.status === 'ok' || state.status === 'unknown') {
      engineCallout.style.display = 'none';
      return;
    }
    var what = state.status === 'degraded'
      ? 'Read-aloud is degraded, so some play clicks may fail. '
      : 'Read-aloud is unavailable, so the play buttons won’t work. ';
    var source = state.provider ? ' (' + state.provider + (state.model ? ', ' + state.model : '') + ')' : '';
    engineCallout.textContent = '';
    engineCallout.appendChild(Webkit.el('strong', {}, what));
    engineCallout.appendChild(document.createTextNode(
      'Reason' + source + ': ' + (state.reason || state.status) + '. '));
    engineHint(state).forEach(function (part) {
      engineCallout.appendChild(typeof part === 'string' ? document.createTextNode(part) : part);
    });
    engineCallout.style.display = '';
  }

  // A failed play already recorded its reason server-side; re-check so the
  // callout shows it next to the toast the component raised, and so the
  // section's audio badge shows a failed part.
  document.addEventListener('wk-read-aloud-error', function () {
    checkEngine();
    refreshDoc();
  });

  // ── Render + mount ──

  // mount swaps in the rendered sections and (re)creates <wk-read-aloud> so it
  // scans the new .doc-section targets — the element reads its targets once on
  // connect, so a fresh upload needs a fresh element (same pattern present uses).
  function mount(name, contentHtml) {
    errBox.style.display = 'none';
    errBox.textContent = '';
    hint.style.display = 'none';
    stopPolling();
    current = null;
    // A playing session holds the old sections, and removing <wk-read-aloud>
    // below doesn't end it: stop it, or it plays on from a detached section.
    Webkit.stopReadAloud();

    doc.innerHTML = '';
    docName.textContent = name || '';
    doc.appendChild(docName);
    audioBar.style.display = 'none';
    doc.appendChild(audioBar);
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

  // ── Audio preparation ──
  //
  // serve synthesizes an upload's parts in the background into its cache, so
  // play replays cached audio. The page shows how far that got, per section
  // and for the page, polling while parts are queued or generating.

  var POLL_MS = 2000;
  // The mounted doc: serve's id for it plus the markdown it came from. serve
  // keeps docs in memory only, so after a restart the page re-posts the
  // markdown; `reposted` marks a doc that already came from a re-post.
  var current = null;
  var pollTimer = null;

  function stopPolling() {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = null;
  }

  function schedulePoll() {
    stopPolling();
    var id = current.id;
    pollTimer = setTimeout(function () {
      pollTimer = null;
      docRequest('GET', '/doc/' + id)
        .then(renderAudio)
        .catch(function (err) {
          if (!current || current.id !== id) return;
          if (err.status === 404) { docGone(); return; }
          schedulePoll(); // serve may be restarting; the next poll tells
        });
    }, POLL_MS);
  }

  // One request about the mounted doc. The error carries the HTTP status and
  // serve's {"error": {"message"}} when it sent one.
  function docRequest(method, path) {
    return fetch(path, { method: method, cache: 'no-store', headers: { accept: 'application/json' } })
      .then(function (r) {
        if (r.ok) return r.json();
        return r.json().catch(function () { return null; }).then(function (body) {
          var err = new Error((body && body.error && body.error.message) || ('request failed (' + r.status + ')'));
          err.status = r.status;
          throw err;
        });
      });
  }

  // One status fetch outside the poll loop (after a failed play, say); starts
  // polling again when that shows work in progress.
  function refreshDoc() {
    if (!current || pollTimer) return;
    var id = current.id;
    docRequest('GET', '/doc/' + id)
      .then(renderAudio)
      .catch(function (err) {
        if (err.status === 404 && current && current.id === id) docGone();
      });
  }

  // serve no longer knows the doc (it restarted): post the markdown again so
  // the page's part keys resolve. Only once: a re-posted doc that goes
  // missing too gets a note instead of another round trip.
  function docGone() {
    stopPolling();
    var c = current;
    if (!c) return;
    if (c.reposted) {
      audioBar.textContent = 'Audio: speak no longer has this document; upload it again to prepare its audio.';
      audioBar.style.display = '';
      return;
    }
    submitMarkdown(c.name, c.text, c);
  }

  function trackDoc(status, name, text, reposted) {
    if (!status || !status.id) return; // an older serve without the audio cache
    current = { id: status.id, name: name, text: text, reposted: reposted };
    renderAudio(status);
  }

  function renderAudio(status) {
    if (!current || !status || status.id !== current.id) return; // a doc since replaced
    renderAudioBar(status);
    (status.sections || []).forEach(renderSectionAudio);
    var t = status.total || {};
    if (t.queued || t.generating) schedulePoll();
  }

  function renderAudioBar(status) {
    var t = status.total || {};
    audioBar.textContent = '';
    if (!t.parts) { audioBar.style.display = 'none'; return; }
    var line = 'Audio: ' + t.ready + ' of ' + t.parts + ' parts ready';
    if (t.queued || t.generating) line += ', ' + (t.queued + t.generating) + ' preparing';
    if (t.idle) line += ', ' + t.idle + ' not prepared';
    if (t.failed) line += ', ' + t.failed + ' failed';
    audioBar.appendChild(Webkit.el('span', {}, line));
    if (t.failed && status.reason) {
      audioBar.appendChild(Webkit.el('span', { class: 'doc-audio-reason' }, status.reason));
    }
    if (t.idle || t.failed) {
      var btn = Webkit.el('button', { type: 'button', class: 'audio-btn' }, 'Prepare all');
      btn.addEventListener('click', function () { prepareAll(btn); });
      audioBar.appendChild(btn);
    }
    if (t.ready === t.parts) {
      audioBar.appendChild(downloadLink('/doc/' + status.id + '/audio', 'Download page audio'));
    }
    audioBar.style.display = '';
  }

  function prepareAll(btn) {
    if (!current) return;
    var id = current.id;
    btn.disabled = true;
    docRequest('POST', '/doc/' + id + '/prepare')
      .then(renderAudio)
      .catch(function (err) {
        if (!current || current.id !== id) return;
        if (err.status === 404) { docGone(); return; }
        btn.disabled = false;
        showError('Prepare audio: ' + err.message);
      });
  }

  function downloadLink(href, label) {
    return Webkit.el('a', { class: 'audio-dl', href: href, download: '' }, label);
  }

  // A section's audio state, most useful fact first: work in progress, then
  // a failure, then what is left unprepared.
  function sectionState(sec) {
    if (sec.ready === sec.parts) return { variant: 'ok', text: 'audio ready' };
    if (sec.generating) {
      return { variant: 'info', text: 'generating ' + Math.min(sec.ready + 1, sec.parts) + ' of ' + sec.parts };
    }
    if (sec.queued) return { variant: 'info', text: 'queued, ' + sec.ready + ' of ' + sec.parts + ' ready' };
    if (sec.failed) return { variant: 'error', text: 'failed: ' + (sec.reason || 'unknown reason') };
    if (sec.ready) return { variant: 'outline', text: sec.ready + ' of ' + sec.parts + ' ready' };
    return { variant: 'outline', text: 'not prepared' };
  }

  // The status sits in its own floated div, outside the p/li blocks that
  // fixation walks and read-aloud reads, and its text is a wk-badge, which
  // the read-aloud text walk skips. Created at mount, before <wk-read-aloud>
  // finishes probing, so the play buttons it inserts later float to its right.
  function renderSectionAudio(sec) {
    var el = doc.querySelector('.doc-section[data-section="' + sec.section + '"]');
    if (!el) return;
    var box = el.querySelector(':scope > .section-audio');
    if (!sec.parts) { if (box) box.remove(); return; }
    if (!box) {
      box = Webkit.el('div', { class: 'section-audio' });
      el.insertBefore(box, el.firstChild);
    }
    var state = sectionState(sec);
    box.textContent = '';
    box.appendChild(Webkit.el('wk-badge', { variant: state.variant, title: state.text }, state.text));
    if (sec.ready === sec.parts) {
      box.appendChild(downloadLink('/doc/' + current.id + '/audio?section=' + sec.section, 'download'));
    }
  }

  // The one path to the server: raw markdown in, rendered sections mounted,
  // doc remembered. FormData keeps /read's multipart contract unchanged.
  // `reposted` is the tracked doc being re-posted after serve forgot it (see
  // docGone).
  function submitMarkdown(name, text, reposted) {
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
        if (reposted) {
          // Another upload replaced the doc while the re-post was in flight.
          if (current !== reposted) return;
          // Same doc id: the page is unchanged, so keep it (and any playing
          // session) and resume tracking.
          if (d.doc && d.doc.id === reposted.id) {
            trackDoc(d.doc, d.name, text, true);
            return;
          }
        }
        // Swap the doc in; after a re-post, keep the reader's place.
        var y = window.scrollY;
        mount(d.name, d.content);
        if (reposted) window.scrollTo(0, y);
        trackDoc(d.doc, d.name, text, !!reposted);
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
