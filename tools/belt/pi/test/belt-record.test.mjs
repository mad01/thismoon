import assert from 'node:assert/strict'
import { join } from 'node:path'
import test from 'node:test'
import { setTimeout as sleep } from 'node:timers/promises'

import hooks, { failureType } from '../extensions/belt-hooks.ts'
import gate from '../extensions/permission-gate.ts'
import { fakePi, headlessCtx, scratch } from './fake.mjs'

function load() {
  const fake = fakePi()
  hooks(fake.pi)
  return fake
}

// A context whose session file is set, so transcript_path is too.
function sessionCtx(env) {
  const ctx = headlessCtx(env.dir)
  const file = join(env.dir, 'session.jsonl')
  return { file, ctx: { ...ctx, sessionManager: { ...ctx.sessionManager, getSessionFile: () => file } } }
}

const base = (env, file) => ({ cwd: env.dir, session_id: 's1', transcript_path: file })
const assistant = (stopReason, errorMessage) => ({ role: 'assistant', content: [], stopReason, errorMessage })

// Records run in the background, so "nothing recorded" is checked after a
// later record has finished and a grace period has passed.
async function assertOnlyMarker(env, call, ctx) {
  await call('session_before_compact', { reason: 'manual' }, ctx)
  assert.ok(await env.record('pre-compact'))
  await sleep(200)
  assert.deepEqual(env.recorded(), ['pre-compact'])
}

for (const [reason, trigger] of [
  ['manual', 'manual'],
  ['threshold', 'auto'],
  ['overflow', 'auto'],
]) {
  test(`compaction by ${reason} records pre-compact and post-compact with trigger ${trigger}`, async (t) => {
    const env = scratch(t)
    const { call } = load()
    const { ctx, file } = sessionCtx(env)
    // undefined, so the record never cancels or customises the compaction
    assert.equal(await call('session_before_compact', { reason }, ctx), undefined)
    assert.deepEqual(await env.record('pre-compact'), {
      argv: ['record', 'pre-compact'],
      stdin: { ...base(env, file), trigger },
    })
    assert.equal(await call('session_compact', { reason }, ctx), undefined)
    assert.deepEqual(await env.record('post-compact'), {
      argv: ['record', 'post-compact'],
      stdin: { ...base(env, file), trigger },
    })
  })
}

for (const reason of ['quit', 'new', 'resume', 'fork']) {
  test(`shutdown by ${reason} records session-end with that reason`, async (t) => {
    const env = scratch(t)
    const { call } = load()
    const { ctx, file } = sessionCtx(env)
    assert.equal(await call('session_shutdown', { reason }, ctx), undefined)
    assert.deepEqual(await env.record('session-end'), {
      argv: ['record', 'session-end'],
      stdin: { ...base(env, file), reason },
    })
  })
}

test('a reload records no session-end, since the session goes on', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const { ctx } = sessionCtx(env)
  await call('session_shutdown', { reason: 'reload' }, ctx)
  await assertOnlyMarker(env, call, ctx)
})

test('a record never waits for belt', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_RECORD_DELAY = '1'
  const { call } = load()
  const { ctx, file } = sessionCtx(env)
  const start = Date.now()
  await call('session_shutdown', { reason: 'quit' }, ctx)
  const elapsed = Date.now() - start
  assert.ok(elapsed < 500, `the handler took ${elapsed} ms`)
  assert.deepEqual(env.recorded(), [])
  // belt reads its stdin after the handler has removed the payload file.
  assert.deepEqual((await env.record('session-end')).stdin, { ...base(env, file), reason: 'quit' })
})

// The gate and the hooks on one pi, as a session loads them, started.
async function gateAndHooks(env, ctx) {
  const fake = fakePi()
  gate(fake.pi)
  hooks(fake.pi)
  await fake.callAll('session_start', { reason: 'new' }, ctx)
  return fake
}

const beltDenies = (emitted) => emitted.filter(([name]) => name === 'belt:deny')

test('a belt guard denial records permission-denied and still counts as belt:deny', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'bash'
  const { ctx, file } = sessionCtx(env)
  const { call, emitted } = await gateAndHooks(env, ctx)
  const res = await call(
    'tool_call',
    { toolName: 'bash', toolCallId: 'call-1', input: { command: 'git push origin main' } },
    ctx,
  )
  assert.equal(res.block, true)
  assert.deepEqual(await env.record('permission-denied'), {
    argv: ['record', 'permission-denied'],
    stdin: { ...base(env, file), tool_name: 'bash', tool_use_id: 'call-1', reason: `belt guard: ${res.reason}` },
  })
  assert.equal(beltDenies(emitted).length, 1)
})

test('a denial names the MCP tool the call resolves to, not the proxy', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { ctx } = sessionCtx(env)
  const { call } = await gateAndHooks(env, ctx)
  const input = { server: 'gh_com', tool: 'create_pull_request', args: { owner: 'o', repo: 'r', body: 'b' } }
  const res = await call('tool_call', { toolName: 'mcp', toolCallId: 'call-2', input }, ctx)
  assert.equal(res.block, true)
  const { stdin } = await env.record('permission-denied')
  assert.equal(stdin.tool_name, 'mcp__gh_com__create_pull_request')
  assert.equal(stdin.tool_use_id, 'call-2')
})

test('a settings deny rule records permission-denied and no belt:deny', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { deny: ['mcp__worklog__*'] } })
  const { ctx } = sessionCtx(env)
  const { call, emitted } = await gateAndHooks(env, ctx)
  const res = await call('tool_call', { toolName: 'mcp__worklog__worklog_list', toolCallId: 'call-3', input: {} }, ctx)
  assert.match(res.reason, /^Denied by Claude settings: /)
  const { stdin } = await env.record('permission-denied')
  assert.equal(stdin.tool_name, 'mcp__worklog__worklog_list')
  assert.equal(stdin.reason, `settings deny rule: ${res.reason}`)
  assert.deepEqual(beltDenies(emitted), [])
})

// A context with a UI whose confirm dialogs answer in turn, and whose select
// dialogs pick choice.
function uiCtx(env, { answers = [], choice } = {}) {
  const { ctx, file } = sessionCtx(env)
  const ui = { confirm: async () => answers.shift() ?? false, select: async () => choice, notify() {} }
  return { file, ctx: { ...ctx, hasUI: true, ui } }
}

test('a user who rejects a shell command records permission-denied as the user', async (t) => {
  const env = scratch(t)
  const { ctx, file } = uiCtx(env, { answers: [false, false] })
  const { call, emitted } = await gateAndHooks(env, ctx)
  const res = await call('tool_call', { toolName: 'bash', toolCallId: 'call-4', input: { command: 'make deploy' } }, ctx)
  assert.deepEqual(res, { block: true, reason: 'Denied by user' })
  assert.deepEqual((await env.record('permission-denied')).stdin, {
    ...base(env, file),
    tool_name: 'bash',
    tool_use_id: 'call-4',
    reason: 'user: Denied by user',
  })
  assert.deepEqual(beltDenies(emitted), [])
})

test("a user's always-deny rule records permission-denied as the user", async (t) => {
  const env = scratch(t)
  // No to the command, yes to always denying it; the second call meets the rule.
  const { ctx } = uiCtx(env, { answers: [false, true] })
  const { call } = await gateAndHooks(env, ctx)
  await call('tool_call', { toolName: 'bash', toolCallId: 'call-5', input: { command: 'make deploy' } }, ctx)
  assert.ok(await env.record('permission-denied'))
  env.clearRecords()
  const res = await call('tool_call', { toolName: 'bash', toolCallId: 'call-6', input: { command: 'make clean' } }, ctx)
  assert.equal(res.reason, 'Denied by rule: make:*')
  const { stdin } = await env.record('permission-denied')
  assert.equal(stdin.tool_use_id, 'call-6')
  assert.equal(stdin.reason, 'user: Denied by rule: make:*')
})

test('a user who denies an edit or a confirmation records permission-denied as the user', async (t) => {
  const env = scratch(t)
  const outside = join(env.dir, '..', 'elsewhere.md')
  const edit = uiCtx(env, { choice: 'Deny' })
  const fake = await gateAndHooks(env, edit.ctx)
  const res = await fake.call('tool_call', { toolName: 'write', toolCallId: 'call-7', input: { path: outside, content: 'x' } }, edit.ctx)
  assert.deepEqual(res, { block: true, reason: 'Denied by user' })
  assert.equal((await env.record('permission-denied')).stdin.reason, 'user: Denied by user')
  env.clearRecords()
  // An mcpScript always asks; with a UI the answer comes from the user.
  const script = await fake.call('tool_call', { toolName: 'mcpScript', toolCallId: 'call-8', input: { code: 'x' } }, edit.ctx)
  assert.deepEqual(script, { block: true, reason: 'Denied by user' })
  const { stdin } = await env.record('permission-denied')
  assert.equal(stdin.tool_use_id, 'call-8')
  assert.equal(stdin.reason, 'user: Denied by user')
})

test('a block nobody decided records nothing', async (t) => {
  const env = scratch(t)
  const { ctx } = sessionCtx(env)
  const { call, emitted } = await gateAndHooks(env, ctx)
  // Headless with no broker: the confirmation fails closed.
  const res = await call('tool_call', { toolName: 'bash', toolCallId: 'call-9', input: { command: 'make deploy' } }, ctx)
  assert.equal(res.block, true)
  assert.notEqual(res.reason, 'Denied by user')
  assert.deepEqual(emitted.filter(([name]) => name === 'belt:permission-denied'), [])
  await assertOnlyMarker(env, call, ctx)
})

test('the deny listener lives as long as its session and is never doubled', async (t) => {
  const env = scratch(t)
  const { callAll, listeners } = load()
  const { ctx } = sessionCtx(env)
  await callAll('session_start', { reason: 'new' }, ctx)
  await callAll('session_start', { reason: 'reload' }, ctx)
  assert.equal(listeners.get('belt:permission-denied').length, 1)
  await callAll('session_shutdown', { reason: 'reload' }, ctx)
  assert.deepEqual(listeners.get('belt:permission-denied'), [])
})

test('a run that ends on an error records stop-failure once it settles', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const { ctx, file } = sessionCtx(env)
  const messages = [{ role: 'user', content: 'hi' }, assistant('error', '429 rate limited')]
  assert.equal(await call('agent_end', { messages }, ctx), undefined)
  assert.equal(await call('agent_settled', { aborted: false }, ctx), undefined)
  assert.deepEqual(await env.record('stop-failure'), {
    argv: ['record', 'stop-failure'],
    stdin: { ...base(env, file), error: 'rate_limit', last_assistant_message: '429 rate limited' },
  })
})

test('a failed run with no error text still records, for the ledger to name', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const { ctx } = sessionCtx(env)
  await call('agent_end', { messages: [assistant('error')] }, ctx)
  await call('agent_settled', { aborted: false }, ctx)
  const { stdin } = await env.record('stop-failure')
  assert.equal(stdin.last_assistant_message, '')
  assert.equal(stdin.error, 'unknown')
})

test('a retried, aborted or already settled run records no stop-failure', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const { ctx } = sessionCtx(env)
  // A retry after agent_end recovered the run.
  await call('agent_end', { messages: [assistant('error', 'overloaded')] }, ctx)
  await call('agent_end', { messages: [assistant('stop')] }, ctx)
  await call('agent_settled', { aborted: false }, ctx)
  // The user aborted the turn.
  await call('agent_end', { messages: [assistant('aborted')] }, ctx)
  await call('agent_settled', { aborted: false }, ctx)
  await call('agent_end', { messages: [assistant('error', 'x')] }, ctx)
  await call('agent_settled', { aborted: true }, ctx)
  // Settling again without a new run.
  await call('agent_settled', { aborted: false }, ctx)
  await assertOnlyMarker(env, call, ctx)
})

test('failureType names an error type only on an unambiguous signal', () => {
  for (const [text, want] of [
    ['429 {"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}', 'rate_limit'],
    ['openrouter (429): {"error":{"message":"Provider returned error"}}', 'rate_limit'],
    ['Rate limit reached for requests per minute', 'rate_limit'],
    ['rate_limit_exceeded', 'rate_limit'],
    ['Too Many Requests', 'rate_limit'],
    ['529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}', 'overloaded'],
    ['The model is overloaded, please try again later', 'overloaded'],
    ['503 Service Unavailable: model overloaded', 'overloaded'],
    ['500 Internal server error', 'server_error'],
    ['502: Bad Gateway', 'server_error'],
    ['anthropic (503): upstream connect error', 'server_error'],
    ['Request failed with status code 504', 'server_error'],
    ['HTTP/1.1 520 Unknown Error', 'server_error'],
    // Account limits answer 429 as well, but the user has to act.
    ['429 You exceeded your current quota, please check your plan and billing details', 'unknown'],
    ['Monthly usage limit reached', 'unknown'],
    // The ledger's vocabulary has no context-overflow type.
    ['prompt is too long: 212000 tokens > 200000 maximum', 'unknown'],
    ['400 {"type":"error","error":{"type":"invalid_request_error"}}', 'unknown'],
    // A number that is not where a status goes, or part of a longer one.
    ['Wrote 512 bytes before the connection closed', 'unknown'],
    ['request req_5003 failed', 'unknown'],
    ['5000 tokens used', 'unknown'],
    ['Connection error.', 'unknown'],
    ['', 'unknown'],
  ]) {
    assert.equal(failureType(text), want, text)
  }
})
