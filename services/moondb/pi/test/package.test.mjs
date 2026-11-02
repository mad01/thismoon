import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { fakePi } from './fake.mjs'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')

test('the manifest loads every extension and nothing else', async () => {
  const manifest = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'))
  assert.deepEqual(manifest.pi.extensions, ['./extensions/*.ts'])
  for (const kind of ['skills', 'prompts', 'themes']) assert.deepEqual(manifest.pi[kind], [])

  const files = readdirSync(join(root, 'extensions')).sort()
  assert.deepEqual(files, ['moondb-record.ts'])
  const { default: extension } = await import(join(root, 'extensions', files[0]))
  const { pi, handlers } = fakePi()
  extension(pi)
  assert.deepEqual([...handlers.keys()].sort(), [
    'agent_end',
    'agent_settled',
    'before_agent_start',
    'session_shutdown',
    'tool_call',
    'tool_result',
  ])
})
