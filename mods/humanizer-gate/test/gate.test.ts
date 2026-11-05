// The PR and commit gates against the engine's own test kit: `claude plugin
// test mods/humanizer-gate`. The hooks registered here stand beneath the
// plugin for the engine: HOME and the binary's presence, a humanizer detect
// answered from memory, the state store, the session id, the dialog, and the
// tools the gated calls reach when they go through.
import { expect, mock, test } from 'claude-code/testing'
import type { On, ProcessRunResult } from 'claude-code'

const HOME = '/home/u'
const CANCELLED = 'humanizer-gate: cancelled by user'

// What the fake detect reports: each word below, wherever it sits on the
// first line of the text, as the finding the real rule pack gives it.
const RULES: Record<string, { rule_id: string; severity: string; message: string }> = {
  delve: { rule_id: 'Humanizer.AIVocabulary', severity: 'warning', message: "AI vocabulary word: 'delve'." },
  'is a testament': {
    rule_id: 'Humanizer.SignificanceInflation',
    severity: 'error',
    message: "Significance-inflation phrase: 'is a testament'.",
  },
}

function detected(text: string): ProcessRunResult {
  const findings = Object.entries(RULES)
    .filter(([match]) => text.includes(match))
    .map(([match, rule]) => ({ ...rule, line: 1, column: text.indexOf(match) + 1, match }))
  return { exitCode: 0, stdout: JSON.stringify({ findings }), stderr: '', isStdoutTruncated: false, isStderrTruncated: false }
}

type World = {
  sessionId: string
  answer: 'Proceed' | 'Cancel'
  asked: string[]
  statuses: Array<string | undefined>
  // The calls that reached the tools beneath the mod, so went through.
  sent: string[]
}

function seat(on: On): World {
  const world: World = { sessionId: 's1', answer: 'Proceed', asked: [], statuses: [], sent: [] }
  mock.env(on, { HOME })
  on('fs.exists', (_$, e) => ({ value: e.path === `${HOME}/code/bin/humanizer` }))
  on('process.run', (_$, e) => ({ value: detected(e.init?.stdin ?? '') }))
  const values = new Map<string, unknown>()
  const versions = new Map<string, number>()
  on('state.get', (_$, e) => {
    const key = `${e.plugin}/${e.key}`
    return { value: { value: values.get(key), version: versions.get(key) ?? 0 } }
  })
  on('state.set', (_$, e) => {
    const key = `${e.plugin}/${e.key}`
    values.set(key, e.value)
    const version = (versions.get(key) ?? 0) + 1
    versions.set(key, version)
    return { value: { isSet: true, version } }
  })
  on('session.id', () => ({ value: world.sessionId }))
  on('ui.status', (_$, e) => {
    world.statuses.push(e.text)
    return { value: undefined }
  })
  on('ui.log', () => ({ value: undefined }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('tool.call', { tool: 'AskUserQuestion' }, (_$, e) => {
    const question = e.questions[0]?.question ?? ''
    world.asked.push(question)
    return { result: { questions: e.questions, answers: { [question]: world.answer } } }
  })
  on('tool.call', { tool: 'mcp__gh_com__create_pull_request' }, (_$, e) => {
    world.sent.push(String(e.body))
    return { result: 'created' }
  })
  on('tool.call', { tool: 'Bash' }, (_$, e) => {
    world.sent.push(e.command)
    return { result: { stdout: '', stderr: '', interrupted: false } }
  })
  return world
}

const pr = (body: string) => ({
  tool: 'mcp__gh_com__create_pull_request' as const,
  owner: 'o',
  repo: 'r',
  title: 't',
  head: 'h',
  base: 'main',
  body,
})
const commit = (message: string) => ({ tool: 'Bash' as const, command: `git commit -m "${message}"` })

const denyOf = (ran: { deny?: string }): string | undefined => ran.deny

async function started($: Parameters<Parameters<typeof test>[1]>[0]): Promise<void> {
  await $.session.start({ cwd: '/repo', surface: null, isInteractive: true })
}

test('a PR body with a warning is denied once, each finding in the reason', async ($, on) => {
  const world = seat(on)
  await started($)
  const ran = await $.tool.call(pr('We delve into it.'))
  const reason = denyOf(ran) ?? ''
  expect(reason).toMatch(/^humanizer-gate: not sent\. humanizer detect found 1 finding in the PR body/)
  expect(reason).toContain(`- Humanizer.AIVocabulary 1:4 "delve": AI vocabulary word: 'delve'.`)
  expect(reason).toMatch(/send the same PR body unchanged and it goes through/)
  expect(world.sent).toEqual([])
  expect(world.asked).toEqual([])
  expect(world.statuses.at(-1)).toBe('1 warning in PR body')
})

test('the same PR body sent again goes through', async ($, on) => {
  const world = seat(on)
  await started($)
  await $.tool.call(pr('We delve into it.'))
  const again = await $.tool.call(pr('We delve into it.'))
  expect(denyOf(again)).toBeUndefined()
  expect(world.sent).toEqual(['We delve into it.'])
})

test('a changed PR body is checked again', async ($, on) => {
  const world = seat(on)
  await started($)
  await $.tool.call(pr('We delve into it.'))
  const changed = await $.tool.call(pr('Now we delve deeper.'))
  expect(denyOf(changed)).toMatch(/Humanizer\.AIVocabulary 1:8 "delve"/)
  const fixed = await $.tool.call(pr('Now we look closer.'))
  expect(denyOf(fixed)).toBeUndefined()
  expect(world.sent).toEqual(['Now we look closer.'])
})

test('a clean PR body goes through on the first send', async ($, on) => {
  const world = seat(on)
  await started($)
  const ran = await $.tool.call(pr('Adds a flag.'))
  expect(denyOf(ran)).toBeUndefined()
  expect(world.sent).toEqual(['Adds a flag.'])
  expect(world.statuses.at(-1)).toBeUndefined()
})

test('an error-level finding still holds the PR for Proceed / Cancel, with no feedback deny', async ($, on) => {
  const world = seat(on)
  await started($)
  world.answer = 'Cancel'
  const cancelled = await $.tool.call(pr('It is a testament; we delve.'))
  expect(denyOf(cancelled)).toBe(CANCELLED)
  world.answer = 'Proceed'
  const proceeded = await $.tool.call(pr('It is a testament; we delve.'))
  expect(denyOf(proceeded)).toBeUndefined()
  expect(world.asked).toHaveLength(2)
  expect(world.asked[0]).toMatch(/1 error-level finding in the PR body/)
  expect(world.sent).toEqual(['It is a testament; we delve.'])
})

test('feedbackWarnings off: a warning reaches the status line only', { options: { feedbackWarnings: false } }, async ($, on) => {
  const world = seat(on)
  await started($)
  const ran = await $.tool.call(pr('We delve into it.'))
  expect(denyOf(ran)).toBeUndefined()
  expect(world.sent).toEqual(['We delve into it.'])
  expect(world.statuses.at(-1)).toBe('1 warning in PR body')
})

test('a commit message with a finding is denied once, errors included, and the same message passes', async ($, on) => {
  const world = seat(on)
  await started($)
  const first = await $.tool.call(commit('feat: it is a testament'))
  expect(denyOf(first)).toContain('- Humanizer.SignificanceInflation 1:10 "is a testament"')
  expect(denyOf(first)).toMatch(/in the commit message/)
  expect(world.asked).toEqual([])
  const again = await $.tool.call(commit('feat: it is a testament'))
  expect(denyOf(again)).toBeUndefined()
  expect(world.sent).toEqual(['git commit -m "feat: it is a testament"'])
})

test('a text denied in another session is checked again', async ($, on) => {
  const world = seat(on)
  await started($)
  await $.tool.call(pr('We delve into it.'))
  world.sessionId = 's2'
  const other = await $.tool.call(pr('We delve into it.'))
  expect(denyOf(other)).toMatch(/humanizer-gate: not sent/)
  expect(world.sent).toEqual([])
})
