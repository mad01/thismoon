import assert from 'node:assert/strict'
import test from 'node:test'
import { setTimeout as sleep } from 'node:timers/promises'

import hooks from '../extensions/belt-hooks.ts'
import gate from '../extensions/permission-gate.ts'
import { fakePi, headlessCtx, scratch } from './fake.mjs'

// Its own file: both extensions remember a missing belt for the life of the
// process, so this must not share one with the tests that need belt.

test('without belt on PATH the guards fail closed and still announce the denial', async (t) => {
  const env = scratch(t, { withBelt: false })
  const { pi, call, emitted } = fakePi()
  gate(pi)
  const ctx = headlessCtx(env.dir)
  const res = await call('tool_call', { toolName: 'bash', input: { command: 'ls' } }, ctx)
  assert.deepEqual(res, { block: true, reason: 'belt is unavailable' })
  const write = await call('tool_call', { toolName: 'edit', input: { path: 'a.md', newText: 'x' } }, ctx)
  assert.deepEqual(write, { block: true, reason: 'belt is unavailable' })
  const beltDenies = emitted.filter(([name]) => name === 'belt:deny')
  assert.deepEqual(beltDenies.map(([name, data]) => [name, data.kind]), [
    ['belt:deny', 'bash'],
    ['belt:deny', 'write'],
  ])
})

test('without belt on PATH the hints stay silent', async (t) => {
  const env = scratch(t, { withBelt: false })
  const { pi, call } = fakePi()
  hooks(pi)
  const res = await call(
    'tool_result',
    { toolName: 'bash', input: { command: 'ls' }, content: [{ type: 'text', text: 'out' }] },
    headlessCtx(env.dir),
  )
  assert.equal(res, undefined)
})

test('without belt on PATH the ledger records drop silently', async (t) => {
  const env = scratch(t, { withBelt: false })
  // A copy of its own: the hints test above may have cached the missing belt
  // already, and the first record must find out through its own spawn.
  const { default: freshHooks } = await import('../extensions/belt-hooks.ts?no-belt-records')
  const { pi, call, callAll } = fakePi()
  gate(pi)
  freshHooks(pi)
  const ctx = headlessCtx(env.dir)
  assert.equal(await call('session_before_compact', { reason: 'manual' }, ctx), undefined)
  // The spawn reports the missing binary asynchronously; an unhandled error
  // event there would fail this file.
  await sleep(100)
  await callAll('session_start', { reason: 'new' }, ctx)
  assert.equal(await call('session_compact', { reason: 'manual' }, ctx), undefined)
  await call('agent_end', { messages: [{ role: 'assistant', stopReason: 'error', errorMessage: 'x' }] }, ctx)
  assert.equal(await call('agent_settled', { aborted: false }, ctx), undefined)
  // The gate fails closed and announces the denial; recording it is a no-op.
  const res = await call('tool_call', { toolName: 'bash', toolCallId: 'c', input: { command: 'ls' } }, ctx)
  assert.equal(res.block, true)
  await callAll('session_shutdown', { reason: 'quit' }, ctx)
  await sleep(100)
})
