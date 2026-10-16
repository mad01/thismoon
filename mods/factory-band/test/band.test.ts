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

const run = (exitCode: number, stdout: string): ProcessRunResult => ({
  exitCode,
  stdout,
  stderr: exitCode === 0 ? '' : 'fatal',
  isStdoutTruncated: false,
  isStderrTruncated: false,
})

// The worktree as git reports it; `mergedTree` is what merge-tree writes.
type Git = { status: string; ahead: string; mergedTree: string }

type World = {
  agents: AgentInfo[]
  events: 'ok' | 'down' | 'hung'
  git: Git | 'hung'
  commands: string[]
  fetches: number
  mergeTrees: number
  clock: MockClock
}

const OPEN: Git = { status: '', ahead: '0', mergedTree: 'b0b0b0b' }

// The engine beneath the plugin.
function seat(on: On, overrides: Partial<Omit<World, 'clock' | 'commands'>> = {}): World {
  const clock = mock.clock(on)
  const world: World = { agents: [w1], events: 'ok', git: OPEN, commands: [], fetches: 0, mergeTrees: 0, clock, ...overrides }
  mock.env(on, { HOME })
  on('process.run', async (_$, e) => {
    const line = e.argv.join(' ')
    if (!line.startsWith(`git -C ${WT} `)) return { value: run(128, '') }
    if (world.git === 'hung') await clock.sleep(10_000)
    const git = world.git === 'hung' ? OPEN : world.git
    if (line.endsWith('rev-parse origin/HEAD^{tree} --symbolic-full-name origin/HEAD')) return { value: run(0, `${MAIN_TREE}\nrefs/remotes/origin/main\n`) }
    if (line.endsWith('status --porcelain')) return { value: run(0, git.status) }
    if (line.endsWith('rev-list --count origin/main..HEAD')) return { value: run(0, `${git.ahead}\n`) }
    if (line.endsWith('merge-tree --write-tree origin/main HEAD')) {
      world.mergeTrees += 1
      return { value: run(0, `${git.mergedTree}\n`) }
    }
    return { value: run(128, '') }
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
  on('agent.spawn', () => ({ model: 'claude-sonnet-5-5', agentId: 'a1' }))
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

test('events off: nothing is fetched', { options: { events: false } }, async ($, on) => {
  const world = seat(on)
  await $.session.start(start)
  await $.agent.spawn(spawn)
  expect(world.fetches).toBe(0)
})

// A push raised in an agent's loop. The call type leaves `agentId` out, but
// the kit hands it to the tool.call hooks as a session does for a subagent.
const pushFrom = (agentId: string) => ({ tool: 'Bash' as const, command: 'git push origin main', agentId })

test('belt denies count for the workers\' own calls only, never the orchestrator\'s', async ($, on) => {
  seat(on, { events: 'down' })
  on('classic.PreToolUse', () => ({ deny: 'belt[git-push-main]: pushes to main are denied' }))
  await $.session.start(start)
  await $.agent.spawn(spawn)
  await $.tool.call({ tool: 'Bash', command: 'git push origin main' })
  await $.tool.call(pushFrom('b7'))
  const ui = await mountBand($)
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 0 · inbox -')
  await $.tool.call(pushFrom('a1'))
  expect((await band(ui))[0]).toBe('factory · 1 worker · denies 1 · inbox -')
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
