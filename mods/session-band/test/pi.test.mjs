import assert from 'node:assert/strict'
import test from 'node:test'

import face from '../pi/index.ts'

// A pi stand-in: handlers by event, an in-process event bus, and the entries
// the face appends. ctx carries only what the face reads.
function fakePi() {
  const handlers = new Map()
  const listeners = new Map()
  const entries = []
  const pi = {
    on(name, handler) {
      handlers.set(name, [...(handlers.get(name) ?? []), handler])
    },
    events: {
      on(name, listener) {
        listeners.set(name, [...(listeners.get(name) ?? []), listener])
      },
      emit(name, data) {
        for (const listener of listeners.get(name) ?? []) listener(data)
      },
    },
    appendEntry(type, data) {
      entries.push({ type, data })
    },
  }
  return { pi, handlers, entries }
}

function fakeCtx(branch = []) {
  const statuses = []
  return {
    ctx: {
      hasUI: true,
      ui: { setStatus: (key, text) => statuses.push([key, text]) },
      sessionManager: { getBranch: () => branch },
    },
    statuses,
  }
}

test('counts belt denials announced on the bus and pins the band', async () => {
  const { pi, handlers, entries } = fakePi()
  face(pi)
  const { ctx, statuses } = fakeCtx()
  await handlers.get('session_start')[0]({ reason: 'new' }, ctx)
  assert.deepEqual(statuses.at(-1), ['session-band', undefined])

  pi.events.emit('belt:deny', { kind: 'bash', target: 'git push origin main', reason: 'DENY git-push-main' })
  pi.events.emit('belt:deny', { kind: 'write', target: '/x/README.md', reason: 'DENY write-internal-names' })
  assert.deepEqual(statuses.at(-1), ['session-band', 'belt denies 2'])
  assert.deepEqual(entries.at(-1), { type: 'session-band', data: { beltDenies: 2 } })
})

test('restores the count from the active branch and keeps counting', async () => {
  const { pi, handlers } = fakePi()
  face(pi)
  const branch = [
    { type: 'custom', customType: 'session-band', data: { beltDenies: 1 } },
    { type: 'message', message: { role: 'user' } },
    { type: 'custom', customType: 'session-band', data: { beltDenies: 4 } },
  ]
  const { ctx, statuses } = fakeCtx(branch)
  await handlers.get('session_tree')[0]({}, ctx)
  assert.deepEqual(statuses.at(-1), ['session-band', 'belt denies 4'])
  pi.events.emit('belt:deny', {})
  assert.deepEqual(statuses.at(-1), ['session-band', 'belt denies 5'])
})

test('stays silent without a UI', async () => {
  const { pi, handlers } = fakePi()
  face(pi)
  const { ctx, statuses } = fakeCtx()
  ctx.hasUI = false
  await handlers.get('session_start')[0]({ reason: 'new' }, ctx)
  pi.events.emit('belt:deny', {})
  assert.equal(statuses.length, 0)
})
