// Theme/font/size controls are owned by <wk-header> (webkit.js).

// ── Helpers ──
// HTML escaping is provided by the shared webkit render helper
// (Webkit.escapeHtml, loaded via /webkit/webkit.js before this file).
// Clipboard helpers (copyText, bindCopyButtons) come from /assets/common.js,
// shared with the health and file pages.

// highlightTerms pulls plain words out of a zoekt query (dropping repo:/file:/
// lang:/case: filters and boolean operators) for best-effort match highlighting.
function highlightTerms(query) {
  return query
    .split(/\s+/)
    .filter(t => t && !/^(repo|file|lang|case|content|sym):/.test(t) && t !== '|')
    .flatMap(t => t.split('|'))
    .map(t => t.replace(/^["']|["']$/g, ''))
    .filter(t => t.length >= 2);
}

// applyHighlight wraps term occurrences in <mark>. It runs on already-escaped
// HTML (no real tags present), so inserting tags directly is safe.
function applyHighlight(escaped, terms) {
  let out = escaped;
  for (const term of terms) {
    const e = Webkit.escapeHtml(term).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    try {
      out = out.replace(new RegExp(e, 'gi'), m => '<mark>' + m + '</mark>');
    } catch (_) { /* skip bad term */ }
  }
  return out;
}

// ── Query syntax tokenizer ──
// Zoekt fields and aliases per zoekt's doc/query_syntax.md. Long names come
// before the one-letter aliases so `case:` is matched as a whole.
const HL_FIELD = /^(archived|branch|case|content|file|fork|lang|public|regex|repo|sym|type|[abcflrt]):/;

// tokenizeQuery splits a zoekt query into [type, text] pairs for the
// highlight overlay: 'field' (repo: etc.), 'neg' (leading -), 'str' (quoted),
// 'paren', 'op' (or), and 'plain' for everything else. Fields, negation, and
// `or` are only recognized at a token boundary (start, after whitespace, or
// after an opening paren), so they don't fire inside regex atoms.
function tokenizeQuery(q) {
  const tokens = [];
  let i = 0;
  let boundary = true;
  while (i < q.length) {
    const rest = q.slice(i);
    let m;
    if ((m = rest.match(/^\s+/))) {
      tokens.push(['plain', m[0]]);
      boundary = true;
    } else if ((m = rest.match(/^"(?:\\.|[^"\\])*(?:"|$)/))) {
      tokens.push(['str', m[0]]);
      boundary = false;
    } else if (rest[0] === '(' || rest[0] === ')') {
      m = [rest[0]];
      tokens.push(['paren', rest[0]]);
      boundary = rest[0] === '(';
    } else if (boundary && rest[0] === '-' && rest.length > 1 && !/\s/.test(rest[1])) {
      m = ['-'];
      tokens.push(['neg', '-']);
      // boundary stays true: the negated field/atom follows directly.
    } else if (boundary && (m = rest.match(HL_FIELD))) {
      tokens.push(['field', m[0]]);
      boundary = false;
    } else if (boundary && (m = rest.match(/^or(?=[\s)]|$)/))) {
      tokens.push(['op', m[0]]);
      boundary = false;
    } else {
      m = rest.match(/^(?:\\.|[^\s()"\\-])+|^./);
      tokens.push(['plain', m[0]]);
      boundary = false;
    }
    i += m[0].length;
  }
  return tokens;
}

const HL_CLASS = { field: 't-field', neg: 't-neg', str: 't-str', paren: 't-paren', op: 't-op' };

// highlightQuery renders a zoekt query as token-colored HTML using the same
// tokenizer as the search box overlay. Used by both the live search input and
// the example queries on the empty-state landing view.
function highlightQuery(q) {
  return tokenizeQuery(q).map(([type, text]) => {
    const cls = HL_CLASS[type];
    return cls ? '<span class="' + cls + '">' + Webkit.escapeHtml(text) + '</span>' : Webkit.escapeHtml(text);
  }).join('');
}

// ── Landing-page example queries ──
// Shown in the empty state (no active search). Each query is rendered with the
// same syntax highlighting as the search box; clicking one runs it. `intro` and
// `desc` carry trusted inline markup (<code>/<em>), so they are inserted as-is.
const EXAMPLE_SECTIONS = [
  {
    title: 'Basics',
    intro: 'A single word matches as a case-insensitive substring (smart case: an uppercase letter forces case-sensitivity). Quote anything with spaces or special characters to match it literally.',
    rows: [
      { q: 'NewServer', desc: 'Single word — no quotes needed.' },
      { q: '"package main"', desc: 'Quoted phrase — exact match with spaces.' },
      { q: '"interface{}"', desc: 'Quote special characters to match them literally.' },
      { q: 'ListenAndServe case:yes', desc: 'Force case-sensitive matching.' },
    ],
  },
  {
    title: 'Regular expressions',
    intro: 'An unquoted query with regex metacharacters is treated as a regular expression. When you want those characters literally, quote instead (see Basics).',
    rows: [
      { q: 'func [A-Z]\\w+\\(', desc: 'Exported function declarations.' },
      { q: 'http\\.(Get|Post)', desc: 'Alternation inside a regex group.' },
      { q: 'fmt\\.Errorf', desc: 'Escaped dot — same as <code>"fmt.Errorf"</code>, which is easier to read.' },
    ],
  },
  {
    title: 'Boolean operators',
    intro: 'An unquoted space means AND, <code>|</code> means OR, and a leading <code>-</code> excludes. Note the difference between two AND-ed terms and one quoted phrase.',
    rows: [
      { q: 'context cancel', desc: 'AND — both terms appear somewhere in the file.' },
      { q: '"context cancel"', desc: 'Phrase — the words appear next to each other.' },
      { q: 'TODO|FIXME|HACK', desc: 'OR — any of the markers (regex alternation in one term).' },
      { q: 'goroutine -_test', desc: 'Exclude test files.' },
    ],
  },
  {
    title: 'Combining AND and OR',
    intro: 'Use the <code>or</code> keyword between terms or phrases, and parentheses to group. Note: <code>|</code> between quoted phrases does <em>not</em> mean OR — it only works as regex alternation inside a single unquoted term.',
    rows: [
      { q: 'context (cancel|timeout)', desc: 'AND a word with a regex alternation.' },
      { q: '"func main" or "func init"', desc: 'OR between two exact phrases.' },
      { q: '(TODO or FIXME) lang:go -file:_test', desc: 'Grouped OR, AND-ed with a language filter and an exclusion.' },
    ],
  },
  {
    title: 'Scoping filters',
    intro: 'Narrow by repo, file path, or language. Filters stay unquoted; quote the search term itself when it has spaces or special characters.',
    rows: [
      { q: 'repo:thismoon "func main"', desc: 'Phrase within repos matching a regex.' },
      { q: 'file:\\.go$ "http.HandleFunc"', desc: 'Literal call, only in <code>.go</code> files.' },
      { q: 'lang:go "interface{}"', desc: 'Language filter plus quoted special characters.' },
      { q: 'lang:yaml replicas', desc: 'Single word needs no quotes.' },
    ],
  },
  {
    title: 'Symbol search',
    intro: 'Restrict matches to symbol <em>definitions</em> — function, type, and method names — with <code>sym:</code>. This skips call sites and comments, so you land on where a thing is declared.',
    rows: [
      { q: 'sym:NewServer', desc: 'Definitions named <code>NewServer</code>, not its call sites.' },
      { q: 'sym:Handler lang:go', desc: 'Go symbols matching <code>Handler</code>.' },
      { q: 'sym:.*Cache$ -file:_test', desc: 'Symbols ending in <code>Cache</code>, excluding tests.' },
    ],
  },
  {
    title: 'Scope & metadata filters',
    intro: 'Filters compose with everything above. Combine path, language, and repo scoping to zero in on a result set.',
    rows: [
      { q: 'TODO file:internal/ lang:go', desc: 'Markers under a path, in one language.' },
      { q: 'repo:^github\\.com/.* "func main"', desc: 'Anchor the repo regex to a host.' },
      { q: 'panic\\( -file:(_test|vendor)', desc: 'Real panics — skip tests and vendored code.' },
    ],
  },
];

// ── Semantic example queries ──
// Natural-language seeds for the semantic mode landing view, grouped by intent.
// Semantic search matches by meaning, so these read as plain descriptions of
// what the code does, not zoekt syntax.
const SEMANTIC_SECTIONS = [
  {
    title: 'Find by behavior',
    intro: 'Describe what the code <em>does</em>. You do not need to know the function name — the ranker matches on meaning, so a plain description finds the implementation.',
    rows: [
      'where do we verify auth tokens',
      'retry a failed network request with backoff',
      'gracefully shut down on a termination signal',
      'parse command line flags',
    ],
  },
  {
    title: 'Data & storage',
    intro: 'Search for how data is read, written, or transformed — serialization, files, and databases.',
    rows: [
      'serialize a struct to JSON',
      'read a file line by line',
      'open a database connection and run a query',
      'run several writes inside a transaction',
    ],
  },
  {
    title: 'HTTP & networking',
    intro: 'Find request handling, routing, and the cross-cutting concerns around a server.',
    rows: [
      'register an HTTP route handler',
      'add a request timeout with context cancellation',
      'rate limit incoming requests',
      'middleware that logs each request',
    ],
  },
  {
    title: 'Cross-cutting concerns',
    intro: 'Concepts that show up across a codebase: caching, errors, hashing, and pagination.',
    rows: [
      'wrap and propagate an error with context',
      'cache an expensive computed result',
      'compute a cryptographic hash of a file',
      'paginate a large result set',
    ],
  },
];

// ── Hybrid example queries ──
// Natural-language seeds for the hybrid mode landing view. These work best with
// hybrid because they contain a likely literal token AND carry meaning — files
// matched by both backends rank higithostst (consensus). Groups by theme.
const HYBRID_SECTIONS = [
  {
    title: 'Infrastructure & lifecycle',
    intro: 'These queries have both a literal token and clear intent — ideal for hybrid. Files that exact-match AND mean-match rank higithostst.',
    rows: [
      'daemon idle timeout shutdown',
      'register an http route handler',
      'parse the yaml config file',
      'background reindex queue worker',
    ],
  },
  {
    title: 'Search & ranking',
    intro: 'Ranking-adjacent code where terminology is precise but behavior matters too — hybrid catches both.',
    rows: [
      'reciprocal rank fusion scoring',
      'vector similarity cosine distance',
      'merge and deduplicate search results',
      'filter results by language and repo',
    ],
  },
  {
    title: 'Auth & network',
    intro: 'Security and networking code uses specific method names alongside broader patterns.',
    rows: [
      'verify a JWT auth token',
      'retry a failed HTTP request',
      'rate limit incoming requests',
      'graceful shutdown on SIGTERM',
    ],
  },
  {
    title: 'Data & errors',
    intro: 'Error handling and data transformation often use specific names alongside broader patterns.',
    rows: [
      'wrap and propagate an error with context',
      'serialize a struct to JSON',
      'open a database connection and run a query',
      'cache an expensive computed result',
    ],
  },
];

// ── Search page ──
async function initSearchPage() {
  const input = document.getElementById('searchInput');
  if (!input) return;

  // searchKind selects which backend the search box routes to: 'lexical'
  // (zoekt, default, unchanged) or 'semantic' (vector /api/semantic_search).
  let searchKind = 'lexical';

  const results = document.getElementById('results');
  const status = document.getElementById('status');

  // Repo / language scope dropdowns. Their values ride along as repo=/lang=
  // params on every backend (all three accept them), so one picker pair scopes
  // lexical, semantic, and hybrid alike. Typing repo:/lang: in a lexical query
  // still works — the dropdowns are additive, not a replacement.
  const repoSelect = document.getElementById('repoSelect');
  const langSelect = document.getElementById('langSelect');

  // Language options for the scope dropdown (zoekt lang: names). Anything not
  // listed can still be typed as lang: in the query.
  const LANG_OPTIONS = ['c', 'c++', 'c#', 'css', 'go', 'html', 'java',
    'javascript', 'json', 'kotlin', 'markdown', 'protobuf', 'python', 'ruby',
    'rust', 'shell', 'sql', 'swift', 'terraform', 'toml', 'typescript', 'yaml'];

  // ensureOption makes sure `value` exists as an option on `sel` and selects
  // it. Used to honour deep-linked ?repo=/?lang= before the async option load.
  function ensureOption(sel, value) {
    if (!sel || !value) return;
    if (![...sel.options].some(o => o.value === value)) {
      const opt = document.createElement('option');
      opt.value = value;
      opt.textContent = value;
      sel.appendChild(opt);
    }
    sel.value = value;
  }

  // loadRepoOptions fills the repo dropdown from /api/repos. Best-effort: on
  // failure the dropdown just stays at "all repos".
  async function loadRepoOptions() {
    if (!repoSelect) return;
    try {
      const res = await fetch('/api/repos');
      const repos = await res.json();
      if (!res.ok || !Array.isArray(repos)) return;
      const have = new Set([...repoSelect.options].map(o => o.value));
      repos.map(r => r.name).sort().forEach(name => {
        if (have.has(name)) return;
        const opt = document.createElement('option');
        opt.value = name;
        opt.textContent = name;
        repoSelect.appendChild(opt);
      });
    } catch (_) { /* keep "all repos" */ }
  }

  // applyScope adds the dropdown selections to a request's query params.
  function applyScope(params) {
    if (repoSelect && repoSelect.value) params.set('repo', repoSelect.value);
    if (langSelect && langSelect.value) params.set('lang', langSelect.value);
  }

  // Search history: the last N queries per search kind, persisted in
  // localStorage and surfaced as native datalist suggestions on the input.
  const HISTORY_MAX = 10;
  const historyList = document.getElementById('searchHistory');
  function historyKey() { return 'csl-history-' + searchKind; }
  function loadHistory() {
    try {
      const arr = JSON.parse(localStorage.getItem(historyKey()));
      return Array.isArray(arr) ? arr.filter(q => typeof q === 'string') : [];
    } catch (_) { return []; }
  }
  function renderHistory() {
    if (!historyList) return;
    historyList.innerHTML = loadHistory().map(q =>
      '<option value="' + Webkit.escapeHtml(q) + '"></option>').join('');
  }
  function pushHistory(q) {
    const arr = [q, ...loadHistory().filter(x => x !== q)].slice(0, HISTORY_MAX);
    try { localStorage.setItem(historyKey(), JSON.stringify(arr)); } catch (_) { /* full/blocked */ }
    renderHistory();
  }

  // updateURL mirrors the query, search kind, and scope filters into the
  // address bar so any search — in any mode — is a shareable deep link.
  function updateURL(q) {
    const url = new URL(location.href);
    url.searchParams.set('q', q);
    if (searchKind === 'lexical') url.searchParams.delete('kind');
    else url.searchParams.set('kind', searchKind);
    for (const [key, sel] of [['repo', repoSelect], ['lang', langSelect]]) {
      if (sel && sel.value) url.searchParams.set(key, sel.value);
      else url.searchParams.delete(key);
    }
    history.replaceState(null, '', url);
  }

  // Query syntax highlighting: redraw the overlay copy of the input and keep
  // it aligned with the input's horizontal scroll. The input's own text is
  // made transparent only once JS is wired up (hl-on).
  const hl = document.getElementById('searchHl');
  function renderQueryHl() {
    // The zoekt token overlay is lexical-only; semantic queries are plain prose.
    if (!hl || searchKind !== 'lexical') return;
    hl.innerHTML = highlightQuery(input.value);
    hl.style.transform = 'translateX(' + -input.scrollLeft + 'px)';
  }
  // setHlEnabled turns the syntax-highlight overlay on (lexical) or off
  // (semantic, where the raw input text is shown unstyled).
  function setHlEnabled(on) {
    if (!hl) return;
    if (on) { input.classList.add('hl-on'); renderQueryHl(); }
    else { input.classList.remove('hl-on'); hl.innerHTML = ''; }
  }
  if (hl) {
    input.classList.add('hl-on');
    input.addEventListener('input', renderQueryHl);
    input.addEventListener('scroll', () => {
      hl.style.transform = 'translateX(' + -input.scrollLeft + 'px)';
    });
  }
  const segBtns = document.querySelectorAll('#modeToggle button');
  let mode = 'files_with_matches';

  // How many files to request per server fetch (it caps at 500), and how many
  // file blocks to reveal per client-side page. Lexical pages the server with an
  // offset for "Load more"; semantic and hybrid fetch a deeper ranked list than
  // they show, then page client-side.
  const PAGE_FETCH = 200;
  const PAGE_SIZE = 20;
  const SEM_FETCH_K = 50;

  // Render + paging state. lastData is the accumulated (merged) lexical result
  // set across fetched pages; baseParams/pageOffset/fetching drive server-side
  // "Load more" (see fetchPage).
  let lastData = null;
  let lastTerms = [];
  let shown = 0;
  let baseParams = null;
  let pageOffset = 0;
  let fetching = false;

  // Quick-filter state: facets from the last unfiltered search (so all chips
  // stay visible while filtering), and the multi-select set of selected
  // suffixes. The model is select-to-show: nothing is selected by default
  // (= show all results), and clicking a chip filters the results down to the
  // selected suffix(es). '' is a real suffix (files without an extension).
  // null until the first results land.
  const facetBar = document.getElementById('facets');
  const examplesEl = document.getElementById('examples');
  let baselineFacets = [];
  let selectedExts = null;

  // Semantic results are filtered client-side (the ranked list is small), so
  // its facet state lives here: the full last hit list, the by-language
  // selection, and how many hits are revealed (same paging as lexical).
  let lastSemHits = [];
  let semSelectedExts = new Set();
  let semShown = 0;

  // Hybrid results page and filter the same way; hybrid hits carry no
  // language, so its chips group by file extension instead.
  let lastHybHits = [];
  let hybNote = '';
  let hybSemAvailable = true;
  let hybSelectedExts = new Set();
  let hybShown = 0;

  // facetsNarrowed is true when the user has selected a non-empty proper subset
  // of the chips, i.e. the next query must carry a file-suffix filter. An empty
  // selection (nothing clicked) and a full selection both mean "show all".
  function facetsNarrowed() {
    return selectedExts !== null && selectedExts.size > 0 &&
      selectedExts.size < baselineFacets.length;
  }

  segBtns.forEach(b => b.addEventListener('click', () => {
    segBtns.forEach(x => x.classList.remove('active'));
    b.classList.add('active');
    mode = b.dataset.mode;
    if (input.value.trim()) runSearch();
  }));

  document.getElementById('searchForm').addEventListener('submit', e => {
    e.preventDefault();
    runActiveSearch();
  });

  // runActiveSearch dispatches to the backend for the active mode. Lexical
  // resets the quick-filter selection (a fresh query starts with everything
  // selected); semantic and hybrid have no facets.
  function runActiveSearch() {
    if (searchKind === 'hybrid') {
      runHybridSearch();
    } else if (searchKind === 'semantic') {
      runSemanticSearch();
    } else {
      selectedExts = null;
      runSearch();
    }
  }

  // applyKind swaps the active backend, the landing examples, and the input
  // overlay. Shared by the toggle buttons and ?kind= deep-link restore.
  const kindBtns = document.querySelectorAll('#searchKindToggle button');
  function applyKind(kind) {
    searchKind = kind;
    kindBtns.forEach(x => x.classList.toggle('active', x.dataset.kind === kind));
    setHlEnabled(kind === 'lexical');
    results.innerHTML = '';
    facetBar.innerHTML = '';
    document.getElementById('modeToggle').hidden = true;
    if (expandCtl) expandCtl.hidden = true;
    renderHistory(); // suggestions are per-kind
    renderExamples();
  }

  // Lexical ⇄ semantic toggle: swap the backend, then re-run if there's a
  // query (else return to the examples).
  kindBtns.forEach(b => b.addEventListener('click', () => {
    const kind = b.dataset.kind;
    if (kind === searchKind) return;
    applyKind(kind);
    if (input.value.trim()) runActiveSearch();
    else showExamples();
  }));

  // Semantic — and the hybrid mode that fuses it — can be turned off per
  // profile (config semantic.enabled). Ask the backend and, when off, disable
  // those toggle buttons with a reason. Native disabled buttons ignore clicks,
  // so no mode swap can reach them; best-effort — leave them enabled on error.
  let semanticEnabled = true;
  async function loadCapabilities() {
    try {
      const res = await fetch('/api/capabilities');
      if (res.ok) semanticEnabled = (await res.json()).semantic !== false;
    } catch { /* leave modes enabled on error */ }
    if (semanticEnabled) return;
    kindBtns.forEach(b => {
      if (b.dataset.kind === 'semantic' || b.dataset.kind === 'hybrid') {
        b.disabled = true;
        b.title = 'Disabled in this profile (semantic.enabled = false in the csl config)';
      }
    });
  }

  // Scope changes re-run the active search so the narrowed results appear
  // immediately; with no query there is nothing to re-run.
  [repoSelect, langSelect].forEach(sel => sel && sel.addEventListener('change', () => {
    if (input.value.trim()) runActiveSearch();
  }));

  // renderExamples paints the landing-page query guide into #examples. Each
  // query is syntax-highlighted and carries a data-q for the delegated click
  // handler below; the href keeps the row a real link (open-in-new-tab, no-JS).
  function renderExamples() {
    if (!examplesEl) return;
    if (searchKind === 'hybrid') { examplesEl.innerHTML = hybridExamplesHTML(); return; }
    if (searchKind === 'semantic') { examplesEl.innerHTML = semanticExamplesHTML(); return; }
    const sections = EXAMPLE_SECTIONS.map(sec =>
      '<div class="ex-section"><h2>' + Webkit.escapeHtml(sec.title) + '</h2>' +
      '<p>' + sec.intro + '</p>' +
      '<wk-table><table>' +
      sec.rows.map(r =>
        '<tr><td class="ex-q"><a href="/?q=' + encodeURIComponent(r.q) + '" data-q="' + Webkit.escapeHtml(r.q) +
        '"><code class="qhl">' + highlightQuery(r.q) + '</code></a></td>' +
        '<td class="ex-desc">' + r.desc + '</td></tr>').join('') +
      '</table></wk-table></div>').join('');
    examplesEl.innerHTML =
      '<p class="ex-lead">csl uses <a class="file-path" href="https://github.com/sourcegraph/zoekt" target="_blank" rel="noopener">zoekt</a> query syntax. Click any query to run it.</p>' +
      sections;
  }

  // semanticExamplesHTML paints the semantic landing view: plain-language seed
  // queries grouped into sections that reuse the same .ex-section / .ex-q /
  // data-q mechanism as the lexical examples, so the delegated click handler
  // below runs them the same way.
  function semanticExamplesHTML() {
    const sections = SEMANTIC_SECTIONS.map(sec =>
      '<div class="ex-section"><h2>' + Webkit.escapeHtml(sec.title) + '</h2>' +
      '<p>' + sec.intro + '</p>' +
      '<wk-table><table>' +
      sec.rows.map(q =>
        '<tr><td class="ex-q"><a href="/?q=' + encodeURIComponent(q) + '" data-q="' + Webkit.escapeHtml(q) +
        '"><code class="qhl">' + Webkit.escapeHtml(q) + '</code></a></td></tr>').join('') +
      '</table></wk-table></div>').join('');
    return '<p class="ex-lead">Semantic search matches code by meaning, not exact text — ' +
      'describe what the code does in plain language. Click any example to run it.</p>' +
      sections;
  }

  // hybridExamplesHTML paints the hybrid landing view: plain-language seed
  // queries grouped into sections. Same data-q mechanism as the other modes so
  // the delegated click handler below runs them the same way.
  function hybridExamplesHTML() {
    const sections = HYBRID_SECTIONS.map(sec =>
      '<div class="ex-section"><h2>' + Webkit.escapeHtml(sec.title) + '</h2>' +
      '<p>' + sec.intro + '</p>' +
      '<wk-table><table>' +
      sec.rows.map(q =>
        '<tr><td class="ex-q"><a href="/?q=' + encodeURIComponent(q) + '" data-q="' + Webkit.escapeHtml(q) +
        '"><code class="qhl">' + Webkit.escapeHtml(q) + '</code></a></td></tr>').join('') +
      '</table></wk-table></div>').join('');
    return '<p class="ex-lead">Hybrid search fuses lexical (zoekt) and semantic (vector) results via ' +
      'Reciprocal Rank Fusion. Files matched by both backends rank higithostst — they are the consensus hits. ' +
      'Click any example to run it.</p>' +
      sections;
  }

  // Delegated click: run an example query in-page instead of navigating. Works
  // for lexical, semantic, and hybrid example sets (runActiveSearch dispatches).
  if (examplesEl) {
    examplesEl.addEventListener('click', e => {
      const a = e.target.closest('[data-q]');
      if (!a) return;
      e.preventDefault();
      input.value = a.getAttribute('data-q');
      renderQueryHl();
      runActiveSearch();
    });
  }

  // showExamples returns the page to its empty landing state: examples visible,
  // results / status / facets cleared. Called on load and when the query is
  // emptied. runSearch hides the examples again as soon as a query is run.
  function showExamples() {
    if (examplesEl) examplesEl.hidden = false;
    results.innerHTML = '';
    status.textContent = '';
    facetBar.innerHTML = '';
    baselineFacets = [];
    selectedExts = null;
    document.getElementById('modeToggle').hidden = true;
    if (expandCtl) expandCtl.hidden = true;
  }

  // Restore the examples view whenever the user clears the query.
  input.addEventListener('input', () => { if (!input.value.trim()) showExamples(); });

  async function runSearch() {
    const q = input.value.trim();
    if (!q) return;
    if (examplesEl) examplesEl.hidden = true; // leave the landing/examples view
    const params = new URLSearchParams({ q, mode });
    // repo:/lang:/file: scoping belongs in the query itself (zoekt syntax);
    // a narrowed quick-filter selection adds a file-suffix filter on top.
    const narrowed = facetsNarrowed();
    if (narrowed) params.set('file', facetFileRegex(selectedExts));
    if (mode === 'content') params.set('context', '2');
    applyScope(params);
    // Remember the query params (without limit/offset) so "Load more" can fetch
    // the next server page; reset the paging cursor for this fresh query.
    baseParams = params;
    pageOffset = 0;
    updateURL(q);
    await fetchPage(true, { q, narrowed });
  }

  // fetchPage requests one server page of lexical results at the current
  // pageOffset. reset=true starts a new query (clears the view and refreshes the
  // facet chips); reset=false appends the next page onto the accumulated result
  // set for a server-side "Load more".
  async function fetchPage(reset, ctx) {
    if (fetching) return;
    fetching = true;
    const params = new URLSearchParams(baseParams);
    params.set('limit', String(PAGE_FETCH));
    params.set('offset', String(pageOffset));
    if (reset) {
      status.textContent = 'Searching…';
      results.innerHTML = '';
    } else {
      const b = document.getElementById('loadMore');
      if (b) { b.disabled = true; b.textContent = 'Loading…'; }
    }
    try {
      const res = await fetch('/api/search?' + params.toString());
      const data = await res.json();
      if (!res.ok) { status.innerHTML = '<span class="err">' + Webkit.escapeHtml(data.error || 'error') + '</span>'; return; }
      if (reset) {
        pushHistory(ctx.q);
        if (!ctx.narrowed) {
          // Unfiltered response: refresh the chip set and start with nothing
          // selected (empty = show all). Clicking a chip narrows from there.
          baselineFacets = data.facets || [];
          selectedExts = new Set();
        }
        document.getElementById('modeToggle').hidden = false;
        if (expandCtl) expandCtl.hidden = false;
        lastData = data;
        lastTerms = highlightTerms(ctx.q);
        shown = PAGE_SIZE;
        pageOffset = data.files;
        renderFacets();
        renderPage();
      } else {
        mergePage(lastData, data);
        pageOffset += data.files;
        shown += PAGE_SIZE;
        renderPage();
      }
    } catch (err) {
      status.innerHTML = '<span class="err">' + Webkit.escapeHtml(String(err)) + '</span>';
    } finally {
      fetching = false;
    }
  }

  // mergePage folds a freshly fetched page into the accumulated result set:
  // files of a repo already shown are appended to that repo group (preserving
  // ranked order), new repos are added in order, and the counts and truncation
  // flag advance to include the new page.
  function mergePage(acc, page) {
    const byRepo = new Map(acc.repos.map(r => [r.repo, r]));
    for (const r of page.repos) {
      const existing = byRepo.get(r.repo);
      if (existing) existing.files.push(...r.files);
      else { acc.repos.push(r); byRepo.set(r.repo, r); }
    }
    acc.total += page.total;
    acc.files += page.files;
    acc.truncated = page.truncated;
  }

  // runSemanticSearch routes the query to the vector backend and renders a flat
  // ranked list. Lexical-only chrome (facets, the Files/Matches toggle) stays
  // hidden in this mode.
  async function runSemanticSearch() {
    const q = input.value.trim();
    if (!q) return;
    if (examplesEl) examplesEl.hidden = true;
    facetBar.innerHTML = '';
    document.getElementById('modeToggle').hidden = true;
    if (expandCtl) expandCtl.hidden = true;

    const params = new URLSearchParams({ q, k: String(SEM_FETCH_K) });
    applyScope(params);
    updateURL(q);

    status.textContent = 'Searching…';
    results.innerHTML = '';
    try {
      const res = await fetch('/api/semantic_search?' + params.toString());
      const data = await res.json();
      if (!res.ok) { status.innerHTML = '<span class="err">' + Webkit.escapeHtml(data.error || 'error') + '</span>'; return; }
      pushHistory(q);
      renderSemantic(data);
    } catch (err) {
      status.innerHTML = '<span class="err">' + Webkit.escapeHtml(String(err)) + '</span>';
    }
  }

  // renderSemantic shows the build hint when the index is unavailable, otherwise
  // stores the ranked hits and hands off to drawSemantic for facet + list paint.
  function renderSemantic(data) {
    if (!data.available) {
      status.textContent = '';
      facetBar.innerHTML = '';
      results.innerHTML = '<div class="more-note">' + Webkit.escapeHtml(data.note || 'Semantic index unavailable.') + '</div>';
      return;
    }
    lastSemHits = data.hits || [];
    semSelectedExts = new Set(); // fresh query: nothing selected = show all
    semShown = PAGE_SIZE;
    drawSemantic();
  }

  // runHybridSearch routes the query to /api/hybrid_search and renders a flat
  // ranked list. Facets and the Files/Matches toggle stay hidden in this mode.
  async function runHybridSearch() {
    const q = input.value.trim();
    if (!q) return;
    if (examplesEl) examplesEl.hidden = true;
    facetBar.innerHTML = '';
    document.getElementById('modeToggle').hidden = true;
    if (expandCtl) expandCtl.hidden = true;

    const params = new URLSearchParams({ q, limit: String(SEM_FETCH_K) });
    applyScope(params);
    updateURL(q);

    status.textContent = 'Searching…';
    results.innerHTML = '';
    try {
      const res = await fetch('/api/hybrid_search?' + params.toString());
      const data = await res.json();
      if (!res.ok) { status.innerHTML = '<span class="err">' + Webkit.escapeHtml(data.error || 'error') + '</span>'; return; }
      pushHistory(q);
      renderHybridResults(data);
    } catch (err) {
      status.innerHTML = '<span class="err">' + Webkit.escapeHtml(String(err)) + '</span>';
    }
  }

  // renderHybridResults shows the semantic build hint when the index is
  // unavailable (but still renders any lexical-only hits), then stores the
  // fused hits and hands off to drawHybrid for facet + paged list paint.
  function renderHybridResults(data) {
    lastHybHits = data.hits || [];
    hybSemAvailable = !!data.semantic_available;
    hybNote = !hybSemAvailable
      ? '<div class="more-note">' +
        Webkit.escapeHtml(data.note || 'Semantic index unavailable — showing lexical results only.') +
        '</div>'
      : '';
    hybSelectedExts = new Set(); // fresh query: nothing selected = show all
    hybShown = PAGE_SIZE;
    drawHybrid();
  }

  // hybridFacets groups fused hits by file extension (hybrid hits carry no
  // language), higithostst count first. '' means files without an extension.
  function hybridFacets(hits) {
    const idx = new Map();
    for (const h of hits) {
      const m = /\.[^./]+$/.exec(h.path || '');
      const key = m ? m[0].toLowerCase() : '';
      const e = idx.get(key) || { ext: key, label: key || 'no ext', files: 0 };
      e.files++;
      idx.set(key, e);
    }
    return Array.from(idx.values()).sort((a, b) => b.files - a.files);
  }

  // drawHybrid paints the extension facet bar (when 2+ extensions are present)
  // and the fused hit list, narrowed to the selection and paged like lexical.
  function drawHybrid() {
    const facets = hybridFacets(lastHybHits);
    if (facets.length >= 2) {
      facetBar.innerHTML = facetChipsHTML(facets, hybSelectedExts);
      bindFacetChips(facetBar, ext => {
        if (hybSelectedExts.has(ext)) hybSelectedExts.delete(ext);
        else hybSelectedExts.add(ext);
        hybShown = PAGE_SIZE; // filter change: restart paging
        drawHybrid();
      });
    } else {
      facetBar.innerHTML = '';
    }

    const narrowed = hybSelectedExts.size > 0 && hybSelectedExts.size < facets.length;
    const hits = narrowed
      ? lastHybHits.filter(h => {
          const m = /\.[^./]+$/.exec(h.path || '');
          return hybSelectedExts.has(m ? m[0].toLowerCase() : '');
        })
      : lastHybHits;

    if (hits.length === 0) {
      results.innerHTML = hybNote + '<div class="empty">No matches.</div>';
      status.textContent = '0 results';
      return;
    }
    status.textContent = hits.length + ' result' + (hits.length === 1 ? '' : 's') +
      (!hybSemAvailable ? ' (lexical only)' : '') +
      (narrowed ? ' · filtered from ' + lastHybHits.length : '') +
      (hits.length > hybShown ? ' · showing ' + hybShown : '');
    results.innerHTML = hybNote + pagedListHTML(hits.slice(0, hybShown).map(hybridHitBlock),
      hits.length - hybShown, 'result');
    bindLoadMore(() => { hybShown += PAGE_SIZE; drawHybrid(); });
  }

  // hybridHitBlock renders one fused hit: file path linked to source, fused
  // score, lex/sem rank badges, the lexical match line when present, and the
  // semantic snippet when present.
  function hybridHitBlock(h) {
    const loc = Webkit.escapeHtml(h.repo) + ' ' + Webkit.escapeHtml(h.path);
    const head = h.fileURL
      ? '<a class="file-path" href="' + Webkit.escapeHtml(h.fileURL) + '" target="_blank" rel="noopener">' + loc + '</a>'
      : '<span class="file-path">' + loc + '</span>';
    const score = '<span class="sem-score">' + (h.score != null ? h.score.toFixed(4) : '') + '</span>';
    const lexBadge = '<span class="host-badge">' + (h.lex_rank ? 'lex#' + h.lex_rank : 'lex:—') + '</span>';
    const semBadge = '<span class="host-badge">' + (h.sem_rank ? 'sem#' + h.sem_rank : 'sem:—') + '</span>';
    let body = '';
    if (h.lex_text) {
      body += '<div class="lines"><div class="line hit"><span class="ln">' + (h.lex_line || '') +
        '</span><span class="code">' + Webkit.escapeHtml(h.lex_text) + '</span></div></div>';
    }
    if (h.snippet) {
      body += '<pre class="sem-snippet">' + Webkit.escapeHtml(h.snippet) + '</pre>';
    }
    return '<div class="file-group sem-hit">' +
      '<div class="file-head">' + head +
      '<span class="file-actions">' + lexBadge + semBadge + score + '</span></div>' +
      body + '</div>';
  }

  // semanticFacets aggregates the ranked hits into per-language chips (one per
  // distinct hit language, counted by hits), higithostst count first. Language is
  // the natural axis here — semantic chunks are language-aware.
  function semanticFacets(hits) {
    const idx = new Map();
    for (const h of hits) {
      const key = (h.lang || '').toLowerCase();
      const e = idx.get(key) || { ext: key, label: h.lang || 'no lang', files: 0 };
      e.files++;
      idx.set(key, e);
    }
    return Array.from(idx.values()).sort((a, b) => b.files - a.files);
  }

  // drawSemantic paints the language facet bar (when 2+ languages are present)
  // and the ranked hit list, narrowed to the selected languages. An empty
  // selection shows everything; the count line flags an active filter.
  function drawSemantic() {
    const facets = semanticFacets(lastSemHits);
    if (facets.length >= 2) {
      facetBar.innerHTML = facetChipsHTML(facets, semSelectedExts);
      bindFacetChips(facetBar, ext => {
        if (semSelectedExts.has(ext)) semSelectedExts.delete(ext);
        else semSelectedExts.add(ext);
        semShown = PAGE_SIZE; // filter change: restart paging
        drawSemantic();
      });
    } else {
      facetBar.innerHTML = '';
    }

    const narrowed = semSelectedExts.size > 0 && semSelectedExts.size < facets.length;
    const hits = narrowed
      ? lastSemHits.filter(h => semSelectedExts.has((h.lang || '').toLowerCase()))
      : lastSemHits;

    if (hits.length === 0) {
      results.innerHTML = '<div class="empty">No matches.</div>';
      status.textContent = '0 results';
      return;
    }
    status.textContent = hits.length + ' result' + (hits.length === 1 ? '' : 's') +
      (narrowed ? ' · filtered from ' + lastSemHits.length : '') +
      (hits.length > semShown ? ' · showing ' + semShown : '');
    results.innerHTML = pagedListHTML(hits.slice(0, semShown).map(semanticHitBlock),
      hits.length - semShown, 'result');
    bindLoadMore(() => { semShown += PAGE_SIZE; drawSemantic(); });
  }

  // pagedListHTML joins rendered hit blocks and appends a "Load more" button
  // when `remaining` hits are still hidden. Shared by semantic and hybrid.
  function pagedListHTML(blocks, remaining, noun) {
    let html = blocks.join('');
    if (remaining > 0) {
      html += '<button type="button" class="load-more" id="loadMore">Load more — ' +
        remaining + ' more ' + noun + (remaining === 1 ? '' : 's') + '</button>';
    }
    return html;
  }

  // bindLoadMore wires the (single) Load more button rendered by pagedListHTML.
  function bindLoadMore(onMore) {
    const more = document.getElementById('loadMore');
    if (more) more.addEventListener('click', onMore);
  }

  // semanticHitBlock renders one hit: "repo path:start-end" (linked to source
  // like the lexical results), a score badge, the chunk kind, and the snippet.
  function semanticHitBlock(h) {
    const loc = Webkit.escapeHtml(h.repo) + ' ' + Webkit.escapeHtml(h.path) + ':' + h.start_line + '-' + h.end_line;
    const head = h.fileURL
      ? '<a class="file-path" href="' + Webkit.escapeHtml(h.fileURL) + '" target="_blank" rel="noopener">' + loc + '</a>'
      : '<span class="file-path">' + loc + '</span>';
    const kind = h.kind ? '<span class="host-badge">' + Webkit.escapeHtml(h.kind) + '</span>' : '';
    const score = '<span class="sem-score">' + (h.score != null ? h.score.toFixed(3) : '') + '</span>';
    const snippet = '<pre class="sem-snippet">' + Webkit.escapeHtml(h.snippet || '') + '</pre>';
    return '<div class="file-group sem-hit">' +
      '<div class="file-head">' + head +
      '<span class="file-actions">' + kind + score + '</span></div>' +
      snippet + '</div>';
  }

  // facetFileRegex turns the selected suffixes into one server-side file
  // regex: {".go", ".md"} → "(\.go$|\.md$)", and "" (no extension) becomes a
  // paths-whose-basename-has-no-dot alternative.
  function facetFileRegex(exts) {
    const parts = Array.from(exts).map(ext =>
      ext === '' ? '(^|/)[^.]+$' : ext.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$');
    return parts.length === 1 ? parts[0] : '(' + parts.join('|') + ')';
  }

  // facetChipsHTML renders the "filter" label plus one clickable chip per facet,
  // marking those in `selected` as active. Shared by the lexical and semantic
  // facet bars so both share the same select-to-show look and markup.
  function facetChipsHTML(facets, selected) {
    const chips = facets.map(f =>
      '<wk-badge variant="filter" role="button" tabindex="0"' +
      (selected.has(f.ext) ? ' active' : '') +
      ' data-ext="' + Webkit.escapeHtml(f.ext) + '">' + Webkit.escapeHtml(f.label) +
      '<span class="facet-count">' + f.files + '</span></wk-badge>').join('');
    return '<span class="label-min">filter</span>' + chips;
  }

  // bindFacetChips wires click/keyboard activation on each chip in `bar` to
  // `onToggle(ext)`.
  function bindFacetChips(bar, onToggle) {
    bar.querySelectorAll('[data-ext]').forEach(c => {
      const fire = () => onToggle(c.getAttribute('data-ext'));
      c.addEventListener('click', fire);
      c.addEventListener('keydown', e => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); fire(); }
      });
    });
  }

  // renderFacets draws one clickable chip per file suffix in the baseline
  // (unfiltered) result set. Nothing is selected by default (= show all);
  // clicking a chip selects it and narrows the results to the selected
  // suffix(es). Clicking the last selected chip clears the filter (show all).
  function renderFacets() {
    if (baselineFacets.length < 2) {
      facetBar.innerHTML = '';
      return;
    }
    facetBar.innerHTML = facetChipsHTML(baselineFacets, selectedExts);
    bindFacetChips(facetBar, ext => {
      if (selectedExts.has(ext)) selectedExts.delete(ext);
      else selectedExts.add(ext);
      runSearch();
    });
  }

  // renderPage draws the first `shown` file blocks across repos (in order),
  // grouping consecutive files under their repo header, plus a "Load more"
  // control and a status line that flags server-side truncation.
  function renderPage() {
    const data = lastData;
    if (!data || data.repos.length === 0) {
      results.innerHTML = '<div class="empty">No matches.</div>';
      status.textContent = data ? statusText(data, 0) : '';
      return;
    }

    let budget = shown;
    let rendered = 0;
    const html = [];
    for (const repo of data.repos) {
      if (budget <= 0) break;
      const slice = repo.files.slice(0, budget);
      if (slice.length === 0) break;
      budget -= slice.length;
      rendered += slice.length;
      const count = slice.reduce((a, f) => a + f.matches.length, 0);
      const host = repo.host ? '<span class="host-badge">' + Webkit.escapeHtml(repo.host) + '</span>' : '';
      // Copy the `repo <org/name>` shell one-liner: `repo` with a single match
      // cd's straight into that repo, so this is the fastest jump from results.
      const repoCmd = 'repo ' + repo.repo;
      const copyRepo = '<button class="mini-btn js-copy" data-copy="' + Webkit.escapeHtml(repoCmd) +
        '" title="Copy &quot;' + Webkit.escapeHtml(repoCmd) + '&quot;">copy repo</button>';
      const files = slice.map(f => fileBlock(repo, f, lastTerms, data.mode)).join('');
      html.push('<div class="repo-group">' +
        '<div class="repo-head">' + Webkit.escapeHtml(repo.repo) + host + copyRepo +
        '<span class="count">' + count + ' match' + (count === 1 ? '' : 'es') + '</span></div>' +
        files + '</div>');
    }

    // Two-level "Load more": reveal more already-fetched files client-side
    // (remaining > 0), otherwise fetch the next server page when the backend
    // reported more results (truncated). Either way it is one button.
    const remaining = data.files - rendered;
    if (remaining > 0) {
      html.push('<button type="button" class="load-more" id="loadMore">Load more — ' +
        remaining + ' more file' + (remaining === 1 ? '' : 's') + '</button>');
    } else if (data.truncated) {
      html.push('<button type="button" class="load-more" id="loadMore">Load more results</button>');
    }

    results.innerHTML = html.join('');
    status.textContent = statusText(data, rendered);

    const more = document.getElementById('loadMore');
    if (more) more.addEventListener('click', () => {
      if (shown < data.files) { shown += PAGE_SIZE; renderPage(); }
      else if (data.truncated) { fetchPage(false); }
    });
    results.querySelectorAll('.js-expand').forEach(btn => btn.addEventListener('click', () => expand(btn)));
    bindCopyButtons(results);
  }

  function statusText(data, rendered) {
    const base = data.total + ' match' + (data.total === 1 ? '' : 'es') +
      ' in ' + data.files + ' file' + (data.files === 1 ? '' : 's') +
      ' across ' + data.repos.length + ' repo' + (data.repos.length === 1 ? '' : 's') +
      (data.truncated ? '+ (more available — Load more)' : '');
    if (rendered > 0 && rendered < data.files) return base + ' · showing ' + rendered;
    return base;
  }

  function fileBlock(repo, f, terms, mode) {
    const link = f.fileURL
      ? '<a class="file-path" href="' + Webkit.escapeHtml(f.fileURL) + '" target="_blank" rel="noopener">' + Webkit.escapeHtml(f.file) + '</a>'
      : '<span class="file-path">' + Webkit.escapeHtml(f.file) + '</span>';
    const firstLine = f.matches.length ? f.matches[0].line : 1;
    const lines = f.matches.map(m => matchLines(m, terms, mode)).join('');
    // Copy the absolute local path (ready to paste into an editor/terminal).
    const copyPath = f.localPath
      ? '<button class="mini-btn js-copy" data-copy="' + Webkit.escapeHtml(f.localPath) + '" title="Copy local path">copy path</button>'
      : '';
    // Open the file on the git host at the matched line (first match URL, which
    // carries the #L fragment; fall back to the file-top URL).
    const webURL = (f.matches[0] && f.matches[0].remoteURL) || f.fileURL;
    const openWeb = webURL
      ? '<a class="mini-btn" href="' + Webkit.escapeHtml(webURL) + '" target="_blank" rel="noopener" title="Open on web">open web</a>'
      : '';
    return '<div class="file-group">' +
      '<div class="file-head">' + link +
      '<span class="file-actions">' + copyPath + openWeb +
      '<button class="mini-btn js-expand" data-repo="' + Webkit.escapeHtml(repo.repo) + '" data-file="' + Webkit.escapeHtml(f.file) + '" data-line="' + firstLine + '">expand</button>' +
      '</span></div>' +
      '<div class="lines">' + lines + '</div>' +
      '<div class="expand-target"></div>' +
      '</div>';
  }

  function matchLines(m, terms, mode) {
    let html = '';
    if (mode === 'content' && m.before) {
      html += m.before.replace(/\n$/, '').split('\n').map(t =>
        '<div class="line ctx"><span class="ln"></span><span class="code">' + Webkit.escapeHtml(t) + '</span></div>').join('');
    }
    html += '<div class="line hit"><span class="ln">' + m.line + '</span><span class="code">' +
      applyHighlight(Webkit.escapeHtml(m.text), terms) + '</span></div>';
    if (mode === 'content' && m.after) {
      html += m.after.replace(/\n$/, '').split('\n').map(t =>
        '<div class="line ctx"><span class="ln"></span><span class="code">' + Webkit.escapeHtml(t) + '</span></div>').join('');
    }
    return html;
  }

  // Expand context size: how many lines above/below the match the expand
  // button fetches. Persisted in localStorage, adjusted by the ± control in
  // the status row (5-line steps, clamped to 5..100).
  const EXPAND_KEY = 'csl-expand-lines';
  function expandLines() {
    const n = parseInt(localStorage.getItem(EXPAND_KEY), 10);
    return isNaN(n) ? 15 : Math.max(5, Math.min(100, n));
  }
  const expandCtl = document.getElementById('expandCtl');
  const expandVal = document.getElementById('expandVal');
  function renderExpandCtl() {
    if (expandVal) expandVal.textContent = '±' + expandLines();
  }
  if (expandCtl) {
    renderExpandCtl();
    expandCtl.querySelectorAll('button').forEach(b => b.addEventListener('click', () => {
      const next = expandLines() + (b.dataset.step === '+' ? 5 : -5);
      localStorage.setItem(EXPAND_KEY, String(Math.max(5, Math.min(100, next))));
      renderExpandCtl();
    }));
  }

  async function expand(btn) {
    const target = btn.closest('.file-group').querySelector('.expand-target');
    if (target.dataset.open === '1') { target.innerHTML = ''; target.dataset.open = '0'; btn.textContent = 'expand'; return; }
    const line = parseInt(btn.dataset.line, 10) || 1;
    const pad = expandLines();
    const start = Math.max(1, line - pad), end = line + pad;
    const params = new URLSearchParams({ repo: btn.dataset.repo, file: btn.dataset.file, start, end });
    btn.textContent = '…';
    try {
      const res = await fetch('/api/read?' + params.toString());
      const data = await res.json();
      if (!res.ok) {
        target.innerHTML = '<div class="line"><span class="code err">' + Webkit.escapeHtml(data.error || 'error') + '</span></div>';
      } else {
        target.innerHTML = '<div class="lines">' + data.lines.map(l =>
          '<div class="line ctx' + (l.number === line ? ' hit' : '') + '"><span class="ln">' + l.number +
          '</span><span class="code">' + Webkit.escapeHtml(l.text) + '</span></div>').join('') + '</div>';
      }
      target.dataset.open = '1';
      btn.textContent = 'collapse';
    } catch (err) {
      target.innerHTML = '<div class="line"><span class="code err">' + Webkit.escapeHtml(String(err)) + '</span></div>';
    }
  }

  // Populate the language dropdown (static list) and start the async repo load.
  if (langSelect) {
    LANG_OPTIONS.forEach(lang => {
      const opt = document.createElement('option');
      opt.value = lang;
      opt.textContent = lang;
      langSelect.appendChild(opt);
    });
  }
  loadRepoOptions();

  renderHistory();
  renderExamples();

  // Resolve which modes are disabled before restoring from the URL, so a
  // ?kind=semantic deep link can't select a mode the backend won't serve.
  await loadCapabilities();

  // Restore state from the URL (?q, ?kind, ?repo, ?lang) so searches in any
  // mode are shareable deep links; then run the query if one came along.
  const urlParams = new URLSearchParams(location.search);
  const urlKind = urlParams.get('kind');
  if ((urlKind === 'semantic' || urlKind === 'hybrid') && semanticEnabled) applyKind(urlKind);
  ensureOption(repoSelect, urlParams.get('repo'));
  ensureOption(langSelect, urlParams.get('lang'));
  const initial = urlParams.get('q');
  if (initial) { input.value = initial; renderQueryHl(); runActiveSearch(); }
  input.focus();
}

initSearchPage();
