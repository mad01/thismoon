import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

// arcade.js is the block page's game kit, a classic browser script. These
// tests run it in a vm context with a small DOM stand-in and a hand-driven
// requestAnimationFrame, so a test picks every frame's timestamp.
const KIT = readFileSync(new URL('./assets/arcade.js', import.meta.url), 'utf8')
const SMASH = readFileSync(new URL('./assets/smash.js', import.meta.url), 'utf8')

const STEP_MS = 1000 / 60
const MAX_STEPS = 4 // the kit's catch-up cap

// loadKit runs arcade.js and returns its API plus the hooks a test drives it
// with: frame(ts) runs the pending animation frame, key(code) presses a key.
function loadKit() {
  const keyListeners = []
  let pending = null
  let boot = null
  const canvas = {
    width: 320,
    height: 240,
    getContext: () => ({ fillRect() {} }),
    addEventListener() {},
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 320, height: 240 })
  }
  const win = {
    document: {
      readyState: 'loading',
      getElementById: id => (id === 'game' ? canvas : null),
      addEventListener: (type, fn) => { if (type === 'DOMContentLoaded') boot = fn }
    },
    location: { hostname: 'blocked.example' },
    localStorage: { getItem: () => null, setItem() {} },
    addEventListener: (type, fn) => { if (type === 'keydown') keyListeners.push(fn) },
    requestAnimationFrame: cb => { pending = cb; return 1 }
  }
  win.window = win
  vm.createContext(win)
  vm.runInContext(KIT, win, { filename: 'arcade.js' })
  return {
    win,
    ARCADE: win.ARCADE,
    boot: () => boot(),
    key: code => keyListeners.forEach(fn => fn({ code, preventDefault() {} })),
    frame: ts => {
      const cb = pending
      pending = null
      cb(ts)
    }
  }
}

const { stepsFor } = loadKit().ARCADE

// stepsPerFrame feeds frame intervals to stepsFor from a fresh carry and
// returns the steps each frame ran.
function stepsPerFrame(intervals) {
  let carry = STEP_MS / 4
  return intervals.map(elapsed => {
    const next = stepsFor(elapsed, carry)
    carry = next.carry
    return next.steps
  })
}

const repeat = (n, v) => Array.from({ length: n }, () => v)
const sum = xs => xs.reduce((a, b) => a + b, 0)

// mulberry32: a small seeded PRNG, so the jitter cases replay the same way.
function rng(seed) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

test('stepsFor: a 60 Hz frame runs one step', () => {
  assert.deepEqual(stepsPerFrame(repeat(600, 1000 / 60)), repeat(600, 1))
})

test('stepsFor: a 120 Hz frame runs one step every second frame', () => {
  const steps = stepsPerFrame(repeat(240, 1000 / 120))
  for (let i = 0; i < steps.length; i += 2) {
    assert.deepEqual(steps.slice(i, i + 2), [0, 1], `frames ${i} and ${i + 1}`)
  }
})

test('stepsFor: a 30 Hz frame runs two steps', () => {
  assert.deepEqual(stepsPerFrame(repeat(300, 1000 / 30)), repeat(300, 2))
})

test('stepsFor: other refresh rates average 60 steps a second', () => {
  for (const hz of [50, 75, 90, 144, 240]) {
    const steps = sum(stepsPerFrame(repeat(hz * 10, 1000 / hz)))
    assert.ok(Math.abs(steps - 600) <= 1, `${hz} Hz ran ${steps} steps in 10 s, want 600 ± 1`)
    assert.ok(stepsPerFrame(repeat(hz, 1000 / hz)).every(s => s <= 2), `${hz} Hz ran over two steps in a frame`)
  }
})

test('stepsFor: a stall runs at most the cap and drops the rest', () => {
  for (const stall of [5000, 5008]) { // a whole number of steps, and not
    const next = stepsFor(stall, STEP_MS / 4)
    assert.equal(next.steps, MAX_STEPS, `${stall} ms stall`)
    assert.ok(next.carry < STEP_MS, `${stall} ms stall kept ${next.carry} ms`)
    assert.equal(stepsFor(1000 / 60, next.carry).steps, 1, `the frame after a ${stall} ms stall`)
  }
})

test('stepsFor: a frame with no time passed runs nothing', () => {
  for (const elapsed of [0, -3, NaN]) {
    assert.deepEqual({ ...stepsFor(elapsed, 2) }, { steps: 0, carry: 2 }, `elapsed ${elapsed}`)
  }
})

// Bound asserted for 60 Hz jitter: none. Over 600 frames every frame runs
// exactly one step, so the total is 600 and no frame doubles or skips.
test('stepsFor: jittered 60 Hz frames run one step each', () => {
  for (let seed = 1; seed <= 50; seed++) {
    const next = rng(seed)
    // frame intervals of 16.6 ms ± 1 ms
    const steps = stepsPerFrame(Array.from({ length: 600 }, () => 16.6 + (next() * 2 - 1)))
    assert.equal(sum(steps), 600, `seed ${seed}: total steps`)
    assert.equal(steps.filter(s => s !== 1).length, 0, `seed ${seed}: frames without exactly one step`)
  }
})

test('stepsFor: jittered 60 Hz timestamps run one step each', () => {
  for (let seed = 1; seed <= 50; seed++) {
    const next = rng(seed)
    // each timestamp ± 1 ms off its vsync, so intervals swing ± 2 ms
    const ts = Array.from({ length: 601 }, (_, i) => i * STEP_MS + (next() * 2 - 1))
    const steps = stepsPerFrame(ts.slice(1).map((t, i) => t - ts[i]))
    assert.equal(sum(steps), 600, `seed ${seed}: total steps`)
    assert.equal(steps.filter(s => s !== 1).length, 0, `seed ${seed}: frames without exactly one step`)
  }
})

// updatesPerSecond boots the kit with the built-in smash game, starts a
// round, and counts game.update calls over each simulated second of
// display frames at hz.
function updatesPerSecond(hz, seconds) {
  const kit = loadKit()
  let updates = 0
  const register = kit.ARCADE.register
  kit.ARCADE.register = (name, factory) => register(name, K => {
    const game = factory(K)
    const update = game.update
    game.update = function () {
      updates++
      return update.apply(this, arguments)
    }
    return game
  })
  vm.runInContext(SMASH, kit.win, { filename: 'smash.js' })
  kit.boot()
  let ts = 1000
  kit.frame(ts) // title screen
  kit.key('Space')
  const perSecond = []
  for (let s = 0; s < seconds; s++) {
    updates = 0
    for (let f = 0; f < hz; f++) {
      ts += 1000 / hz
      kit.frame(ts)
    }
    perSecond.push(updates)
  }
  return perSecond
}

test('kit: a game updates 60 times a second at 60 Hz and at 120 Hz', () => {
  assert.deepEqual(updatesPerSecond(60, 3), [60, 60, 60])
  assert.deepEqual(updatesPerSecond(120, 3), [60, 60, 60])
})
