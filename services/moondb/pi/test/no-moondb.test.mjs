import assert from 'node:assert/strict'
import test from 'node:test'

import record from '../extensions/moondb-record.ts'
import { fakePi, printCtx, scratch } from './fake.mjs'

// Its own file: the extension remembers a missing moondb for the life of
// the process, so this must not share one with the tests that need it.

test('without moondb on PATH every handler passes and nothing waits', async (t) => {
  const env = scratch(t, { withMoondb: false, repo: true })
  const { pi, handlers, call } = fakePi()
  record(pi)
  const ctx = printCtx(env.repoDir)
  const started = Date.now()
  assert.equal(await call('before_agent_start', { messageType: 'user', prompt: 'p', systemPrompt: '' }, ctx), undefined)
  const edit = { toolName: 'edit', toolCallId: 'e', input: { path: 'a.txt', oldText: 'one', newText: 'two' }, isError: false }
  // The first record learns moondb is missing without waiting out its timeout.
  assert.equal(await call('tool_result', edit, ctx), undefined)
  for (const name of handlers.keys()) {
    const event = name === 'tool_call' ? { toolName: 'bash', toolCallId: 'b', input: { command: 'ls' } } : { ...edit }
    assert.equal(await call(name, event, ctx), undefined, name)
  }
  assert.ok(Date.now() - started < 1_000)
})
