// The band, the weave button and the guard against the engine's own test
// kit: `claude plugin test mods/factory-band`. The hooks registered here
// stand beneath the plugin for the engine: git (process.run), the agent
// list, the events service (http.fetch), the clock, the spawn, the weave
// command and belt's PreToolUse decision.
import { expect, mock, test } from 'claude-code/testing'
import type { Engine, MockClock } from 'claude-code/testing'
import type { AgentInfo, AgentSpawnInput, HttpResponse, On, ProcessRunResult } from 'claude-code'

import { idAt } from '../lib/band'

const HOME = '/home/u'
const WT = '/home/u/.worktrees/thismoon/alexander-docs-sweep-events'
const WT2 = '/home/u/.worktrees/thismoon/alexander-csl-prune'
const SESSION = 's-lead'
const RUN_START_MS = 1_792_000_000_000
const MAIN_TREE = 'aaff749'

const TASK = [
  'kind: task',
  'unit: docs-sweep/events',
  'worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events',
  'branch: alexander/docs-sweep-events',
  'model: sonnet',
  'effort: medium',
  'reply_to: -',
  'needs_reply: true',
  '',
  '1. Send ready when both files pass.',
  '',
  'Report format: one ready message.',
].join('\n')

const READY = TASK.replace('kind: task', 'kind: ready').replace('needs_reply: true', 'needs_reply: false')

const later = (n: number): string => `${idAt(RUN_START_MS + n * 1_000)}-00aa`

// belt's ledger and inbox as the events service answers them, newest first.
const ledgerJson = JSON.stringify([
  { id: later(4), source: 'belt-ledger', title: 'permission denied (Bash)', tags: { event: 'permission-denied', session_id: SESSION, tool: 'Bash' } },
  { id: later(3), source: 'belt-ledger', title: 'permission denied (Bash)', tags: { event: 'permission-denied', session_id: SESSION, agent_id: 'a1', tool: 'Bash' } },
  { id: later(1), source: 'belt-ledger', title: 'subagent started (factory-worker-medium)', tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a1', worktree: WT, effort: 'medium' } },
])
const inboxJson = JSON.stringify([{ id: later(2), source: 'belt-inbox', title: 'ci green', tags: { to: SESSION, from: 'ci' } }])

const BAND = { hasSurvey: false, isWorking: false, maxRows: 12, bodyColumns: 100, scroll: { offset: 0, bodyRows: 11 }, view: {} }

const w1: AgentInfo = { id: 'a1', name: 'events', type: 'factory-worker-medium', status: 'running', description: 'events docs' }
const w2: AgentInfo = { id: 'a2', name: 'csl', type: 'factory-worker-high', status: 'running', description: 'csl prune' }

// The agent id each spawn name resolves to.
const IDS: Record<string, string> = { events: 'a1', csl: 'a2' }

const spawn: AgentSpawnInput = {
  tool_use_id: 'tu1',
  prompt: TASK,
  description: 'events docs',
  subagentType: 'factory-worker-medium',
  provider: { plugin: 'engine', tier: 'core' },
  parentModel: 'claude-opus-5-5',
  background: true,
  fork: false,
  name: 'events',
}

// A second worker in its own worktree; the spawn's cwd wins over the header.
const spawn2: AgentSpawnInput = { ...spawn, tool_use_id: 'tu2', subagentType: 'factory-worker-high', name: 'csl', cwd: WT2 }

const run = (exitCode: number, stdout: string, stderr = exitCode === 0 ? '' : 'fatal'): ProcessRunResult => ({
  exitCode,
  stdout,
  stderr,
  isStdoutTruncated: false,
  isStderrTruncated: false,
})

// git's words for a worktree that is gone: its own under LC_ALL=C, a
// translation otherwise, so a call that drops the locale misses them.
const gone = (dir: string, isUntranslated: boolean): string =>
  isUntranslated ? `fatal: cannot change to '${dir}': No such file or directory` : `fatal: kann nicht in Verzeichnis '${dir}' wechseln`

// The worktree as git reports it; `mergedTree` is what merge-tree writes.
type Git = { status: string; ahead: string; mergedTree: string }

// A worktree: there, slower than any budget, or removed by loom.
type Tree = Git | 'hung' | 'removed'

type World = {
  agents: AgentInfo[]
  events: 'ok' | 'down' | 'hung'
  git: Tree
  others: Record<string, Tree>
  commands: string[]
  fetches: number
  mergeTrees: number
  clock: MockClock
}

const OPEN: Git = { status: '', ahead: '0', mergedTree: 'b0b0b0b' }

// git's answer to one call in a worktree that is there.
function gitAnswer(world: World, git: Git, args: readonly string[]): ProcessRunResult {
  switch (args.join(' ')) {
    case 'rev-parse origin/HEAD^{tree} --symbolic-full-name origin/HEAD':
      return run(0, `${MAIN_TREE}\nrefs/remotes/origin/main\n`)
    case 'status --porcelain':
      return run(0, git.status)
    case 'rev-list --count origin/main..HEAD':
      return run(0, `${git.ahead}\n`)
    case 'merge-tree --write-tree origin/main HEAD':
      world.mergeTrees += 1
      return run(0, `${git.mergedTree}\n`)
  }
  return run(128, '')
}

// The engine beneath the plugin. `git` is the first worker's worktree,
// `others` any more.
function seat(on: On, overrides: Partial<Omit<World, 'clock' | 'commands'>> = {}): World {
  const clock = mock.clock(on)
  const world: World = { agents: [w1], events: 'ok', git: OPEN, others: {}, commands: [], fetches: 0, mergeTrees: 0, clock, ...overrides }
  mock.env(on, { HOME })
  on('process.run', async (_$, e) => {
    const [command, flag, dir = '', ...args] = e.argv
    const tree = command === 'git' && flag === '-C' ? (dir === WT ? world.git : world.others[dir]) : undefined
    if (tree === undefined) return { value: run(128, '') }
    if (tree === 'removed') return { value: run(128, '', gone(dir, e.init?.env?.LC_ALL === 'C')) }
    if (tree === 'hung') await clock.sleep(10_000)
    return { value: gitAnswer(world, tree === 'hung' ? OPEN : tree, args) }
  })
  on('agent.list', () => ({ value: world.agents }))
  on('session.id', () => ({ value: SESSION }))
  on('http.fetch', async (_$, e) => {
    world.fetches += 1
    if (world.events === 'down') return { deny: `connect ECONNREFUSED ${e.url}` }
    if (world.events === 'hung') await clock.sleep(10_000)
    const text = e.url.includes('source=belt-ledger') ? ledgerJson : inboxJson
    const answer: HttpResponse = { status: 200, ok: true, headers: {}, text }
    return { value: answer }
  })
  on('ui.log', () => ({ value: undefined }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('agent.spawn', (_$, e) => ({ model: 'claude-sonnet-5-5', agentId: IDS[e.name ?? ''] ?? 'a1' }))
  on('turn.complete', () => ({ text: '' }))
  // Stands for the engine's own band: an empty Box the mod keeps under its rows.
  on('ui.render', { component: 'AbovePrompt' }, () => ({ type: 'Box' }))
  on('command.run', { command: 'loom' }, (_$, e) => {
    world.commands.push(e.args)
    return { text: '' }
  })
  return world
}

const start = { cwd: WT, surface: null, isInteractive: true } as const

const workerTurn = (answer: string) =>
  ({ answer, durationMs: 1, isAborted: false, turnId: 't1', reason: 'answer', agentId: 'a1' }) as const

const mainTurn = { answer: '', durationMs: 1, isAborted: false, turnId: 't0', reason: 'answer' } as const

async function band(ui: { findAll: (q: { type: 'Text' }) => Promise<Array<{ text?: string }>> }): Promise<string[]> {
  return (await ui.findAll({ type: 'Text' })).map(found => found.text ?? '')
}

async function mountBand($: Engine) {
  return $.ui.mount({ plugin: 'factory-band', surface: 'terminal', component: 'AbovePrompt', props: BAND })
}

test('the band stays hidden until the first worker spawns', async ($, on) => {
  seat(on)
  await $.session.start(start)
  const ui = await mountBand($)
  expect(await ui.find({ type: 'Button' })).toBeUndefined()
})

test('a spawned worker gets a row: model from the spawn, effort from the ledger, state from git', async ($, on) => {
  const world = seat(on)
  await world.clock.set(RUN_START_MS)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ plugin: 'factory-band', surface, component: 'AbovePrompt', props: BAND })
    expect(await band(ui)).toEqual(['factory · 1 worker · denies 1 · inbox 1', 'events  in flight  claude-sonnet-5-5  medium'])
    await ui.unmount()
  }
})

test('a ready message on a clean tree main has not taken reads ready; weave now runs /loom weave', async ($, on) => {
  const world = seat(on, { git: { ...OPEN, ahead: '2' } })
  await world.clock.set(RUN_START_MS)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  world.agents = [{ ...w1, status: 'completed' }]
  await $.turn.complete(workerTurn(READY))
  await world.clock.advance(5_000)
  const ui = await mountBand($)
  expect((await band(ui))[1]).toBe('events  ready  claude-sonnet-5-5  medium')
  expect(world.mergeTrees).toBe(1)
  await ui.press({ key: 'weave' })
  expect(world.commands).toEqual(['weave'])
})

test('the agent list still says running at the worker\'s final turn end: in flight, then ready with no other event', async ($, on) => {
  const world = seat(on, { git: { ...OPEN, ahead: '2' } })
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await world.clock.advance(5_000)
  await $.turn.complete(workerTurn(READY))
  const ui = await mountBand($)
  expect((await band(ui))[1]).toBe('events  in flight  claude-sonnet-5-5  medium')
  world.agents = [{ ...w1, status: 'completed' }]
  await world.clock.advance(5_000)
  expect((await band(ui))[1]).toBe('events  ready  claude-sonnet-5-5  medium')
})

test('a squash-merged worker reads integrated and drops off once its loop ended; the last one hides the band', async ($, on) => {
  const world = seat(on, { git: { status: '', ahead: '2', mergedTree: MAIN_TREE } })
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await world.clock.advance(5_000)
  await $.turn.complete(workerTurn(READY))
  const ui = await mountBand($)
  expect((await band(ui))[1]).toBe('events  in flight  claude-sonnet-5-5  medium')
  world.agents = [{ ...w1, status: 'idle' }]
  await world.clock.advance(5_000)
  expect((await band(ui))[1]).toBe('events  integrated  claude-sonnet-5-5  medium')
  world.agents = []
  await world.clock.advance(5_000)
  await $.turn.complete(mainTurn)
  expect(await ui.find({ type: 'Button' })).toBeUndefined()
})

test('a blocked message reads blocked once the worker stopped', async ($, on) => {
  const world = seat(on, { git: { ...OPEN, status: ' M README.md\n' } })
  await $.session.start(start)
  await $.agent.spawn(spawn)
  world.agents = []
  await $.turn.complete(workerTurn(READY.replace('kind: ready', 'kind: blocked')))
  await world.clock.advance(5_000)
  const ui = await mountBand($)
  expect((await band(ui))[1]).toMatch(/^events {2}blocked {2}/)
})

test('an ended worker that never sent ready reads stopped', async ($, on) => {
  const world = seat(on, { git: { ...OPEN, ahead: '1' } })
  await $.session.start(start)
  await $.agent.spawn(spawn)
  world.agents = []
  await world.clock.advance(5_000)
  await $.turn.complete(mainTurn)
  const ui = await mountBand($)
  expect((await band(ui))[1]).toMatch(/^events {2}stopped {2}/)
  expect(world.mergeTrees).toBe(0)
})

test('an events service that is down leaves dashes and the guard count, no error', async ($, on) => {
  const world = seat(on, { events: 'down' })
  await world.clock.set(RUN_START_MS)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  expect(world.fetches).toBe(2)
  const ui = await mountBand($)
  expect(await band(ui)).toEqual(['factory · 1 worker · denies 0 · inbox -', 'events  in flight  claude-sonnet-5-5  -'])
})

test('a hung events service and a hung git cost the 400 ms budget and no more', async ($, on) => {
  const world = seat(on, { events: 'hung', git: 'hung' })
  await $.session.start(start)
  const spawned = $.agent.spawn(spawn)
  await world.clock.advance(400)
  await spawned
  const ui = await mountBand($)
  expect(await band(ui)).toEqual(['factory · 1 worker · denies 0 · inbox -', 'events  -  claude-sonnet-5-5  -'])
})

test('a git that outruns the budget is no removed worktree: an ended worker keeps its row', async ($, on) => {
  const world = seat(on, { events: 'down', git: 'hung', agents: [] })
  await $.session.start(start)
  const spawned = $.agent.spawn(spawn)
  await world.clock.advance(400)
  await spawned
  const ui = await mountBand($)
  expect(await band(ui)).toEqual(['factory · 1 worker · denies 0 · inbox -', 'events  -  claude-sonnet-5-5  -'])
})

test('events off: nothing is fetched', { options: { events: false } }, async ($, on) => {
  const world = seat(on)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  expect(world.fetches).toBe(0)
})

// A push raised in an agent's loop. The call type leaves `agentId` out, but
// the kit hands it to the tool.call hooks as a session does for a subagent.
const pushFrom = (agentId: string) => ({ tool: 'Bash' as const, command: 'git push origin main', agentId })

// belt's guard refusing the call, as a settings PreToolUse hook does.
const beltDeny = () => ({ deny: 'belt[git-push-main]: pushes to main are denied' })

test('belt denies count for the workers\' own calls only, never the orchestrator\'s', async ($, on) => {
  seat(on, { events: 'down' })
  on('classic.PreToolUse', beltDeny)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await $.tool.call({ tool: 'Bash', command: 'git push origin main' })
  await $.tool.call(pushFrom('b7'))
  const ui = await mountBand($)
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 0 · inbox -')
  await $.tool.call(pushFrom('a1'))
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 1 · inbox -')
})

test('a worker\'s failed call that is no belt denial does not count, belt in its output included', async ($, on) => {
  seat(on, { events: 'down' })
  on('tool.call', () => ({ result: undefined, text: 'Exit code 1\nbelt[git-push-main]: quoted from a log', isError: true }))
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await $.tool.call(pushFrom('a1'))
  const ui = await mountBand($)
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 0 · inbox -')
})

test('a worker whose worktree loom removed drops off once its loop ended; the run\'s denies stay', async ($, on) => {
  const world = seat(on, { agents: [w1, w2], others: { [WT2]: OPEN } })
  on('classic.PreToolUse', beltDeny)
  await world.clock.set(RUN_START_MS)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await $.agent.spawn(spawn2)
  await $.tool.call(pushFrom('a1'))
  await world.clock.advance(5_000)
  const ui = await mountBand($)
  expect(await band(ui)).toEqual([
    'factory · 2 workers · denies 2 · inbox 1',
    'events  in flight  claude-sonnet-5-5  medium',
    'csl     in flight  claude-sonnet-5-5  -',
  ])
  world.git = 'removed'
  world.agents = [w2]
  await $.turn.complete(workerTurn(READY))
  await world.clock.advance(5_000)
  expect(await band(ui)).toEqual(['factory · 1 worker · denies 2 · inbox 1', 'csl  in flight  claude-sonnet-5-5  -'])
  await $.turn.complete(mainTurn)
  await world.clock.advance(5_000)
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 2 · inbox 1')
})

test('a worker spawned while a refresh reads stays when that refresh drops the last earlier worker', async ($, on) => {
  const world = seat(on, { events: 'hung', git: 'removed', agents: [w2], others: { [WT2]: OPEN } })
  await $.session.start(start)
  const first = $.agent.spawn(spawn)
  await $.agent.spawn(spawn2)
  await world.clock.advance(400)
  await first
  const ui = await mountBand($)
  expect(await band(ui)).toEqual(['factory · 1 worker · denies 0 · inbox -', 'csl  -  claude-sonnet-5-5  -'])
})

test('the last worker dropped closes the run: the next run counts denies and inbox from its own start', async ($, on) => {
  const world = seat(on)
  on('classic.PreToolUse', beltDeny)
  await world.clock.set(RUN_START_MS)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await $.tool.call(pushFrom('a1'))
  const ui = await mountBand($)
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 2 · inbox 1')
  world.git = 'removed'
  world.agents = []
  await $.turn.complete(workerTurn(READY))
  await world.clock.advance(5_000)
  expect(await ui.find({ type: 'Button' })).toBeUndefined()
  world.others = { [WT2]: OPEN }
  world.agents = [w2]
  await $.agent.spawn(spawn2)
  await world.clock.advance(5_000)
  expect(await band(ui)).toEqual(['factory · 1 worker · denies 0 · inbox 0', 'csl  in flight  claude-sonnet-5-5  -'])
})

test('the contract guard is off by default: a spawn without a task header starts', async ($, on) => {
  seat(on)
  await $.session.start(start)
  const spawned = await $.agent.spawn({ ...spawn, prompt: 'Fix the events docs.' })
  expect(spawned.deny).toBeUndefined()
})

test('the contract guard on: a worker spawn without a task header is denied; a header passes', { options: { contractGuard: true } }, async ($, on) => {
  seat(on)
  await $.session.start(start)
  const denied = await $.agent.spawn({ ...spawn, prompt: 'Fix the events docs.' })
  expect(denied.deny).toMatch(/eight-line task header/)
  const plain = await $.agent.spawn({ ...spawn, subagentType: 'general-purpose', prompt: 'Look around.' })
  expect(plain.deny).toBeUndefined()
  const started = await $.agent.spawn(spawn)
  expect(started.deny).toBeUndefined()
})
