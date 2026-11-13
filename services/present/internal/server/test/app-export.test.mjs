// app.js's export branch (renderExport), run via `node --test`. app.js is a
// classic browser script, so it runs here against a small fake DOM holding
// what an exported file holds: <html data-present-mode="export">, the body
// in #root, and the page data in #present-export. No library is defined,
// which is the file opened with cdn.jsdelivr.net out of reach. The same fake
// DOM, served from a page URL, also covers the state a failed load shows.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const appSrc = readFileSync(new URL('../app.js', import.meta.url), 'utf8');

// The selectors the export path asks for beyond a bare tag, class, or id.
const SELECTORS = {
  'a[href]:not([href^="#"])': (e) =>
    e.tagName === 'A' && e.hasAttribute('href') && !e.getAttribute('href').startsWith('#'),
};

function matcher(sel) {
  if (SELECTORS[sel]) return SELECTORS[sel];
  let m;
  if ((m = /^\.([\w-]+)$/.exec(sel))) return (e) => e.classList.contains(m[1]);
  if ((m = /^#([\w-]+)$/.exec(sel))) return (e) => e.id === m[1];
  if ((m = /^([a-z][\w-]*)$/i.exec(sel))) return (e) => e.tagName === m[1].toUpperCase();
  throw new Error('fake DOM: unsupported selector ' + sel);
}

// El is as much of an element as the export path touches.
class El {
  constructor(tag, attrs = {}, children = [], text = '') {
    this.tagName = tag.toUpperCase();
    this.attrs = { ...attrs };
    this.children = [];
    this.parentNode = null;
    this.textContent = text;
    children.forEach((c) => this.appendChild(c));
  }
  get id() { return this.attrs.id || ''; }
  set id(v) { this.attrs.id = v; }
  get className() { return this.attrs.class || ''; }
  set className(v) { this.attrs.class = v; }
  get classList() {
    const list = () => (this.attrs.class || '').split(/\s+/).filter(Boolean);
    const set = (names) => { this.attrs.class = names.join(' '); };
    return {
      contains: (c) => list().includes(c),
      add: (...cs) => set([...new Set([...list(), ...cs])]),
      remove: (...cs) => set(list().filter((c) => !cs.includes(c))),
      toggle: (c, force) => {
        const on = force === undefined ? !list().includes(c) : force;
        set(on ? [...new Set([...list(), c])] : list().filter((x) => x !== c));
        return on;
      },
    };
  }
  getAttribute(n) { return n in this.attrs ? this.attrs[n] : null; }
  setAttribute(n, v) { this.attrs[n] = String(v); }
  hasAttribute(n) { return n in this.attrs; }
  removeAttribute(n) { delete this.attrs[n]; }
  appendChild(c) {
    if (c.parentNode) c.remove();
    c.parentNode = this;
    this.children.push(c);
    return c;
  }
  remove() {
    const p = this.parentNode;
    if (!p) return;
    p.children.splice(p.children.indexOf(this), 1);
    this.parentNode = null;
  }
  addEventListener() {}
  descendants() { return this.children.flatMap((c) => [c, ...c.descendants()]); }
  querySelectorAll(sel) { return this.descendants().filter(matcher(sel)); }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
}

function chartBlock(kind) {
  return new El('div', { class: 'present-chart' }, [
    new El('div', { class: 'present-chart-canvas' }, [new El('canvas')]),
    new El('script', { type: 'application/json', class: 'chart-spec' }, [], JSON.stringify({ kind })),
  ]);
}

// exportedPage builds the fake document of an exported brief holding
// blocks, with extra merged into its page data, runs app.js over it, and
// returns the document, every element app.js created, and every fetch it
// made.
function exportedPage(blocks, extra = {}) {
  const brief = new El('div', { class: 'brief' }, blocks);
  const data = {
    id: 'p1', title: 'Exported', version: 3, view: 'brief', has_graph: false, graph: '', references: [], ...extra,
  };
  const html = new El('html', { 'data-present-mode': 'export' }, [
    new El('body', {}, [
      new El('main', { id: 'root' }, [brief]),
      new El('script', { type: 'application/json', id: 'present-export' }, [], JSON.stringify(data)),
    ]),
  ]);
  const listeners = {};
  const created = [];
  const document = {
    documentElement: html,
    body: html.children[0],
    title: '',
    getElementById: (id) => html.descendants().find((e) => e.id === id) || null,
    querySelector: (sel) => html.querySelector(sel),
    querySelectorAll: (sel) => html.querySelectorAll(sel),
    createElement: (tag) => { const el = new El(tag); created.push(el); return el; },
    addEventListener: (type, fn) => { (listeners[type] ||= []).push(fn); },
  };
  const window = { addEventListener() {} };
  const fetches = [];
  const fetch = (url) => { fetches.push(url); return new Promise(() => {}); };
  new Function('window', 'document', 'fetch', appSrc)(window, document, fetch);
  return { document, brief, created, fetches, listeners };
}

function note(block) {
  const n = block.querySelector('.present-chart-note');
  return n ? n.textContent : null;
}

test('export: every chart says Chart.js or d3 did not load from the CDN', () => {
  const kinds = ['bar', 'line', 'sankey', 'doughnut', 'ribbon'];
  const blocks = kinds.map(chartBlock);
  const { document, fetches, listeners } = exportedPage(blocks);
  kinds.forEach((kind, i) => {
    const text = note(blocks[i]);
    assert.ok(text, kind + ' chart shows no note');
    assert.match(text, /from cdn\.jsdelivr\.net, which did not load: open this page online\./, kind);
    assert.match(text, kind === 'ribbon' ? /^Ribbon needs the d3 asset/ : /^Chart needs the chart\.js asset/, kind);
  });
  assert.equal(document.title, 'Exported');
  assert.deepEqual(fetches, [], 'an exported page asks no server for anything');
  assert.ok((listeners['wk-themechange'] || []).length >= 2, 'the brief follows the theme toggle');
});

test('export: a diagram without ELK and d3 says so, and the prose stays', () => {
  const diagram = new El('div', { class: 'present-diagram' }, [
    new El('script', { type: 'application/json', class: 'diagram-spec' }, [], '{"nodes":[]}'),
  ]);
  const prose = new El('p', {}, [], 'The resolver kept a dead upstream.');
  const { brief, fetches } = exportedPage([prose, diagram]);
  assert.match(note(diagram), /^Diagram needs the elk and d3 assets from cdn\.jsdelivr\.net/);
  assert.equal(brief.children[0], prose);
  assert.equal(prose.textContent, 'The resolver kept a dead upstream.');
  assert.deepEqual(fetches, []);
});

test('export: links in the body open in a new tab, fragment links stay', () => {
  const out = new El('a', { href: 'https://example.com/runbook' });
  const frag = new El('a', { href: '#references' });
  exportedPage([new El('p', {}, [out, frag])]);
  assert.equal(out.target, '_blank');
  assert.equal(out.rel, 'noopener');
  assert.equal(frag.target, undefined);
});

test('export: no read-aloud control or other server call, even when the page data names a speak URL', () => {
  const section = new El('wk-section', {}, [new El('p', {}, [], 'Prose to read.')]);
  const { document, created, fetches } = exportedPage([section], {
    speak_url: 'http://localhost:7423/speak',
    share: { enabled: true },
    deck_control: true,
    has_deck: true,
  });
  assert.equal(document.querySelector('wk-read-aloud'), null, 'the TTS control is in the page');
  assert.deepEqual(created.filter((e) => e.tagName === 'WK-READ-ALOUD'), [], 'app.js built a TTS control');
  assert.deepEqual(fetches, [], 'an exported page asks no server for anything');
});

// servedPage runs app.js the way a served shell does: no export island, the
// page URL in location, and /api/p/{id} answering status. Webkit.el builds
// nodes as the kit's does. It resolves to #root once the load has settled.
async function servedPage(path, status, mode) {
  const root = new El('main', { id: 'root' });
  const attrs = mode ? { 'data-present-mode': mode } : {};
  const html = new El('html', attrs, [new El('body', {}, [root])]);
  const document = {
    documentElement: html,
    body: html.children[0],
    title: '',
    getElementById: (id) => html.descendants().find((e) => e.id === id) || null,
    querySelector: (sel) => html.querySelector(sel),
    querySelectorAll: (sel) => html.querySelectorAll(sel),
    createElement: (tag) => new El(tag),
    createTextNode: (text) => new El('#text', {}, [], text),
    addEventListener() {},
  };
  const Webkit = {
    el(tag, attrs = {}, children = []) {
      const node = document.createElement(tag);
      for (const [k, v] of Object.entries(attrs)) {
        if (k === 'class') node.className = String(v);
        else if (v != null) node.setAttribute(k, String(v));
      }
      for (const c of [].concat(children)) {
        if (c == null) continue;
        node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
      }
      return node;
    },
  };
  const fetch = () => Promise.resolve({ ok: false, status, json: () => Promise.resolve({}) });
  new Function('window', 'document', 'fetch', 'location', 'Webkit', appSrc)(
    { addEventListener() {} }, document, fetch, { pathname: path }, Webkit,
  );
  await new Promise((resolve) => setTimeout(resolve, 0));
  return root;
}

function textOf(e) {
  return e.textContent + e.children.map(textOf).join('');
}

test('load: an unknown page shows the error state with a link to the page list', async () => {
  for (const path of ['/p/gone', '/p/gone/deck']) {
    const root = await servedPage(path, 404);
    const box = root.querySelector('wk-empty');
    assert.ok(box, path + ': no wk-empty');
    assert.equal(box.parentNode.className, 'brief', path + ': not inside the page wrapper');
    assert.equal(box.getAttribute('variant'), 'error');
    assert.equal(textOf(box.querySelector('wk-empty-title')), 'No page at this address');
    assert.match(textOf(box), /This page may have been deleted\. Back to all pages$/);
    assert.equal(box.querySelector('a').getAttribute('href'), '/');
  }
});

test('load: on a shared instance the unknown page offers no page list', async () => {
  const box = (await servedPage('/p/gone', 404, 'shared')).querySelector('wk-empty');
  assert.equal(textOf(box.querySelector('wk-empty-title')), 'No page at this address');
  assert.equal(box.querySelector('a'), null, 'a shared instance has no page list to link');
  assert.match(textOf(box), /This page may have been deleted\.$/);
});

test('load: any other failure shows its error under a title', async () => {
  const box = (await servedPage('/p/abc', 500)).querySelector('wk-empty');
  assert.equal(box.getAttribute('variant'), 'error');
  assert.equal(textOf(box.querySelector('wk-empty-title')), 'Could not load this page');
  assert.match(textOf(box), /load failed \(500\)$/);
  assert.equal(box.querySelector('a'), null);
});
