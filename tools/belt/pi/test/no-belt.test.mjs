import assert from 'node:assert/strict'
import test from 'node:test'

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
  assert.deepEqual(emitted.map(([name, data]) => [name, data.kind]), [
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
