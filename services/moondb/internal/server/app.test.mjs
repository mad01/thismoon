import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

// app.js is the page script. These tests run its sessions list in a vm
// context with a small DOM stand-in and a Webkit stub, so each fetch is held
// until the test settles it, in whatever order the test picks.
const APP = readFileSync(new URL('./app.js', import.meta.url), 'utf8')

const ONE = 'github.com/a/one'
const TWO = 'github.com/a/two'

// FakeNode is the part of an element the list view touches.
class FakeNode {
  constructor(tag) {
    this.tag = tag
    this.attrs = {}
    this.children = []
    this.listeners = {}
    this.className = ''
    this.classList = {
      add: c => { this.className += ' ' + c },
      contains: c => this.className.split(' ').includes(c)
    }
  }
  setAttribute(k, v) { this.attrs[k] = String(v) }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn) }
  appendChild(c) { this.children.push(c); return c }
  replaceChildren(...nodes) { this.children = nodes.map(asNode) }
  get textContent() { return this.children.map(c => c.textContent).join('') }
  set textContent(v) { this.children = [asNode(String(v))] }
  click() { (this.listeners.click || []).forEach(fn => fn({})) }
}

function asNode(c) { return typeof c === 'string' ? { textContent: c, children: [] } : c }

// el builds a FakeNode the way Webkit.el builds an element.
function el(tag, attrs = {}, children = []) {
  const node = new FakeNode(tag)
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') node.className = String(v)
    else if (k.startsWith('on') && typeof v === 'function') node.addEventListener(k.slice(2), v)
    else if (v != null) node.setAttribute(k, v)
  }
  for (const c of [].concat(children)) if (c != null) node.appendChild(asNode(c))
  return node
}

function find(node, match) {
  if (match(node)) return node
  for (const c of node.children) {
    const hit = find(c, match)
    if (hit) return hit
  }
  return null
}

// openList runs app.js on the bare URL, which renders the sessions list.
// fetches holds every request in the order the page made it; tick fires the
// poll timer and back lands on another entry, as back and forward do.
function openList() {
  const app = new FakeNode('main')
  const fetches = []
  let timer = null
  let onPop = null
  const ctx = vm.createContext({
    URLSearchParams,
    document: { getElementById: () => app, title: '', body: {} },
    fetch: url => new Promise((resolve, reject) => fetches.push({ url: String(url), resolve, reject })),
    setInterval: fn => { timer = fn; return 1 },
    clearInterval: () => { timer = null },
    Webkit: {
      el,
      urlState: { read: () => ({}), write: () => true, subscribe: fn => { onPop = fn } },
      dayName: () => '',
      formatDate: () => '',
      formatClock: () => '',
      formatDuration: () => '',
      timeAgo: () => ''
    }
  })
  vm.runInContext(APP, ctx)
  return {
    fetches,
    text: () => app.textContent,
    chip: name => find(app, n => n.tag === 'wk-badge' && n.children[0]?.textContent === name),
    tick: () => timer(),
    back: params => onPop(params)
  }
}

// page is a sessions response holding one session titled title.
function page(title) {
  const now = new Date().toISOString()
  return {
    sessions: [{ id: title, title, days: [{ date: now.slice(0, 10), last_at: now, turns: 1 }] }],
    window: { sessions: 1 },
    repos: [{ repo: ONE, name: 'a/one', sessions: 1 }, { repo: TWO, name: 'a/two', sessions: 1 }],
    dirs: []
  }
}

// settle lets every promise callback the page chained run.
const settle = () => new Promise(r => setImmediate(r))

async function respond(f, title) {
  f.resolve({ ok: true, status: 200, json: async () => page(title) })
  await settle()
}

async function fail(f) {
  f.reject(new TypeError('network down'))
  await settle()
}

// pickTwoFilters loads the list, then clicks a/one and a/two before either
// fetch lands, and returns the two filter fetches.
async function pickTwoFilters(p) {
  await respond(p.fetches[0], 'rows for all')
  p.chip('a/one').click()
  p.chip('a/two').click()
  const [first, second] = p.fetches.slice(1)
  assert.match(first.url, /repo=github\.com%2Fa%2Fone/)
  assert.match(second.url, /repo=github\.com%2Fa%2Ftwo/)
  return [first, second]
}

test('sessions list: the last filter picked keeps its rows in either landing order', async () => {
  for (const lateFirst of [true, false]) {
    const p = openList()
    const [first, second] = await pickTwoFilters(p)
    if (lateFirst) {
      await respond(second, 'rows for a/two')
      await respond(first, 'rows for a/one')
    } else {
      await respond(first, 'rows for a/one')
      await respond(second, 'rows for a/two')
    }
    const order = lateFirst ? 'older lands last' : 'older lands first'
    assert.ok(p.text().includes('rows for a/two'), order)
    assert.ok(!p.text().includes('rows for a/one'), order)
  }
})

test('sessions list: an older filter\'s failure paints no error, the newest one\'s does', async () => {
  const p = openList()
  const [first, second] = await pickTwoFilters(p)
  await respond(second, 'rows for a/two')
  await fail(first)
  assert.ok(p.text().includes('rows for a/two'))
  assert.ok(!p.text().includes('Could not load sessions'))

  p.chip('a/one').click()
  await fail(p.fetches.at(-1))
  assert.ok(p.text().includes('Could not load sessions'))
})

test('sessions list: a poll tick and back each outrank the fetch in flight before them', async () => {
  const p = openList()
  await respond(p.fetches[0], 'rows for all')
  p.chip('a/one').click()
  const clicked = p.fetches.at(-1)
  p.tick()
  await respond(p.fetches.at(-1), 'polled rows')
  await respond(clicked, 'stale clicked rows')
  assert.ok(p.text().includes('polled rows'))
  assert.ok(!p.text().includes('stale clicked rows'))

  p.chip('a/two').click()
  const before = p.fetches.at(-1)
  p.back({})
  await respond(p.fetches.at(-1), 'rows after back')
  await respond(before, 'stale rows before back')
  assert.ok(p.text().includes('rows after back'))
  assert.ok(!p.text().includes('stale rows before back'))
})
