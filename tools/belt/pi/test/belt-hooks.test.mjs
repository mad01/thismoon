import assert from 'node:assert/strict'
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
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
  const res = await call('tool_result', { toolName: 'ls', input: { path: 'x' }, content: OUT }, headlessCtx(env.dir))
  assert.equal(res, undefined)
  assert.equal(env.argv(), null)
})

test('csl searches pass through without running belt', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const res = await call('tool_result', { toolName: 'csl_search', input: { query: 'x' }, content: OUT }, headlessCtx(env.dir))
  assert.equal(res, undefined)
  assert.equal(env.argv(), null)
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

// A file of n newline-terminated lines, which pi's read tool counts as n + 1.
const writeLines = (path, n) => writeFileSync(path, 'x\n'.repeat(n))

test('an unranged read runs the read hint with the Claude Read payload', async (t) => {
  const env = scratch(t)
  writeLines(join(env.dir, 'big.ts'), 350)
  const { call } = load()
  const event = { toolName: 'read', input: { path: 'big.ts' }, content: OUT, isError: false }
  const res = await call('tool_result', event, headlessCtx(env.dir))
  assert.deepEqual(res, { content: [...OUT, advice('read')] })
  assert.deepEqual(env.argv(), ['hint', 'read'])
  assert.deepEqual(env.stdin(), {
    cwd: env.dir,
    session_id: 's1',
    transcript_path: '',
    tool_name: 'Read',
    tool_input: { file_path: join(env.dir, 'big.ts') },
    tool_response: { type: 'text', file: { totalLines: 351 } },
  })
})

test('a ranged read passes its range and counts the whole file on disk', async (t) => {
  const env = scratch(t)
  const file = join(env.dir, 'big.ts')
  writeLines(file, 350)
  const { call } = load()
  // A ranged read's details count the lines from its offset, not the file's.
  const details = { truncation: { totalLines: 41 } }
  const event = { toolName: 'read', input: { path: file, offset: 10, limit: 20 }, content: OUT, details }
  await call('tool_result', event, headlessCtx(env.dir))
  const { tool_input, tool_response } = env.stdin()
  assert.deepEqual(tool_input, { file_path: file, offset: 10, limit: 20 })
  assert.deepEqual(tool_response, { type: 'text', file: { totalLines: 351 } })
})

test('an unranged read pi truncated takes its line count from the details', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const details = { truncation: { totalLines: 5000 } }
  await call('tool_result', { toolName: 'read', input: { path: 'gone.ts' }, content: OUT, details }, headlessCtx(env.dir))
  assert.deepEqual(env.stdin().tool_response, { type: 'text', file: { totalLines: 5000 } })
})

test('a file that cannot be read counts no lines, for belt to treat as unknown', async (t) => {
  const env = scratch(t)
  const { call } = load()
  await call('tool_result', { toolName: 'read', input: { path: 'gone.ts' }, content: OUT }, headlessCtx(env.dir))
  assert.deepEqual(env.stdin().tool_response, { type: 'text', file: { totalLines: 0 } })
})

test('a read path gets the ~ and @ expansion pi gives it', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = headlessCtx(join(env.dir, 'work'))
  for (const [path, want] of [
    ['~/notes.ts', join(env.dir, 'notes.ts')],
    ['@src/a.ts', join(env.dir, 'work', 'src', 'a.ts')],
    ['/abs/b.ts', '/abs/b.ts'],
  ]) {
    await call('tool_result', { toolName: 'read', input: { path }, content: OUT }, ctx)
    assert.equal(env.stdin().tool_input.file_path, want, path)
  }
})

test('a failed read or an image read runs no hint', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = headlessCtx(env.dir)
  assert.equal(await call('tool_result', { toolName: 'read', input: { path: 'a.ts' }, content: OUT, isError: true }, ctx), undefined)
  const image = [{ type: 'text', text: 'Read image file [image/png]' }, { type: 'image', data: '', mimeType: 'image/png' }]
  assert.equal(await call('tool_result', { toolName: 'read', input: { path: 'a.png' }, content: image }, ctx), undefined)
  assert.equal(env.argv(), null)
})

test('csl_read runs the read hint under every name pi gives it', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = headlessCtx(env.dir)
  const args = { repo: 'thismoon', file: 'tools/belt/README.md' }
  const content = [{ type: 'text', text: 'mad01/thismoon/tools/belt/README.md lines 1-400 of 400\n...' }]
  for (const [toolName, input] of [
    ['mcp__csl__csl_read', args],
    ['csl_read', args],
    ['mcp', { server: 'csl', tool: 'csl_read', args }],
  ]) {
    env.clearLog()
    const res = await call('tool_result', { toolName, input, content }, ctx)
    assert.deepEqual(res, { content: [...content, advice('read')] }, toolName)
    assert.deepEqual(env.argv(), ['hint', 'read'], toolName)
    const { tool_name, tool_input, tool_response } = env.stdin()
    assert.deepEqual({ tool_name, tool_input, tool_response }, {
      tool_name: 'mcp__csl__csl_read',
      tool_input: args,
      tool_response: content,
    }, toolName)
  }
})

test('a failed csl_read runs no hint, and advice keeps a structured result', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = headlessCtx(env.dir)
  const input = { repo: 'thismoon', file: 'a.go' }
  assert.equal(await call('tool_result', { toolName: 'mcp__csl__csl_read', input, content: OUT, isError: true }, ctx), undefined)
  assert.equal(env.argv(), null)
  const structuredContent = { repo: 'mad01/thismoon', path: 'a.go', total_lines: 900 }
  const res = await call('tool_result', { toolName: 'mcp__csl__csl_read', input, content: OUT, structuredContent }, ctx)
  assert.deepEqual(res, { content: [...OUT, advice('read')], structuredContent })
})

test('a call a codemode script made gets no hint of any kind', async (t) => {
  const env = scratch(t)
  writeLines(join(env.dir, 'big.ts'), 350)
  const { call } = load()
  const ctx = headlessCtx(env.dir)
  const nested = { parentToolCallId: 'script-1', toolCallId: 'script-1/1', content: OUT }
  for (const [toolName, input] of [
    ['bash', { command: 'ls' }],
    ['read', { path: 'big.ts' }],
    ['mcp__csl__csl_read', { repo: 'thismoon', file: 'a.go' }],
    ['mcp__gh_com__add_issue_comment', { body: 'b' }],
  ]) {
    assert.equal(await call('tool_result', { ...nested, toolName, input }, ctx), undefined, toolName)
  }
  assert.equal(env.argv(), null)
})
