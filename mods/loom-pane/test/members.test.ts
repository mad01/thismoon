// The member line and the claim against the engine's own test kit:
// `claude plugin test mods/loom-pane`. The hooks registered here stand
// beneath the plugin for the engine: git and `claude agents`
// (process.run), the agent list, the events service (http.fetch), the
// clock, the tools and the status line.
import { expect, mock, test } from 'claude-code/testing'
import type { MockClock } from 'claude-code/testing'
import type { AgentInfo, AgentSpawnInput, HttpResponse, On, ProcessRunResult } from 'claude-code'

const HOME = '/home/u'
const CANONICAL = '/home/u/code/thismoon'
const OWN = '/home/u/.worktrees/thismoon/feat-pane'
const OTHER = '/home/u/.worktrees/thismoon/fix-band'
const QUIET = '/home/u/.worktrees/thismoon/old-docs'
const NESTED = `${CANONICAL}/.claude/worktrees/agent-a1`

const run = (exitCode: number, stdout: string): ProcessRunResult => ({
  exitCode,
  stdout,
  stderr: exitCode === 0 ? '' : 'fatal',
  isStdoutTruncated: false,
  isStderrTruncated: false,
})

const porcelain = [CANONICAL, OWN, OTHER, QUIET, NESTED].map(path => `worktree ${path}\nHEAD 1111\nbranch refs/heads/x\n`).join('\n')

// Sessions `claude agents --json --all` lists: one at work in another loom
// worktree, one done, the orchestrator in the canonical checkout, and this
// session.
const sessionsJson = JSON.stringify([
  { id: 'ec2f68c4', cwd: OTHER, kind: 'background', sessionId: 's-bg', name: 'band fixer', state: 'busy' },
  { id: 'f00dcafe', cwd: QUIET, kind: 'background', sessionId: 's-done', name: 'docs pass', state: 'done' },
  { pid: 9110, cwd: CANONICAL, kind: 'interactive', sessionId: 's-lead', name: 'thismoon-6d', status: 'busy' },
  { pid: 9200, cwd: OWN, kind: 'interactive', sessionId: 's-self', name: 'pane-owner', status: 'busy' },
])

const ledgerJson = JSON.stringify([
  { id: '1', source: 'belt-ledger', title: 'subagent started (factory-worker-high)', tags: { event: 'subagent-start', agent_id: 'a1', worktree: OTHER, model: 'opus', effort: 'high' } },
])

const w1: AgentInfo = { id: 'a1', name: 'w1', type: 'factory-worker-high', status: 'running', description: 'unit 1' }

const BAND = 'band fixer (owner, fix-band, -, -) busy'

type World = {
  agents: AgentInfo[] | 'down' | 'slow'
  claude: 'ok' | 'missing' | 'hung'
  events: 'ok' | 'down' | 'hung'
  deniesBash: (command: string) => boolean
  statuses: Array<string | undefined>
  listings: number
  fetches: number
  clock: MockClock
}

// The engine beneath the plugin, for a session in `cwd` (an owner in OWN by
// default).
function seat(on: On, overrides: Partial<Omit<World, 'clock'>> = {}, cwd = OWN): World {
  const clock = mock.clock(on)
  const world: World = {
    agents: [w1],
    claude: 'ok',
    events: 'ok',
    deniesBash: () => false,
    statuses: [],
    listings: 0,
    fetches: 0,
    clock,
    ...overrides,
  }
  mock.env(on, { HOME })
  on('process.run', async (_$, e) => {
    const line = e.argv.join(' ')
    if (line === 'claude agents --json --all') {
      world.listings += 1
      if (world.claude === 'missing') throw new Error('spawn claude ENOENT')
      if (world.claude === 'hung') await clock.sleep(10_000)
      return { value: run(0, sessionsJson) }
    }
    if (line === 'git rev-parse --show-toplevel') return { value: run(0, cwd) }
    if (line.endsWith('--git-common-dir')) return { value: run(0, `${CANONICAL}/.git`) }
    if (line === 'git worktree list --porcelain') return { value: run(0, porcelain) }
    return { value: run(128, '') }
  })
  on('agent.list', async () => {
    if (world.agents === 'down') return { deny: 'no agent list' }
    // 'slow': an agent list that takes 6 s, longer than the refresh gap.
    if (world.agents === 'slow') await clock.sleep(6_000)
    return { value: world.agents === 'slow' ? [w1] : world.agents }
  })
  on('session.id', () => ({ value: 's-self' }))
  on('http.fetch', async (_$, e) => {
    world.fetches += 1
    if (world.events === 'down') return { deny: `connect ECONNREFUSED ${e.url}` }
    if (world.events === 'hung') await clock.sleep(10_000)
    const answer: HttpResponse = { status: 200, ok: true, headers: {}, text: ledgerJson }
    return { value: answer }
  })
  on('session.cwd', () => ({ value: cwd }))
  on('ui.status', (_$, e) => {
    world.statuses.push(e.text)
    return { value: undefined }
  })
  on('ui.log', () => ({ value: undefined }))
  on('session.start', (_$, e) => ({ cwd: e.cwd }))
  on('agent.spawn', () => ({ model: 'claude-opus-5-5', agentId: 'a1' }))
  on('turn.complete', () => ({ text: '' }))
  on('tool.call', { tool: 'Write' }, () => ({ result: { type: 'create', filePath: '', content: '', structuredPatch: [] } }))
  // Stands for belt or the person: a command it refuses never runs.
  on('tool.call', { tool: 'Bash' }, (_$, e) =>
    world.deniesBash(e.command) ? { deny: 'belt[test]: refused' } : { result: { stdout: '', stderr: '', interrupted: false } },
  )
  return world
}

const last = (world: World): string | undefined => world.statuses[world.statuses.length - 1]

// A state store beneath the plugin, seeded with values a past refresh left
// (the kit's `$` has no state noun to seed through).
function seedState(on: On, seeded: Readonly<Record<string, unknown>>): void {
  const values = new Map<string, unknown>(Object.entries(seeded))
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
}

const refusalOf = (ran: { deny?: string; isError?: true; text?: string }): string | undefined =>
  ran.deny ?? (ran.isError === true ? ran.text : undefined)

const start = (cwd = OWN) => ({ cwd, surface: null, isInteractive: true }) as const

const mainTurn = (turnId: string) => ({ answer: '', durationMs: 1, isAborted: false, turnId, reason: 'answer' }) as const

test('the status line lists the members beside the role, a done session left out', async ($, on) => {
  const world = seat(on)
  await $.session.start(start())
  expect(last(world)).toBe(`owner feat-pane · members w1 (worker-high, agent-a1, opus, high), ${BAND}`)
})

test('a spawn and a main-loop turn end re-read the members; a subagent turn does not', async ($, on) => {
  const world = seat(on, { agents: [] })
  await $.session.start(start())
  expect(last(world)).toBe(`owner feat-pane · members ${BAND}`)

  world.agents = [w1]
  const spawn: AgentSpawnInput = {
    tool_use_id: 'tu1',
    prompt: 'kind: task',
    description: 'unit 1',
    subagentType: 'factory-worker-high',
    provider: { plugin: 'engine', tier: 'core' },
    parentModel: 'claude-opus-5-5',
    background: true,
    fork: false,
  }
  await world.clock.advance(5_000)
  await $.agent.spawn(spawn)
  expect(last(world)).toMatch(/members w1 \(worker-high, agent-a1, opus, high\)/)

  world.agents = []
  await world.clock.advance(5_000)
  const shown = world.statuses.length
  await $.turn.complete({ ...mainTurn('t1'), agentId: 'a1' })
  expect(world.statuses.length).toBe(shown)
  await $.turn.complete(mainTurn('t2'))
  expect(last(world)).toBe(`owner feat-pane · members ${BAND}`)
})

test('no refresh starts within 5 s of the last one', async ($, on) => {
  const world = seat(on)
  await $.session.start(start())
  expect(world.listings).toBe(1)
  await world.clock.advance(4_999)
  await $.turn.complete(mainTurn('t1'))
  expect(world.listings).toBe(1)
  // The skipped turn end's trailing refresh, at the 5 s boundary.
  await world.clock.advance(1)
  expect(world.listings).toBe(2)
  await world.clock.advance(5_000)
  await $.turn.complete(mainTurn('t2'))
  expect(world.listings).toBe(3)
})

test('a skipped refresh is not lost: one trailing refresh at the gap boundary', async ($, on) => {
  const world = seat(on)
  await $.session.start(start())
  expect(last(world)).toMatch(/members w1 /)

  world.agents = []
  await world.clock.advance(2_000)
  await $.turn.complete(mainTurn('t1'))
  await world.clock.advance(1_000)
  await $.turn.complete(mainTurn('t2'))
  expect(world.listings).toBe(1)
  expect(last(world)).toMatch(/members w1 /)

  await world.clock.advance(2_001)
  expect(world.listings).toBe(2)
  expect(last(world)).toBe(`owner feat-pane · members ${BAND}`)
  await world.clock.advance(10_000)
  expect(world.listings).toBe(2)
})

test('one refresh at a time: a turn end during a slow refresh starts none of its own', async ($, on) => {
  const world = seat(on, { agents: 'slow' })
  const started = $.session.start(start())
  await world.clock.advance(5_000)
  await $.turn.complete(mainTurn('t1'))
  await world.clock.advance(1_000)
  await started
  expect(world.listings).toBe(1)
  expect(last(world)).toMatch(/members w1 /)
})

test('a hung claude agents costs the 400 ms probe budget and no more', async ($, on) => {
  const world = seat(on, { claude: 'hung' })
  const started = $.session.start(start())
  await world.clock.advance(400)
  await started
  expect(last(world)).toBe('owner feat-pane · members w1 (worker-high, agent-a1, opus, high)')
})

test('with no ledger entry the fields nobody recorded are dashes', async ($, on) => {
  const world = seat(on, { agents: [{ ...w1, id: 'a9', name: 'w9' }] })
  await $.session.start(start())
  expect(last(world)).toBe(`owner feat-pane · members w9 (worker-high, -, -, -), ${BAND}`)
})

test('no claude, no agent list, no events service: the role line alone, no error', async ($, on) => {
  const world = seat(on, { agents: 'down', claude: 'missing', events: 'down' })
  await $.session.start(start())
  expect(last(world)).toBe('owner feat-pane')
})

test('an events service that is down leaves dashes, not a guess', async ($, on) => {
  const world = seat(on, { claude: 'missing', events: 'down' })
  await $.session.start(start())
  expect(world.fetches).toBe(1)
  expect(last(world)).toBe('owner feat-pane · members w1 (worker-high, agent-a1, -, -)')
})

test('a hung events service costs the 400 ms probe budget and no more', async ($, on) => {
  const world = seat(on, { claude: 'missing', events: 'hung' })
  const started = $.session.start(start())
  await world.clock.advance(400)
  await started
  expect(last(world)).toBe('owner feat-pane · members w1 (worker-high, agent-a1, -, -)')
})

test('members off: no probes, and members a past refresh stored are not shown', { options: { members: false } }, async ($, on) => {
  const stale = { name: 'w0', role: 'worker-low', worktree: 'gone', model: '-', effort: '-', state: null }
  seedState(on, { 'loom-pane/members': [stale] })
  const world = seat(on)
  await $.session.start(start())
  expect(world.listings).toBe(0)
  expect(world.fetches).toBe(0)
  expect(last(world)).toBe('owner feat-pane')
})

const CLAIM_AND_PULL = `git -C ${CANONICAL} pull --ff-only && git -C ${CANONICAL} worktree add ~/.worktrees/thismoon/feat-new -b feat/new`

test('a claim denied beneath the mod records no ownership', async ($, on) => {
  const world = seat(on, { deniesBash: command => command.includes(' pull ') }, CANONICAL)
  await $.session.start(start(CANONICAL))
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: CLAIM_AND_PULL }))).toMatch(/belt\[test\]/)
  const write = await $.tool.call({ tool: 'Write', file_path: `${CANONICAL}/README.md`, content: 'x' })
  expect(refusalOf(write)).toBeUndefined()
  expect(world.statuses.some(text => text?.startsWith('owner') === true)).toBe(false)
})

test('a claim-plus-pull the owner guard denies leaves the ownership where it was', async ($, on) => {
  const world = seat(on)
  await $.session.start(start())
  expect(refusalOf(await $.tool.call({ tool: 'Bash', command: CLAIM_AND_PULL }))).toMatch(/belongs to the canonical checkout/)
  expect(last(world)).toMatch(/^owner feat-pane/)
  const elsewhere = await $.tool.call({ tool: 'Write', file_path: '/home/u/.worktrees/thismoon/feat-new/x.md', content: 'x' })
  expect(refusalOf(elsewhere)).toMatch(/belongs to another worktree/)
})

test('a claim that ran makes the session the owner', async ($, on) => {
  const world = seat(on, {}, CANONICAL)
  await $.session.start(start(CANONICAL))
  await $.tool.call({ tool: 'Bash', command: `git -C ${CANONICAL} worktree add ~/.worktrees/thismoon/feat-new -b feat/new` })
  expect(last(world)).toMatch(/^owner feat-new/)
})
