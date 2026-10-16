import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { after, before, describe, test } from 'node:test'

import {
  DEFAULT_BRANCH_ARGS,
  REFRESH_GAP_MS,
  STATUS_ARGS,
  UNKNOWN,
  aheadArgs,
  bandView,
  countInbox,
  eventsUrl,
  expandHome,
  gitFactsFrom,
  idAt,
  isBeltRefusal,
  isIntegratedTree,
  isReadyCandidate,
  isRefreshDue,
  isTaskMessage,
  isWorkerType,
  kindOf,
  livenessOf,
  mergeTreeArgs,
  parseDefaultBranch,
  parseHeader,
  parseLedger,
  probeFrom,
  prunableIds,
  trailingDelay,
  upsertWorker,
  withKind,
  workerFrom,
  workerState,
} from '../lib/band.ts'

const HOME = '/home/u'
const WT = `${HOME}/.worktrees/thismoon/alexander-docs-sweep-events`
const SESSION = 's-lead'

// The canonical task from the factory skill's contract.md.
const TASK = `kind: task
unit: docs-sweep/events
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events
branch: alexander/docs-sweep-events
model: sonnet
effort: medium
reply_to: -
needs_reply: true

Do not edit Go source files or recipe TOML files.

1. Open README.md and CLAUDE.md of the events service.
2. Send ready when both files pass.

Report format: one ready message. List the files you changed.`

const READY = `kind: ready
unit: docs-sweep/events
worktree: ~/.worktrees/thismoon/alexander-docs-sweep-events
branch: alexander/docs-sweep-events
model: sonnet
effort: medium
reply_to: docs-sweep/events#task-1
needs_reply: false

files changed:
- services/events/README.md

commit range: 3f1c2ab..9e04d17`

// Events ids are unix nanoseconds, zero-padded, plus two random bytes.
const RUN_START_MS = 1_792_000_000_000
const earlier = `${idAt(RUN_START_MS - 60_000)}-00aa`
const later = n => `${idAt(RUN_START_MS + n * 1_000)}-0${n}bb`

// belt's ledger entries as the events service answers them, newest first,
// tagged the way tools/belt/internal/ledger/ledger.go builds them.
const ledgerJson = JSON.stringify([
  {
    id: later(6),
    source: 'belt-ledger',
    title: 'permission denied (Bash)',
    tags: { event: 'permission-denied', session_id: SESSION, tool: 'Bash', reason: 'the orchestrator itself' },
  },
  {
    id: later(5),
    time: '2026-10-16T10:00:05Z',
    source: 'belt-ledger',
    level: 'info',
    title: 'permission denied (Bash)',
    message: 'auto mode denied the command',
    tags: { event: 'permission-denied', session_id: SESSION, agent_id: 'a1', agent_type: 'factory-worker-medium', tool: 'Bash', worktree: WT },
  },
  {
    id: later(4),
    source: 'belt-ledger',
    title: 'permission denied (Bash)',
    tags: { event: 'permission-denied', session_id: 's-other', agent_id: 'b1', tool: 'Bash' },
  },
  {
    id: later(3),
    source: 'belt-ledger',
    title: 'permission denied (Edit)',
    tags: { event: 'permission-denied', session_id: SESSION, agent_id: 'x9', agent_type: 'Explore', tool: 'Edit' },
  },
  {
    id: later(2),
    source: 'belt-ledger',
    title: 'subagent started (factory-worker-medium)',
    tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a1', agent_type: 'factory-worker-medium', worktree: WT, effort: 'medium', model: 'sonnet' },
  },
  {
    id: later(1),
    source: 'belt-ledger',
    title: 'subagent started (factory-worker-medium)',
    tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a1', effort: 'low' },
  },
  {
    id: later(1),
    source: 'belt-ledger',
    title: 'subagent started (factory-worker-high)',
    tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a2', agent_type: 'factory-worker-high' },
  },
])

// belt-inbox items: `to` addresses one session, `from` names the sender.
const inboxJson = JSON.stringify([
  { id: later(6), source: 'belt-inbox', title: 'question for the lead', tags: { to: SESSION, from: 'w1' } },
  { id: later(3), source: 'belt-inbox', title: 'mentions s-lead only', message: SESSION, tags: { to: 's-other' } },
  { id: later(2), source: 'belt-inbox', title: 'ci green', tags: { to: SESSION, from: 'ci' } },
  { id: earlier, source: 'belt-inbox', title: 'last run', tags: { to: SESSION } },
])

const workerIds = new Set(['a1', 'a2'])
const ledgerScope = { sessionId: SESSION, workerIds }
const inboxScope = { sessionId: SESSION, sinceId: idAt(RUN_START_MS) }

describe('task header', () => {
  test('the canonical task parses into its eight fields', () => {
    const header = parseHeader(TASK)
    assert.equal(header?.kind, 'task')
    assert.equal(header?.unit, 'docs-sweep/events')
    assert.equal(header?.worktree, '~/.worktrees/thismoon/alexander-docs-sweep-events')
    assert.equal(header?.needs_reply, 'true')
    assert.equal(isTaskMessage(TASK), true)
  })

  test('CRLF line ends and leading blank lines still parse', () => {
    assert.equal(parseHeader(`\n${TASK.replaceAll('\n', '\r\n')}`)?.branch, 'alexander/docs-sweep-events')
  })

  test('a missing, empty or reordered line is no header', () => {
    assert.equal(parseHeader(TASK.replace('effort: medium\n', '')), null)
    assert.equal(parseHeader(TASK.replace('effort: medium', 'effort:')), null)
    assert.equal(parseHeader(TASK.replace('kind: task\nunit: docs-sweep/events', 'unit: docs-sweep/events\nkind: task')), null)
    assert.equal(isTaskMessage('Fix the band, then report.'), false)
  })

  test('a ready message has a header but is no task', () => {
    assert.equal(isTaskMessage(READY), false)
  })
})

describe('workers', () => {
  test('factory worker types, with or without a plugin prefix', () => {
    assert.equal(isWorkerType('factory-worker-low'), true)
    assert.equal(isWorkerType('dotfiles:factory-worker-high'), true)
    assert.equal(isWorkerType('factory-worker-max'), false)
    assert.equal(isWorkerType('general-purpose'), false)
  })

  test('the worktree comes from the task header, expanded under HOME', () => {
    const worker = workerFrom({ agentId: 'a1', name: 'events', prompt: TASK, model: 'claude-sonnet-5-5', home: HOME })
    assert.deepEqual(worker, { agentId: 'a1', name: 'events', worktree: WT, model: 'claude-sonnet-5-5', lastKind: null })
  })

  test('a spawn cwd wins over the header; the unit names an unnamed worker', () => {
    const worker = workerFrom({ agentId: 'a1', cwd: '/elsewhere/wt', prompt: TASK, model: 'claude-sonnet-5-5', home: HOME })
    assert.equal(worker.worktree, '/elsewhere/wt')
    assert.equal(worker.name, 'docs-sweep/events')
  })

  test('no header and no cwd: no worktree, the id as the name', () => {
    const worker = workerFrom({ agentId: 'a9', prompt: 'do the thing', model: '', home: HOME })
    assert.deepEqual(worker, { agentId: 'a9', name: 'a9', worktree: null, model: null, lastKind: null })
  })

  test('home expansion refuses what it cannot resolve', () => {
    assert.equal(expandHome('~/x', HOME), `${HOME}/x`)
    assert.equal(expandHome('~', HOME), HOME)
    assert.equal(expandHome('~/x', null), null)
    assert.equal(expandHome('relative/wt', HOME), null)
  })

  test('a respawn replaces the earlier record; a kind lands on its worker only', () => {
    const w1 = workerFrom({ agentId: 'a1', prompt: TASK, model: 'm1', home: HOME })
    const w2 = workerFrom({ agentId: 'a2', prompt: TASK, model: 'm2', home: HOME })
    const list = upsertWorker(upsertWorker([w1, w2], { ...w1, model: 'm3' }), w2)
    assert.deepEqual(list.map(w => [w.agentId, w.model]), [['a1', 'm3'], ['a2', 'm2']])
    assert.deepEqual(withKind(list, 'a2', 'ready').map(w => w.lastKind), [null, 'ready'])
  })
})

describe('message kinds', () => {
  test('the kind of a ready message', () => {
    assert.equal(kindOf(READY), 'ready')
  })

  test('an answer led by the engine\'s warning still names its kind', () => {
    assert.equal(kindOf(`Warning: the review raised a concern.\n\n${READY.replace('kind: ready', 'kind: blocked')}`), 'blocked')
  })

  test('an answer that quotes its task above its ready header reads ready', () => {
    assert.equal(kindOf(`Task as received:\n\n${TASK}\n\n${READY}`), 'ready')
  })

  test('the last worker kind wins: a status, then a ready', () => {
    assert.equal(kindOf(`${READY.replace('kind: ready', 'kind: status')}\n\nprogress: done.\n\n${READY}`), 'ready')
  })

  test('the orchestrator\'s kinds, plain text and unknown kinds name no worker kind', () => {
    assert.equal(kindOf(TASK), null)
    assert.equal(kindOf(READY.replace('kind: ready', 'kind: answer')), null)
    assert.equal(kindOf('All done, the files are committed.'), null)
    assert.equal(kindOf('kind: finished\nunit: x'), null)
  })
})

describe('ledger and inbox', () => {
  test('newest start facts per agent, and only the workers\' denials in this session', () => {
    const read = parseLedger(ledgerJson, ledgerScope)
    assert.deepEqual(read.facts.get('a1'), { model: 'sonnet', effort: 'medium' })
    assert.deepEqual(read.facts.get('a2'), { model: null, effort: null })
    assert.equal(read.denies, 1)
  })

  test('an answer that is not an event array reads as unknown', () => {
    for (const body of [null, '', 'not json', '{"events":[]}']) {
      const read = parseLedger(body, ledgerScope)
      assert.equal(read.facts.size, 0)
      assert.equal(read.denies, null)
      assert.equal(countInbox(body, inboxScope), null)
    }
  })

  test('inbox counts items addressed to this session since the run started', () => {
    assert.equal(countInbox(inboxJson, inboxScope), 2)
    assert.equal(countInbox('[]', inboxScope), 0)
  })

  test('ids at the run start sort before ids after it and after ids before it', () => {
    assert.ok(earlier < inboxScope.sinceId)
    assert.ok(later(0) >= inboxScope.sinceId)
    assert.equal(idAt(1).length, 20)
  })

  test('the query names the source and the session, encoded', () => {
    assert.equal(eventsUrl('belt-inbox', 'a b'), 'http://localhost:7430/api/events?source=belt-inbox&q=a%20b&limit=100')
    assert.match(eventsUrl('belt-ledger', SESSION), /source=belt-ledger&q=s-lead&limit=200$/)
  })
})

describe('worker state', () => {
  const clean = (ahead = 2, isIntegrated = false) => ({ isDirty: false, ahead, isIntegrated })
  const dirty = { isDirty: true, ahead: 0, isIntegrated: false }
  const cases = [
    ['a loop at work is in flight', { git: clean(), liveness: 'live', lastKind: 'ready' }, 'in flight'],
    ['clean, ahead and ready', { git: clean(), liveness: 'idle', lastKind: 'ready' }, 'ready'],
    ['ready and no longer ahead is integrated', { git: clean(0), liveness: 'gone', lastKind: 'ready' }, 'integrated'],
    ['ready, ahead, but squash-merged is integrated', { git: clean(2, true), liveness: 'gone', lastKind: 'ready' }, 'integrated'],
    ['a dirty tree after a ready is in flight, loop ended or not', { git: dirty, liveness: 'gone', lastKind: 'ready' }, 'in flight'],
    ['an ended loop without a ready is stopped', { git: clean(), liveness: 'gone', lastKind: null }, 'stopped'],
    ['an ended loop after a status is stopped', { git: dirty, liveness: 'gone', lastKind: 'status' }, 'stopped'],
    ['a blocked worker', { git: dirty, liveness: 'gone', lastKind: 'blocked' }, 'blocked'],
    ['a question waits for an answer', { git: clean(0), liveness: 'idle', lastKind: 'question' }, 'blocked'],
    ['an idle worker with nothing said yet', { git: dirty, liveness: 'idle', lastKind: 'status' }, 'in flight'],
    ['git could not say', { git: null, liveness: 'live', lastKind: null }, UNKNOWN],
    ['the agent list could not say', { git: clean(), liveness: 'unknown', lastKind: 'ready' }, UNKNOWN],
  ]
  for (const [name, input, want] of cases) {
    test(name, () => assert.equal(workerState(input), want))
  }

  test('liveness from the agent list; an agent the list dropped is gone', () => {
    const agents = [
      { id: 'a1', status: 'running' },
      { id: 'a2', status: 'idle' },
      { id: 'a3', status: 'completed' },
    ]
    assert.deepEqual(['a1', 'a2', 'a3', 'a4'].map(id => livenessOf(agents, id)), ['live', 'idle', 'gone', 'gone'])
    assert.equal(livenessOf(null, 'a1'), 'unknown')
  })

  test('git facts from status and rev-list output', () => {
    assert.deepEqual(gitFactsFrom('', '3\n'), { isDirty: false, ahead: 3, isIntegrated: false })
    assert.deepEqual(gitFactsFrom(' M README.md\n', '0\n'), { isDirty: true, ahead: 0, isIntegrated: false })
    assert.equal(gitFactsFrom(null, '3'), null)
    assert.equal(gitFactsFrom('', 'fatal'), null)
  })

  test('only a clean, ahead tree whose worker sent ready gets the merge-tree check', () => {
    assert.equal(isReadyCandidate(clean(), 'ready'), true)
    assert.equal(isReadyCandidate(clean(0), 'ready'), false)
    assert.equal(isReadyCandidate(dirty, 'ready'), false)
    assert.equal(isReadyCandidate(clean(), 'status'), false)
    assert.equal(isReadyCandidate(null, 'ready'), false)
  })

  test('the default branch and its tree from one rev-parse; main and no tree without origin HEAD', () => {
    assert.deepEqual(parseDefaultBranch('aaff749\nrefs/remotes/origin/main\n'), { ref: 'origin/main', tree: 'aaff749' })
    assert.deepEqual(parseDefaultBranch(null), { ref: 'main', tree: null })
    assert.deepEqual(parseDefaultBranch('garbage'), { ref: 'main', tree: null })
  })

  test('an unknown merge or default tree is never integrated', () => {
    assert.equal(isIntegratedTree('fe56c1c\n', 'fe56c1c'), true)
    assert.equal(isIntegratedTree(null, 'fe56c1c'), false)
    assert.equal(isIntegratedTree('fe56c1c\n', null), false)
    assert.equal(isIntegratedTree('0b1d2e3\n', 'fe56c1c'), false)
  })
})

// A real repo: main squash-merged `feat` and moved on; `open` was never
// merged. The argv the mod runs, run here by git itself.
describe('squash merge, against a fixture repo', () => {
  const env = { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1' }
  const id = ['-c', 'user.name=t', '-c', 'user.email=t@example.com', '-c', 'init.defaultBranch=main']
  let dir = ''
  const git = (...args) => execFileSync('git', ['-C', dir, ...id, ...args], { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
  const commitFile = (name, text, message) => {
    writeFileSync(join(dir, name), text)
    git('add', name)
    git('commit', '-q', '-m', message)
  }
  // The mod's git calls in order, on the branch checked out now.
  const factsOfHead = () => {
    const branch = parseDefaultBranch(git(...DEFAULT_BRANCH_ARGS))
    const facts = gitFactsFrom(git(...STATUS_ARGS), git(...aheadArgs(branch.ref)))
    return { facts, isIntegrated: isIntegratedTree(git(...mergeTreeArgs(branch.ref)), branch.tree) }
  }

  before(() => {
    dir = mkdtempSync(join(tmpdir(), 'factory-band-'))
    git('init', '-q', '.')
    commitFile('a.md', 'base\n', 'base')
    git('checkout', '-q', '-b', 'feat')
    commitFile('b.md', 'one\n', 'feat 1')
    commitFile('b.md', 'one\ntwo\n', 'feat 2')
    git('checkout', '-q', '-b', 'open', 'main')
    commitFile('c.md', 'open\n', 'open 1')
    git('checkout', '-q', 'main')
    git('merge', '-q', '--squash', 'feat')
    git('commit', '-q', '-m', 'feat (squash)')
    commitFile('z.md', 'later\n', 'later work on main')
    git('update-ref', 'refs/remotes/origin/main', 'main')
    git('symbolic-ref', 'refs/remotes/origin/HEAD', 'refs/remotes/origin/main')
  })

  after(() => rmSync(dir, { recursive: true, force: true }))

  test('a squash-merged branch is ahead by commits yet integrated', () => {
    git('checkout', '-q', 'feat')
    const { facts, isIntegrated } = factsOfHead()
    assert.deepEqual(facts, { isDirty: false, ahead: 2, isIntegrated: false })
    assert.equal(isIntegrated, true)
    assert.equal(workerState({ git: { ...facts, isIntegrated }, liveness: 'gone', lastKind: 'ready' }), 'integrated')
  })

  test('a branch main never took stays ready', () => {
    git('checkout', '-q', 'open')
    const { facts, isIntegrated } = factsOfHead()
    assert.equal(facts?.ahead, 1)
    assert.equal(isIntegrated, false)
    assert.equal(workerState({ git: { ...facts, isIntegrated }, liveness: 'gone', lastKind: 'ready' }), 'ready')
  })
})

describe('band', () => {
  const w1 = { agentId: 'a1', name: 'events', worktree: WT, model: 'claude-sonnet-5-5', lastKind: 'ready' }
  const w2 = { agentId: 'a2', name: 'csl-prune', worktree: null, model: null, lastKind: null }

  test('one aligned row per worker; fields nobody recorded are dashes', () => {
    const probe = probeFrom({
      workers: [w1, w2],
      agents: [{ id: 'a1', status: 'completed' }, { id: 'a2', status: 'running' }],
      gits: [{ isDirty: false, ahead: 1, isIntegrated: false }, null],
      ledger: parseLedger(ledgerJson, ledgerScope),
      inbox: countInbox(inboxJson, inboxScope),
    })
    const view = bandView({ workers: [w1, w2], probe, guardDenies: 2 })
    assert.equal(view.summary, 'factory · 2 workers · denies 3 · inbox 2')
    assert.deepEqual(view.rows, [
      'events     ready  claude-sonnet-5-5  medium',
      'csl-prune  -      -                  -',
    ])
    assert.equal(view.isAnyReady, true)
  })

  test('the spawn\'s model wins over the ledger\'s; the ledger fills one the spawn lacked', () => {
    const ledger = { facts: new Map([['a1', { model: 'opus', effort: 'high' }], ['a2', { model: 'haiku', effort: null }]]), denies: 0 }
    const probe = probeFrom({ workers: [w1, w2], agents: [], gits: [null, null], ledger, inbox: 0 })
    assert.deepEqual(bandView({ workers: [w1, w2], probe, guardDenies: 0 }).rows, [
      'events     -  claude-sonnet-5-5  high',
      'csl-prune  -  haiku              -',
    ])
  })

  test('before the first refresh: dashes, guard denies alone, inbox unknown', () => {
    const view = bandView({ workers: [w2], probe: null, guardDenies: 1 })
    assert.equal(view.summary, 'factory · 1 worker · denies 1 · inbox -')
    assert.deepEqual(view.rows, ['csl-prune  -  -  -'])
    assert.equal(view.isAnyReady, false)
  })

  test('integrated workers whose loop ended are pruned; a live or not yet integrated one stays', () => {
    const w3 = { ...w1, agentId: 'a3', name: 'live-one' }
    const ledger = { facts: new Map(), denies: 0 }
    const merged = { isDirty: false, ahead: 0, isIntegrated: false }
    const agents = [{ id: 'a3', status: 'running' }]
    const probe = probeFrom({ workers: [w1, w2, w3], agents, gits: [merged, merged, merged], ledger, inbox: 0 })
    assert.equal(probe.facts.a1?.state, 'integrated')
    assert.deepEqual([...prunableIds([w1, w2, w3], probe, agents)], ['a1'])
    assert.equal(prunableIds([w1], probe, null).size, 0)
  })
})

describe('belt denies', () => {
  test('a belt reason on the first line of an errored result counts', () => {
    assert.equal(isBeltRefusal({ isError: true, text: 'belt[git-push-main]: pushes to main are denied', result: undefined }), true)
    assert.equal(isBeltRefusal({ isError: true, text: 'PreToolUse:Bash hook error: belt[x]: no\nmore', result: undefined }), true)
    assert.equal(isBeltRefusal({ deny: 'belt[x]: no' }), true)
  })

  test('other results do not count, belt in the output included', () => {
    assert.equal(isBeltRefusal({ isError: true, text: 'Exit code 1\nbelt[x]: quoted from a log', result: undefined }), false)
    assert.equal(isBeltRefusal({ isError: true, text: 'settings rule denied the call', result: undefined }), false)
    assert.equal(isBeltRefusal({ result: { stdout: 'belt[x]: no' }, text: 'belt[x]: no' }), false)
    assert.equal(isBeltRefusal({ deny: 'loom-pane: outside your worktree' }), false)
    assert.equal(isBeltRefusal(undefined), false)
  })
})

describe('refresh throttle', () => {
  test('one refresh per gap, and the wait until the next', () => {
    assert.equal(isRefreshDue(0, null), true)
    assert.equal(isRefreshDue(REFRESH_GAP_MS - 1, 0), false)
    assert.equal(isRefreshDue(REFRESH_GAP_MS, 0), true)
    assert.equal(trailingDelay(1_000, 0), REFRESH_GAP_MS - 1_000)
    assert.equal(trailingDelay(9_000, 0), 0)
    assert.equal(trailingDelay(9_000, null), 0)
  })
})
