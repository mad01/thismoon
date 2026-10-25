'use strict';
// moondb page: client-side render (ADR-0005). The backend serves a static
// chrome-only shell plus JSON under /api/; this script fetches it and builds
// the DOM with webkit's shared helpers. Four views, picked by the query
// string:
//
//   /                              sessions, newest first; ?repo= and ?since= filter
//   /?session=<id>                 one session: its turns and the worktrees it touched
//   /?turn=<id>                    one turn: prompt, final message, agents, edits with
//                                  their diffs, unattributed deltas, commit links and
//                                  the raw transcript
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

  // promptLine resolves to the first line of a turn's prompt, '' without one.
  function promptLine(t, max) {
    if (!t.prompt_chunk) return Promise.resolve('');
    return chunk(t.prompt_chunk).then(
      function (text) { return firstLine(text, max); },
      function () { return ''; });
  }

  // turnLabel resolves to "#<seq> <prompt first line>" for a turn id.
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

  // firstLine is the first non-blank line of text, cut to max characters.
  function firstLine(text, max) {
    var lines = (text || '').split('\n');
    var line = '';
    for (var i = 0; i < lines.length && !line; i++) line = lines[i].trim();
    return line.length > max ? line.slice(0, max - 1) + '…' : line;
  }

  function plural(n, word) { return n + ' ' + word + (n === 1 ? '' : 's'); }

  function short(sha) { return (sha || '').slice(0, 10); }

  function kb(n) { return n < 1024 ? n + ' bytes' : Math.round(n / 1024) + ' KB'; }

  // agentName labels the agent behind an edit: its type, or main for the
  // main conversation.
  function agentName(type, id) { return type || (id ? 'subagent' : 'main'); }

  // mainFirst orders agent names with the main conversation first.
  function mainFirst(a, b) {
    if (a === b) return 0;
    if (a === 'main') return -1;
    if (b === 'main') return 1;
    return a < b ? -1 : 1;
  }

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

  var SINCE = [['', 'any time'], ['24h', 'last 24 hours'], ['7d', 'last 7 days'], ['30d', 'last 30 days']];

  function renderSessions() {
    document.title = 'Sessions · moondb';
    var state = {
      repo: params.get('repo') || '',
      since: params.get('since') || '',
      limit: parseInt(params.get('limit'), 10) || PAGE
    };
    var list = el('div', {}, el('div', { class: 'empty' }, 'Loading…'));
    var sub = el('wk-subtitle', {}, '');
    var filters = sessionFilters(state, function (repo, since) {
      state.repo = repo;
      state.since = since;
      state.limit = PAGE;
      sync();
      load();
    });

    function sync() {
      var q = qs({ repo: state.repo, since: state.since, limit: state.limit === PAGE ? '' : state.limit });
      history.replaceState(null, '', q ? '?' + q : '/');
    }
    function more() {
      state.limit += PAGE;
      sync();
      load();
    }
    function load() {
      getJSON('/api/sessions?' + qs({ repo: state.repo, since: state.since, limit: state.limit }))
        .then(function (data) {
          var sessions = data.sessions || [];
          sessions.forEach(function (s) { filters.offer(s.repos || []); });
          sub.textContent = plural(sessions.length, 'session') + ', newest first';
          list.replaceChildren.apply(list, sessionList(sessions, Boolean(state.repo || state.since),
            sessions.length === state.limit ? more : null));
        })
        .catch(function (err) { list.replaceChildren(el('div', { class: 'empty' }, 'Failed to load: ' + err.message)); });
    }

    replace([
      el('wk-page-header', {}, [el('wk-title', {}, 'Sessions'), sub]),
      el('p', { class: 'hint' }, [
        'Edits between commits, each linked to the agent turn that made it. Ask about one line with ',
        el('code', {}, 'moondb why'), ' or the ', el('code', {}, 'moondb_why'), ' MCP tool.'
      ]),
      filters.node,
      list
    ]);
    load();
    setInterval(load, 60000);
  }

  // sessionFilters builds the repo and since controls from state and calls
  // onChange with both values when either one changes. offer adds repos to
  // the repo input's suggestions.
  function sessionFilters(state, onChange) {
    var repoInput = el('input', {
      type: 'search', placeholder: 'any repository', value: state.repo,
      list: 'repo-options', 'aria-label': 'Repository'
    });
    var options = el('datalist', { id: 'repo-options' });
    var since = el('select', { 'aria-label': 'Since' }, SINCE.map(function (o) {
      return el('option', { value: o[0] }, o[1]);
    }));
    if (!SINCE.some(function (o) { return o[0] === state.since; })) {
      since.appendChild(el('option', { value: state.since }, 'since ' + state.since));
    }
    since.value = state.since;
    function changed() { onChange(repoInput.value.trim(), since.value); }
    repoInput.addEventListener('change', changed);
    since.addEventListener('change', changed);
    var seen = {};
    return {
      node: el('div', { class: 'filters' }, [
        el('label', {}, ['Repo', repoInput]), options, el('label', {}, ['Since', since])
      ]),
      offer: function (repos) {
        repos.forEach(function (r) {
          if (seen[r]) return;
          seen[r] = true;
          options.appendChild(el('option', { value: r }));
        });
      }
    };
  }

  // sessionList is one page of the listing; more, when set, loads a longer one.
  function sessionList(sessions, filtered, more) {
    if (!sessions.length) {
      return [el('div', { class: 'empty' },
        filtered ? 'No sessions match the filters.' : 'No sessions recorded yet.')];
    }
    var nodes = [table(['Started', 'Session', 'Repos', 'Turns', 'Edits'], sessions.map(sessionRow))];
    if (more) {
      nodes.push(el('button', { class: 'btn btn-ghost more', type: 'button', onclick: more },
        'Show ' + PAGE + ' more'));
    }
    return nodes;
  }

  function sessionRow(s) {
    return el('tr', {}, [
      el('td', { class: 'muted nowrap' }, fmtTime(s.started_at)),
      el('td', { class: 'prompt-cell', 'data-fixation': '' },
        el('a', { href: '?session=' + enc(s.id), title: s.cwd || '' }, s.title || s.cwd || s.id)),
      monoCell((s.repos || []).join(', ') || '-'),
      el('td', { class: 'num' }, String(s.turn_count)),
      el('td', { class: 'num' }, String(s.edit_count))
    ]);
  }

  // ── session view ──

  function renderSession(id) {
    Promise.all([
      getJSON('/api/sessions/' + enc(id)),
      getJSON('/api/edits?' + qs({ session: id, limit: SESSION_EDITS }))
    ]).then(function (r) { drawSession(r[0], r[1].edits || []); }).catch(renderError);
  }

  // summarizeEdits counts a session's edits per turn (with the agents seen)
  // and per worktree (with its repo and branches).
  function summarizeEdits(edits) {
    var byTurn = {};
    var byWorktree = {};
    edits.forEach(function (e) {
      var t = byTurn[e.turn_id] || (byTurn[e.turn_id] = { count: 0, agents: [] });
      var agent = agentName(e.agent_type, e.agent_id);
      t.count++;
      if (t.agents.indexOf(agent) < 0) t.agents.push(agent);
      var key = e.worktree || '';
      var w = byWorktree[key] || (byWorktree[key] = { repo: e.repo || '', branches: [], count: 0 });
      w.count++;
      if (e.branch && w.branches.indexOf(e.branch) < 0) w.branches.push(e.branch);
    });
    Object.keys(byTurn).forEach(function (id) { byTurn[id].agents.sort(mainFirst); });
    return { byTurn: byTurn, byWorktree: byWorktree };
  }

  function drawSession(d, edits) {
    var label = d.title || d.cwd || d.id;
    document.title = label + ' · moondb';
    var sum = summarizeEdits(edits);
    var turnList = (d.turns || []).slice().sort(function (a, b) { return a.seq - b.seq; });
    var repos = [];
    Object.keys(sum.byWorktree).forEach(function (k) {
      var r = sum.byWorktree[k].repo;
      if (r && repos.indexOf(r) < 0) repos.push(r);
    });
    var span = fmtTime(d.started_at) + (d.ended_at ? ' to ' + fmtTime(d.ended_at) : '');
    replace([
      backLinks([['← All sessions', '/']]),
      el('wk-page-header', {}, [
        el('wk-title', {}, label),
        el('wk-subtitle', {}, [span, d.harness, d.model].filter(Boolean).join(' · '))
      ]),
      el('wk-kv', { variant: 'card' }, [
        kvRow('Session', code(d.id)),
        kvRow('Cwd', code(d.cwd || '-')),
        kvRow('Harness', d.harness || '-'),
        kvRow('Started', fmtTime(d.started_at) || '-'),
        kvRow('Ended', fmtTime(d.ended_at) || 'not ended'),
        kvRow('Repos', repos.join(', ') || '-'),
        kvRow('Edits', String(d.edit_count))
      ]),
      el('div', { class: 'spaced' }, section('Worktrees', [worktreeTable(sum.byWorktree)])),
      section('Turns', [turnList.length
        ? table(['#', 'Prompt', 'Status', 'Edits', 'Agents', 'Started'], turnList.map(function (t) {
          return turnRow(t, sum.byTurn[t.id] || { count: 0, agents: [] });
        }))
        : el('div', { class: 'empty' }, 'No turns recorded.')])
    ]);
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

  function turnRow(t, s) {
    var link = el('a', { href: '?turn=' + enc(t.id) }, t.prompt_chunk ? '…' : '(no prompt recorded)');
    if (t.prompt_chunk) fill(link, promptLine(t, 160), '(prompt unavailable)');
    return el('tr', {}, [
      el('td', { class: 'num' }, String(t.seq)),
      el('td', { class: 'prompt-cell', 'data-fixation': '' }, link),
      el('td', {}, statusBadge(t.status)),
      el('td', { class: 'num' }, String(s.count)),
      monoCell(s.agents.join(', ') || '-'),
      el('td', { class: 'muted nowrap' }, fmtTime(t.started_at))
    ]);
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
          ? ', ' + plural(deltas.length, 'unattributed delta') : ''))
      ]),
      el('div', { class: 'spaced' }, section('Prompt', [prose(t.prompt_chunk, '(no prompt recorded)')])),
      section('Final message', [prose(t.final_chunk,
        t.status === 'open' ? 'The turn is still open.' : '(no final message recorded)')]),
      section('Agents', [agentTable(edits)]),
      section('Edits', editGroups(edits)),
      section('Unattributed deltas', deltas.length
        ? [note('Changes no recorded tool call explains: a formatter hook, a human, or another process wrote the file.')]
          .concat(deltas.map(deltaPanel))
        : [el('div', { class: 'empty' }, 'None. Every change in this turn came from a recorded tool call.')]),
      section('Commit links', [commitTable(t.commit_links || [], edits)]),
      transcriptSection(t)
    ]);
  }

  function agentTable(edits) {
    var groups = [];
    var byID = {};
    edits.forEach(function (e) {
      var key = e.agent_id || '';
      if (!byID[key]) {
        byID[key] = { type: agentName(e.agent_type, e.agent_id), id: key, count: 0 };
        groups.push(byID[key]);
      }
      byID[key].count++;
    });
    if (!groups.length) return el('div', { class: 'empty' }, 'No edits, so no agents seen.');
    groups.sort(function (a, b) { return (a.id ? 1 : 0) - (b.id ? 1 : 0); });
    return table(['Agent', 'Kind', 'Agent id', 'Edits'], groups.map(function (g) {
      return el('tr', {}, [
        el('td', {}, g.type),
        el('td', {}, badge(g.id ? 'c' : 'outline', g.id ? 'subagent' : 'main conversation')),
        monoCell(g.id || '-'),
        el('td', { class: 'num' }, String(g.count))
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
