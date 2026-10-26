'use strict';
// moondb page: client-side render (ADR-0005). The backend serves a static
// chrome-only shell plus JSON under /api/; this script fetches it and builds
// the DOM with webkit's shared helpers. Four views, picked by the query
// string:
//
//   /                              sessions active in the window (last 7 days by default),
//                                  grouped by the day they started, newest first;
//                                  ?since= and ?repo= filter, ?limit= pages
//   /?session=<id>                 one session: its title, repos and status, its trace (a
//                                  timeline lane per turn), its agents with their cost,
//                                  its turns and the worktrees it touched
//   /?turn=<id>                    one turn: prompt, final message, agents, edits with
//                                  their diffs, unattributed deltas, snapshots, commit
//                                  links and the raw transcript
//   /?worktree=<abs>&path=<rel>    one file: the turn and agent behind every line, and
//                                  the path's recorded history
//
// The page is read-only: moondb records through hooks and serves what it
// recorded. webkit.js loads before this file, so Webkit.el is available.
(function () {
  var app = document.getElementById('app');
  var el = Webkit.el;
  var params = new URLSearchParams(location.search);

  // Caps that keep one huge record from freezing the page.
  var DIFF_ROWS = 400; // diff rows drawn before a "show all" button
  var TRANSCRIPT_CHARS = 200 * 1024; // transcript shown inline; the rest is a download
  var SESSION_EDITS = 5000; // edits loaded to summarize one session's turns
  var HISTORY_LIMIT = 200; // edits in a file's history timeline
  var PAGE = 50; // sessions per page of the listing, the API default
  var TRACE_LANES = 25; // turn lanes drawn in a session's trace before a "show all" button

  // Turn bands in the file view cycle through these webkit roles.
  var BANDS = ['--series-1', '--series-2', '--series-3', '--series-4',
    '--amber', '--purple', '--red', '--yellow'];

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

  function enc(s) { return encodeURIComponent(s); }

  // qs builds a query string from an object, leaving out empty values.
  function qs(obj) {
    var p = new URLSearchParams();
    Object.keys(obj).forEach(function (k) {
      if (obj[k] !== '' && obj[k] != null) p.set(k, obj[k]);
    });
    return p.toString();
  }

  function chunkURL(sha) { return '/api/chunks/' + enc(sha); }

  // A chunk's key is the hash of its bytes, so each one is fetched once.
  var chunks = {};
  function chunk(sha) {
    if (!chunks[sha]) chunks[sha] = getText(chunkURL(sha));
    return chunks[sha];
  }

  var turns = {};
  function loadTurn(id) {
    if (!turns[id]) turns[id] = getJSON('/api/turns/' + enc(id));
    return turns[id];
  }

  // turnPrompt resolves to a turn's prompt label and origin: the label
  // promptLabel gives its text, '' without one, and the origin the turn
  // records, else the one its text shows.
  function turnPrompt(t, max) {
    if (!t.prompt_chunk) return Promise.resolve({ label: '', origin: t.origin || 'none' });
    return chunk(t.prompt_chunk).then(
      function (text) { return { label: promptLabel(text, max), origin: t.origin || promptOrigin(text) }; },
      function () { return { label: '', origin: t.origin || '' }; });
  }

  // promptLine resolves to a turn's prompt label, '' without one.
  function promptLine(t, max) {
    return turnPrompt(t, max).then(function (p) { return p.label; });
  }

  // turnLabel resolves to "#<seq> <prompt label>" for a turn id.
  function turnLabel(id, max) {
    return loadTurn(id).then(function (t) {
      return promptLine(t, max).then(function (p) {
        return '#' + t.seq + ' ' + (p || '(no prompt recorded)');
      });
    }, function () { return 'turn ' + id; });
  }

  // fill sets node's text when the promise resolves, or fallback on failure.
  function fill(node, promise, fallback) {
    promise.then(function (text) { node.textContent = text || fallback || ''; },
      function () { node.textContent = fallback || ''; });
    return node;
  }

  // ── formatting ──

  var DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  function parseTime(iso) {
    if (!iso) return null;
    var d = new Date(iso);
    return isNaN(d) || d.getFullYear() <= 1 ? null : d;
  }

  // fmtTime renders an ISO timestamp in the browser's local zone, or '' for
  // an absent or zero time.
  function fmtTime(iso) {
    var d = parseTime(iso);
    if (!d) return '';
    return DAYS[d.getDay()] + ' ' + MONTHS[d.getMonth()] + ' ' + d.getDate() +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function fmtClock(iso) {
    var d = parseTime(iso);
    return d ? pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds()) : '';
  }

  // MARKUP mirrors trace.markup: an XML-like tag, which a prompt line
  // reads better without.
  var MARKUP = /<\/?[A-Za-z][\w:-]*(\s[^<>]*)?\/?>/g;

  // promptText mirrors trace.PromptLine: the first line of text that holds
  // words once its tags are gone, runs of space collapsed, cut to max.
  function promptText(text, max) {
    var lines = (text || '').split('\n');
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].replace(MARKUP, ' ').split(/\s+/).filter(Boolean).join(' ');
      if (line) return cut(line, max);
    }
    return '';
  }

  // cut mirrors trace.cutLine: text past max characters keeps max of them
  // and gains an ellipsis.
  function cut(text, max) {
    var chars = Array.from(text);
    return chars.length > max ? chars.slice(0, max).join('') + '…' : text;
  }

  // ── turn origins ──

  // RELAY_HEAD mirrors store.relayHead: the line before a message the
  // harness relays from another session, whose wrapper says who sent it.
  var RELAY_HEAD = 'Another Claude session sent a message:';

  // ORIGIN_HEADS mirrors store.PromptOrigin: the head the harness gives a
  // prompt it wrote itself, and the origin that head marks. A turn recorded
  // before moondb kept origins is classified here from its prompt.
  var ORIGIN_HEADS = [
    ['<task-notification>', 'task'],
    ['<teammate-message', 'teammate'],
    ['<agent-message', 'subagent'],
    ['<cross-session-message', 'subagent'],
    ['This session is being continued from a previous conversation', 'continuation']
  ];

  // ORIGIN_BADGES mark the turns a person did not start.
  var ORIGIN_BADGES = {
    task: ['b', 'task', 'A background task finished, and its notification started this turn'],
    teammate: ['c', 'teammate', 'A teammate\'s message started this turn'],
    subagent: ['c', 'subagent', 'A message from a subagent or another session started this turn'],
    continuation: ['stat', 'continued', 'The session continued after compaction with this turn']
  };

  // promptOrigin mirrors store.PromptOrigin: a relayed message is
  // classified by the wrapper after RELAY_HEAD, a teammate's when none
  // follows.
  function promptOrigin(text) {
    var head = (text || '').trim();
    if (!head) return 'none';
    if (head.indexOf(RELAY_HEAD) === 0) {
      var relayed = headOrigin(head.slice(RELAY_HEAD.length).trim());
      return relayed === 'person' ? 'teammate' : relayed;
    }
    return headOrigin(head);
  }

  function headOrigin(head) {
    for (var i = 0; i < ORIGIN_HEADS.length; i++) {
      if (head.indexOf(ORIGIN_HEADS[i][0]) === 0) return ORIGIN_HEADS[i][1];
    }
    return 'person';
  }

  // promptLabel mirrors trace.PromptLabel: a prompt the harness wrote is
  // named by what started the turn, a person's by its first line.
  function promptLabel(text, max) {
    var origin = promptOrigin(text);
    if (origin === 'task') {
      var sum = /<summary>([^<]*)<\/summary>/.exec(text);
      return cut(labelWith('background task finished', sum && sum[1]), max);
    }
    if (origin === 'teammate') {
      var tag = (/<teammate-message\b[^>]*>/.exec(text) || [''])[0];
      return cut(labelWith('teammate message' + fromWho(attr(tag, 'teammate_id')), attr(tag, 'summary')), max);
    }
    if (origin === 'subagent') {
      var who = /<(?:agent|cross-session)-message\b[^>]*\bfrom="([^"]*)"/.exec(text);
      return cut(('message from subagent ' + (who ? who[1] : '')).trim(), max);
    }
    if (origin === 'continuation') return 'continued after compaction';
    return promptText(text, max);
  }

  function labelWith(label, detail) {
    var d = (detail || '').split(/\s+/).filter(Boolean).join(' ');
    return d ? label + ': ' + d : label;
  }

  function fromWho(who) { return who ? ' from ' + who : ''; }

  function attr(tag, name) {
    var m = new RegExp('\\b' + name + '="([^"]*)"').exec(tag);
    return m ? m[1] : '';
  }

  function originBadge(origin) {
    var b = ORIGIN_BADGES[origin];
    return b ? badge(b[0], b[1], b[2]) : null;
  }

  function plural(n, word) { return n + ' ' + word + (n === 1 ? '' : 's'); }

  function short(sha) { return (sha || '').slice(0, 10); }

  function kb(n) { return n < 1024 ? n + ' bytes' : Math.round(n / 1024) + ' KB'; }

  // agentName labels the agent behind an edit: its type, or main for the
  // main conversation.
  function agentName(type, id) { return type || (id ? 'subagent' : 'main'); }

  // agentLabel labels one of the API's agent rows: main for the main
  // conversation, else the subagent's name or type.
  function agentLabel(r) { return r.id ? r.name || r.type || 'subagent' : 'main'; }

  function where(worktree) { return worktree || 'outside any repository'; }

  // fileHref links the file view of a path: by worktree when the file sits
  // in one, by its absolute path otherwise.
  function fileHref(loc, path) {
    return '?' + qs({ worktree: loc.worktree, path: path, repo: loc.repo });
  }

  // ── components ──

  function table(head, rows) {
    return el('wk-table', {}, el('table', {}, [
      el('thead', {}, el('tr', {}, head.map(function (h) { return el('th', {}, h); }))),
      el('tbody', {}, rows)
    ]));
  }

  function kvRow(label, value) {
    return el('wk-kv-row', {}, [el('wk-kv-label', {}, label), el('wk-kv-value', {}, value)]);
  }

  function badge(variant, text, title) {
    return el('wk-badge', { variant: variant, title: title }, text);
  }

  function statusBadge(status) {
    var variant = { open: 'info', closed: 'ok', failed: 'error' }[status] || 'stat';
    return badge(variant, status || 'unknown');
  }

  // kindBadge uses the non-semantic tag hues: a kind is not a severity.
  function kindBadge(kind) {
    return badge({ create: 'b', update: 'c', delete: 'a' }[kind] || 'stat', kind || 'unknown');
  }

  function code(text, title) { return el('code', { class: 'inline-code', title: title }, text); }

  // monoCell holds an identifier (a path, a repo, an agent id) in code, so
  // the fixation walk leaves it alone.
  function monoCell(text) { return el('td', { class: 'mono' }, el('code', {}, text)); }

  function note(text) { return el('p', { class: 'e-note' }, text); }

  function section(title, children) {
    return el('wk-section', {}, [el('wk-section-heading', {}, title)].concat(children));
  }

  function backLinks(links) {
    return el('div', {}, links.map(function (l) { return el('a', { class: 'back', href: l[1] }, l[0]); }));
  }

  // prose holds a prompt or a message: line breaks kept, fixation on.
  function prose(sha, missing) {
    var node = el('div', { class: 'prose', 'data-fixation': '' }, sha ? 'Loading…' : missing);
    if (sha) fill(node, chunk(sha), missing);
    return node;
  }

  function rawLinks(pairs) {
    return el('span', { class: 'e-links' }, pairs.filter(function (p) { return p[1]; }).map(function (p) {
      return el('a', { href: chunkURL(p[1]), target: '_blank', title: p[0] + ' chunk ' + p[1] }, p[0]);
    }));
  }

  // ── diffs ──

  // hunksFromPatch reads a structuredPatch chunk: a JSON array of jsdiff
  // hunks. null when the text is not one.
  function hunksFromPatch(text) {
    try {
      var hunks = JSON.parse(text);
      return Array.isArray(hunks) ? hunks : (hunks === null ? [] : null);
    } catch (e) {
      return null;
    }
  }

  // hunksFromUnified reads a unified diff text, as an unattributed delta
  // stores it, into the same hunk shape. The file header lines are skipped.
  function hunksFromUnified(text) {
    var hunks = [];
    var cur = null;
    var head = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;
    text.split('\n').forEach(function (l) {
      var m = head.exec(l);
      if (m) {
        cur = {
          oldStart: +m[1], oldLines: m[2] == null ? 1 : +m[2],
          newStart: +m[3], newLines: m[4] == null ? 1 : +m[4], lines: []
        };
        hunks.push(cur);
      } else if (cur && /^[ +\-\\]/.test(l)) {
        cur.lines.push(l);
      }
    });
    return hunks;
  }

  // hunksFromCreate shows a created file as one hunk of added lines: a
  // Write reports no hunks for a new file.
  function hunksFromCreate(after) {
    var lines = after.split('\n');
    if (lines[lines.length - 1] === '') lines.pop();
    if (!lines.length) return [];
    return [{
      oldStart: 0, oldLines: 0, newStart: 1, newLines: lines.length,
      lines: lines.map(function (l) { return '+' + l; })
    }];
  }

  // diffRows numbers every hunk line with its old and new line number.
  function diffRows(hunks) {
    var rows = [];
    hunks.forEach(function (h) {
      rows.push({
        kind: '@',
        text: '@@ -' + h.oldStart + ',' + h.oldLines + ' +' + h.newStart + ',' + h.newLines + ' @@'
      });
      var o = h.oldStart;
      var n = h.newStart;
      (h.lines || []).forEach(function (l) {
        var c = l.charAt(0);
        var text = l.slice(1).replace(/\r$/, '');
        if (c === '+') rows.push({ kind: '+', nw: n++, text: text });
        else if (c === '-') rows.push({ kind: '-', old: o++, text: text });
        else if (c === '\\') rows.push({ kind: '\\', text: l });
        else rows.push({ kind: ' ', old: o++, nw: n++, text: text });
      });
    });
    return rows;
  }

  function lnCell(n) {
    return el('td', { class: 'd-ln' }, el('code', {}, n == null ? '' : String(n)));
  }

  // diffRow draws one row. An added line with text gets the why
  // affordance when ctx says which file it belongs to.
  function diffRow(r, ctx) {
    if (r.kind === '@') {
      return el('tr', { class: 'd-hunk' }, el('td', { colspan: '4' }, el('code', {}, r.text)));
    }
    if (r.kind === '\\') {
      return el('tr', { class: 'd-mark' }, [lnCell(), lnCell(), el('td', { class: 'd-sign' }),
        el('td', {}, el('code', {}, r.text))]);
    }
    var tr = el('tr', { class: { '+': 'd-add', '-': 'd-del', ' ': 'd-ctx' }[r.kind] }, [
      lnCell(r.old), lnCell(r.nw),
      el('td', { class: 'd-sign' }, el('code', {}, r.kind)),
      el('td', { class: 'd-tx' }, el('code', {}, r.text))
    ]);
    if (r.kind === '+' && ctx && r.text.trim()) {
      tr.classList.add('d-why');
      tr.title = 'Why is this line here? Click to ask moondb.';
      tr.addEventListener('click', function () { toggleWhy(tr, ctx, r); });
    }
    return tr;
  }

  // renderDiff draws hunks as a unified diff with line numbers, the first
  // DIFF_ROWS rows at once and the rest behind a button.
  function renderDiff(hunks, ctx) {
    var rows = diffRows(hunks);
    var box = el('div', { class: 'diff' });
    var wrap = el('div', {}, box);
    function draw(limit) {
      box.replaceChildren(el('table', {}, el('tbody', {}, rows.slice(0, limit).map(function (r) {
        return diffRow(r, ctx);
      }))));
    }
    draw(DIFF_ROWS);
    if (rows.length > DIFF_ROWS) {
      var more = el('button', {
        class: 'btn btn-ghost diff-more', type: 'button',
        onclick: function () { draw(rows.length); more.remove(); }
      }, 'Show all ' + rows.length + ' lines');
      wrap.appendChild(more);
    }
    return wrap;
  }

  // editDiff resolves to the diff of one Edit: its hunks, or the whole
  // after-content for a create that reported none.
  function editDiff(e) {
    var ctx = { repo: e.repo, path: e.path };
    if (e.excluded) {
      return Promise.resolve(note('Content not captured: the path matches a capture.exclude glob.'));
    }
    if (!e.patch_chunk) return Promise.resolve(note('No patch recorded for this tool call.'));
    return chunk(e.patch_chunk).then(function (text) {
      var hunks = hunksFromPatch(text);
      if (hunks === null) return note('The recorded patch does not parse.');
      if (hunks.length) return renderDiff(hunks, ctx);
      if (e.kind !== 'create' || !e.after_chunk) return note('The tool reported no hunks.');
      return chunk(e.after_chunk).then(function (after) {
        var created = hunksFromCreate(after);
        return created.length ? renderDiff(created, ctx) : note('Created an empty file.');
      });
    }).catch(function (err) { return note('Diff unavailable: ' + err.message); });
  }

  // ── why ──

  function toggleWhy(tr, ctx, r) {
    var next = tr.nextElementSibling;
    if (next && next.classList.contains('d-whyrow')) {
      next.remove();
      return;
    }
    tr.after(el('tr', { class: 'd-whyrow' }, el('td', { colspan: '4' }, whyCard(ctx, r.nw, r.text))));
  }

  // whyCard asks /api/why about one added line by its text, without the
  // worktree: git blame would read today's line at that number, which may
  // be a different line by now. The answer is the newest recorded edit
  // that added the text, with its turn and the commit it landed in.
  function whyCard(ctx, line, text) {
    var card = el('div', {
      class: 'why-card',
      style: 'font-family:' + getComputedStyle(document.body).fontFamily
    }, el('span', { class: 'muted' }, 'Asking moondb why this line is here…'));
    var q = qs({ repo: ctx.repo, path: ctx.path, line: String(line || 1), text: text });
    getJSON('/api/why?' + q).then(function (res) {
      card.replaceChildren.apply(card, whyNodes(res));
    }).catch(function (err) { card.textContent = 'Why failed: ' + err.message; });
    return card;
  }

  var METHODS = {
    blame: ['ok', 'git blame', 'git blame named the commit, and its blob is the after-content of this edit'],
    content: ['info', 'content match', 'the line text matched a line a recorded edit added'],
    none: ['stat', 'no match', 'no recorded edit explains the line']
  };

  function whyNodes(res) {
    var m = METHODS[res.method] || ['stat', res.method, ''];
    var head = [badge(m[0], m[1], m[2])];
    if (res.commit) head.push(el('span', {}, ['committed in ', code(short(res.commit), res.commit)]));
    var nodes = [el('div', { class: 'row' }, head)];
    if (!res.edit) {
      nodes.push(el('div', { class: 'muted' }, 'No recorded edit added this text. ' +
        'Braces, bare returns and imports are too common to attribute.'));
      return nodes;
    }
    var e = res.edit;
    var turnLink = el('a', { href: '?turn=' + enc(e.turn_id) }, 'turn');
    if (res.turn) fill(turnLink, promptLine(res.turn, 90).then(function (p) {
      return '#' + res.turn.seq + ' ' + (p || '(no prompt recorded)');
    }), 'turn');
    nodes.push(el('div', {}, [e.tool + ' by ' + agentName(e.agent_type, e.agent_id) + ', ' +
      fmtTime(e.at) + ', in ', turnLink]));
    if (res.session) {
      nodes.push(el('div', { class: 'muted' }, ['session ', el('a', {
        href: '?session=' + enc(res.session.id)
      }, res.session.title || res.session.cwd || res.session.id)]));
    }
    if ((res.candidates || []).length > 1) {
      nodes.push(el('div', { class: 'muted' }, 'Other edits that added the same text:'));
      nodes.push(el('ol', {}, res.candidates.slice(1).map(function (c) {
        var link = el('a', { href: '?turn=' + enc(c.edit.turn_id) }, 'turn');
        fill(link, turnLabel(c.edit.turn_id, 70), 'turn');
        return el('li', {}, [c.score.toFixed(2) + ', ' + c.reason + ', ' + fmtTime(c.edit.at) + ', ', link]);
      })));
    }
    return nodes;
  }

  // ── sessions list ──

  // SINCE are the time windows the list offers, DEFAULT_SINCE the one it
  // opens on. 'any' sends no bound to the API.
  var SINCE = [['24h', '24 hours'], ['7d', '7 days'], ['30d', '30 days'], ['any', 'Any time']];
  var DEFAULT_SINCE = '7d';
  var SINCE_PHRASE = { '24h': 'active in the last 24 hours', '7d': 'active in the last 7 days',
    '30d': 'active in the last 30 days', 'any': 'since recording began' };

  // Repo badges take a series hue picked by the repo's name, so one repo
  // keeps its colour from row to row.
  var REPO_HUES = ['--series-1', '--series-2', '--series-3', '--series-4'];

  function renderSessions() {
    document.title = 'Sessions · moondb';
    var state = {
      repo: params.get('repo') || '',
      since: params.get('since') || DEFAULT_SINCE,
      limit: parseInt(params.get('limit'), 10) || PAGE
    };
    var controls = el('div', { class: 'ls-controls' });
    var strip = el('div', { class: 'ls-strip' });
    var list = el('div', {}, el('div', { class: 'empty' }, 'Loading…'));

    function sync() {
      var q = qs({
        repo: state.repo,
        since: state.since === DEFAULT_SINCE ? '' : state.since,
        limit: state.limit === PAGE ? '' : state.limit
      });
      history.replaceState(null, '', q ? '?' + q : '/');
    }
    function set(change) {
      Object.keys(change).forEach(function (k) { state[k] = change[k]; });
      state.limit = PAGE;
      sync();
      load();
    }
    function more() {
      state.limit += PAGE;
      sync();
      load();
    }
    function load() {
      var since = state.since === 'any' ? '' : state.since;
      getJSON('/api/sessions?' + qs({ repo: state.repo, since: since, limit: state.limit }))
        .then(function (data) {
          var sessions = data.sessions || [];
          var win = data.window || {};
          controls.replaceChildren(sinceControl(state, set), repoChips(data.repos || [], state, set));
          strip.replaceChildren.apply(strip, windowStrip(win, state));
          list.replaceChildren.apply(list, sessionDays(sessions, state,
            (win.sessions || 0) > sessions.length ? more : null));
        })
        .catch(function (err) { list.replaceChildren(el('div', { class: 'empty' }, 'Failed to load: ' + err.message)); });
    }

    replace([
      el('wk-page-header', {}, [el('wk-title', {}, 'Sessions')]),
      el('p', { class: 'hint' },
        'Each row is one Claude Code session: the repo it worked in, what it was about, and what it cost. ' +
        'The list holds the sessions active in the window, grouped by the day they started.'),
      controls,
      strip,
      list
    ]);
    load();
    setInterval(load, 60000);
  }

  // sinceControl is the time window switch. A window the URL names that
  // the switch lacks, such as since=12h, gets a button of its own.
  function sinceControl(state, set) {
    var options = SINCE.slice();
    if (!SINCE.some(function (o) { return o[0] === state.since; })) {
      options.push([state.since, 'since ' + state.since]);
    }
    return el('wk-seg', { 'aria-label': 'Time window' }, options.map(function (o) {
      return el('button', {
        type: 'button', class: o[0] === state.since ? 'active' : '',
        'aria-pressed': String(o[0] === state.since),
        onclick: function () { set({ since: o[0] }); }
      }, o[1]);
    }));
  }

  // repoChips offers every repo the window's sessions edited, most used
  // first, and one to clear the filter.
  function repoChips(repos, state, set) {
    var chips = [chip('All repos', null, !state.repo, function () { set({ repo: '' }); })];
    var listed = repos.some(function (r) { return r.repo === state.repo; });
    if (state.repo && !listed) repos = [{ repo: state.repo, name: repoName(state.repo), sessions: 0 }].concat(repos);
    repos.forEach(function (r) {
      chips.push(chip(r.name, r.sessions, r.repo === state.repo, function () { set({ repo: r.repo }); }, r.repo));
    });
    return el('div', { class: 'ls-chips', role: 'group', 'aria-label': 'Repository' }, chips);
  }

  function chip(label, n, active, onPick, title) {
    var node = el('wk-badge', {
      variant: 'filter', role: 'button', tabindex: '0', title: title || '',
      'aria-pressed': String(active)
    }, [label, n == null ? null : el('span', { class: 'ls-n' }, String(n))]);
    if (active) node.setAttribute('active', '');
    node.addEventListener('click', onPick);
    node.addEventListener('keydown', function (ev) {
      if (ev.key === 'Enter' || ev.key === ' ') {
        ev.preventDefault();
        onPick();
      }
    });
    return node;
  }

  // windowStrip is one line adding up every session the filters keep.
  function windowStrip(w, state) {
    var scope = SINCE_PHRASE[state.since] || 'active since ' + state.since;
    if (state.repo) scope += ', in ' + repoName(state.repo);
    function figure(n, word) {
      return el('span', { class: 'ls-fig' }, [el('b', {}, fmtCount(n)), plural(n || 0, word).replace(/^\d+ /, '')]);
    }
    return [
      figure(w.sessions, 'session'),
      figure(w.turns, 'turn'),
      figure(w.edits, 'edit'),
      figure(w.files, 'file'),
      el('span', { class: 'ls-fig', title: 'Time inside the sessions\' turns, less waiting for input' },
        [el('b', {}, fmtSpan(w.active_ms || 0)), 'active']),
      el('span', { class: 'ls-fig', title: costTitle(w) }, [el('b', {}, money(w.cost, w.unpriced)), 'cost']),
      w.cost_agents != null ? el('span', { class: 'ls-fig ls-sub', title: costTitle(w) },
        [el('b', {}, money(w.cost_agents)), 'of it by subagents']) : null,
      el('span', { class: 'ls-scope' }, scope)
    ];
  }

  // sessionDays lays one page of sessions out as a table with a heading
  // row per local day; more, when set, loads a longer page.
  function sessionDays(sessions, state, more) {
    if (!sessions.length) {
      var scope = SINCE_PHRASE[state.since] || 'active since ' + state.since;
      return [el('div', { class: 'empty' }, state.repo
        ? 'No sessions in ' + repoName(state.repo) + ' ' + scope + '.'
        : 'No sessions ' + scope + '. Older ones are under Any time.')];
    }
    var now = new Date();
    var head = el('thead', {}, el('tr', {}, [
      el('th', { class: 'c-repo' }, 'Repo'), el('th', {}, 'Session'),
      el('th', { class: 'c-when' }, 'Last active'),
      el('th', { class: 'num c-n c-sm' }, 'Turns'), el('th', { class: 'num c-n' }, 'Edits'),
      el('th', { class: 'num c-n c-sm' }, 'Files'), el('th', { class: 'num c-n c-sm' }, 'Agents'),
      el('th', { class: 'num c-cost' }, 'Cost')
    ]));
    var bodies = byDay(sessions).map(function (day) {
      return el('tbody', {}, [dayRow(day, now)].concat(day.sessions.map(function (s) {
        return sessionRow(s, now, state);
      })));
    });
    var nodes = [el('wk-table', { class: 'ls-table' }, el('table', {}, [head].concat(bodies)))];
    if (more) {
      nodes.push(el('button', { class: 'btn btn-ghost more', type: 'button', onclick: more },
        'Show ' + PAGE + ' more'));
    }
    return nodes;
  }

  // byDay groups sessions, newest first already, by the local day each
  // one started.
  function byDay(sessions) {
    var days = [];
    sessions.forEach(function (s) {
      var d = parseTime(s.started_at) || new Date(0);
      var key = d.getFullYear() + '-' + d.getMonth() + '-' + d.getDate();
      var last = days[days.length - 1];
      if (!last || last.key !== key) days.push(last = { key: key, date: d, sessions: [] });
      last.sessions.push(s);
    });
    return days;
  }

  // dayRow heads one day: its session count and their summed cost, with
  // the plus its rows show when any of them leaves a model unpriced. It
  // adds up the sessions on this page, so a page that cuts a day short
  // sums only the part of the day it holds.
  function dayRow(day, now) {
    var cost = 0;
    var priced = false;
    var unpriced = [];
    day.sessions.forEach(function (s) {
      if (s.cost != null) {
        cost += s.cost;
        priced = true;
      }
      (s.unpriced || []).forEach(function (m) {
        if (unpriced.indexOf(m) < 0) unpriced.push(m);
      });
    });
    var name = dayName(day.date, now);
    var date = DAYS[day.date.getDay()] + ' ' + MONTHS[day.date.getMonth()] + ' ' + day.date.getDate();
    if (day.date.getFullYear() !== now.getFullYear()) date += ', ' + day.date.getFullYear();
    return el('tr', { class: 'ls-day' }, el('td', { colspan: '8' }, el('div', { class: 'ls-day-in' }, [
      el('span', { class: 'ls-day-name' }, name || date),
      name ? el('span', { class: 'ls-day-date' }, date) : null,
      el('span', { class: 'ls-day-sum' },
        plural(day.sessions.length, 'session') + (priced ? ' · ' + money(cost, unpriced) : ''))
    ])));
  }

  // dayName is Today or Yesterday, '' for an older day.
  function dayName(d, now) {
    var start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    var diff = Math.round((start - new Date(d.getFullYear(), d.getMonth(), d.getDate())) / 864e5);
    return diff === 0 ? 'Today' : diff === 1 ? 'Yesterday' : '';
  }

  function sessionRow(s, now, state) {
    return el('tr', { class: 'ls-row' }, [
      el('td', { class: 'ls-repo' }, repoBadges(s, state)),
      el('td', { class: 'ls-title', 'data-fixation': '' }, el('a', {
        href: '?session=' + enc(s.id), title: s.title + (s.cwd ? '\n' + s.cwd : '')
      }, s.title || s.id)),
      el('td', { class: 'ls-when' }, whenCell(s, now)),
      count(s.turn_count, 'c-sm'), count(s.edit_count), count(s.file_count, 'c-sm'),
      agentCount(s),
      el('td', { class: 'num ls-cost', title: costTitle(s) }, money(s.cost, s.unpriced))
    ]);
  }

  // agentCount is the session's agent count, naming its subagents on hover.
  function agentCount(s) {
    var cell = count(s.agent_count, 'c-sm');
    if ((s.agents || []).length) cell.title = 'Subagents: ' + s.agents.join(', ');
    return cell;
  }

  function count(n, cls) {
    return el('td', { class: 'num ' + (cls || '') + (n ? '' : ' zero') }, fmtCount(n));
  }

  // fmtCount is a count with thousands separators: 2,099.
  function fmtCount(n) { return (n || 0).toLocaleString('en-US'); }

  // repoBadges is the session's first repo as a badge that filters the
  // list to it, with a count of the others.
  function repoBadges(s, state) {
    var repos = s.repos || [];
    if (!repos.length) return el('span', { class: 'ls-norepo' }, 'no repo');
    var names = s.repo_names || repos;
    var badge = repoBadge(repos[0], names[0], '?' + qs({
      repo: repos[0], since: state.since === DEFAULT_SINCE ? '' : state.since
    }));
    badge.title = 'Show only ' + repos[0] + (repos.length > 1 ? '\nAlso edited: ' + repos.slice(1).join(', ') : '');
    return el('span', { class: 'row ls-repos' }, [badge,
      repos.length > 1 ? el('span', { class: 'ls-more' }, '+' + (repos.length - 1)) : null]);
  }

  // repoBadge is a repo's short name on its hue; with href it is a link.
  function repoBadge(repo, name, href) {
    return el(href ? 'a' : 'span', {
      class: 'ls-badge', href: href, style: '--hue:var(' + repoHue(repo) + ')'
    }, [el('span', { class: 'ls-swatch' }), el('span', { class: 'ls-badge-name' }, name || repoName(repo))]);
  }

  function repoHue(repo) {
    var h = 0;
    for (var i = 0; i < repo.length; i++) h = (h * 31 + repo.charCodeAt(i)) >>> 0;
    return REPO_HUES[h % REPO_HUES.length];
  }

  // repoName is a repo identity's last two path segments, as the API's
  // repo_names are.
  function repoName(repo) {
    var parts = repo.replace(/^\/+|\/+$/g, '').split('/');
    return parts.length <= 2 ? repo : parts.slice(-2).join('/');
  }

  // whenCell is the status dot, how long ago the session was last active,
  // and its shown time. The days its wall span touched follow when that is
  // more than one, in a span of their own that wraps under the rest when
  // the column is too narrow for one line.
  function whenCell(s, now) {
    var running = s.status === 'running';
    var at = parseTime(s.last_activity_at);
    var shown = shownTime(s);
    var days = shown ? wallDays(s) : 1;
    return [
      el('span', { class: 'ls-when-main' }, [
        statusDot(running),
        el('span', {
          class: running ? 'ls-ago is-on' : 'ls-ago',
          title: (running ? 'Running, last active ' : 'Ended, last active ') + fmtTime(s.last_activity_at)
        }, at ? ago(at, now) : '-'),
        shown ? el('span', { class: 'ls-span', title: timeTitle(s) }, fmtSpan(shown)) : null
      ]),
      days > 1 ? ' ' : null,
      days > 1 ? el('span', { class: 'ls-days', title: timeTitle(s) }, 'over ' + days + ' days') : null
    ];
  }

  // shownTime is the time a session row shows: its active time, or its
  // wall span when it has no active time, zero when it has neither.
  function shownTime(s) {
    return s.active_ms > 0 ? s.active_ms : (s.duration_ms || 0);
  }

  // timeTitle explains a session's two times: active inside its turns,
  // wall from its first prompt to its last turn's end.
  function timeTitle(s) {
    return 'Active ' + fmtSpan(s.active_ms || 0) + ': time inside turns, less waiting for input\n' +
      'Wall ' + fmtSpan(s.duration_ms || 0) + ': first prompt to the last turn\'s end, pauses included';
  }

  // wallDays is how many local calendar days a session's wall span
  // touches. The span is duration_ms long and ends at ended_at, or at the
  // last activity while the session runs.
  function wallDays(s) {
    var end = parseTime(s.ended_at) || parseTime(s.last_activity_at);
    if (!end || !(s.duration_ms > 0)) return 1;
    var start = new Date(end.getTime() - s.duration_ms);
    var a = new Date(start.getFullYear(), start.getMonth(), start.getDate());
    var b = new Date(end.getFullYear(), end.getMonth(), end.getDate());
    return Math.round((b - a) / 864e5) + 1;
  }

  function statusDot(running) {
    return el('span', {
      class: 'ls-dot' + (running ? ' is-on' : ''), role: 'img',
      'aria-label': running ? 'running' : 'ended', title: running ? 'running' : 'ended'
    });
  }

  // ago is how long before now t was: just now, 12 min ago, 3 h ago, 4 d ago.
  function ago(t, now) {
    var min = Math.floor((now - t) / 60000);
    if (min < 1) return 'just now';
    if (min < 60) return min + ' min ago';
    if (min < 48 * 60) return Math.floor(min / 60) + ' h ago';
    return Math.floor(min / 1440) + ' d ago';
  }

  // fmtSpan is a duration for a list: <1m, 12m, 1h 05m, 2d 3h.
  function fmtSpan(ms) {
    var m = Math.floor(ms / 60000);
    if (m < 1) return '<1m';
    if (m < 60) return m + 'm';
    var h = Math.floor(m / 60);
    if (h < 24) return h + 'h ' + pad(m % 60) + 'm';
    return Math.floor(h / 24) + 'd ' + (h % 24) + 'h';
  }

  // money is USD for a list: two decimals, a dash when no price covers the
  // tokens, and a plus when some models are left out.
  function money(usd, unpriced) {
    if (usd === undefined || usd === null) return '-';
    var text = usd > 0 && usd < 0.01 ? '<$0.01' : '$' + (usd < 1000 ? usd.toFixed(2) : Math.round(usd));
    return text + ((unpriced || []).length ? '+' : '');
  }

  // costTitle says what a cost covers: list prices for the main
  // conversation and its subagents, each one's part, and the models it
  // leaves out.
  function costTitle(x) {
    var tokens = tokenTotal(x.tokens);
    if (x.cost == null && !tokens) return 'No token counts recorded yet';
    var parts = [fmtTokens(tokens) + ' tokens at list prices', agentCostText(x)];
    if ((x.unpriced || []).length) parts.push('No price for ' + x.unpriced.join(', '));
    return parts.join('. ');
  }

  // agentCostText splits a listing cost between the main conversation and
  // its subagents. A row that names subagents without their cost (no
  // priced model, or totals the backfill has not computed yet) says so,
  // rather than that none ran.
  function agentCostText(x) {
    var main = 'Main conversation ' + fmtCost(x.cost_main);
    if (x.cost_agents != null) return main + ', subagents ' + fmtCost(x.cost_agents);
    if ((x.agents || []).length) return main + ', subagents not priced yet';
    return main + '; no subagent cost recorded';
  }

  // ── session view ──

  // renderSession loads the session, its edits and its trace together. A
  // trace that fails to load costs the Trace section, never the page.
  function renderSession(id) {
    Promise.all([
      getJSON('/api/sessions/' + enc(id)),
      getJSON('/api/edits?' + qs({ session: id, limit: SESSION_EDITS })),
      getJSON('/api/sessions/' + enc(id) + '/trace').catch(function (err) { return { error: err }; })
    ]).then(function (r) { drawSession(r[0], r[1].edits || [], r[2]); }).catch(renderError);
  }

  // summarizeEdits counts a session's edits per turn and per worktree (with
  // its repo and branches).
  function summarizeEdits(edits) {
    var byTurn = {};
    var byWorktree = {};
    edits.forEach(function (e) {
      byTurn[e.turn_id] = (byTurn[e.turn_id] || 0) + 1;
      var key = e.worktree || '';
      var w = byWorktree[key] || (byWorktree[key] = { repo: e.repo || '', branches: [], count: 0 });
      w.count++;
      if (e.branch && w.branches.indexOf(e.branch) < 0) w.branches.push(e.branch);
    });
    return { byTurn: byTurn, byWorktree: byWorktree };
  }

  // agentsByTurn maps each turn id to the labels of the subagents that ran
  // under it, from the session's agent rows.
  function agentsByTurn(rows) {
    var byTurn = {};
    rows.forEach(function (r) {
      if (!r.id) return;
      (r.turn_ids || []).forEach(function (id) {
        var list = byTurn[id] || (byTurn[id] = []);
        if (list.indexOf(agentLabel(r)) < 0) list.push(agentLabel(r));
      });
    });
    return byTurn;
  }

  // drawSession leads with what the listing row says about the session,
  // then its trace, turns and worktrees; the ids and paths fold away last.
  function drawSession(d, edits, tr) {
    var s = d.summary || {};
    var label = s.title || d.title || d.cwd || d.id;
    document.title = label + ' · moondb';
    var sum = summarizeEdits(edits);
    var agents = d.agents || [];
    var ran = agentsByTurn(agents);
    var turnList = (d.turns || []).slice().sort(function (a, b) { return a.seq - b.seq; });
    var seqs = {};
    turnList.forEach(function (t) { seqs[t.id] = t.seq; });
    replace([
      backLinks([['← All sessions', '/']]),
      el('wk-page-header', {}, [el('wk-title', {}, label), sessionLine(s)]),
      section('Trace', traceNodes(tr)),
      section('Agents', agentsSection(agents, seqs)),
      section('Turns', [turnList.length
        ? table(['#', 'Prompt', 'Status', 'Edits', 'Agents', 'Started'], turnList.map(function (t) {
          return turnRow(t, sum.byTurn[t.id] || 0, ['main'].concat(ran[t.id] || []));
        }))
        : el('div', { class: 'empty' }, 'No turns recorded.')]),
      section('Worktrees', [worktreeTable(sum.byWorktree)]),
      el('wk-section', {}, el('details', {}, [
        el('summary', {}, 'Session details'),
        el('wk-kv', {}, [
          kvRow('Session', code(d.id)),
          kvRow('Cwd', code(d.cwd || '-')),
          kvRow('Harness', d.harness || '-'),
          kvRow('Model', d.model || '-'),
          kvRow('Transcript', d.transcript_path ? code(d.transcript_path) : '-'),
          kvRow('Started', fmtTime(d.started_at) || '-'),
          kvRow('Ended', fmtTime(d.ended_at) || 'no session end recorded')
        ])
      ]))
    ]);
  }

  // sessionLine sits under a session's title: the repos it edited, whether
  // it is running and when it was last active, its active time over its
  // wall span, and when it started.
  function sessionLine(s) {
    var running = s.status === 'running';
    var at = parseTime(s.last_activity_at);
    var names = s.repo_names || [];
    var badges = (s.repos || []).map(function (r, i) {
      var b = repoBadge(r, names[i], '/?' + qs({ repo: r }));
      b.title = 'Sessions in ' + r;
      return b;
    });
    return el('div', { class: 'ss-line' }, badges.concat([
      el('span', { class: running ? 'ss-status is-on' : 'ss-status' }, [statusDot(running),
        (running ? 'Running' : 'Ended') + (at ? ', last active ' + ago(at, new Date()) : '')]),
      shownTime(s) ? el('span', { title: timeTitle(s) }, activeLine(s)) : null,
      s.started_at ? el('span', {}, 'started ' + fmtTime(s.started_at)) : null
    ]));
  }

  // activeLine is a session's active time over its wall span, with the
  // days the span touched when that is more than one: active 1h 51m over
  // 23h 05m (2 days). A wall span that reads the same as the active time
  // is left out, and a session with no active time shows its wall span
  // alone.
  function activeLine(s) {
    var wall = fmtSpan(s.duration_ms || 0);
    var days = wallDays(s);
    var tail = days > 1 ? ' (' + days + ' days)' : '';
    if (!(s.active_ms > 0)) return 'wall ' + wall + tail;
    var active = fmtSpan(s.active_ms);
    if (wall === active && days < 2) return 'active ' + active;
    return 'active ' + active + ' over ' + wall + tail;
  }

  function worktreeTable(byWorktree) {
    var keys = Object.keys(byWorktree).sort();
    if (!keys.length) return el('div', { class: 'empty' }, 'No edits recorded.');
    return table(['Worktree', 'Repo', 'Branch', 'Edits'], keys.map(function (k) {
      var w = byWorktree[k];
      return el('tr', {}, [
        monoCell(where(k)),
        monoCell(w.repo || '-'),
        monoCell(w.branches.join(', ') || '-'),
        el('td', { class: 'num' }, String(w.count))
      ]);
    }));
  }

  // turnRow is one turn of the session: its prompt label behind a badge
  // when a person did not start it, its edits, and the agents that ran.
  function turnRow(t, editCount, agents) {
    var link = el('a', { href: '?turn=' + enc(t.id) }, t.prompt_chunk ? '…' : '(no prompt recorded)');
    var cell = el('td', { class: 'prompt-cell', 'data-fixation': '' }, link);
    if (t.prompt_chunk) {
      turnPrompt(t, 160).then(function (p) {
        link.textContent = p.label || '(prompt unavailable)';
        var b = originBadge(p.origin);
        if (b) cell.prepend(b, ' ');
      });
    }
    return el('tr', {}, [
      el('td', { class: 'num' }, String(t.seq)),
      cell,
      el('td', {}, statusBadge(t.status)),
      el('td', { class: 'num' }, String(editCount)),
      el('td', { class: 'mono', title: agents.join(', ') }, el('code', {}, shortList(agents, 3))),
      el('td', { class: 'muted nowrap' }, fmtTime(t.started_at))
    ]);
  }

  // shortList joins the first n items and counts the rest: "a, b, c +4".
  function shortList(items, n) {
    var more = items.length - n;
    return items.slice(0, n).join(', ') + (more > 0 ? ' +' + more : '');
  }

  // ── session agents ──

  // agentsSection lists the session's agents, the main conversation first
  // (the API sends it first), then the subagents in the order they
  // started, or costliest first once the Cost heading is clicked.
  function agentsSection(rows, seqs) {
    if (!rows.length) return [el('div', { class: 'empty' }, 'No agents recorded.')];
    var byCost = false;
    var box = el('div');
    function draw() {
      var subs = rows.slice(1);
      if (byCost) subs.sort(function (a, b) { return costRank(b) - costRank(a); });
      var sort = el('button', {
        class: 'linkish th-sort', type: 'button', title: byCost ? 'Sort by start' : 'Sort by cost, costliest first',
        onclick: function () { byCost = !byCost; draw(); }
      }, byCost ? 'Cost ▾' : 'Cost');
      box.replaceChildren(table(['Agent', 'Model', 'Turns', 'Time', 'Model calls', 'Tool calls',
        'Edits', 'Tokens', sort], [rows[0]].concat(subs).map(function (r) {
        return agentRow(r, seqs);
      })));
    }
    draw();
    var nodes = [box];
    if (rows.length === 1) nodes.unshift(note('No subagent ran in this session, or none stopped while recording.'));
    return nodes;
  }

  // costRank orders rows by cost, the ones without a priced call last.
  function costRank(r) { return r.cost == null ? -1 : r.cost; }

  function agentRow(r, seqs) {
    var who = [el('span', { class: 'ag-name' }, agentLabel(r))];
    if (!r.id) who.push(badge('outline', 'main conversation'));
    else if (r.name && r.type && r.name !== r.type) who.push(el('span', { class: 'muted' }, r.type));
    if (r.id && !r.recorded) who.push(badge('stat', 'edits only', 'No agent transcript was recorded for this agent, only its edits'));
    if (r.lines_cut) who.push(badge('warn', 'cut', plural(r.lines_cut, 'line') + ' cut by the size cap; their calls add no cost'));
    return el('tr', {}, [
      el('td', { title: [r.description, r.id].filter(Boolean).join('\n') }, el('div', { class: 'row ag-who' }, who)),
      monoCell(r.model || '-'),
      el('td', { class: 'ag-turns' }, r.id ? turnLinks(r.turn_ids || [], seqs) : 'every turn'),
      el('td', { class: 'num nowrap' }, r.duration_ms ? fmtDuration(r.duration_ms) : '-'),
      el('td', { class: 'num' }, r.recorded ? fmtCount(r.model_calls) : '-'),
      el('td', { class: 'num' }, r.recorded ? fmtCount(r.tool_calls) : '-'),
      el('td', { class: 'num' }, fmtCount(r.edit_count)),
      el('td', { class: 'num' }, r.recorded ? fmtTokens(tokenTotal(r.tokens)) : '-'),
      el('td', { class: 'num ag-cost', title: (r.unpriced || []).length ? 'No price for ' + r.unpriced.join(', ') : '' },
        r.recorded ? money(r.cost, r.unpriced) : '-')
    ]);
  }

  // turnLinks links the turns an agent ran under by number, the first few
  // and a count of the rest.
  function turnLinks(ids, seqs) {
    var shown = ids.slice(0, 4).map(function (id) {
      return el('a', { href: '?turn=' + enc(id) }, '#' + (seqs[id] || '?'));
    });
    var nodes = [];
    shown.forEach(function (a, i) { nodes.push(i ? ', ' : '', a); });
    if (ids.length > 4) nodes.push(el('span', { class: 'muted' }, ' +' + (ids.length - 4)));
    return nodes.length ? nodes : '-';
  }

  // ── session trace ──

  // Trace bars take the webkit series roles. A failed tool call is red and
  // hatched, so it reads without its hue; series-1 is left out because it
  // sits next to red in the default theme.
  var SPAN_COLORS = { model: '--series-2', tool: '--series-3', subagent: '--series-4' };
  var SPAN_ROWS = ['model', 'tool', 'subagent'];
  var SUBAGENT_ROWS = 4; // rows a lane gives its subagents before bars share one

  // fmtDuration renders milliseconds the way the CLI does: 850ms, 4.2s,
  // 3m12s, 1h02m.
  function fmtDuration(ms) {
    if (ms < 1000) return Math.round(ms) + 'ms';
    if (ms < 60000) return (ms / 1000).toFixed(1) + 's';
    var m = Math.floor(ms / 60000);
    if (m < 60) return m + 'm' + pad(Math.floor(ms / 1000) % 60) + 's';
    return Math.floor(m / 60) + 'h' + pad(m % 60) + 'm';
  }

  function fmtTokens(n) {
    if (n < 1000) return String(n);
    if (n < 1e6) return (n / 1000).toFixed(1) + 'k';
    return (n / 1e6).toFixed(1) + 'M';
  }

  // fmtCost renders USD with the precision its size needs; a cost no price
  // covers is unknown, never zero.
  function fmtCost(usd) {
    if (usd === undefined || usd === null) return 'unknown';
    if (usd === 0) return '$0';
    if (usd < 0.01) return '$' + usd.toFixed(4);
    if (usd < 1) return '$' + usd.toFixed(3);
    return '$' + usd.toFixed(2);
  }

  function tokenTotal(t) {
    if (!t) return 0;
    return (t.input || 0) + (t.cache_write_5m || 0) + (t.cache_write_1h || 0) +
      (t.cache_read || 0) + (t.output || 0);
  }

  // traceNodes is the Trace section: the totals strip, a legend and one
  // timeline lane per turn.
  function traceNodes(tr) {
    if (!tr || tr.error) {
      return [note('Trace unavailable' + (tr && tr.error ? ': ' + tr.error.message : '.'))];
    }
    var turns = (tr.turns || []).filter(function (t) { return (t.spans || []).length; });
    if (!turns.length) {
      return [el('div', { class: 'empty' },
        'No transcript lines recorded for these turns, so there is nothing to trace.')];
    }
    var lanes = el('div', { class: 'tr-lanes' });
    function draw(limit) {
      lanes.replaceChildren.apply(lanes, turns.slice(0, limit).map(traceLane));
    }
    draw(TRACE_LANES);
    var nodes = [traceStats(tr.totals || {}, (tr.turns || []).length), traceLegend(), lanes];
    if (turns.length > TRACE_LANES) {
      var more = el('button', {
        class: 'btn btn-ghost more', type: 'button',
        onclick: function () { draw(turns.length); more.remove(); }
      }, 'Show all ' + turns.length + ' traced turns');
      nodes.push(more);
    }
    return nodes;
  }

  function stat(value, label, sub) {
    return el('wk-stat', {}, [
      el('wk-stat-value', {}, value),
      el('wk-stat-label', {}, label),
      el('wk-stat-sub', {}, sub)
    ]);
  }

  // traceStats is the totals strip. Time and calls are the main
  // conversation's, a subagent's running beside it; tokens and cost add
  // the subagents', and the cost's sub-label splits it.
  function traceStats(t, turnCount) {
    var k = t.tokens || {};
    var unpriced = t.unpriced && t.unpriced.length;
    // A trace that billed no token has no cost, as its listing row has none.
    var costText = t.cost == null && !unpriced ? '-' : fmtCost(t.cost);
    return el('div', { class: 'tr-stats' }, [
      stat(fmtDuration(t.wall_ms || 0), 'Wall',
        plural(turnCount, 'turn') + ', ' + fmtDuration(t.user_ms || 0) + ' waiting for input'),
      stat(fmtDuration(t.model_ms || 0), 'Model', plural(t.model_calls || 0, 'main call') +
        (t.agent_model_calls ? ', ' + fmtCount(t.agent_model_calls) + ' by subagents' : '')),
      stat(fmtDuration(t.tool_ms || 0), 'Tool', plural(t.tool_calls || 0, 'main call') +
        ', ' + (t.tool_errors || 0) + ' failed' +
        (t.agent_tool_calls ? ', ' + fmtCount(t.agent_tool_calls) + ' by subagents' : '')),
      stat(fmtTokens(tokenTotal(k)), 'Tokens',
        fmtTokens(k.cache_read || 0) + ' cache read, ' + fmtTokens(k.output || 0) + ' output'),
      stat(costText, 'Cost', costSplit(t))
    ]);
  }

  // costSplit says what a trace's cost covers: the main conversation's
  // part and the subagents', and the models no price covers.
  function costSplit(t) {
    var text = t.cost_agents != null || t.tokens_agents
      ? 'main ' + fmtCost(t.cost_main) + ', subagents ' + fmtCost(t.cost_agents)
      : 'list prices, main conversation; no subagent cost recorded';
    if (t.unpriced && t.unpriced.length) text += '; unpriced: ' + t.unpriced.join(', ');
    return text;
  }

  function traceLegend() {
    function key(color, text, failed) {
      return el('span', { class: 'row', style: '--bar:var(' + color + ')' },
        [el('span', { class: 'swatch tr-swatch' + (failed ? ' tr-failed' : '') }), text]);
    }
    return el('div', { class: 'tr-legend' }, [
      key(SPAN_COLORS.model, 'model call'),
      key(SPAN_COLORS.tool, 'tool call'),
      key('--red', 'failed tool call', true),
      key(SPAN_COLORS.subagent, 'subagent run'),
      el('span', { class: 'muted' }, 'Each lane runs from its prompt to Stop; a subagent bar from its spawn ' +
        'to its stop, cut at the lane\'s end. Click a bar to open the turn.')
    ]);
  }

  // traceLane is one turn: its label, a track with a row per span kind, and
  // its time and cost. A turn without a start time has nothing to place
  // its bars against, so its track stays empty.
  function traceLane(tt) {
    var started = parseTime(tt.span.start);
    var wall = Math.max(tt.span.duration_ms, 1);
    var href = '?turn=' + enc(tt.turn_id);
    var spans = started ? tt.spans || [] : [];
    function ofKind(kind) { return spans.filter(function (s) { return s.kind === kind; }); }
    var rows = [ofKind('model'), ofKind('tool')].concat(packRows(ofKind('subagent'), SUBAGENT_ROWS))
      .map(function (row) {
        return el('div', { class: 'tr-row' }, row.map(function (s) {
          return traceBar(s, started.getTime(), wall, href);
        }));
      });
    var label = '#' + tt.seq + ' ' + (tt.prompt || '(no prompt recorded)');
    return el('div', { class: 'tr-lane' }, [
      el('a', { class: 'tr-label', href: href, title: label }, [
        el('span', { class: 'muted' }, '#' + tt.seq + ' '), tt.prompt || '(no prompt recorded)'
      ]),
      el('div', { class: 'tr-track' }, rows),
      el('span', { class: 'tr-meta', title: 'Cost: ' + costSplit(tt.totals) },
        fmtDuration(tt.totals.wall_ms) + ' · ' + fmtCost(tt.totals.cost))
    ]);
  }

  // packRows lays spans out in rows, each span in the first row whose last
  // span ended before it starts, so parallel subagents sit one above the
  // other. Past max rows a span joins the row that frees up first.
  function packRows(spans, max) {
    var rows = [];
    var ends = [];
    spans.slice().sort(function (a, b) { return startMS(a) - startMS(b); })
      .forEach(function (s) {
        var start = startMS(s);
        var i = 0;
        while (i < rows.length && ends[i] > start) i++;
        if (i === rows.length && rows.length >= max) i = ends.indexOf(Math.min.apply(null, ends));
        if (i === rows.length) rows.push([]);
        rows[i].push(s);
        ends[i] = Math.max(ends[i] || 0, start + s.duration_ms);
      });
    return rows;
  }

  // startMS is a span's start in milliseconds, 0 for one without a start.
  function startMS(s) {
    var at = parseTime(s.start);
    return at ? at.getTime() : 0;
  }

  // traceBar is one span placed on its turn's track, or null (which el
  // skips) for a span without a start time.
  function traceBar(s, start, wall, href) {
    var at = parseTime(s.start);
    if (!at) return null;
    var left = Math.min(Math.max((at.getTime() - start) / wall, 0), 1);
    var width = Math.min(s.duration_ms / wall, 1 - left);
    var failed = s.kind === 'tool' && s.is_error;
    var color = failed ? '--red' : SPAN_COLORS[s.kind];
    var title = spanTitle(s);
    if (s.kind === 'subagent' && (at.getTime() - start + s.duration_ms) > wall) {
      title += ' · ran on past the turn\'s Stop';
    }
    return el('a', {
      class: 'tr-bar' + (failed ? ' tr-failed' : ''), href: href, title: title,
      style: 'left:' + (left * 100).toFixed(3) + '%;width:' + (width * 100).toFixed(3) +
        '%;--bar:var(' + color + ')'
    });
  }

  // spanTitle is a bar's hover text: what ran, for how long, and for a model
  // call its tokens and cost.
  function spanTitle(s) {
    var parts = [s.name || s.kind, fmtDuration(s.duration_ms)];
    if (s.kind === 'model') {
      var k = s.tokens || {};
      parts.push(fmtTokens(tokenTotal(k)) + ' tokens (input ' + fmtTokens(k.input || 0) +
        ', cache write ' + fmtTokens((k.cache_write_5m || 0) + (k.cache_write_1h || 0)) +
        ', cache read ' + fmtTokens(k.cache_read || 0) + ', output ' + fmtTokens(k.output || 0) + ')');
      parts.push(fmtCost(s.cost) + (s.speed ? ' at ' + s.speed + ' speed' : ''));
    }
    if (s.kind === 'tool') {
      if (s.is_error) parts.push('failed');
      if (s.unfinished) parts.push('no result recorded');
      if (s.edit_ids && s.edit_ids.length) parts.push(plural(s.edit_ids.length, 'edit'));
    }
    if (s.kind === 'subagent' && s.tokens) {
      parts.push(plural(s.model_calls || 0, 'model call'), plural(s.tool_calls || 0, 'tool call'),
        fmtTokens(tokenTotal(s.tokens)) + ' tokens', fmtCost(s.cost));
    }
    if (s.kind === 'subagent') {
      parts.push(plural(s.edit_count || 0, 'edit') + (s.tokens ? '' : ', no agent transcript recorded'));
    }
    return parts.join(' · ');
  }

  // ── turn view ──

  function renderTurn(id) {
    loadTurn(id).then(function (t) {
      return getJSON('/api/sessions/' + enc(t.session_id))
        .catch(function () { return null; })
        .then(function (s) { drawTurn(t, s); });
    }).catch(renderError);
  }

  function drawTurn(t, s) {
    var edits = (t.edits || []).slice().sort(function (a, b) { return a.at < b.at ? -1 : a.at > b.at ? 1 : 0; });
    var deltas = t.deltas || [];
    var sessionLabel = s ? (s.title || s.cwd || s.id) : t.session_id;
    document.title = 'Turn #' + t.seq + ' · ' + sessionLabel + ' · moondb';
    var span = fmtTime(t.started_at) + (t.ended_at ? ' to ' + fmtClock(t.ended_at) : '');
    var skipped = skippedFilesText(t.skipped || []);
    replace([
      backLinks([['← Session', '?session=' + enc(t.session_id)], ['All sessions', '/']]),
      el('wk-page-header', {}, [
        el('wk-title', {}, 'Turn #' + t.seq),
        el('wk-subtitle', {}, [sessionLabel, span].filter(Boolean).join(' · '))
      ]),
      el('wk-kv', { variant: 'card' }, [
        kvRow('Turn', code(t.id)),
        kvRow('Session', el('a', { href: '?session=' + enc(t.session_id) }, sessionLabel)),
        kvRow('Status', statusBadge(t.status)),
        kvRow('Started', fmtTime(t.started_at) || '-'),
        kvRow('Ended', fmtTime(t.ended_at) || (t.status === 'open' ? 'still open' : '-')),
        kvRow('Edits', plural(edits.length, 'edit') + (deltas.length
          ? ', ' + plural(deltas.length, 'unattributed delta') : '')),
        t.trace_totals ? kvRow('Cost', fmtCost(t.trace_totals.cost) + ' · ' + costSplit(t.trace_totals)) : null,
        skipped ? kvRow('Skipped', skipped) : null,
        originRow(t)
      ].filter(Boolean)),
      el('div', { class: 'spaced' }, section('Prompt', [prose(t.prompt_chunk, '(no prompt recorded)')])),
      section('Final message', [prose(t.final_chunk,
        t.status === 'open' ? 'The turn is still open.' : '(no final message recorded)')]),
      section('Agents', [agentTable(t.agents || [])]),
      section('Edits', editGroups(edits)),
      section('Unattributed deltas', deltas.length
        ? [note('Changes no recorded tool call explains: a formatter hook, a human, or another process wrote the file.')]
          .concat(deltas.map(deltaPanel))
        : [el('div', { class: 'empty' }, 'None. Every change in this turn came from a recorded tool call.')]),
      section('Snapshots', [snapshotTable(t.snapshots || [])]),
      section('Commit links', [commitTable(t.commit_links || [], edits)]),
      transcriptSection(t)
    ]);
  }

  // skippedFilesText sums the turn's skipped files, the ones its Bash
  // commands changed that capture counted instead of recording as edits:
  // clean after the command (a pull, checkout or reset brought them back
  // to their committed state), past the hook's budget, or with a status
  // git could not give. '' when none were skipped. Named apart from the
  // snapshot table's skippedText, which a second declaration would
  // replace in this scope.
  function skippedFilesText(list) {
    var clean = 0;
    var timeout = 0;
    var failed = 0;
    list.forEach(function (s) {
      clean += s.clean || 0;
      timeout += s.timeout || 0;
      failed += s.failed || 0;
    });
    var parts = [];
    if (clean) parts.push(plural(clean, 'file') + ' clean after the command');
    if (timeout) parts.push(plural(timeout, 'file') + ' past the budget');
    if (failed) parts.push(plural(failed, 'file') + ' whose status failed');
    return parts.join(', ');
  }

  // originRow says who started the turn when a person did not: filled
  // once the prompt loads, for a turn recorded before moondb kept origins.
  function originRow(t) {
    var value = el('span', {}, 'a person');
    var row = kvRow('Started by', value);
    turnPrompt(t, 160).then(function (p) {
      var b = ORIGIN_BADGES[p.origin];
      if (b) value.replaceChildren(badge(b[0], b[1]), ' ' + b[2].toLowerCase());
      else if (p.origin !== 'person') value.textContent = 'no prompt recorded';
    });
    return row;
  }

  // agentTable lists the agents that worked in the turn: the main
  // conversation first, then every subagent with its work under the turn,
  // with or without edits.
  function agentTable(rows) {
    if (!rows.length) return el('div', { class: 'empty' }, 'No agents recorded.');
    return table(['Agent', 'Kind', 'Model calls', 'Tool calls', 'Time', 'Edits', 'Cost', 'Agent id'],
      rows.map(function (r) {
        return el('tr', {}, [
          el('td', { title: r.description || '' }, agentLabel(r) + (r.name && r.type && r.name !== r.type
            ? ' (' + r.type + ')' : '')),
          el('td', {}, badge(r.id ? 'c' : 'outline', r.id ? 'subagent' : 'main conversation')),
          el('td', { class: 'num' }, r.recorded ? fmtCount(r.model_calls) : '-'),
          el('td', { class: 'num' }, r.recorded ? fmtCount(r.tool_calls) : '-'),
          el('td', { class: 'num nowrap' }, r.duration_ms ? fmtDuration(r.duration_ms) : '-'),
          el('td', { class: 'num' }, fmtCount(r.edit_count)),
          el('td', { class: 'num', title: r.recorded ? '' : 'No agent transcript was recorded for this agent' },
            r.recorded ? fmtCost(r.cost) : '-'),
          monoCell(r.id || '-')
        ]);
      }));
  }

  // editGroups lays the turn's edits out by worktree, then path, each path
  // in the order its edits happened.
  function editGroups(edits) {
    if (!edits.length) return [el('div', { class: 'empty' }, 'No edits recorded in this turn.')];
    var worktrees = [];
    var groups = {};
    edits.forEach(function (e) {
      var wk = e.worktree || '';
      if (!groups[wk]) {
        groups[wk] = { first: e, paths: [], byPath: {} };
        worktrees.push(wk);
      }
      var g = groups[wk];
      if (!g.byPath[e.path]) {
        g.byPath[e.path] = [];
        g.paths.push(e.path);
      }
      g.byPath[e.path].push(e);
    });
    var nodes = [];
    worktrees.forEach(function (wk) {
      var g = groups[wk];
      nodes.push(el('div', { class: 'wt-head' }, [
        el('span', { class: 'muted' }, 'Worktree'),
        el('code', {}, where(wk)),
        g.first.repo ? badge('outline', g.first.repo) : null,
        g.first.branch ? badge('stat', 'branch ' + g.first.branch) : null
      ]));
      g.paths.forEach(function (p) {
        var list = g.byPath[p];
        nodes.push(el('div', { class: 'path-head' }, [
          el('a', { href: fileHref(list[0], p), title: 'Every line of this file, with the turn behind it' }, p),
          badge('stat', plural(list.length, 'edit'))
        ]));
        list.forEach(function (e) { nodes.push(editPanel(e)); });
      });
    });
    return nodes;
  }

  function observedBadge(e) {
    if (e.observed_matches === true) {
      return badge('ok', 'observed matches', 'The file on disk equals the content derived from the tool call.');
    }
    if (e.observed_matches === false) {
      return badge('warn', 'observed differs', 'Something else changed the file too; see the unattributed deltas.');
    }
    return badge('stat', 'observed unknown', 'One side of the comparison is not known.');
  }

  function editPanel(e) {
    var body = el('div', {}, note('Loading diff…'));
    editDiff(e).then(function (node) { body.replaceChildren(node); });
    return el('wk-panel', { id: 'edit-' + e.id }, [
      el('div', { class: 'e-head' }, [
        badge('outline', e.tool),
        kindBadge(e.kind),
        el('span', { title: e.agent_id || 'main conversation' }, 'by ' + agentName(e.agent_type, e.agent_id)),
        el('span', { class: 'muted' }, fmtClock(e.at)),
        badge('stat', 'before: ' + e.before_source, 'Where the before-content came from'),
        e.excluded ? badge('warn', 'excluded') : observedBadge(e),
        rawLinks([['before', e.before_chunk], ['after', e.after_chunk], ['observed', e.observed_chunk]])
      ]),
      body
    ]);
  }

  function deltaPanel(d) {
    var body = el('div', {}, note(d.patch_chunk ? 'Loading diff…' : 'No diff recorded.'));
    if (d.patch_chunk) {
      chunk(d.patch_chunk).then(function (text) {
        var hunks = hunksFromUnified(text);
        body.replaceChildren(hunks.length ? renderDiff(hunks, { repo: d.repo, path: d.path })
          : el('pre', { class: 'raw' }, el('code', {}, text)));
      }).catch(function (err) { body.replaceChildren(note('Diff unavailable: ' + err.message)); });
    }
    return el('wk-panel', {}, [
      el('div', { class: 'e-head' }, [
        el('a', { class: 'mono', href: fileHref(d, d.path) }, d.path),
        badge('warn', d.cause),
        d.ambiguous ? badge('error', 'ambiguous',
          'Another session had an open turn in this worktree, so the change may be theirs.') : null,
        el('span', { class: 'muted' }, fmtClock(d.at)),
        rawLinks([['from', d.from_chunk], ['to', d.to_chunk]])
      ]),
      body
    ]);
  }

  // snapshotTable lists the worktree snapshots taken at the turn's edges:
  // the dirty and touched files' content, read when the prompt arrived and
  // when the turn stopped.
  function snapshotTable(snaps) {
    if (!snaps.length) {
      return el('div', { class: 'empty' }, 'No snapshot: the worktrees were clean, or unchanged since the last one.');
    }
    var rows = snaps.slice().sort(function (a, b) { return a.at < b.at ? -1 : a.at > b.at ? 1 : 0; });
    return table(['Taken', 'Phase', 'Worktree', 'Files', 'Skipped', 'Map'], rows.map(function (s) {
      return el('tr', {}, [
        el('td', { class: 'muted nowrap' }, fmtClock(s.at)),
        el('td', {}, badge('outline', s.phase === 'pre' ? 'at prompt' : 'at stop')),
        monoCell(where(s.worktree)),
        el('td', { class: 'num' }, String(s.file_count)),
        el('td', {}, skippedText(s)),
        el('td', {}, rawLinks([['files', s.map_chunk]]))
      ]);
    }));
  }

  function skippedText(s) {
    var k = s.skipped || {};
    var parts = [['timeout', k.timeout], ['over the file cap', k.too_many],
      ['too large', k.too_large], ['excluded', k.excluded]]
      .filter(function (p) { return p[1]; })
      .map(function (p) { return p[1] + ' ' + p[0]; });
    var text = parts.join(', ') || '-';
    return s.truncated ? badge('warn', text, 'The snapshot lacks paths its scope held.') : text;
  }

  function commitTable(links, edits) {
    if (!links.length) {
      return el('div', { class: 'empty' }, 'No commit has taken this turn\'s content yet.');
    }
    var byID = {};
    edits.forEach(function (e) { byID[e.id] = e; });
    return table(['Commit', 'Path', 'Branch', 'Edit', 'Linked'], links.map(function (l) {
      var e = byID[l.edit_id];
      return el('tr', {}, [
        el('td', {}, code(short(l.commit), l.commit)),
        el('td', { class: 'mono' }, el('a', { href: fileHref(l, l.path) }, el('code', {}, l.path))),
        monoCell(l.branch || '-'),
        el('td', {}, e ? el('a', { href: '#edit-' + e.id }, e.tool + ' at ' + fmtClock(e.at)) : code(l.edit_id)),
        el('td', { class: 'muted nowrap' }, fmtTime(l.at))
      ]);
    }));
  }

  // transcriptSection shows the turn's raw transcript lines behind a closed
  // disclosure, fetched on first open and cut at TRANSCRIPT_CHARS.
  function transcriptSection(t) {
    if (!t.transcript_chunk) {
      return section('Raw transcript', [el('div', { class: 'empty' },
        t.status === 'open' ? 'Recorded when the turn closes.' : 'No transcript lines recorded.')]);
    }
    var body = el('div', {}, note('Loading…'));
    var details = el('details', {}, [el('summary', {}, 'Transcript lines written during the turn'), body]);
    var loaded = false;
    details.addEventListener('toggle', function () {
      if (!details.open || loaded) return;
      loaded = true;
      chunk(t.transcript_chunk).then(function (text) {
        var nodes = [];
        if (text.length > TRANSCRIPT_CHARS) {
          nodes.push(note('Showing the first ' + kb(TRANSCRIPT_CHARS) + ' of ' + kb(text.length) + '.'));
        }
        nodes.push(el('pre', { class: 'raw' }, el('code', {}, text.slice(0, TRANSCRIPT_CHARS))));
        nodes.push(el('p', { class: 'e-note' }, el('a', {
          href: chunkURL(t.transcript_chunk), download: 'turn-' + t.id + '.jsonl'
        }, 'Download the full transcript chunk (' + kb(text.length) + ')')));
        body.replaceChildren.apply(body, nodes);
      }).catch(function (err) { body.replaceChildren(note('Transcript unavailable: ' + err.message)); });
    });
    return section('Raw transcript', [details]);
  }

  // ── file view ──

  // bandFor hands each turn id a band colour in order of first sight, so
  // the annotation and the timeline agree.
  function bandMap() {
    var seen = {};
    var n = 0;
    return function (id) {
      if (!seen[id]) seen[id] = 'var(' + BANDS[n++ % BANDS.length] + ')';
      return seen[id];
    };
  }

  function renderFile(worktree, path, repo) {
    document.title = path + ' · moondb';
    var bandFor = bandMap();
    var lines = el('div', {}, note(worktree ? 'Reading the file and running git blame…'
      : 'Line annotation needs a git worktree; this file is outside any repository.'));
    var history = el('div', {}, note('Loading…'));
    replace([
      backLinks([['← All sessions', '/']]),
      el('wk-page-header', {}, [
        el('wk-title', {}, path),
        el('wk-subtitle', {}, [where(worktree), repo].filter(Boolean).join(' · '))
      ]),
      section('Lines', [lines]),
      section('Recorded history', [history])
    ]);
    var annotated = worktree
      ? getJSON('/api/annotate?' + qs({ worktree: worktree, repo: repo, path: path })).then(
        function (res) { drawAnnotation(lines, res, bandFor); },
        function (err) { lines.replaceChildren(note('No annotation: ' + err.message)); })
      : Promise.resolve();
    var edits = getJSON('/api/history?' + qs({ worktree: worktree, repo: repo, path: path, limit: HISTORY_LIMIT }));
    annotated.then(function () { return edits; }).then(function (d) {
      drawHistory(history, d.edits || [], bandFor);
    }).catch(function (err) { history.replaceChildren(note('No history: ' + err.message)); });
  }

  // drawAnnotation renders the file with a gutter band per turn: solid when
  // git blame tied the line to the edit, dashed when only its text matched.
  // Hovering a line names its turn's prompt; clicking it opens the turn.
  function drawAnnotation(box, res, bandFor) {
    var lines = res.lines || [];
    if (!lines.length) {
      box.replaceChildren(note('The file is empty.'));
      return;
    }
    var st = annotationStats(lines, bandFor);
    var summary = st.order.length
      ? plural(lines.length, 'line') + ', ' + plural(st.order.length, 'turn') + '. ' +
        (st.unexplained ? plural(st.unexplained, 'line') + ' no recorded edit explains.' : '')
      : 'No recorded edit explains any line of this file.';
    box.replaceChildren(
      el('p', { class: 'hint' }, summary),
      annotationLegend(st, bandFor),
      el('div', { class: 'diff annot' }, annotationTable(lines, st.order, bandFor))
    );
  }

  // annotationStats lists the turns in order of first appearance, with how
  // many lines each explains by which method and the agents behind them.
  function annotationStats(lines, bandFor) {
    var st = { order: [], byTurn: {}, unexplained: 0 };
    lines.forEach(function (l) {
      if (!l.turn_id) {
        st.unexplained++;
        return;
      }
      if (!st.byTurn[l.turn_id]) {
        st.byTurn[l.turn_id] = { blame: 0, content: 0, agents: [] };
        st.order.push(l.turn_id);
        bandFor(l.turn_id);
      }
      var s = st.byTurn[l.turn_id];
      s[l.method] = (s[l.method] || 0) + 1;
      var agent = agentName(l.agent_type);
      if (s.agents.indexOf(agent) < 0) s.agents.push(agent);
    });
    return st;
  }

  function annotationLegend(st, bandFor) {
    return el('div', { class: 'legend' }, st.order.map(function (id) {
      var s = st.byTurn[id];
      var counts = [s.blame ? s.blame + ' by blame' : '', s.content ? s.content + ' by content' : '']
        .filter(Boolean).join(', ');
      var link = el('a', { href: '?turn=' + enc(id) }, '…');
      fill(link, turnLabel(id, 110), 'turn ' + id);
      return el('div', { class: 'row', style: '--band:' + bandFor(id) }, [
        el('span', { class: 'swatch' + (s.blame ? '' : ' content') }),
        el('span', { 'data-fixation': '' }, link),
        el('span', { class: 'muted nowrap' }, counts + ' · ' + s.agents.join(', '))
      ]);
    }));
  }

  // annotationTable draws one row per line. The gutter names the turn and
  // agent where a run starts; a line no edit explains does not end a run.
  function annotationTable(lines, order, bandFor) {
    var prev = null;
    var labels = {};
    var rows = {};
    var tbody = el('tbody', {}, lines.map(function (l) {
      var label = null;
      if (l.turn_id && l.turn_id !== prev) {
        label = el('code', {}, agentName(l.agent_type));
        (labels[l.turn_id] = labels[l.turn_id] || []).push({ node: label, agent: agentName(l.agent_type) });
      }
      if (l.turn_id) prev = l.turn_id;
      var tr = el('tr', l.turn_id ? {
        'data-turn': l.turn_id, 'data-method': l.method, style: '--band:' + bandFor(l.turn_id),
        title: l.method + (l.commit ? ', committed in ' + short(l.commit) : ', not committed')
      } : {}, [
        el('td', { class: 'a-gut' }, label && el('a', { href: '?turn=' + enc(l.turn_id) }, label)),
        lnCell(l.line),
        el('td', { class: 'd-tx' }, el('code', {}, l.text))
      ]);
      if (l.turn_id) (rows[l.turn_id] = rows[l.turn_id] || []).push(tr);
      return tr;
    }));
    order.forEach(function (id) {
      loadTurn(id).then(function (t) {
        labels[id].forEach(function (l) { l.node.textContent = '#' + t.seq + ' ' + l.agent; });
      }, function () {});
      turnLabel(id, 110).then(function (text) {
        rows[id].forEach(function (tr) { tr.title = text + '\n' + tr.title; });
      });
    });
    var tableEl = el('table', {}, tbody);
    tableEl.addEventListener('click', function (ev) {
      var tr = ev.target.closest('tr[data-turn]');
      if (!tr || ev.target.closest('a') || String(window.getSelection())) return;
      location.href = '?turn=' + enc(tr.getAttribute('data-turn'));
    });
    return tableEl;
  }

  // drawHistory lists the path's recorded edits newest first, each with its
  // turn and a diff on demand.
  function drawHistory(box, edits, bandFor) {
    if (!edits.length) {
      box.replaceChildren(el('div', { class: 'empty' }, 'No recorded edits of this path.'));
      return;
    }
    box.replaceChildren(el('ol', { class: 'timeline' }, edits.map(function (e) {
      var turnLink = el('a', { href: '?turn=' + enc(e.turn_id) }, '…');
      fill(turnLink, turnLabel(e.turn_id, 140), 'turn ' + e.turn_id);
      var diff = null;
      var toggle = el('button', { class: 'linkish', type: 'button' }, 'Show diff');
      toggle.addEventListener('click', function () {
        if (diff) {
          diff.remove();
          diff = null;
          toggle.textContent = 'Show diff';
          return;
        }
        diff = el('div', {}, note('Loading diff…'));
        li.appendChild(diff);
        toggle.textContent = 'Hide diff';
        var mine = diff;
        editDiff(e).then(function (node) { if (diff === mine) mine.replaceChildren(node); });
      });
      var li = el('li', { style: '--band:' + bandFor(e.turn_id) }, [
        el('div', { class: 'row' }, [
          el('span', { class: 'muted nowrap' }, fmtTime(e.at)),
          badge('outline', e.tool),
          kindBadge(e.kind),
          el('span', { class: 'muted' }, 'by ' + agentName(e.agent_type, e.agent_id)),
          toggle
        ]),
        el('div', { class: 'tl-prompt', 'data-fixation': '' }, turnLink)
      ]);
      return li;
    })));
  }

  // ── plumbing ──

  function replace(nodes) {
    app.replaceChildren.apply(app, nodes.filter(Boolean));
  }

  function renderError(err) {
    replace([
      backLinks([['← All sessions', '/']]),
      el('div', { class: 'empty' }, 'Failed to load: ' + ((err && err.message) || String(err)))
    ]);
  }

  if (params.get('turn')) {
    renderTurn(params.get('turn'));
  } else if (params.get('session')) {
    renderSession(params.get('session'));
  } else if (params.get('path')) {
    renderFile(params.get('worktree') || '', params.get('path'), params.get('repo') || '');
  } else {
    renderSessions();
  }
})();
