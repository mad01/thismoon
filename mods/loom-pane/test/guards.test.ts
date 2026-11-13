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

type Repo = {
  dirty: readonly string[]
  ahead: number
  remoteAhead: number | null
  isIntegrated?: boolean
  mergeTreeExit?: number
}

const MAIN_TREE = 'fe56c1cfe56c1cfe56c1cfe56c1cfe56c1cfe56c'
const OTHER_TREE = '0b1d2e30b1d2e30b1d2e30b1d2e30b1d2e30b1d'

// A git answered from memory: the two worktrees above, every branch `ahead`
// commits past main, `remoteAhead` of them not on origin (null: no upstream),
// and squash-merged into main when `isIntegrated`. merge-tree exits with
// `mergeTreeExit` (default 0), whatever tree it prints.
function fakeGit(repo: Repo, argv: readonly string[], cwd: string): ProcessRunResult {
  const line = argv.slice(1).join(' ')
  if (line === 'rev-parse --show-toplevel') return ok(cwd.startsWith(OWN) ? OWN : CANONICAL)
  if (line.endsWith('--git-common-dir')) return ok(`${CANONICAL}/.git`)
  if (line === 'rev-parse main^{tree}') return ok(`${MAIN_TREE}\n`)
  if (line.startsWith('merge-tree --write-tree main ')) {
    const merged = ok(`${repo.isIntegrated === true ? MAIN_TREE : OTHER_TREE}\n`)
    return { ...merged, exitCode: repo.mergeTreeExit ?? 0 }
  }
  if (line === 'worktree list --porcelain') return ok(porcelain)
  if (line.startsWith('symbolic-ref')) return failed()
  if (line === 'status --porcelain') return ok(repo.dirty.includes(cwd) ? ' M x\n' : '')
  if (line.startsWith('rev-list --count origin/')) return repo.remoteAhead === null ? failed() : ok(`${repo.remoteAhead}\n`)
  if (line.startsWith('rev-list --count')) return ok(`${repo.ahead}\n`)
  if (line.startsWith('log -1')) return ok('3 days ago\n')
  return failed()
}

type Seat = {
  asked: string[]
  answer: 'Proceed' | 'Cancel' | 'dismiss'
  sessionId: string
  // Stands for belt or the person: a command it refuses never runs.
  deniesBash: (command: string) => boolean
  // What the Agent tool answers: a finished agent and the worktree it names,
  // a background launch, or a refusal.
  agent: 'completed' | 'background' | 'denied'
}

// The engine beneath the plugin.
function seat(on: On, repo: Repo, cwd: string, answer: Seat['answer'] = 'Proceed'): Seat {
  const seated: Seat = { asked: [], answer, sessionId: 's-self', deniesBash: () => false, agent: 'completed' }
  mock.env(on, { HOME })
  on('session.id', () => ({ value: seated.sessionId }))
  on('turn.start', (_$, e) => ({ turnId: e.turnId }))
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
  on('ui.status', () => ({ value: undefined }))
  on('ui.log', () => ({ value: undefined }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('tool.call', { tool: 'Write' }, () => ({ result: { type: 'create', filePath: '', content: '', structuredPatch: [] } }))
  on('tool.call', { tool: 'Bash' }, (_$, e) =>
    seated.deniesBash(e.command) ? { deny: 'belt[test]: refused' } : { result: { stdout: '', stderr: '', interrupted: false } },
  )
  on('tool.call', { tool: 'Agent' }, () => {
    if (seated.agent === 'denied') return { deny: 'belt[test]: refused' }
    if (seated.agent === 'background') {
      return { result: { status: 'async_launched', agentId: 'a2', description: 'build', prompt: 'p', outputFile: '/tmp/a2' } }
    }
    return { result: finishedAgent(ISOLATED) }
  })
  on('tool.call', { tool: 'AskUserQuestion' }, (_$, e) => {
    const question = e.questions[0]?.question ?? ''
    seated.asked.push(question)
    if (seated.answer === 'dismiss') return { deny: 'dismissed' }
    return { result: { questions: e.questions, answers: { [question]: seated.answer } } }
  })
  return seated
}

// The isolation worktree the Agent tool made for agent a2, as its finished
// result names it.
const ISOLATED = `${CANONICAL}/.claude/worktrees/agent-a2`

function finishedAgent(worktreePath: string) {
  return {
    agentId: 'a2',
    content: [{ type: 'text' as const, text: 'done' }],
    totalToolUseCount: 1,
    totalDurationMs: 1,
    totalTokens: 1,
    usage: {
      input_tokens: 1,
      output_tokens: 1,
      cache_creation_input_tokens: null,
      cache_read_input_tokens: null,
      server_tool_use: null,
      service_tier: null,
      cache_creation: null,
    },
    status: 'completed' as const,
    prompt: 'p',
    worktreePath,
    worktreeBranch: 'worktree-agent-a2',
  }
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

const BUILDER = '/home/u/.worktrees/thismoon/feat-builder'
const OTHER = '/home/u/.worktrees/thismoon/fix-band'
const ADD_BUILDER = `git -C ${CANONICAL} worktree add ~/.worktrees/thismoon/feat-builder -b feat/builder`
const PUSH_BUILDER = `git -C ${BUILDER} status && git -C ${BUILDER} log origin/main..feat/builder && git -C ${BUILDER} push -u origin feat/builder`

test('the main loop pushes from a worktree its subagent created, and from no other session', async ($, on) => {
  seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_BUILDER }))).toMatch(/feat-builder belongs to another worktree/)

  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: ADD_BUILDER, agentId: 'a1' }))).toBeUndefined()
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_BUILDER }))).toBeUndefined()

  // Push only: a commit or a file write there is still refused.
  const commit = await $.tool.call({ tool: 'Bash', command: `git -C ${BUILDER} commit -m x && git -C ${BUILDER} push` })
  expect(refusalOf(commit)).toMatch(/feat-builder belongs to another worktree/)
  const write = await $.tool.call({ tool: 'Write', file_path: `${BUILDER}/README.md`, content: 'x' })
  expect(refusalOf(write)).toMatch(/feat-builder\/README.md belongs to another worktree/)

  // A worktree no subagent of this session created stays refused.
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git -C ${OTHER} push` }))).toMatch(/fix-band belongs to another worktree/)
})

test('a subagent add that did not run records nothing', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  seated.deniesBash = command => command.includes('worktree add')
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: ADD_BUILDER, agentId: 'a1' }))).toMatch(/belt\[test\]/)
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_BUILDER }))).toMatch(/feat-builder belongs to another worktree/)
})

test('a worktree the main loop added for a subagent stays pushable once it moves back to its own', async ($, on) => {
  seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  await $.tool.call({ tool: 'Bash', command: ADD_BUILDER })
  await $.tool.call({ tool: 'Bash', command: `cd ${OWN} && git status` })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git -C ${BUILDER} commit -m x` }))).toMatch(/belongs to another worktree/)
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_BUILDER }))).toBeUndefined()
})

test('another session never inherits the record: after an in-process /resume the push is refused', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  await $.tool.call({ tool: 'Bash', command: ADD_BUILDER, agentId: 'a1' })
  await $.turn.start({ text: '', turnId: 't1' })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_BUILDER }))).toBeUndefined()

  seated.sessionId = 's-resumed'
  await $.turn.start({ text: '', turnId: 't2' })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_BUILDER }))).toMatch(/feat-builder belongs to another worktree/)
})

const PUSH_ISOLATED = `git -C ${ISOLATED} log origin/main..HEAD && git -C ${ISOLATED} push -u origin worktree-agent-a2`
const SPAWN = { tool: 'Agent', description: 'build', prompt: 'p', isolation: 'worktree' } as const

test('the main loop pushes from the isolation worktree its Agent call made, and writes nothing there', async ($, on) => {
  seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_ISOLATED }))).toMatch(/belongs to the canonical checkout/)

  expect(refusalOf(await $.tool.call(SPAWN))).toBeUndefined()
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_ISOLATED }))).toBeUndefined()

  const commit = await $.tool.call({ tool: 'Bash', command: `git -C ${ISOLATED} commit -m x` })
  expect(refusalOf(commit)).toMatch(/agent-a2 belongs to the canonical checkout/)
  const write = await $.tool.call({ tool: 'Write', file_path: `${ISOLATED}/README.md`, content: 'x' })
  expect(refusalOf(write)).toMatch(/agent-a2\/README.md belongs to the canonical checkout/)
})

test('an Agent call without isolation, a background launch or a refused spawn records nothing', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 0, remoteAhead: 0 }, OWN)
  await $.session.start({ cwd: OWN, surface: null, isInteractive: true })
  await $.tool.call({ tool: 'Agent', description: 'build', prompt: 'p' })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_ISOLATED }))).toMatch(/belongs to the canonical checkout/)

  seated.agent = 'background'
  expect(refusalOf(await $.tool.call(SPAWN))).toBeUndefined()
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_ISOLATED }))).toMatch(/belongs to the canonical checkout/)

  seated.agent = 'denied'
  expect(refusalOf(await $.tool.call(SPAWN))).toMatch(/belt\[test\]/)
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: PUSH_ISOLATED }))).toMatch(/belongs to the canonical checkout/)
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

test('a squash-merged branch with no upstream is force-deleted without a question', async ($, on) => {
  const seated = seat(on, { dirty: [], ahead: 2, remoteAhead: null, isIntegrated: true }, CANONICAL, 'Cancel')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' }))).toBeUndefined()
  expect(seated.asked.length).toBe(0)
})

test('a merge-tree that exits non-zero is no proof, so the force-delete still asks', async ($, on) => {
  // The default branch's own tree on stdout, so only the exit code keeps the
  // question. Exit 1 is a conflict or a bad ref; 129 stands for a git that
  // can't run the check at all.
  const repo: Repo = { dirty: [], ahead: 2, remoteAhead: null, isIntegrated: true, mergeTreeExit: 1 }
  const seated = seat(on, repo, CANONICAL, 'Cancel')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' }))).toMatch(/feat\/x kept/)
  repo.mergeTreeExit = 129
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: 'git branch -D feat/x' }))).toMatch(/feat\/x kept/)
  expect(seated.asked).toEqual([
    'feat/x has 2 unmerged commits. Remove anyway?',
    'feat/x has 2 unmerged commits. Remove anyway?',
  ])
})

test('a forced removal of a dirty worktree asks', async ($, on) => {
  const seated = seat(on, { dirty: [OWN], ahead: 2, remoteAhead: 0 }, CANONICAL, 'Cancel')
  await $.session.start({ cwd: CANONICAL, surface: null, isInteractive: true })
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git worktree remove ${OWN}` }))).toBeUndefined()
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: `git worktree remove --force ${OWN}` }))).toMatch(/kept/)
  expect(seated.asked).toEqual([`${OWN} has uncommitted changes. Remove anyway?`])
})
