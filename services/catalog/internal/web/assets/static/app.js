'use strict';

// ── Theme ────────────────────────────────────────────────────────────────
// The webkit <wk-header> owns the theme toggle (persists `webkit-theme`, sets
// data-theme, and dispatches `wk-themechange` on document). We only listen to
// recolor the Cytoscape graph, which reads data-theme via graphColors().
document.addEventListener('wk-themechange', function () { buildGraph(); });

// ── Helpers ──────────────────────────────────────────────────────────────
// DOM building and HTML escaping use the shared webkit runtime helpers
// (Webkit.el / Webkit.escapeHtml, loaded via /webkit/webkit.js before this
// script). Both are behaviourally identical to the local copies they replaced.

async function api(path, opts) {
  const res = await fetch(path, opts);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `request failed (${res.status})`);
  return body;
}

function toast(msg, kind = 'ok') {
  const host = document.getElementById('toasts');
  const t = Webkit.el('wk-toast', { variant: kind }, msg);
  host.appendChild(t);
  setTimeout(() => { t.style.opacity = '0'; t.style.transition = 'opacity .3s'; setTimeout(() => t.remove(), 300); }, 3200);
}

const view = document.getElementById('view');

// ── Card rendering ───────────────────────────────────────────────────────
function entityCard(e, i) {
  const kindClass = e.kind === 'System' ? 'k-system' : 'k-component';
  const href = (e.kind === 'System' ? '/systems/' : '/components/') + encodeURIComponent(e.metadata.name);
  const tags = (e.metadata.tags || []).slice(0, 4).map((t) => Webkit.el('span', { class: 'tag' }, t));
  return Webkit.el('a', { class: `wk-card ${kindClass}`, href, style: `animation-delay:${Math.min(i * 35, 400)}ms`, onclick: navigate }, [
    Webkit.el('div', { class: 'card-top' }, [
      Webkit.el('span', { class: 'card-name' }, e.metadata.name),
      Webkit.el('wk-badge', { variant: 'accent' }, e.kind),
    ]),
    Webkit.el('div', { class: 'card-desc' }, e.metadata.description || ''),
    Webkit.el('div', { class: 'card-meta' }, [
      Webkit.el('span', { class: 'owner-pill', html: `owner&nbsp;<b>${Webkit.escapeHtml(e.spec.owner || '—')}</b>` }),
      ...tags,
    ]),
  ]);
}

// filterBadge builds a clickable <wk-badge variant="filter"> filter pill.
// wk-badge is not a <button>, so we add role/tabindex and keyboard activation
// to keep it operable (the prior .chip used native <button> elements).
function filterBadge(active, onActivate, label) {
  const attrs = {
    variant: 'filter', role: 'button', tabindex: '0',
    onclick: onActivate,
    onkeydown: (ev) => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); onActivate(); } },
  };
  if (active) attrs.active = '';
  return Webkit.el('wk-badge', attrs, label);
}

// repoLink builds an outbound link to an entity's source on its git remote.
// Returns null when the entity has no resolvable remote, so callers can spread
// it into Webkit.el() children unconditionally.
function repoLink(url) {
  if (!url) return null;
  const label = url.replace(/^https?:\/\//, '').replace(/\/(tree|blob)\/HEAD\/.*$/, '');
  const icon = '<svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>';
  const a = Webkit.el('a', { class: 'repo-link', href: url, target: '_blank', rel: 'noopener' });
  a.innerHTML = icon + '<span>' + Webkit.escapeHtml(label) + '</span>';
  return a;
}

// ── List view ────────────────────────────────────────────────────────────
const state = { q: '', owner: '', kind: '', owners: [] };

async function renderList() {
  view.innerHTML = '';
  graphData = null;
  view.appendChild(Webkit.el('section', { class: 'hero' }, [
    Webkit.el('p', { class: 'hero-sub' }, 'A catalog of your tools, apps and services. Search by name or owner; open any entity for its metadata.'),
  ]));

  const search = Webkit.el('wk-search', {}, [
    Webkit.el('input', { type: 'search', placeholder: 'Search names, descriptions, tags…', value: state.q,
      oninput: debounce((ev) => { state.q = ev.target.value.trim(); refreshResults(); }, 160) }),
  ]);
  // Leading magnifier icon must be a direct child <svg> (wk-search :has(> svg)).
  search.insertAdjacentHTML('afterbegin', '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></svg>');

  const kindFilters = ['', 'System', 'Component'].map((k) =>
    filterBadge(state.kind === k, () => { state.kind = k; renderListChrome(); refreshResults(); }, k === '' ? 'All' : k + 's'));

  if (!state.owners.length) {
    try { state.owners = await api('/api/owners'); } catch (_) { /* ignore */ }
  }
  const ownerFilters = [filterBadge(state.owner === '', () => { state.owner = ''; renderListChrome(); refreshResults(); }, 'Anyone')]
    .concat(state.owners.map((o) =>
      filterBadge(state.owner === o, () => { state.owner = o; renderListChrome(); refreshResults(); }, o)));

  const controls = Webkit.el('div', { class: 'controls' }, [
    search,
    Webkit.el('div', { class: 'filters' }, [Webkit.el('span', { class: 'filters-label' }, 'Kind'), ...kindFilters]),
    Webkit.el('div', { class: 'filters' }, [Webkit.el('span', { class: 'filters-label' }, 'Owner'), ...ownerFilters]),
  ]);
  view.appendChild(controls);
  view.appendChild(Webkit.el('div', { id: 'results' }));
  refreshResults();
}

// Re-render only the filter chrome (chips active states) without refetching.
function renderListChrome() { renderList(); }

async function refreshResults() {
  const params = new URLSearchParams();
  if (state.q) params.set('q', state.q);
  if (state.owner) params.set('owner', state.owner);
  if (state.kind) params.set('kind', state.kind);
  let entities = [];
  try { entities = await api('/api/search?' + params.toString()); } catch (e) { toast(e.message, 'err'); }

  const results = document.getElementById('results');
  if (!results) return;
  results.innerHTML = '';

  if (!entities.length) {
    results.appendChild(Webkit.el('div', { class: 'empty' }, [Webkit.el('div', { class: 'big' }, 'Nothing here'), Webkit.el('div', {}, 'No entities match your filters.')]));
    return;
  }

  const systems = entities.filter((e) => e.kind === 'System');
  const components = entities.filter((e) => e.kind === 'Component');
  if (systems.length) results.appendChild(sectionGrid('Systems', systems));
  if (components.length) results.appendChild(sectionGrid('Components', components));
}

function sectionGrid(title, entities) {
  return Webkit.el('section', { class: 'section' }, [
    Webkit.el('div', { class: 'section-head' }, [Webkit.el('h2', {}, title), Webkit.el('span', { class: 'count' }, String(entities.length))]),
    Webkit.el('div', { class: 'grid' }, entities.map((e, i) => entityCard(e, i))),
  ]);
}

// ── System detail ────────────────────────────────────────────────────────
async function renderSystem(name) {
  view.innerHTML = '';
  let data;
  try { data = await api('/api/systems/' + encodeURIComponent(name)); }
  catch (e) { return renderNotFound(e.message); }
  const sys = data.system;

  view.appendChild(backLink());
  view.appendChild(Webkit.el('header', { class: 'detail-head', style: '--accent:var(--sys)' }, [
    Webkit.el('div', { class: 'detail-kind' }, 'System'),
    Webkit.el('h1', { class: 'detail-title' }, sys.metadata.name),
    sys.metadata.description ? Webkit.el('p', { class: 'detail-desc' }, sys.metadata.description) : null,
    Webkit.el('div', { class: 'detail-metarow' }, [
      Webkit.el('span', { class: 'owner-pill', html: `owner&nbsp;<b>${Webkit.escapeHtml(sys.spec.owner || '—')}</b>` }),
      ...(sys.metadata.tags || []).map((t) => Webkit.el('span', { class: 'tag' }, t)),
      repoLink(sys.repoURL),
    ]),
  ]));

  view.appendChild(metaTable(sys));

  const comps = data.components || [];

  if (comps.length) {
    graphData = { sys, comps };
    view.appendChild(Webkit.el('section', { class: 'section', style: 'margin-top:2.5rem' }, [
      Webkit.el('div', { class: 'section-head' }, [Webkit.el('h2', {}, 'Dependency graph'), Webkit.el('span', { class: 'count' }, String(comps.length))]),
      Webkit.el('div', { class: 'cy-wrapper' }, [Webkit.el('div', { class: 'cy-container', id: 'cy-graph' })]),
      Webkit.el('div', { class: 'cy-hint' }, 'Tap a component to open it · scroll to zoom · drag to pan'),
    ]));
  } else {
    graphData = null;
  }

  view.appendChild(Webkit.el('section', { class: 'section', style: 'margin-top:2.5rem' }, [
    Webkit.el('div', { class: 'section-head' }, [Webkit.el('h2', {}, 'Components'), Webkit.el('span', { class: 'count' }, String(comps.length))]),
    comps.length
      ? Webkit.el('div', { class: 'grid' }, comps.map((e, i) => entityCard(e, i)))
      : Webkit.el('div', { class: 'empty' }, [Webkit.el('div', {}, 'No components in this system yet.')]),
  ]));

  buildGraph();
}

// ── Dependency graph (Cytoscape) ───────────────────────────────────────────
let cy = null;
// graphData holds the current System page's { sys, comps } so the graph can be
// rebuilt on theme toggle; null on pages without a graph.
let graphData = null;

// graphColors mirrors the present tool's terracotta/cream palette, per theme.
function graphColors() {
  const dark = document.documentElement.getAttribute('data-theme') === 'dark';
  return dark
    ? { center: '#3A2A20', centerBorder: '#E8956A', leaf: '#252320', leafBorder: '#4A453D', edge: '#5A554C', text: '#EDEAE3' }
    : { center: '#FFF5F0', centerBorder: '#C4704B', leaf: '#FFFFFF', leafBorder: '#D8D4CD', edge: '#CFCAC2', text: '#2B2926' };
}

// buildGraph (re)renders the System→Components graph into #cy-graph. No-op when
// the page has no graph container or Cytoscape failed to load.
function buildGraph() {
  if (typeof cytoscape === 'undefined' || !graphData) return;
  const container = document.getElementById('cy-graph');
  if (!container) return;
  const { sys, comps } = graphData;
  const c = graphColors();
  const sid = 'sys:' + sys.metadata.name;
  const elements = [
    { data: { id: sid, label: sys.metadata.name, role: 'center' } },
    ...comps.map((co) => ({ data: { id: 'comp:' + co.metadata.name, label: co.metadata.name, role: 'leaf', name: co.metadata.name } })),
    ...comps.map((co) => ({ data: { source: sid, target: 'comp:' + co.metadata.name } })),
  ];
  if (cy) { cy.destroy(); cy = null; }
  cy = cytoscape({
    container,
    elements,
    style: [
      { selector: 'node', style: {
        label: 'data(label)', 'font-family': "'Fira Code', monospace", 'font-size': '12px', color: c.text,
        'text-valign': 'center', 'text-halign': 'center', shape: 'round-rectangle',
        width: 'label', height: 'label', padding: '11px',
        'background-color': c.leaf, 'border-width': 1.5, 'border-color': c.leafBorder } },
      { selector: 'node[role="center"]', style: { 'background-color': c.center, 'border-color': c.centerBorder, 'border-width': 2, 'font-weight': 'bold' } },
      { selector: 'node[role="leaf"]', style: { cursor: 'pointer' } },
      { selector: 'edge', style: {
        width: 1.5, 'line-color': c.edge, 'curve-style': 'bezier',
        'target-arrow-shape': 'triangle', 'target-arrow-color': c.edge, 'arrow-scale': 0.85 } },
    ],
    layout: { name: 'breadthfirst', directed: true, roots: [sid], padding: 26, spacingFactor: 1.1 },
    minZoom: 0.4, maxZoom: 2.5, wheelSensitivity: 0.2,
  });
  cy.on('tap', 'node[role="leaf"]', (ev) => {
    const n = ev.target.data('name');
    history.pushState({}, '', '/components/' + encodeURIComponent(n));
    window.scrollTo(0, 0);
    route();
  });
}

// ── Component detail ─────────────────────────────────────────────────────
async function renderComponent(name) {
  view.innerHTML = '';
  graphData = null;
  let comp;
  try { comp = await api('/api/components/' + encodeURIComponent(name)); }
  catch (e) { return renderNotFound(e.message); }

  view.appendChild(backLink());
  view.appendChild(Webkit.el('header', { class: 'detail-head', style: '--accent:var(--comp)' }, [
    Webkit.el('div', { class: 'detail-kind' }, 'Component'),
    Webkit.el('h1', { class: 'detail-title' }, comp.metadata.name),
    comp.metadata.description ? Webkit.el('p', { class: 'detail-desc' }, comp.metadata.description) : null,
    Webkit.el('div', { class: 'detail-metarow' }, [
      Webkit.el('span', { class: 'owner-pill', html: `owner&nbsp;<b>${Webkit.escapeHtml(comp.spec.owner || '—')}</b>` }),
      ...(comp.metadata.tags || []).map((t) => Webkit.el('span', { class: 'tag' }, t)),
      repoLink(comp.repoURL),
    ]),
  ]));
  view.appendChild(metaTable(comp));
}

function metaTable(e) {
  const rows = [];
  const add = (k, v) => { if (v) rows.push([k, v]); };
  add('apiVersion', e.apiVersion);
  add('kind', e.kind);
  add('name', e.metadata.name);
  add('type', e.spec.type);
  add('lifecycle', e.spec.lifecycle);
  add('owner', e.spec.owner);
  if (e.spec.system) {
    rows.push(['system', Webkit.el('a', { href: '/systems/' + encodeURIComponent(e.spec.system), onclick: navigate }, e.spec.system)]);
  }
  if (e.metadata.tags && e.metadata.tags.length) rows.push(['tags', e.metadata.tags.join(', ')]);
  if (e.repoURL) rows.push(['repo', repoLink(e.repoURL)]);
  add('source', e.sourcePath);

  const kv = Webkit.el('wk-kv', { variant: 'card' });
  for (const [k, v] of rows) {
    kv.appendChild(Webkit.el('wk-kv-row', {}, [
      Webkit.el('wk-kv-label', {}, k),
      Webkit.el('wk-kv-value', {}, typeof v === 'string' ? v : [v]),
    ]));
  }
  return kv;
}

function backLink() {
  return Webkit.el('a', { class: 'back', href: '/', onclick: navigate, html: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M12 19l-7-7 7-7"/></svg> back to catalog' });
}

function renderNotFound(msg) {
  view.innerHTML = '';
  view.appendChild(backLink());
  view.appendChild(Webkit.el('div', { class: 'empty' }, [Webkit.el('div', { class: 'big' }, 'Not found'), Webkit.el('div', {}, msg)]));
}

// ── Router ───────────────────────────────────────────────────────────────
function route() {
  const path = location.pathname;
  let m;
  if ((m = path.match(/^\/systems\/(.+)$/))) renderSystem(decodeURIComponent(m[1]));
  else if ((m = path.match(/^\/components\/(.+)$/))) renderComponent(decodeURIComponent(m[1]));
  else renderList();
}

function navigate(ev) {
  const a = ev.currentTarget;
  const href = a.getAttribute('href');
  if (!href || href.startsWith('http')) return;
  ev.preventDefault();
  history.pushState({}, '', href);
  window.scrollTo(0, 0);
  route();
}

window.addEventListener('popstate', route);

// ── Refresh ──────────────────────────────────────────────────────────────
function wireRefresh() {
  const btn = document.getElementById('refreshBtn');
  btn.addEventListener('click', async () => {
    const icon = btn.querySelector('svg');
    icon.classList.add('spin');
    try {
      const r = await api('/api/refresh', { method: 'POST' });
      toast(`Refreshed — ${r.systems} systems, ${r.components} components`);
      state.owners = [];
      route();
    } catch (e) { toast(e.message, 'err'); }
    finally { icon.classList.remove('spin'); }
  });
}

// ── Add modal ────────────────────────────────────────────────────────────
function wireModal() {
  const modal = document.getElementById('modal');
  const form = document.getElementById('addForm');
  const open = () => { modal.hidden = false; form.querySelector('input[name=name]').focus(); };
  const close = () => { modal.hidden = true; form.reset(); setKind('Component'); };

  document.getElementById('addBtn').addEventListener('click', open);
  document.getElementById('modalClose').addEventListener('click', close);
  document.getElementById('modalCancel').addEventListener('click', close);
  modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !modal.hidden) close(); });

  let kind = 'Component';
  function setKind(k) {
    kind = k;
    document.querySelectorAll('#kindSeg button').forEach((b) => b.classList.toggle('active', b.dataset.kind === k));
    // System has no system/type/lifecycle fields.
    document.querySelectorAll('.field-component').forEach((f) => { f.style.display = k === 'System' ? 'none' : ''; });
  }
  document.querySelectorAll('#kindSeg button').forEach((b) => b.addEventListener('click', () => setKind(b.dataset.kind)));

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(form);
    const tags = (fd.get('tags') || '').toString().split(',').map((s) => s.trim()).filter(Boolean);
    const payload = {
      kind, name: fd.get('name'), owner: fd.get('owner'), description: fd.get('description'),
      tags, dir: fd.get('dir'),
    };
    if (kind === 'Component') {
      payload.system = fd.get('system');
      payload.type = fd.get('type');
      payload.lifecycle = fd.get('lifecycle');
    }
    try {
      const r = await api('/api/entities', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      toast(`Created ${payload.name} → ${r.path}`);
      close();
      state.owners = [];
      history.pushState({}, '', (kind === 'System' ? '/systems/' : '/components/') + encodeURIComponent(payload.name));
      route();
    } catch (err) { toast(err.message, 'err'); }
  });
}

function debounce(fn, ms) {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

// ── Boot ─────────────────────────────────────────────────────────────────
wireRefresh();
wireModal();
route();
