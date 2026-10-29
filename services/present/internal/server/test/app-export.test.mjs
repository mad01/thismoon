// app.js's export branch (renderExport), run via `node --test`. app.js is a
// classic browser script, so it runs here against a small fake DOM holding
// what an exported file holds: <html data-present-mode="export">, the body
// in #root, and the page data in #present-export. No library is defined,
// which is the file opened with cdn.jsdelivr.net out of reach.
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
// blocks, runs app.js over it, and returns the document and every fetch
// app.js made.
function exportedPage(blocks) {
  const brief = new El('div', { class: 'brief' }, blocks);
  const data = { id: 'p1', title: 'Exported', version: 3, view: 'brief', has_graph: false, graph: '', references: [] };
  const html = new El('html', { 'data-present-mode': 'export' }, [
    new El('body', {}, [
      new El('main', { id: 'root' }, [brief]),
      new El('script', { type: 'application/json', id: 'present-export' }, [], JSON.stringify(data)),
    ]),
  ]);
  const listeners = {};
  const document = {
    documentElement: html,
    body: html.children[0],
    title: '',
    getElementById: (id) => html.descendants().find((e) => e.id === id) || null,
    querySelector: (sel) => html.querySelector(sel),
    querySelectorAll: (sel) => html.querySelectorAll(sel),
    createElement: (tag) => new El(tag),
    addEventListener: (type, fn) => { (listeners[type] ||= []).push(fn); },
  };
  const window = { addEventListener() {} };
  const fetches = [];
  const fetch = (url) => { fetches.push(url); return new Promise(() => {}); };
  new Function('window', 'document', 'fetch', appSrc)(window, document, fetch);
  return { document, brief, fetches, listeners };
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
