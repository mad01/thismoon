import assert from 'node:assert/strict'
import test from 'node:test'

import hooks from '../extensions/belt-hooks.ts'
import { fakePi, headlessCtx, scratch } from './fake.mjs'

function load() {
  const fake = fakePi()
  hooks(fake.pi)
  return fake
}

const OUT = [{ type: 'text', text: 'out' }]
const advice = (event) => ({ type: 'text', text: `\nbelt[fake]: advice for ${event}` })

test('bash results get belt hint advice appended and never a block', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const res = await call('tool_result', { toolName: 'bash', input: { command: 'ls' }, content: OUT }, headlessCtx(env.dir))
  assert.deepEqual(res, { content: [...OUT, advice('bash')] })
  assert.deepEqual(env.argv(), ['hint', 'bash'])
  assert.deepEqual(env.stdin(), {
    cwd: env.dir,
    session_id: 's1',
    transcript_path: '',
    tool_name: 'Bash',
    tool_input: { command: 'ls' },
  })
})

test('a tool no hint watches passes through untouched', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const res = await call('tool_result', { toolName: 'read', input: { path: 'x' }, content: OUT }, headlessCtx(env.dir))
  assert.equal(res, undefined)
  assert.equal(env.argv(), null)
})

test('csl searches run the search hint under the Claude tool name', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const res = await call('tool_result', { toolName: 'csl_search', input: { query: 'x' }, content: OUT }, headlessCtx(env.dir))
  assert.deepEqual(res.content.at(-1), advice('search'))
  assert.equal(env.stdin().tool_name, 'mcp__csl__csl_search')
  assert.equal(env.stdin().tool_response, 'out')
})

test('publishing calls run the external-text hint for the configured servers only', async (t) => {
  const env = scratch(t)
  const ctx = headlessCtx(env.dir)
  const ghCall = { toolName: 'mcp__gh_com__add_issue_comment', input: { body: 'b' }, content: OUT }
  const chatCall = { toolName: 'chat_send_message', input: { text: 'hi' }, content: OUT }

  const defaults = load()
  assert.deepEqual((await defaults.call('tool_result', ghCall, ctx)).content.at(-1), advice('external-text'))
  assert.equal(env.stdin().tool_name, 'mcp__gh_com__add_issue_comment')
  env.clearLog()
  assert.equal(await defaults.call('tool_result', chatCall, ctx), undefined)
  assert.equal(env.argv(), null)

  env.writeConfig({ mcpPrefixes: { chat_: 'chat' }, externalTextHintServers: ['chat'] })
  const configured = load()
  assert.deepEqual((await configured.call('tool_result', chatCall, ctx)).content.at(-1), advice('external-text'))
  assert.equal(env.stdin().tool_name, 'mcp__chat__chat_send_message')
})

test('an unreadable belt.json leaves the hints on their defaults', async (t) => {
  const env = scratch(t)
  env.writeConfig('{not json')
  const { call } = load()
  const res = await call('tool_result', { toolName: 'mcp__gh_com__add_issue_comment', input: {}, content: OUT }, headlessCtx(env.dir))
  assert.deepEqual(res.content.at(-1), advice('external-text'))
})

test('session-start advice reaches the next system prompt once', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = headlessCtx(env.dir)
  await call('session_start', { reason: 'new' }, ctx)
  assert.deepEqual(env.argv(), ['hint', 'session-start'])
  const first = await call('before_agent_start', { systemPrompt: 'base' }, ctx)
  assert.equal(first.systemPrompt, 'base\n\n## Belt lifecycle hints\n\nbelt[fake]: advice for session-start')
  assert.equal(await call('before_agent_start', { systemPrompt: 'base' }, ctx), undefined)
})
