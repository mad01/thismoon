'use strict';
// moondb page: client-side render. The backend serves a static chrome-only
// shell plus JSON under /api/; this script fetches it and builds the DOM with
// webkit's shared helpers. Two views, picked by the URL: the sessions list at
// "/" and one session's turns and edits at "/?session=<id>". The page is
// read-only: moondb records through hooks and serves what it recorded.
// webkit.js loads before this file, so Webkit.el is available.
(function () {
  var app = document.getElementById('app');
  var el = Webkit.el;

  // ── fetch helpers ──

  function getJSON(url) {
    return fetch(url).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (body) {
        if (!r.ok) throw new Error(body.error || ('load failed (' + r.status + ')'));
        return body;
      });
    });
  }

  function getText(url) {
    return fetch(url).then(function (r) {
      if (!r.ok) throw new Error('load failed (' + r.status + ')');
      return r.text();
    });
  }

  function chunkURL(sha) { return '/api/chunks/' + encodeURIComponent(sha); }

  // ── formatting ──

  var DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  // fmtTime renders an ISO timestamp in the browser's local zone, or '' for
  // an absent or zero time.
  function fmtTime(iso) {
    if (!iso) return '';
    var d = new Date(iso);
    if (isNaN(d) || d.getFullYear() <= 1) return '';
    return DAYS[d.getDay()] + ' ' + MONTHS[d.getMonth()] + ' ' + d.getDate() +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function firstLine(text, max) {
    var line = (text || '').split('\n')[0].trim();
    return line.length > max ? line.slice(0, max - 1) + '…' : line;
  }

  function plural(n, word) { return n + ' ' + word + (n === 1 ? '' : 's'); }

  function statusBadge(status) {
    var variant = { open: 'info', closed: 'ok', failed: 'error' }[status] || 'stat';
    return el('wk-badge', { variant: variant }, status || 'unknown');
  }

  function table(head, rows) {
    return el('wk-table', {}, el('table', {}, [
      el('thead', {}, el('tr', {}, head.map(function (h) { return el('th', {}, h); }))),
      el('tbody', {}, rows)
    ]));
  }

  // ── sessions list ──

  function sessionRow(s) {
    var label = s.title || s.cwd || s.id;
    return el('tr', {}, [
      el('td', { class: 'muted' }, fmtTime(s.started_at)),
      el('td', {}, el('a', { href: '?session=' + encodeURIComponent(s.id) }, label)),
      el('td', { class: 'mono' }, (s.repos || []).join(', ')),
      el('td', { class: 'num' }, String(s.turn_count)),
      el('td', { class: 'num' }, String(s.edit_count))
    ]);
  }

  function renderSessions(data) {
    var sessions = data.sessions || [];
    var nodes = [
      el('wk-page-header', {}, [
        el('wk-title', {}, 'Sessions'),
        el('wk-subtitle', {}, plural(sessions.length, 'session') + ', newest first')
      ]),
      el('p', { class: 'hint' }, [
        'Edits between commits, each linked to the agent turn that made it. Ask about one line with ',
        el('code', {}, 'moondb why'),
        ' or the ',
        el('code', {}, 'moondb_why'),
        ' MCP tool.'
      ])
    ];
    if (!sessions.length) {
      nodes.push(el('div', { class: 'empty' }, 'No sessions recorded yet.'));
    } else {
      nodes.push(table(['Started', 'Session', 'Repo', 'Turns', 'Edits'], sessions.map(sessionRow)));
    }
    replace(nodes);
  }

  // ── session view ──

  // chunkLink shows a chunk's bytes inline under the card on click; the href
  // still opens the raw bytes for a middle-click.
  function chunkLink(label, sha, card) {
    if (!sha) return el('span', { class: 'muted' }, '·');
    return el('a', {
      href: chunkURL(sha),
      onclick: function (ev) {
        ev.preventDefault();
        getText(chunkURL(sha)).then(function (text) {
          var old = card.querySelector('.chunk');
          if (old) old.remove();
          card.appendChild(el('pre', { class: 'chunk', title: label + ' ' + sha }, text));
        }).catch(function (err) { toast('Chunk: ' + err.message); });
      }
    }, label);
  }

  function editRow(e, card) {
    return el('tr', {}, [
      el('td', { class: 'mono' }, e.path),
      el('td', {}, e.kind),
      el('td', {}, e.tool),
      el('td', { class: 'muted' }, e.agent_type || e.agent_id || 'main'),
      el('td', {}, [
        chunkLink('before', e.before_chunk, card), ' ',
        chunkLink('after', e.after_chunk, card)
      ])
    ]);
  }

  // turnCard renders the turn from the session detail at once, then fills in
  // the prompt and the edits from their own requests.
  function turnCard(t) {
    var count = el('wk-badge', { variant: 'stat' }, '… edits');
    var prompt = el('div', { class: 't-prompt' }, t.prompt_chunk ? '…' : '(no prompt recorded)');
    var edits = el('div', { class: 't-edits' });
    var card = el('wk-card', { id: 'turn-' + t.id }, [
      el('div', { class: 't-head' }, [
        el('wk-badge', { variant: 'outline' }, '#' + t.seq),
        statusBadge(t.status),
        count,
        el('span', { class: 'muted' }, fmtTime(t.started_at))
      ]),
      prompt,
      edits
    ]);
    if (t.prompt_chunk) {
      getText(chunkURL(t.prompt_chunk))
        .then(function (text) { prompt.textContent = firstLine(text, 200) || '(empty prompt)'; })
        .catch(function () { prompt.textContent = '(prompt unavailable)'; });
    }
    getJSON('/api/turns/' + encodeURIComponent(t.id)).then(function (d) {
      var list = d.edits || [];
      count.textContent = plural(list.length, 'edit');
      if (list.length) {
        edits.appendChild(table(['Path', 'Kind', 'Tool', 'Agent', 'Content'],
          list.map(function (e) { return editRow(e, card); })));
      }
    }).catch(function (err) { count.textContent = 'edits unavailable: ' + err.message; });
    return card;
  }

  function kvRow(label, value) {
    return el('wk-kv-row', {}, [el('wk-kv-label', {}, label), el('wk-kv-value', {}, value)]);
  }

  function renderSession(d) {
    var turns = d.turns || [];
    var sub = [fmtTime(d.started_at), d.harness, d.model].filter(Boolean).join(' · ');
    replace([
      el('a', { class: 'back', href: '/' }, '← All sessions'),
      el('wk-page-header', {}, [
        el('wk-title', {}, d.title || d.cwd || d.id),
        el('wk-subtitle', {}, sub)
      ]),
      el('wk-kv', { variant: 'card' }, [
        kvRow('Session', el('code', {}, d.id)),
        kvRow('Cwd', el('code', {}, d.cwd || '')),
        kvRow('Worktrees', (d.worktrees || []).join(', ') || '-'),
        kvRow('Edits', String(d.edit_count))
      ]),
      turns.length
        ? el('div', { class: 'turns' }, turns.map(turnCard))
        : el('div', { class: 'empty' }, 'No turns recorded.')
    ]);
  }

  // ── plumbing ──

  function replace(nodes) {
    app.innerHTML = '';
    nodes.forEach(function (n) { app.appendChild(n); });
  }

  function toast(msg) {
    var host = document.getElementById('toast-host');
    if (host) host.appendChild(el('wk-toast', { variant: 'err' }, msg));
  }

  function renderError(err) {
    replace([el('div', { class: 'empty' }, 'Failed to load: ' + ((err && err.message) || String(err)))]);
  }

  var session = new URLSearchParams(location.search).get('session');
  if (session) {
    getJSON('/api/sessions/' + encodeURIComponent(session)).then(renderSession).catch(renderError);
  } else {
    var refresh = function () { return getJSON('/api/sessions').then(renderSessions).catch(renderError); };
    refresh();
    setInterval(refresh, 60000);
  }
})();
