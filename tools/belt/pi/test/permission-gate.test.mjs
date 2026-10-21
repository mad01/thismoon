import assert from 'node:assert/strict'
import { symlinkSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

import gate from '../extensions/permission-gate.ts'
import { fakePi, headlessCtx, scratch } from './fake.mjs'

// One gate per test: it reads belt.json and the Claude settings when it loads.
function load() {
  const fake = fakePi()
  gate(fake.pi)
  return fake
}

const DENY = (sub) => `DENY  fake-${sub}  denied by the fake`

test('a belt bash denial blocks the call and announces belt:deny', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'bash'
  const { call, emitted } = load()
  const res = await call('tool_call', { toolName: 'bash', input: { command: 'git push origin main' } }, headlessCtx(env.dir))
  assert.deepEqual(res, { block: true, reason: DENY('bash') })
  assert.deepEqual(env.argv(), ['check', 'bash', 'git push origin main', '--cwd', env.dir])
  assert.deepEqual(emitted, [['belt:deny', { kind: 'bash', target: 'git push origin main', reason: DENY('bash') }]])
})

test('a belt write denial names the resolved file', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'write'
  const { call, emitted } = load()
  const file = join(env.dir, 'notes.md')
  const res = await call('tool_call', { toolName: 'write', input: { path: 'notes.md', content: 'text' } }, headlessCtx(env.dir))
  assert.deepEqual(res, { block: true, reason: DENY('write') })
  assert.deepEqual(env.argv(), ['check', 'write', '--file', file, '--content', 'text'])
  assert.deepEqual(emitted, [['belt:deny', { kind: 'write', target: file, reason: DENY('write') }]])
})

test('github.com MCP calls go through belt external-text, proxied or direct', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call, emitted } = load()
  const ctx = headlessCtx(env.dir)

  const args = { owner: 'mad01', repo: 'x', title: 't' }
  const proxied = await call('tool_call', { toolName: 'mcp', input: { server: 'gh_com', tool: 'create_pull_request', args } }, ctx)
  assert.deepEqual(proxied, { block: true, reason: DENY('external-text') })
  const argv = env.argv()
  assert.deepEqual(argv.slice(0, 5), ['check', 'external-text', '--tool', 'mcp__gh_com__create_pull_request', '--input'])
  assert.deepEqual(JSON.parse(argv[5]), args)

  const direct = await call('tool_call', { toolName: 'mcp__gh_com__add_issue_comment', input: { body: 'b' } }, ctx)
  assert.equal(direct.block, true)
  assert.equal(env.argv()[3], 'mcp__gh_com__add_issue_comment')
  assert.deepEqual(emitted.map(([name, data]) => [name, data.kind]), [
    ['belt:deny', 'external-text'],
    ['belt:deny', 'external-text'],
  ])
})

test('without belt.json another server never reaches external-text', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  const res = await call('tool_call', { toolName: 'mcp__vcs__create_pull_request', input: { body: 'b' } }, headlessCtx(env.dir))
  assert.equal(res, undefined)
  assert.equal(env.argv(), null)
})

test('belt.json adds guarded servers and maps direct adapter tools to them', async (t) => {
  const env = scratch(t)
  env.writeConfig({ mcpPrefixes: { vcs_: 'vcs' }, externalTextGuardServers: ['vcs'] })
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  const ctx = headlessCtx(env.dir)

  const named = await call('tool_call', { toolName: 'mcp__vcs__create_pull_request', input: { body: 'b' } }, ctx)
  assert.equal(named.block, true)
  assert.equal(env.argv()[3], 'mcp__vcs__create_pull_request')

  const direct = await call('tool_call', { toolName: 'vcs_add_comment', input: { body: 'b' } }, ctx)
  assert.equal(direct.block, true)
  assert.equal(env.argv()[3], 'mcp__vcs__vcs_add_comment')
})

test('a mapped direct tool matches Claude rules under its server name', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { deny: ['mcp__kof__*'] } })
  const { call } = load()
  const res = await call('tool_call', { toolName: 'kof_assert', input: { subject: 'x' } }, headlessCtx(env.dir))
  assert.match(res.reason, /^Denied by Claude settings: mcp__kof__\*/)
  assert.equal(env.argv(), null)
})

test('unknown shell commands fail closed headless unless belt.json names an active sandbox', async (t) => {
  const env = scratch(t)
  const saved = process.env.BELT_PI_TEST_SANDBOX
  t.after(() => {
    if (saved === undefined) delete process.env.BELT_PI_TEST_SANDBOX
    else process.env.BELT_PI_TEST_SANDBOX = saved
  })
  process.env.BELT_PI_TEST_SANDBOX = '1'
  const bash = { toolName: 'bash', input: { command: 'echo hi' } }

  // The variable is set, but no config names it: the gate confirms, and
  // with no UI and no broker that is a block.
  const plain = await load().call('tool_call', bash, headlessCtx(env.dir))
  assert.deepEqual(plain, { block: true, reason: 'No parent permission broker is available' })
  assert.deepEqual(env.argv(), ['check', 'bash', 'echo hi', '--cwd', env.dir])

  env.writeConfig({ sandboxActiveEnv: 'BELT_PI_TEST_SANDBOX' })
  assert.equal(await load().call('tool_call', bash, headlessCtx(env.dir)), undefined)
})

test('belt.json can name the variable that switches unsandboxed retries off', async (t) => {
  const env = scratch(t)
  const saved = process.env.BELT_PI_TEST_ESCAPE
  t.after(() => {
    if (saved === undefined) delete process.env.BELT_PI_TEST_ESCAPE
    else process.env.BELT_PI_TEST_ESCAPE = saved
  })
  process.env.BELT_PI_TEST_ESCAPE = '0'
  env.writeConfig({ sandboxAllowUnsandboxedEnv: 'BELT_PI_TEST_ESCAPE' })
  const res = await load().call(
    'tool_call',
    { toolName: 'bash', input: { command: 'make', sandbox_permissions: 'require_escalated' } },
    headlessCtx(env.dir),
  )
  assert.deepEqual(res, { block: true, reason: 'Unsandboxed commands are disabled by Pi sandbox settings' })
})

test('an unreadable belt.json blocks everything but reads until it is fixed', async (t) => {
  const env = scratch(t)
  env.writeConfig('{not json')
  const { call } = load()
  const ctx = headlessCtx(env.dir)

  const bash = await call('tool_call', { toolName: 'bash', input: { command: 'ls' } }, ctx)
  assert.equal(bash.block, true)
  assert.match(bash.reason, /^belt pi config: .*belt\.json unreadable/)
  assert.equal(env.argv(), null)
  const mcp = await call('tool_call', { toolName: 'mcp__vcs__create_pull_request', input: {} }, ctx)
  assert.equal(mcp.block, true)
  assert.equal(await call('tool_call', { toolName: 'read', input: { path: 'x' } }, ctx), undefined)

  // A new session re-reads the file.
  env.writeConfig({})
  await call('session_start', { reason: 'new' }, ctx)
  const fixed = await call('tool_call', { toolName: 'mcp__vcs__create_pull_request', input: {} }, ctx)
  assert.equal(fixed, undefined)
})

test('a dangling belt.json symlink blocks like an unreadable file', async (t) => {
  const env = scratch(t)
  symlinkSync(join(env.dir, 'missing', 'belt.json'), join(env.agentDir, 'belt.json'))
  const { call } = load()
  const ctx = headlessCtx(env.dir)
  const bash = await call('tool_call', { toolName: 'bash', input: { command: 'ls' } }, ctx)
  assert.equal(bash.block, true)
  assert.match(bash.reason, /belt\.json unreadable \(symlink to missing .*missing\/belt\.json\)/)
  assert.equal(await call('tool_call', { toolName: 'read', input: { path: 'x' } }, ctx), undefined)
})

test('session start names the unknown belt.json keys, and stays quiet otherwise', async (t) => {
  const env = scratch(t)
  const notes = []
  const ctx = { ...headlessCtx(env.dir), hasUI: true, ui: { notify: (text, level) => notes.push([text, level]) } }
  const { call } = load()

  env.writeConfig({ externalTextGuardServers: ['vcs'] })
  await call('session_start', { reason: 'new' }, ctx)
  assert.deepEqual(notes, [])

  env.writeConfig({ externalTextGuardServer: ['vcs'], sandboxActive: 'S' })
  await call('session_start', { reason: 'new' }, ctx)
  assert.equal(notes.length, 1)
  assert.match(notes[0][0], /\(unknown keys ignored: externalTextGuardServer, sandboxActive\)$/)
  assert.equal(notes[0][1], 'warning')
})
