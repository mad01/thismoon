import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

import record from '../extensions/moondb-record.ts'
import { fakePi, printCtx, scratch } from './fake.mjs'

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

function load() {
  const fake = fakePi()
  record(fake.pi)
  return fake
}

// prompt opens a turn and returns its UserPromptSubmit payload.
async function prompt(env, call, ctx, text = 'fix the bug') {
  const before = env.calls().length
  assert.equal(await call('before_agent_start', { messageType: 'user', prompt: text, systemPrompt: 'base' }, ctx), undefined)
  return (await env.waitCalls(before + 1)).at(-1).payload
}

test('a prompt opens a turn and a settle closes it under the same prompt id', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir, { name: 'Refactor auth' })

  const open = await prompt(env, call, ctx)
  assert.match(open.prompt_id, UUID)
  assert.deepEqual(open, {
    hook_event_name: 'UserPromptSubmit',
    session_id: 's1',
    transcript_path: join(env.dir, 's1.jsonl'),
    cwd: env.dir,
    session_title: 'Refactor auth',
    prompt_id: open.prompt_id,
    prompt: 'fix the bug',
  })

  const assistant = { role: 'assistant', content: [{ type: 'thinking', thinking: 'x' }, { type: 'text', text: 'Done.' }] }
  await call('agent_end', { messages: [{ role: 'user', content: 'fix the bug' }, assistant] }, ctx)
  await call('agent_settled', { aborted: false }, ctx)
  const calls = await env.waitCalls(2)
  assert.deepEqual(calls[1].payload, {
    hook_event_name: 'Stop',
    session_id: 's1',
    transcript_path: join(env.dir, 's1.jsonl'),
    cwd: env.dir,
    session_title: 'Refactor auth',
    prompt_id: open.prompt_id,
    last_assistant_message: 'Done.',
  })
  for (const c of calls) {
    assert.deepEqual(c.argv, ['record'])
    // The inherited Claude entrypoint is replaced by pi's mode, and the
    // harness names pi.
    assert.equal(c.entrypoint, 'pi-print')
    assert.equal(c.harness, 'pi')
  }

  // A settle with no turn open records nothing more.
  await call('agent_settled', { aborted: false }, ctx)
  await call('session_shutdown', { reason: 'quit' }, ctx)
  assert.equal(env.calls().length, 2)
})

test('each prompt gets its own turn, and a custom run one without a prompt', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir)
  const first = await prompt(env, call, ctx)
  await call('agent_settled', { aborted: false }, ctx)
  await env.waitCalls(2)
  const second = await prompt(env, call, ctx, 'and the docs')
  assert.notEqual(second.prompt_id, first.prompt_id)

  // A custom run starting while a turn is open closes it first.
  await call('before_agent_start', { messageType: 'custom', message: { content: 'x' } }, ctx)
  const calls = await env.waitCalls(5)
  assert.deepEqual(calls.slice(3).map((c) => [c.payload.hook_event_name, c.payload.prompt]).sort(), [
    ['Stop', undefined],
    ['UserPromptSubmit', ''],
  ])
})

test('an edit records as a Claude Edit with the hunks of pi\'s patch, and is waited for', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir)
  const open = await prompt(env, call, ctx)
  process.env.FAKE_MOONDB_SLEEP = '0.2'

  const patch = [
    '--- src/a.go',
    '+++ src/a.go',
    '@@ -1,2 +1,2 @@',
    ' package a',
    '-var x = 1',
    '+var x = 2',
    '',
  ].join('\n')
  const event = {
    toolName: 'edit',
    toolCallId: 'toolu_1',
    input: { path: 'src/a.go', edits: [{ oldText: 'var x = 1', newText: 'var x = 2' }] },
    details: { diff: '...', patch, firstChangedLine: 2 },
    content: [{ type: 'text', text: 'ok' }],
    isError: false,
  }
  assert.equal(await call('tool_call', { ...event, type: 'tool_call' }, ctx), undefined)
  assert.equal(await call('tool_result', event, ctx), undefined)
  // Waited for: the record finished before tool_result resolved.
  const calls = env.calls()
  assert.equal(calls.length, 2)
  const file = join(env.dir, 'src/a.go')
  assert.deepEqual(calls[1].payload, {
    hook_event_name: 'PostToolUse',
    session_id: 's1',
    transcript_path: join(env.dir, 's1.jsonl'),
    cwd: env.dir,
    prompt_id: open.prompt_id,
    tool_name: 'Edit',
    tool_input: { file_path: file, old_string: 'var x = 1', new_string: 'var x = 2', replace_all: false },
    tool_response: {
      filePath: file,
      userModified: false,
      structuredPatch: [{ oldStart: 1, oldLines: 2, newStart: 1, newLines: 2, lines: [' package a', '-var x = 1', '+var x = 2'] }],
    },
    tool_use_id: 'toolu_1',
  })
})

test('several replacements record as a MultiEdit, the legacy pair too', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir)
  await prompt(env, call, ctx)
  const edits = [{ oldText: 'a', newText: 'b' }, { oldText: 'c', newText: 'd' }]
  await call('tool_result', { toolName: 'edit', toolCallId: 't1', input: { path: '/abs/f.md', edits }, isError: false }, ctx)
  await call('tool_result', { toolName: 'edit', toolCallId: 't2', input: { path: '@rel.md', oldText: 'x', newText: 'y' }, isError: false }, ctx)
  const [multi, single] = env.calls().slice(1).map((c) => c.payload)
  assert.equal(multi.tool_name, 'MultiEdit')
  assert.deepEqual(multi.tool_input, {
    file_path: '/abs/f.md',
    edits: [
      { old_string: 'a', new_string: 'b', replace_all: false },
      { old_string: 'c', new_string: 'd', replace_all: false },
    ],
  })
  // No patch in the details: no structuredPatch either.
  assert.deepEqual(multi.tool_response, { filePath: '/abs/f.md', userModified: false })
  assert.equal(single.tool_name, 'Edit')
  assert.equal(single.tool_input.file_path, join(env.dir, 'rel.md'))
})

test('a write is a create when the file was absent and an update with its original otherwise', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir)
  await prompt(env, call, ctx)

  const fresh = { toolName: 'write', toolCallId: 'w1', input: { path: 'new.txt', content: 'hello\n' }, isError: false }
  await call('tool_call', fresh, ctx)
  writeFileSync(join(env.dir, 'new.txt'), 'hello\n')
  await call('tool_result', fresh, ctx)

  const over = { toolName: 'write', toolCallId: 'w2', input: { path: 'new.txt', content: 'hello\nworld\n' }, isError: false }
  await call('tool_call', over, ctx)
  writeFileSync(join(env.dir, 'new.txt'), 'hello\nworld\n')
  await call('tool_result', over, ctx)

  const [create, update] = env.calls().slice(1).map((c) => c.payload)
  const file = join(env.dir, 'new.txt')
  assert.equal(create.tool_name, 'Write')
  assert.deepEqual(create.tool_input, { file_path: file, content: 'hello\n' })
  assert.deepEqual(create.tool_response, {
    filePath: file, content: 'hello\n', type: 'create', originalFile: null, structuredPatch: [],
  })
  assert.deepEqual(update.tool_response, {
    filePath: file,
    content: 'hello\nworld\n',
    type: 'update',
    originalFile: 'hello\n',
    structuredPatch: [{ oldStart: 1, oldLines: 1, newStart: 1, newLines: 2, lines: [' hello', '+world'] }],
  })
})

test('a failed edit or write and tools moondb does not track record nothing', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir)
  await prompt(env, call, ctx)
  const results = [
    { toolName: 'edit', toolCallId: 'e', input: { path: 'a', edits: [{ oldText: 'a', newText: 'b' }] }, isError: true },
    { toolName: 'write', toolCallId: 'w', input: { path: 'a', content: 'x' }, isError: true },
    { toolName: 'read', toolCallId: 'r', input: { path: 'a' }, isError: false },
    { toolName: 'edit', toolCallId: 'x', input: undefined, isError: false },
    { toolName: 'write', toolCallId: 'y', input: { path: 'a' }, isError: false },
  ]
  for (const event of results) assert.equal(await call('tool_result', event, ctx), undefined)
  assert.equal(env.calls().length, 1)
})

function git(dir, ...args) {
  execFileSync('git', ['-C', dir, '-c', 'user.email=t@example.com', '-c', 'user.name=t', ...args])
}

test('a bash call records the files it changed as a Bash bashEditDiff', async (t) => {
  const env = scratch(t, { repo: true })
  const { call } = load()
  const ctx = printCtx(env.repoDir)
  const open = await prompt(env, call, ctx)
  // Dirty before the command: left alone it is not the command's work,
  // changed it is, but its before is unknown, so it gets no hunks.
  writeFileSync(join(env.repoDir, 'untouched.txt'), 'already here\n')
  writeFileSync(join(env.repoDir, 'dirty.txt'), 'draft\n')
  writeFileSync(join(env.repoDir, 'b.txt'), 'b\n')
  git(env.repoDir, 'add', 'b.txt')
  git(env.repoDir, 'commit', '-qm', 'b')

  const bash = { toolName: 'bash', toolCallId: 'b1', input: { command: 'make gen' } }
  assert.equal(await call('tool_call', bash, ctx), undefined)
  writeFileSync(join(env.repoDir, 'a.txt'), 'one\ntwo\n')
  writeFileSync(join(env.repoDir, 'gen.txt'), 'generated\n')
  writeFileSync(join(env.repoDir, 'dirty.txt'), 'draft\nmore\n')
  git(env.repoDir, 'rm', '-q', 'b.txt')
  // An edit tool's write while the command ran is left to its own record.
  writeFileSync(join(env.repoDir, 'edited.txt'), 'by the edit tool\n')
  await call('tool_result', { toolName: 'edit', toolCallId: 'e1', input: { path: 'edited.txt', oldText: 'a', newText: 'b' }, isError: false }, ctx)
  await call('tool_result', { ...bash, content: [], isError: true, details: { exit_code: 2 } }, ctx)

  const calls = await env.waitCalls(3)
  const p = calls[2].payload
  const at = (name) => join(env.repoDir, name)
  assert.deepEqual(p, {
    hook_event_name: 'PostToolUse',
    session_id: 's1',
    transcript_path: join(env.repoDir, 's1.jsonl'),
    cwd: env.repoDir,
    prompt_id: open.prompt_id,
    tool_name: 'Bash',
    tool_input: { command: 'make gen' },
    tool_response: {
      bashEditDiff: {
        // Hunks for what was clean before: a.txt against HEAD, gen.txt new.
        // b.txt is gone and dirty.txt was dirty before: path only.
        files: [
          { filePath: at('a.txt'), hunks: [{ oldStart: 1, oldLines: 1, newStart: 1, newLines: 2, lines: [' one', '+two'] }] },
          { filePath: at('gen.txt'), hunks: [{ oldStart: 0, oldLines: 0, newStart: 1, newLines: 1, lines: ['+generated'] }], created: true },
        ],
        changedFiles: [at('a.txt'), at('b.txt'), at('dirty.txt'), at('gen.txt')],
      },
    },
    tool_use_id: 'b1',
  })
})

test('a bash call that changed nothing, or ran outside a repository, records nothing', async (t) => {
  const env = scratch(t, { repo: true })
  const { call } = load()
  await prompt(env, call, printCtx(env.repoDir))
  const bash = { toolName: 'bash', toolCallId: 'b1', input: { command: 'ls' } }
  await call('tool_call', bash, printCtx(env.repoDir))
  await call('tool_result', { ...bash, isError: false }, printCtx(env.repoDir))
  // env.dir holds the repo but is not one.
  await call('tool_call', { ...bash, toolCallId: 'b2' }, printCtx(env.dir))
  writeFileSync(join(env.dir, 'x.txt'), 'x\n')
  await call('tool_result', { ...bash, toolCallId: 'b2', isError: false }, printCtx(env.dir))
  await call('session_shutdown', { reason: 'quit' }, printCtx(env.repoDir))
  assert.deepEqual(env.calls().map((c) => c.payload.hook_event_name), ['UserPromptSubmit', 'Stop'])
})

test('shutdown closes the open turn and waits for the records in flight', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = printCtx(env.dir)
  process.env.FAKE_MOONDB_SLEEP = '0.2'
  await call('before_agent_start', { messageType: 'user', prompt: 'p', systemPrompt: '' }, ctx)
  assert.equal(env.calls().length, 0)
  await call('session_shutdown', { reason: 'quit' }, ctx)
  assert.deepEqual(env.calls().map((c) => c.payload.hook_event_name).sort(), ['Stop', 'UserPromptSubmit'])
})

test('a session without a file sends an empty transcript path', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const ctx = { ...printCtx(env.dir), mode: 'tui', sessionManager: { getSessionId: () => 's2', getSessionFile: () => undefined } }
  await call('before_agent_start', { messageType: 'user', prompt: 'p' }, ctx)
  const [c] = await env.waitCalls(1)
  assert.equal(c.payload.transcript_path, '')
  assert.equal(c.payload.session_title, undefined)
  assert.equal(c.entrypoint, 'pi-tui')
})

test('a handler that hits bad input stays silent', async (t) => {
  const env = scratch(t)
  const { call } = load()
  const broken = { cwd: env.dir, mode: 'print', sessionManager: {} }
  assert.equal(await call('before_agent_start', { messageType: 'user', prompt: 'p' }, broken), undefined)
  assert.equal(await call('tool_call', { toolName: 'write', toolCallId: 'w', input: { path: 7 } }, broken), undefined)
  assert.equal(await call('tool_result', { toolName: 'edit', toolCallId: 'e', input: null }, broken), undefined)
  assert.equal(await call('agent_end', {}, broken), undefined)
  assert.equal(env.calls().length, 0)
})
