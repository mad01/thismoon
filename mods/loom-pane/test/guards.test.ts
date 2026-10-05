// The guards against the engine's own test kit: `claude plugin test
// mods/loom-pane`. The hooks registered here stand beneath the plugin for
// the engine (git, the cwd, the state store, the dialog), answered from
// memory.
import { expect, mock, test } from 'claude-code/testing'
import type { On, ProcessRunResult } from 'claude-code'

const HOME = '/home/u'
const CANONICAL = '/home/u/code/thismoon'
const OWN = '/home/u/.worktrees/thismoon/feat-pane'

const ok = (stdout: string): ProcessRunResult => ({
  exitCode: 0,
  stdout,
  stderr: '',
  isStdoutTruncated: false,
  isStderrTruncated: false,
})
const failed = (): ProcessRunResult => ({
  exitCode: 128,
  stdout: '',
  stderr: 'fatal',
  isStdoutTruncated: false,
  isStderrTruncated: false,
})

const porcelain = [
  `worktree ${CANONICAL}`,
  'HEAD 1111111111111111111111111111111111111111',
  'branch refs/heads/main',
  '',
  `worktree ${OWN}`,
  'HEAD 2222222222222222222222222222222222222222',
  'branch refs/heads/feat/pane',
  '',
].join('\n')

type Repo = { dirty: readonly string[]; ahead: number; remoteAhead: number | null }

// A git answered from memory: the two worktrees above, every branch `ahead`
// commits past main, `remoteAhead` of them not on origin (null: no upstream).
function fakeGit(repo: Repo, argv: readonly string[], cwd: string): ProcessRunResult {
  const line = argv.slice(1).join(' ')
  if (line === 'rev-parse --show-toplevel') return ok(cwd.startsWith(OWN) ? OWN : CANONICAL)
  if (line.endsWith('--git-common-dir')) return ok(`${CANONICAL}/.git`)
  if (line === 'worktree list --porcelain') return ok(porcelain)
  if (line.startsWith('symbolic-ref')) return failed()
  if (line === 'status --porcelain') return ok(repo.dirty.includes(cwd) ? ' M x\n' : '')
  if (line.startsWith('rev-list --count origin/')) return repo.remoteAhead === null ? failed() : ok(`${repo.remoteAhead}\n`)
  if (line.startsWith('rev-list --count')) return ok(`${repo.ahead}\n`)
  if (line.startsWith('log -1')) return ok('3 days ago\n')
  return failed()
}

type Seat = { asked: string[]; answer: 'Proceed' | 'Cancel' | 'dismiss' }

// The engine beneath the plugin.
function seat(on: On, repo: Repo, cwd: string, answer: Seat['answer'] = 'Proceed'): Seat {
  const seated: Seat = { asked: [], answer }
  mock.env(on, { HOME })
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
  on('process.run', (_$, e) => ({ value: fakeGit(repo, e.argv, e.init?.cwd ?? cwd) }))
  on('session.cwd', () => ({ value: cwd }))
  on('command.register', (_$, e) => ({ value: { command: e.name } }))
  on('ui.status', () => ({ value: undefined }))
  on('ui.log', () => ({ value: undefined }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('tool.call', { tool: 'Write' }, () => ({ result: { type: 'create', filePath: '', content: '', structuredPatch: [] } }))
  on('tool.call', { tool: 'Bash' }, () => ({ result: { stdout: '', stderr: '', interrupted: false } }))
  on('tool.call', { tool: 'AskUserQuestion' }, (_$, e) => {
    const question = e.questions[0]?.question ?? ''
    seated.asked.push(question)
    if (seated.answer === 'dismiss') return { deny: 'dismissed' }
    return { result: { questions: e.questions, answers: { [question]: seated.answer } } }
  })
  return seated
}

const refusalOf = (ran: { deny?: string; isError?: true; text?: string }): string | undefined =>
  ran.deny ?? (ran.isError === true ? ran.text : undefined)

test('an owner writes its own worktree and nothing else of the repo', async ($, on) => {
  seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })

  const canonical = await $.tool.call({ tool: 'Write', file_path: `${CANONICAL}/README.md`, content: 'x' })
  expect(refusalOf(canonical)).toMatch(/belongs to the canonical checkout/)

  const dotted = await $.tool.call({ tool: 'Write', file_path: `${OWN}/../../../code/thismoon/README.md`, content: 'x' })
  expect(refusalOf(dotted)).toMatch(/belongs to the canonical checkout/)

  const own = await $.tool.call({ tool: 'Write', file_path: `${OWN}/README.md`, content: 'x' })
  expect(refusalOf(own)).toBeUndefined()

  const outside = await $.tool.call({ tool: 'Write', file_path: '/tmp/notes.md', content: 'x' })
  expect(refusalOf(outside)).toBeUndefined()

  const commit = await $.tool.call({ tool: 'Bash', command: `git -C ${CANONICAL} commit -m x` })
  expect(refusalOf(commit)).toMatch(/owners edit only their own worktree/)

  const look = await $.tool.call({ tool: 'Bash', command: `git -C ${CANONICAL} log -1 && (cd ${CANONICAL} && git status)` })
  expect(refusalOf(look)).toBeUndefined()
})

test('guards off lets every call through', { options: { guards: false } }, async ($, on) => {
  seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  const ran = await $.tool.call({ tool: 'Write', file_path: `${CANONICAL}/README.md`, content: 'x' })
  expect(refusalOf(ran)).toBeUndefined()
})

test('a cd into a loom worktree makes the session its owner', async ($, on) => {
  seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, CANONICAL)
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })

  const before = await $.tool.call({ tool: 'Write', file_path: `${CANONICAL}/README.md`, content: 'x' })
  expect(refusalOf(before)).toBeUndefined()

  const claim = await $.tool.call({ tool: 'Bash', command: 'cd ~/.worktrees/thismoon/feat-pane && git status' })
  expect(refusalOf(claim)).toBeUndefined()

  const after = await $.tool.call({ tool: 'Write', file_path: `${CANONICAL}/README.md`, content: 'x' })
  expect(refusalOf(after)).toMatch(/belongs to the canonical checkout/)

  const own = await $.tool.call({ tool: 'Bash', command: `cd ${OWN} && git commit -m x` })
  expect(refusalOf(own)).toBeUndefined()
})

test('a dismissed removal dialog keeps the branch in an interactive session', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 2, remoteAhead: null }, CANONICAL, 'dismiss')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  const ran = await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' })
  expect(seated.asked.length).toBe(1)
  expect(seated.asked[0]).toMatch(/feat\/x has 2 unmerged commits/)
  expect(refusalOf(ran)).toMatch(/feat\/x kept/)
})

test('Cancel keeps the branch, Proceed removes it', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 2, remoteAhead: null }, CANONICAL, 'Cancel')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' }))).toMatch(/kept/)
  seated.answer = 'Proceed'
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' }))).toBeUndefined()
  expect(seated.asked.length).toBe(2)
})

test('headless, a dismissed dialog lets the removal through', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 2, remoteAhead: null }, CANONICAL, 'dismiss')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: false })
  const ran = await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' })
  expect(seated.asked.length).toBe(1)
  expect(refusalOf(ran)).toBeUndefined()
})

test('a pushed branch and a clean worktree are removed without a question', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 2, remoteAhead: 0 }, CANONICAL, 'Cancel')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' }))).toBeUndefined()
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git worktree remove ${OWN}` }))).toBeUndefined()
  expect(seated.asked.length).toBe(0)
})

test('a forced removal of a dirty worktree asks', async ($, on) => {
  const seated = seat(on, { dirty: [OWN], ahead: 2, remoteAhead: 0 }, CANONICAL, 'Cancel')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git worktree remove ${OWN}` }))).toBeUndefined()
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git worktree remove --force ${OWN}` }))).toMatch(/kept/)
  expect(seated.asked).toEqual([`${OWN} has uncommitted changes. Remove anyway?`])
})
