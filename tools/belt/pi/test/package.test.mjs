import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { fakePi, scratch } from './fake.mjs'

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
    const { default: factory } = await import(join(root, 'extensions', file))
    const { pi, handlers } = fakePi()
    factory(pi)
    events[file] = [...handlers.keys()].sort()
  }
  assert.deepEqual(events, {
    'belt-hooks.ts': ['before_agent_start', 'input', 'session_start', 'tool_result'],
    'permission-broker.ts': ['session_shutdown', 'session_start'],
    'permission-gate.ts': ['session_start', 'session_tree', 'tool_call'],
  })
})
