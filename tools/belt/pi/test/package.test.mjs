import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import gate from '../extensions/permission-gate.ts'
import { fakePi, headlessCtx, scratch } from './fake.mjs'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')

test('the manifest loads every extension and nothing else', async (t) => {
  scratch(t)
  const manifest = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'))
  assert.deepEqual(manifest.pi.extensions, ['./extensions/*.ts'])
  for (const kind of ['skills', 'prompts', 'themes']) assert.deepEqual(manifest.pi[kind], [])

  const files = readdirSync(join(root, 'extensions')).sort()
  assert.deepEqual(files, ['belt-hooks.ts', 'permission-broker.ts', 'permission-gate.ts'])
  const events = {}
  for (const file of files) {
    const { default: extension } = await import(join(root, 'extensions', file))
    const { pi, handlers } = fakePi()
    extension(pi)
    events[file] = [...handlers.keys()].sort()
  }
  assert.deepEqual(events, {
    'belt-hooks.ts': ['before_agent_start', 'input', 'session_start', 'tool_result'],
    'permission-broker.ts': ['session_shutdown', 'session_start'],
    'permission-gate.ts': ['session_shutdown', 'session_start', 'session_tree', 'tool_call'],
  })
})

// The marker is the package's contract with the consuming repo's own
// fail-closed check, which blocks tool calls while no gate has loaded.
test('the gate marks itself loaded until its session shuts down', async (t) => {
  const env = scratch(t)
  const marker = Symbol.for('mad01.pi.belt-gate')
  delete globalThis[marker]
  t.after(() => delete globalThis[marker])
  const ctx = headlessCtx(env.dir)

  const { pi, call } = fakePi()
  gate(pi)
  assert.equal(globalThis[marker], true)
  // A reload or session switch shuts the old runtime down first; a gate
  // that does not load again must leave no marker behind.
  await call('session_shutdown', { reason: 'reload' }, ctx)
  assert.equal(globalThis[marker], undefined)
  await call('session_start', { reason: 'new' }, ctx)
  assert.equal(globalThis[marker], true)
})
