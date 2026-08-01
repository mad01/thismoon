'use strict';
// wire page — client-side render. The backend serves a chrome-only shell plus
// JSON; this script fetches it and builds the DOM with webkit's helpers.
// Two views, picked by the ?channel= query param: the channel list, and one
// channel's transcript. The transcript follows the server's event stream, so
// a conversation between two sessions updates itself while you watch.
// The page is read-only: opening a channel, posting, and closing all go
// through the MCP or the CLI. webkit.js loads before this file, so
// Webkit.el/escapeHtml are available.
(function () {
  var app = document.getElementById('app');
  var stream = null;

  // channelRef reads the channel from the URL. A wire:// connection string is
  // reduced to its channel, so pasting the token another session was given
  // straight into the address bar opens the transcript.
  function channelRef() {
    var raw = new URLSearchParams(location.search).get('channel') || '';
    var at = raw.indexOf('://');
    return at < 0 ? raw : raw.slice(raw.indexOf('/', at + 3) + 1);
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

  function plural(n, word) { return n + ' ' + word + (n === 1 ? '' : 's'); }

  function stateBadge(c) {
    return c.closed_at
      ? { variant: 'outline', label: 'closed' }
      : { variant: 'ok', label: 'open' };
  }

  // ── channel list ──

  function channelCard(c) {
    var sb = stateBadge(c);
    var kids = [
      Webkit.el('div', { class: 'c-head' }, [
        Webkit.el('span', { class: 'c-name' }, c.name),
        Webkit.el('wk-badge', { variant: sb.variant }, sb.label)
      ])
    ];
    if (c.topic) kids.push(Webkit.el('div', { class: 'c-topic' }, c.topic));
    if (c.last_from) {
      kids.push(Webkit.el('div', { class: 'c-last' }, [
        Webkit.el('b', {}, c.last_from + ': '),
        c.last_body
      ]));
    }

    var meta = plural(c.messages, 'message');
    if (c.participants && c.participants.length) meta += ' · ' + c.participants.join(', ');
    var when = fmtTime(c.last_at || c.created_at);
    if (when) meta += ' · ' + when;
    kids.push(Webkit.el('div', { class: 'c-meta' }, meta));

    var card = Webkit.el('wk-card', { href: '?channel=' + encodeURIComponent(c.name) }, kids);
    card.addEventListener('click', function () { go(c.name); });
    card.style.cursor = 'pointer';
    return card;
  }

  function renderList(data) {
    var channels = (data && data.channels) || [];
    var nodes = [
      Webkit.el('wk-page-header', {}, [
        Webkit.el('wk-title', {}, 'Channels'),
        Webkit.el('wk-subtitle', {}, plural(channels.length, 'channel'))
      ]),
      Webkit.el('p', { class: 'hint' }, [
        'Conversations between sessions. One agent opens a channel with the ',
        Webkit.el('code', {}, 'wire'),
        ' MCP or CLI and hands the name to another. This view is read-only.'
      ])
    ];
    nodes.push(channels.length
      ? Webkit.el('div', { class: 'c-list' }, channels.map(channelCard))
      : Webkit.el('div', { class: 'empty' }, 'No channels yet.'));
    paint(nodes);
  }

  // ── transcript ──

  var transcript = { list: null, seen: {} };

  function messageEl(m, openedBy) {
    var attrs = { class: 't-msg' + (openedBy && m.from === openedBy ? ' t-own' : '') };
    var head = [
      Webkit.el('span', { class: 't-from' }, m.from),
      Webkit.el('span', { class: 't-when' }, fmtTime(m.created_at))
    ];
    // The protocol fields, when present: what the message is, what it
    // answers, and whether it still expects an answer itself.
    if (m.kind) head.push(Webkit.el('wk-badge', { variant: 'outline' }, m.kind));
    if (m.reply_to) head.push(Webkit.el('wk-badge', { variant: 'outline' }, '→#' + m.reply_to));
    if (m.reply_needed) head.push(Webkit.el('wk-badge', { variant: 'warn' }, 'reply needed'));
    return Webkit.el('div', attrs, [
      Webkit.el('div', {}, head),
      Webkit.el('div', { class: 't-body' }, m.body)
    ]);
  }

  // liveEl is the connection indicator: a dot that fills in once the event
  // stream is open, so a stalled transcript is visible rather than silent.
  var liveDot = Webkit.el('span', { class: 't-dot' }, '');
  var liveText = Webkit.el('span', {}, 'connecting…');
  var liveEl = Webkit.el('div', { class: 't-live' }, [liveDot, liveText]);

  function setLive(on, text) {
    liveDot.className = 't-dot' + (on ? ' on' : '');
    liveText.textContent = text;
  }

  function renderTranscript(sum) {
    var closed = !!sum.closed_at;
    transcript.list = Webkit.el('div', { class: 't-list' }, []);
    transcript.seen = {};

    var subtitle = plural(sum.messages, 'message');
    if (sum.participants && sum.participants.length) {
      subtitle += ' · ' + sum.participants.join(', ');
    }

    var nodes = [
      Webkit.el('a', { class: 'back', href: '?' }, '← All channels'),
      Webkit.el('wk-page-header', {}, [
        Webkit.el('wk-title', {}, sum.name),
        Webkit.el('wk-subtitle', {}, subtitle)
      ])
    ];
    if (sum.topic) nodes.push(Webkit.el('p', { class: 'hint' }, sum.topic));
    if (sum.conventions) {
      // The opener's ground rules ride on the channel, not on message #1, so
      // they belong above the transcript where a late joiner sees them first.
      nodes.push(Webkit.el('wk-callout', {}, [
        Webkit.el('b', {}, 'Conventions: '),
        sum.conventions
      ]));
    }
    if (sum.connect) {
      nodes.push(Webkit.el('div', { class: 't-connect' }, [
        Webkit.el('span', { class: 't-connect-label' }, 'connection string'),
        Webkit.el('code', {}, sum.connect)
      ]));
    }
    if (closed) {
      nodes.push(Webkit.el('wk-callout', { variant: 'info' },
        'Closed' + (sum.close_note ? ': ' + sum.close_note : '') + '. No further messages.'));
    } else {
      nodes.push(liveEl);
    }
    nodes.push(transcript.list);
    paint(nodes);

    var backLink = app.querySelector('.back');
    if (backLink) {
      backLink.addEventListener('click', function (e) { e.preventDefault(); go(''); });
    }

    if (closed) {
      // A closed channel is a finished transcript: fetch it once, no stream.
      fetchJSON('/api/channels/' + encodeURIComponent(sum.name) + '/messages')
        .then(function (b) { appendMessages(b.messages, sum.opened_by); })
        .catch(showError);
      return;
    }
    follow(sum);
  }

  function appendMessages(msgs, openedBy) {
    (msgs || []).forEach(function (m) {
      if (transcript.seen[m.seq]) return;
      transcript.seen[m.seq] = true;
      transcript.list.appendChild(messageEl(m, openedBy));
    });
  }

  // follow streams the channel with EventSource. It starts at cursor 0 so the
  // stream delivers the backlog and every later message through one code path;
  // the browser reconnects on its own and resumes from the last event id.
  function follow(sum) {
    closeStream();
    var url = '/api/channels/' + encodeURIComponent(sum.name) + '/stream';
    stream = new EventSource(url);
    stream.addEventListener('open', function () { setLive(true, 'live'); });
    stream.addEventListener('message', function (e) {
      appendMessages([JSON.parse(e.data)], sum.opened_by);
    });
    stream.addEventListener('closed', function () {
      setLive(false, 'channel closed');
      closeStream();
    });
    stream.addEventListener('error', function () {
      setLive(false, 'reconnecting…');
    });
  }

  function closeStream() {
    if (stream) { stream.close(); stream = null; }
  }

  // ── plumbing ──

  function paint(nodes) {
    app.innerHTML = '';
    nodes.forEach(function (n) { app.appendChild(n); });
  }

  function showError(err) {
    paint([Webkit.el('div', { class: 'empty' },
      'Failed to load: ' + ((err && err.message) || String(err)))]);
  }

  function fetchJSON(path) {
    return fetch(path).then(function (r) {
      if (!r.ok) throw new Error('request failed (' + r.status + ')');
      return r.json();
    });
  }

  // go switches view without a page load, keeping the URL shareable.
  function go(ref) {
    history.pushState(null, '', ref ? '?channel=' + encodeURIComponent(ref) : location.pathname);
    route();
  }

  function route() {
    closeStream();
    var ref = channelRef();
    if (!ref) {
      setLive(false, 'connecting…');
      fetchJSON('/api/channels?all=1').then(renderList).catch(showError);
      return;
    }
    setLive(false, 'connecting…');
    fetchJSON('/api/channels/' + encodeURIComponent(ref)).then(renderTranscript).catch(showError);
  }

  window.addEventListener('popstate', route);
  route();
})();
