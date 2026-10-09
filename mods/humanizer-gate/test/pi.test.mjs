import assert from 'node:assert/strict'
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import face from '../pi/index.ts'
import { CANCELLED_REASON } from '../lib/gate.ts'

// A fake humanizer at ~/code/bin/humanizer (HOME is a temp dir) that reports
// one error-level finding for any input and records how it was called.
function fakeHome() {
  const home = mkdtempSync(join(tmpdir(), 'humanizer-gate-pi-'))
  const argvLog = join(home, 'argv')
  mkdirSync(join(home, 'code', 'bin'), { recursive: true })
  writeFileSync(
    join(home, 'code', 'bin', 'humanizer'),
    [
      '#!/bin/sh',
      // The real binary reads the text from stdin; consume it so spawnSync never
      // sees a broken pipe (Linux closes the pipe at child exit).
      'cat > /dev/null',
      `printf '%s\\n' "$@" > "${argvLog}"`,
      `printf '%s' '{"findings":[{"severity":"error","match":"delve into"},{"severity":"warning","match":"robust"}]}'`,
    ].join('\n'),
  )
  chmodSync(join(home, 'code', 'bin', 'humanizer'), 0o755)
  return { home, argvLog }
}

function fakePi() {
  const handlers = new Map()
  const pi = {
    on(name, handler) {
      handlers.set(name, [...(handlers.get(name) ?? []), handler])
    },
  }
  return { pi, handlers }
}

function fakeCtx(cwd, answer) {
  const statuses = []
  const questions = []
  return {
    ctx: {
      cwd,
      hasUI: true,
      ui: {
        setStatus: (key, text) => statuses.push([key, text]),
        select: async (question, options) => {
          questions.push({ question, options })
          return answer
        },
      },
    },
    statuses,
    questions,
  }
}

async function started(home) {
  const saved = process.env.HOME
  process.env.HOME = home
  const { pi, handlers } = fakePi()
  face(pi)
  await handlers.get('session_start')[0]({ reason: 'new' }, {})
  process.env.HOME = saved
  return handlers
}

test('holds a PR body with an error-level finding until the person answers', async t => {
  const { home, argvLog } = fakeHome()
  t.after(() => rmSync(home, { recursive: true, force: true }))
  const handlers = await started(home)
  const toolCall = handlers.get('tool_call')[0]

  const cancel = fakeCtx(home, 'Cancel')
  const blocked = await toolCall(
    { toolName: 'mcp', input: { server: 'gh_com', tool: 'create_pull_request', args: { body: 'We delve into it.' } } },
    cancel.ctx,
  )
  assert.deepEqual(blocked, { block: true, reason: CANCELLED_REASON })
  assert.deepEqual(cancel.statuses.at(-1), ['humanizer-gate', '1 error, 1 warning in PR body'])
  assert.deepEqual(cancel.questions[0].options, ['Proceed', 'Cancel'])
  assert.match(cancel.questions[0].question, /1 error-level finding in the PR body \("delve into", "robust"\)/)

  const proceed = fakeCtx(home, 'Proceed')
  const allowed = await toolCall({ toolName: 'mcp__gh_com__update_pull_request', input: { body: 'text' } }, proceed.ctx)
  assert.equal(allowed, undefined)
  assert.equal(proceed.questions.length, 1)

  // Other servers and other gh_com operations are never scanned.
  rmSync(argvLog, { force: true })
  const other = fakeCtx(home, 'Cancel')
  assert.equal(await toolCall({ toolName: 'mcp__gh_com__add_issue_comment', input: { body: 'x' } }, other.ctx), undefined)
  assert.equal(await toolCall({ toolName: 'mcp__linear__save_issue', input: { body: 'x' } }, other.ctx), undefined)
  assert.equal(other.questions.length, 0)
})

test('reports a commit message in the status line and does not hold it by default', async t => {
  const { home, argvLog } = fakeHome()
  t.after(() => rmSync(home, { recursive: true, force: true }))
  const handlers = await started(home)
  const { ctx, statuses, questions } = fakeCtx(home, 'Cancel')
  const result = await handlers.get('tool_call')[0](
    { toolName: 'bash', input: { command: 'git add -A && git commit -m "feat: delve into it"' } },
    ctx,
  )
  assert.equal(result, undefined)
  assert.equal(questions.length, 0)
  assert.deepEqual(statuses.at(-1), ['humanizer-gate', '1 error, 1 warning in commit message'])
  const { readFileSync } = await import('node:fs')
  assert.deepEqual(readFileSync(argvLog, 'utf8').trimEnd().split('\n'), ['detect', '--json', '--min-severity', 'warning'])
})

test('scans a markdown write after it landed and ignores other files', async t => {
  const { home, argvLog } = fakeHome()
  t.after(() => rmSync(home, { recursive: true, force: true }))
  const handlers = await started(home)
  const toolResult = handlers.get('tool_result')[0]
  const { ctx, statuses } = fakeCtx(home, 'Proceed')
  await toolResult({ toolName: 'write', input: { path: 'docs/notes.md', content: 'x' }, content: [] }, ctx)
  assert.deepEqual(statuses.at(-1), ['humanizer-gate', '1 error, 1 warning in notes.md'])
  const { readFileSync } = await import('node:fs')
  assert.equal(readFileSync(argvLog, 'utf8').trimEnd().split('\n').at(-1), join(home, 'docs/notes.md'))
  rmSync(argvLog)
  await toolResult({ toolName: 'write', input: { path: 'main.go', content: 'x' }, content: [] }, ctx)
  await toolResult({ toolName: 'read', input: { path: 'README.md' }, content: [] }, ctx)
  const { existsSync } = await import('node:fs')
  assert.equal(existsSync(argvLog), false)
})

test('passes everything through when the humanizer binary is missing', async t => {
  const home = mkdtempSync(join(tmpdir(), 'humanizer-gate-pi-nobin-'))
  t.after(() => rmSync(home, { recursive: true, force: true }))
  const handlers = await started(home)
  const { ctx, statuses, questions } = fakeCtx(home, 'Cancel')
  const result = await handlers.get('tool_call')[0](
    { toolName: 'mcp__gh_com__create_pull_request', input: { body: 'delve' } },
    ctx,
  )
  assert.equal(result, undefined)
  assert.equal(statuses.length, 0)
  assert.equal(questions.length, 0)
})
