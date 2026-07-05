'use strict';
// status dashboard — client-side render. The backend serves a static shell
// (chrome only) plus the full snapshot as JSON at /api/status; this script
// fetches it and builds the DOM with webkit's shared helpers. Presentation
// logic that used to live in Go (internal/server/view.go) lives here now.
// webkit.js loads before this file, so Webkit.el/escapeHtml/poll are available.
(function () {
  var app = document.getElementById('app');

  // banner mirrors view.go buildView: text + class from the down count.
  function banner(snap) {
    var services = snap.services || [];
    var down = snap.down || 0;
    if (services.length === 0) return { text: 'Waiting for first check cycle…', cls: 'none' };
    if (down === 0) return { text: 'All systems operational', cls: 'ok' };
    if (down === 1) return { text: '1 service down', cls: 'down' };
    return { text: down + ' services down', cls: 'down' };
  }

  // statusState mirrors buildSvcView's status text/class.
  function statusState(s) {
    if (!s.known) return { text: 'Pending', cls: 'none' };
    if (s.up) return { text: 'Operational', cls: 'ok' };
    return { text: 'Down', cls: 'down' };
  }

  function uptimeText(s) {
    return s.has_uptime ? fixed2(s.uptime_pct) + ' % uptime' : 'no data yet';
  }

  // dayClass mirrors view.go: the statuspage uptime thresholds.
  function dayClass(d) {
    if (!d.has_data) return 'none';
    if (d.pct >= 99.5) return 'ok';
    if (d.pct >= 95) return 'warn';
    return 'down';
  }

  function dayTitle(d) {
    if (!d.has_data) return d.date + ' · no data';
    return d.date + ' · ' + fixed2(d.pct) + '% (' + d.ok + ' ok / ' + d.fail + ' fail)';
  }

  // shortenWebkit trims a Go pseudo-version like
  // v0.1.1-0.20260610130446-062432b0e180 down to its 12-char commit sha tail.
  var SHA_LEN = 12;
  function shortenWebkit(v) {
    var i = v.lastIndexOf('-');
    if (i > 0 && v.length - i - 1 === SHA_LEN) return v.slice(i + 1);
    return v;
  }

  // metaRows mirrors buildSvcView's kv pairs. HTTP() is derived from port > 0
  // (the Go method does not serialize).
  function metaRows(s) {
    var rows = [];
    rows.push(['check', s.port > 0 ? 'HTTP :' + s.port : 'launchd']);
    if (s.daemon) rows.push(['scope', 'daemon']);
    if (s.version) rows.push(['version', s.version]);
    if (s.webkit) rows.push(['webkit', shortenWebkit(s.webkit)]);
    if (s.known) rows.push(['last check', (s.detail || '') + ' · ' + clock(s.checked_at)]);
    return rows;
  }

  // fixed2 formats a number to two decimals (Go's %.2f).
  function fixed2(n) { return (typeof n === 'number' ? n : 0).toFixed(2); }

  // fmtGenerated / clock format RFC3339 timestamps in local time, matching the
  // Go layouts "2006-01-02 15:04:05" and "15:04:05".
  function pad(n) { return n < 10 ? '0' + n : '' + n; }
  function fmtGenerated(iso) {
    var d = new Date(iso);
    if (isNaN(d)) return iso || '';
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) +
      ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
  }
  function clock(iso) {
    var d = new Date(iso);
    if (isNaN(d)) return iso || '';
    return pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
  }

  function svcCard(s, windowDays) {
    var st = statusState(s);
    var name = s.link
      ? Webkit.el('a', { class: 'svc-name', href: s.link }, s.label)
      : Webkit.el('span', { class: 'svc-name' }, s.label);

    var bars = (s.days || []).map(function (d) {
      return Webkit.el('span', { class: 'bar ' + dayClass(d), title: dayTitle(d) });
    });

    var meta = metaRows(s).map(function (kv) {
      return Webkit.el('span', { class: 'meta-item' }, [
        Webkit.el('span', { class: 'meta-k' }, kv[0]),
        ' ',
        Webkit.el('span', { class: 'meta-v' }, kv[1])
      ]);
    });

    return Webkit.el('wk-card', { class: 'svc' }, [
      Webkit.el('div', { class: 'svc-head' }, [
        name,
        Webkit.el('span', { class: 'svc-state ' + st.cls }, st.text)
      ]),
      Webkit.el('div', { class: 'bars' }, bars),
      Webkit.el('div', { class: 'svc-axis' }, [
        Webkit.el('span', {}, windowDays + ' days ago'),
        Webkit.el('span', { class: 'axis-line' }),
        Webkit.el('span', {}, uptimeText(s)),
        Webkit.el('span', { class: 'axis-line' }),
        Webkit.el('span', {}, 'Today')
      ]),
      Webkit.el('div', { class: 'svc-meta' }, meta)
    ]);
  }

  function render(snap) {
    var windowDays = snap.window_days || 0;
    var b = banner(snap);

    var grid = Webkit.el('div', { class: 'svc-grid' },
      (snap.services || []).map(function (s) { return svcCard(s, windowDays); }));

    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'banner ' + b.cls }, b.text));
    app.appendChild(Webkit.el('div', { class: 'legend' }, [
      Webkit.el('span', {}, 'Uptime over the past ' + windowDays + ' days'),
      Webkit.el('span', {}, 'generated ' + fmtGenerated(snap.generated_at))
    ]));
    app.appendChild(grid);
  }

  // Webkit.poll fetches immediately and then every 60s, replacing the old
  // inline setTimeout(location.reload, 60000).
  Webkit.poll('/api/status', render, 60000, function (err) {
    app.innerHTML = '';
    app.appendChild(Webkit.el('div', { class: 'banner none' },
      'Failed to load status: ' + ((err && err.message) || String(err))));
  });
})();
