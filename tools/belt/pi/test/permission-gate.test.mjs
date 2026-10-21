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

// pi-mcp-adapter's documented proxy call names no server, and the adapter
// then finds the tool by name on every enabled server. ARGS is a call's
// arguments with the owner/repo pair belt resolves the target from.
const ARGS = { owner: 'mad01', repo: 'x', title: 't' }

// The guard's belt argv for one proxy call: [tool name, input object].
async function proxyGuard(env, call, input, toolName = 'mcp') {
  env.clearLog()
  const res = await call('tool_call', { toolName, input }, headlessCtx(env.dir))
  const argv = env.argv()
  assert.deepEqual(argv.slice(0, 3), ['check', 'external-text', '--tool'])
  assert.equal(argv[4], '--input')
  return { res, tool: argv[3], input: JSON.parse(argv[5]) }
}

test('a server-less proxy call by adapter display name is guarded under its server', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  for (const [tool, guarded] of [
    ['gh_com_create_pull_request', 'mcp__gh_com__create_pull_request'],
    ['mcp__gh_com_create_gist', 'mcp__gh_com__create_gist'],
  ]) {
    const got = await proxyGuard(env, call, { tool, args: ARGS })
    assert.deepEqual(got.res, { block: true, reason: DENY('external-text') }, tool)
    assert.equal(got.tool, guarded, tool)
    assert.deepEqual(got.input, ARGS, tool)
  }
})

test('a server-less proxy call by original tool name still goes through belt', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call, emitted } = load()
  const got = await proxyGuard(env, call, { tool: 'create_pull_request', args: ARGS })
  assert.deepEqual(got.res, { block: true, reason: DENY('external-text') })
  assert.equal(got.tool, 'mcp__mcp__create_pull_request')
  assert.deepEqual(got.input, ARGS)
  assert.deepEqual(emitted, [
    ['belt:deny', { kind: 'external-text', target: 'mcp__mcp__create_pull_request', reason: DENY('external-text') }],
  ])
})

test('a configured prefix names the server before a guarded server prefix does', async (t) => {
  const env = scratch(t)
  env.writeConfig({ mcpPrefixes: { vcs_: 'vcs' }, externalTextGuardServers: ['vcs'] })
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  // The belt.json prefix keeps the whole name; the guarded vcs_ prefix
  // would have stripped it to add_comment.
  const got = await proxyGuard(env, call, { tool: 'vcs_add_comment', args: { body: 'b' } })
  assert.equal(got.res.block, true)
  assert.equal(got.tool, 'mcp__vcs__vcs_add_comment')

  // A default prefix on an unguarded server resolves too, and skips belt.
  env.clearLog()
  delete process.env.FAKE_BELT_DENY
  const csl = await call('tool_call', { toolName: 'mcp', input: { tool: 'csl_search', args: { query: 'q' } } }, headlessCtx(env.dir))
  assert.equal(csl, undefined)
  assert.equal(env.argv(), null)
})

test('a proxy call whose server stays unknown runs only when belt allows it', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const got = await proxyGuard(env, call, { tool: 'list_issues', args: ARGS })
  assert.equal(got.res, undefined)
  assert.equal(got.tool, 'mcp__mcp__list_issues')
})

test('the adapter display forms map to the guarded server, named or direct', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  const ctx = headlessCtx(env.dir)

  const named = await call('tool_call', { toolName: 'mcp', input: { server: 'gh_com', tool: 'gh_com_create_gist', args: {} } }, ctx)
  assert.equal(named.block, true)
  assert.equal(env.argv()[3], 'mcp__gh_com__create_gist')

  const direct = await call('tool_call', { toolName: 'gh_com_add_issue_comment', input: { body: 'b' } }, ctx)
  assert.equal(direct.block, true)
  assert.equal(env.argv()[3], 'mcp__gh_com__add_issue_comment')
})

test('proxy args sent as a JSON string reach belt and the close check as an object', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const args = { ...ARGS, pullNumber: 1, state: 'closed' }
  // belt resolves the target from owner/repo, so it must see the object.
  const got = await proxyGuard(env, call, { server: 'gh_com', tool: 'update_pull_request', args: JSON.stringify(args) })
  assert.deepEqual(got.input, args)
  // Closing still needs a confirmation, which headless with no broker blocks.
  assert.deepEqual(got.res, { block: true, reason: 'No parent permission broker is available' })
})

test('unparseable proxy args reach belt as raw text inside the whole input', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  const input = { tool: 'create_pull_request', args: '{"body": "internal text' }
  const got = await proxyGuard(env, call, input)
  assert.deepEqual(got.res, { block: true, reason: DENY('external-text') })
  assert.deepEqual(got.input, input)
})

// pi-mcp-adapter registers one mcp__<server> namespace proxy per proxy-only
// server by default. It takes { tool, args } like the mcp proxy, bound to
// that server.
test('a namespace proxy call reaches belt under its server', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  const got = await proxyGuard(env, call, { tool: 'create_pull_request', args: { ...ARGS, body: 'b' } }, 'mcp__gh_com')
  assert.deepEqual(got.res, { block: true, reason: DENY('external-text') })
  assert.equal(got.tool, 'mcp__gh_com__create_pull_request')
  assert.deepEqual(got.input, { ...ARGS, body: 'b' })
})

test('a namespace proxy merge meets the server ask rule before a broad allow', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { allow: ['mcp__gh_com__*'], ask: ['mcp__gh_com__merge_pull_request'] } })
  const { call } = load()
  const asked = []
  const ctx = {
    ...headlessCtx(env.dir),
    hasUI: true,
    ui: { confirm: async (title, message) => (asked.push([title, message]), false) },
  }
  const input = { tool: 'merge_pull_request', args: { owner: 'mad01', repo: 'x', pullNumber: 1 } }
  const res = await call('tool_call', { toolName: 'mcp__gh_com', input }, ctx)
  assert.deepEqual(res, { block: true, reason: 'Denied by user' })
  // The ask rule's prompt, not the merge fallback's: the call matched it.
  assert.equal(asked.length, 1)
  assert.match(asked[0][0], /: mcp__gh_com__merge_pull_request$/)
  assert.match(asked[0][1], /^Requires confirmation\n/)
})

// The adapter still accepts its legacy names, which encode the server's
// own underscore (gh_com becomes gh_5f_com), with or without the server.
test('a legacy display name reaches belt as the server operation', async (t) => {
  const env = scratch(t)
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  const args = { description: 'd', public: true, files: { 'a.md': { content: 'x' } } }
  for (const [toolName, input] of [
    ['mcp', { tool: 'gh_5f_com_create_gist', args }],
    ['mcp', { tool: 'mcp__gh_5f_com_create_gist', args }],
    ['mcp', { server: 'gh_com', tool: 'gh_5f_com_create_gist', args }],
    ['mcp__gh_com', { tool: 'gh_5f_com_create_gist', args }],
  ]) {
    const label = JSON.stringify([toolName, input.server, input.tool])
    const got = await proxyGuard(env, call, input, toolName)
    assert.deepEqual(got.res, { block: true, reason: DENY('external-text') }, label)
    assert.equal(got.tool, 'mcp__gh_com__create_gist', label)
    assert.deepEqual(got.input, args, label)
  }
})

// csl's tools carry csl_ in their own names, which is also the adapter's
// display prefix for csl, so a named call could resolve either way.
test('a deny rule on a tool name blocks the call named or server-less', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { allow: ['mcp__csl__*'], deny: ['mcp__csl__csl*'] } })
  const { call } = load()
  for (const [toolName, input] of [
    ['mcp', { server: 'csl', tool: 'csl_search', args: { query: 'q' } }],
    ['mcp', { tool: 'csl_search', args: { query: 'q' } }],
    ['mcp__csl', { tool: 'csl_search', args: { query: 'q' } }],
    ['csl_search', { query: 'q' }],
  ]) {
    const res = await call('tool_call', { toolName, input }, headlessCtx(env.dir))
    assert.match(res?.reason ?? 'allowed', /^Denied by Claude settings: mcp__csl__csl\*/, JSON.stringify([toolName, input.server]))
  }
})

test('a namespace proxy finds a guarded server under the adapter namespace form', async (t) => {
  const env = scratch(t)
  env.writeConfig({ externalTextGuardServers: ['git-hub', 'gh.enterprise'] })
  process.env.FAKE_BELT_DENY = 'external-text'
  const { call } = load()
  // The adapter folds "-" to "_" and encodes other characters behind _mcpns_.
  for (const [toolName, server] of [['mcp__git_hub', 'git-hub'], ['mcp___mcpns_gh_2e_enterprise', 'gh.enterprise']]) {
    const got = await proxyGuard(env, call, { tool: 'create_pull_request', args: ARGS }, toolName)
    assert.equal(got.res?.block, true, toolName)
    assert.equal(got.tool, `mcp__${server}__create_pull_request`, toolName)
  }
})

// A deny or ask rule on a proxy tool's own name covers every call through
// it, whatever the call reads as.
test('a deny rule on a proxy tool name blocks calls through that proxy', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { deny: ['mcp', 'mcp__linear'] } })
  const { call } = load()
  for (const [toolName, input, rule] of [
    ['mcp', { tool: 'create_gist', args: {} }, 'mcp'],
    ['mcp__linear', { tool: 'save_issue', args: {} }, 'mcp__linear'],
  ]) {
    const res = await call('tool_call', { toolName, input }, headlessCtx(env.dir))
    assert.equal(res?.reason, `Denied by Claude settings: ${rule}()`, toolName)
  }
})

test('an ask rule on a proxy tool name asks before a call through it', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { allow: ['mcp__gh_com__*'], ask: ['mcp'] } })
  const { call } = load()
  const res = await call('tool_call', { toolName: 'mcp', input: { server: 'gh_com', tool: 'get_me', args: {} } }, headlessCtx(env.dir))
  assert.deepEqual(res, { block: true, reason: 'No parent permission broker is available' })
})

// Claude reads a rule that names only a server as covering every tool on it.
test('a server-level deny rule blocks every tool on that server', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { deny: ['mcp__linear'] } })
  const { call } = load()
  for (const [toolName, input] of [
    ['mcp__linear__save_issue', { title: 't' }],
    ['mcp__linear', { tool: 'save_issue', args: { title: 't' } }],
    ['mcp', { server: 'linear', tool: 'save_issue', args: { title: 't' } }],
  ]) {
    const res = await call('tool_call', { toolName, input }, headlessCtx(env.dir))
    assert.equal(res?.reason, 'Denied by Claude settings: mcp__linear()', toolName)
  }
  // Another server whose name starts the same way is not covered.
  const other = await call('tool_call', { toolName: 'mcp__linear_ops__save_issue', input: {} }, headlessCtx(env.dir))
  assert.equal(other, undefined)
})

// pi-mcp-adapter's mcpScript makes its MCP calls inside the adapter, so
// neither this gate nor belt sees them.
test('an mcpScript call always asks, so headless it is blocked', async (t) => {
  const env = scratch(t)
  env.claudeSettings({ permissions: { allow: ['mcpScript', 'mcp__gh_com__*'] } })
  const { call } = load()
  const code = "await tools.call('gh_com_create_gist', { files: { 'a.md': { content: 'x' } } })"
  const res = await call('tool_call', { toolName: 'mcpScript', input: { code } }, headlessCtx(env.dir))
  assert.deepEqual(res, { block: true, reason: 'No parent permission broker is available' })
  assert.equal(env.argv(), null)

  const asked = []
  const ctx = {
    ...headlessCtx(env.dir),
    hasUI: true,
    ui: { confirm: async (title, message) => (asked.push([title, message]), false) },
  }
  assert.deepEqual(await call('tool_call', { toolName: 'mcpScript', input: { code } }, ctx), { block: true, reason: 'Denied by user' })
  assert.equal(asked.length, 1)
  assert.match(asked[0][1], /^An MCP script runs its calls without belt checks/)
})
