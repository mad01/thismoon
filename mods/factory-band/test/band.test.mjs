import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  REFRESH_GAP_MS,
  UNKNOWN,
  bandView,
  countParked,
  eventsUrl,
  expandHome,
  gitFactsFrom,
  idAt,
  isBeltDeny,
  isRefreshDue,
  isTaskMessage,
  isWorkerType,
  kindOf,
  livenessOf,
  parseDefaultRef,
  parseHeader,
  parseLedger,
  probeFrom,
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
const before = `${idAt(RUN_START_MS - 60_000)}-00aa`
const after = n => `${idAt(RUN_START_MS + n * 1_000)}-0${n}bb`

// belt's ledger entries as the events service answers them, newest first,
// tagged the way tools/belt/internal/ledger/ledger.go builds them.
const ledgerJson = JSON.stringify([
  {
    id: after(5),
    time: '2026-10-16T10:00:05Z',
    source: 'belt-ledger',
    level: 'info',
    title: 'permission denied (Bash)',
    message: 'auto mode denied the command',
    tags: { event: 'permission-denied', session_id: SESSION, agent_id: 'a1', agent_type: 'factory-worker-medium', tool: 'Bash', worktree: WT },
  },
  {
    id: after(4),
    source: 'belt-ledger',
    title: 'permission denied (Bash)',
    tags: { event: 'permission-denied', session_id: 's-other', agent_id: 'b1', tool: 'Bash' },
  },
  {
    id: after(2),
    source: 'belt-ledger',
    title: 'subagent started (factory-worker-medium)',
    tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a1', agent_type: 'factory-worker-medium', worktree: WT, effort: 'medium', model: 'sonnet' },
  },
  {
    id: after(1),
    source: 'belt-ledger',
    title: 'subagent started (factory-worker-medium)',
    tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a1', effort: 'low' },
  },
  {
    id: after(1),
    source: 'belt-ledger',
    title: 'subagent started (factory-worker-high)',
    tags: { event: 'subagent-start', session_id: SESSION, agent_id: 'a2', agent_type: 'factory-worker-high' },
  },
  {
    id: before,
    source: 'belt-ledger',
    title: 'permission denied (Edit)',
    tags: { event: 'permission-denied', session_id: SESSION, tool: 'Edit' },
  },
])

// belt-inbox items: `to` addresses one session, `from` names the sender.
const inboxJson = JSON.stringify([
  { id: after(6), source: 'belt-inbox', title: 'question parked', tags: { to: SESSION, from: 'w1' } },
  { id: after(3), source: 'belt-inbox', title: 'mentions s-lead only', message: SESSION, tags: { to: 's-other' } },
  { id: after(2), source: 'belt-inbox', title: 'ci green', tags: { to: SESSION, from: 'ci' } },
  { id: before, source: 'belt-inbox', title: 'last run', tags: { to: SESSION } },
])

const scope = { sessionId: SESSION, sinceId: idAt(RUN_START_MS) }

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

  test('plain text or an unknown kind is no contract message', () => {
    assert.equal(kindOf('All done, the files are committed.'), null)
    assert.equal(kindOf('kind: finished\nunit: x'), null)
  })
})

describe('ledger and inbox', () => {
  test('newest start facts per agent, and this session\'s denials since the run started', () => {
    const read = parseLedger(ledgerJson, scope)
    assert.deepEqual(read.facts.get('a1'), { model: 'sonnet', effort: 'medium' })
    assert.deepEqual(read.facts.get('a2'), { model: null, effort: null })
    assert.equal(read.denies, 1)
  })

  test('an answer that is not an event array reads as unknown', () => {
    for (const body of [null, '', 'not json', '{"events":[]}']) {
      const read = parseLedger(body, scope)
      assert.equal(read.facts.size, 0)
      assert.equal(read.denies, null)
      assert.equal(countParked(body, scope), null)
    }
  })

  test('parked counts items addressed to this session since the run started', () => {
    assert.equal(countParked(inboxJson, scope), 2)
    assert.equal(countParked('[]', scope), 0)
  })

  test('ids at the run start sort before ids after it and after ids before it', () => {
    assert.ok(before < scope.sinceId)
    assert.ok(after(0) >= scope.sinceId)
    assert.equal(idAt(1).length, 20)
  })

  test('the query names the source and the session, encoded', () => {
    assert.equal(eventsUrl('belt-inbox', 'a b'), 'http://localhost:7430/api/events?source=belt-inbox&q=a%20b&limit=100')
    assert.match(eventsUrl('belt-ledger', SESSION), /source=belt-ledger&q=s-lead&limit=200$/)
  })
})

describe('worker state', () => {
  const clean = (ahead = 2) => ({ isDirty: false, ahead })
  const dirty = { isDirty: true, ahead: 0 }
  const cases = [
    ['a loop at work is in flight', { git: clean(), liveness: 'live', lastKind: 'ready' }, 'in flight'],
    ['clean, ahead and ready', { git: clean(), liveness: 'idle', lastKind: 'ready' }, 'ready'],
    ['ready and no longer ahead is integrated', { git: clean(0), liveness: 'gone', lastKind: 'ready' }, 'integrated'],
    ['ready claimed on a dirty tree is not ready', { git: dirty, liveness: 'idle', lastKind: 'ready' }, 'in flight'],
    ['clean and ahead without a ready message', { git: clean(), liveness: 'gone', lastKind: null }, 'abandoned'],
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
    assert.deepEqual(gitFactsFrom('', '3\n'), { isDirty: false, ahead: 3 })
    assert.deepEqual(gitFactsFrom(' M README.md\n', '0\n'), { isDirty: true, ahead: 0 })
    assert.equal(gitFactsFrom(null, '3'), null)
    assert.equal(gitFactsFrom('', 'fatal'), null)
  })

  test('the default ref from origin HEAD, main when it is unset', () => {
    assert.equal(parseDefaultRef('refs/remotes/origin/main\n'), 'origin/main')
    assert.equal(parseDefaultRef(null), 'main')
    assert.equal(parseDefaultRef('garbage'), 'main')
  })
})

describe('band', () => {
  const w1 = { agentId: 'a1', name: 'events', worktree: WT, model: 'claude-sonnet-5-5', lastKind: 'ready' }
  const w2 = { agentId: 'a2', name: 'csl-prune', worktree: null, model: null, lastKind: null }

  test('one aligned row per worker; fields nobody recorded are dashes', () => {
    const probe = probeFrom({
      workers: [w1, w2],
      agents: [{ id: 'a1', status: 'completed' }, { id: 'a2', status: 'running' }],
      gits: [{ isDirty: false, ahead: 1 }, null],
      ledger: parseLedger(ledgerJson, scope),
      parked: countParked(inboxJson, scope),
    })
    const view = bandView({ workers: [w1, w2], probe, guardDenies: 2 })
    assert.equal(view.summary, 'factory · 2 workers · denies 3 · parked 2')
    assert.deepEqual(view.rows, [
      'events     ready  claude-sonnet-5-5  medium',
      'csl-prune  -      -                  -',
    ])
    assert.equal(view.isAnyReady, true)
  })

  test('the spawn\'s model wins over the ledger\'s; the ledger fills one the spawn lacked', () => {
    const ledger = { facts: new Map([['a1', { model: 'opus', effort: 'high' }], ['a2', { model: 'haiku', effort: null }]]), denies: 0 }
    const probe = probeFrom({ workers: [w1, w2], agents: [], gits: [null, null], ledger, parked: 0 })
    assert.deepEqual(bandView({ workers: [w1, w2], probe, guardDenies: 0 }).rows, [
      'events     -  claude-sonnet-5-5  high',
      'csl-prune  -  haiku              -',
    ])
  })

  test('before the first refresh: dashes, guard denies alone, parked unknown', () => {
    const view = bandView({ workers: [w2], probe: null, guardDenies: 1 })
    assert.equal(view.summary, 'factory · 1 worker · denies 1 · parked -')
    assert.deepEqual(view.rows, ['csl-prune  -  -  -'])
    assert.equal(view.isAnyReady, false)
  })
})

describe('belt denies', () => {
  test('a belt reason counts, wrapped or not; other decisions do not', () => {
    assert.equal(isBeltDeny({ deny: 'belt[git-push-main]: pushes to main are denied' }), true)
    assert.equal(isBeltDeny({ deny: 'PreToolUse:Bash hook error: belt[x]: no' }), true)
    assert.equal(isBeltDeny({ deny: 'settings rule' }), false)
    assert.equal(isBeltDeny({ allow: true }), false)
    assert.equal(isBeltDeny({ hookSpecificOutput: { permissionDecisionReason: 'belt[y]: no' } }), true)
    assert.equal(isBeltDeny(undefined), false)
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
