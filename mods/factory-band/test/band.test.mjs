import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { after, before, describe, test } from 'node:test'

import {
  DEFAULT_BRANCH_ARGS,
  GIT_ENV,
  NO_RUN,
  REFRESH_GAP_MS,
  STATUS_ARGS,
  UNKNOWN,
  WALK_LIMIT,
  WALK_MERGE_ARGS,
  aheadArgs,
  bandView,
  countInbox,
  eventsUrl,
  expandHome,
  gitFactsFrom,
  idAt,
  integratedOf,
  isBeltRefusal,
  isIntegratedTree,
  isLoopAtRest,
  isReadyCandidate,
  isRefreshDue,
  isTaskMessage,
  isUnrelated,
  isWorkerType,
  isWorktreeGone,
  kindOf,
  landedIn,
  livenessOf,
  mergeBaseArgs,
  mergeTreeArgs,
  parseDefaultBranch,
  parseHeader,
  parseLedger,
  parseMerges,
  parseWalk,
  probeFrom,
  prunableIds,
  settleRun,
  stdoutOf,
  trailingDelay,
  upsertWorker,
  walkArgs,
  walkInput,
  withGuardDeny,
  withKind,
  withWorker,
  workerFrom,
  workerState,
} from '../lib/band.ts'

const HOME = '/home/u'
const WT = `${HOME}/.worktrees/thismoon/alexander-docs-sweep-events`
const SESSION = 's-lead'

// git's env for a fixture repo: no global or system config, and none of
// the variables a git hook exports, which would point `git -C <tmp>` at
// the outer repo.
const fixtureEnv = () => {
  const env = { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1' }
  for (const key of ['GIT_DIR', 'GIT_INDEX_FILE', 'GIT_WORK_TREE']) delete env[key]
  return env
}

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
    ['ready and ahead, but git could not say whether main holds it', { git: clean(2, null), liveness: 'gone', lastKind: 'ready' }, UNKNOWN],
    ['a loop at work is in flight even when git could not say that', { git: clean(2, null), liveness: 'live', lastKind: 'ready' }, 'in flight'],
    ['a dirty tree after a ready is in flight, loop ended or not', { git: dirty, liveness: 'gone', lastKind: 'ready' }, 'in flight'],
    ['an ended loop without a ready is stopped', { git: clean(), liveness: 'gone', lastKind: null }, 'stopped'],
    ['an ended loop after a status is stopped', { git: dirty, liveness: 'gone', lastKind: 'status' }, 'stopped'],
    ['a blocked worker', { git: dirty, liveness: 'gone', lastKind: 'blocked' }, 'blocked'],
    ['a question waits for an answer', { git: clean(0), liveness: 'idle', lastKind: 'question' }, 'blocked'],
    ['an idle worker with nothing said yet', { git: dirty, liveness: 'idle', lastKind: 'status' }, 'in flight'],
    ['git could not say', { git: null, liveness: 'live', lastKind: null }, UNKNOWN],
    ['the agent list could not say', { git: clean(), liveness: 'unknown', lastKind: 'ready' }, UNKNOWN],
    ['a worktree that is gone is removed, loop at work or not', { git: 'removed', liveness: 'live', lastKind: null }, 'removed'],
    ['removed even when the agent list could not say', { git: 'removed', liveness: 'unknown', lastKind: 'ready' }, 'removed'],
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

  test('only a loop that waits idle or has ended gets the merge-tree check', () => {
    assert.deepEqual(['live', 'idle', 'gone', 'unknown'].map(isLoopAtRest), [false, true, true, false])
  })

  test('the default branch and its tree from one rev-parse; main and no tree without origin HEAD', () => {
    assert.deepEqual(parseDefaultBranch('aaff749\nrefs/remotes/origin/main\n'), { ref: 'origin/main', tree: 'aaff749' })
    assert.deepEqual(parseDefaultBranch(null), { ref: 'main', tree: null })
    assert.deepEqual(parseDefaultBranch('garbage'), { ref: 'main', tree: null })
  })

  test('exit 128 with git saying the worktree is gone reads gone; a timeout or another failure does not', () => {
    const failed = (exitCode, stderr) => ({ exitCode, stdout: '', stderr })
    assert.equal(isWorktreeGone(failed(128, "fatal: cannot change to '/x': No such file or directory")), true)
    assert.equal(isWorktreeGone(failed(128, 'fatal: not a git repository: /r/.git/worktrees/x')), true)
    assert.equal(isWorktreeGone(failed(128, "fatal: ambiguous argument 'origin/HEAD^{tree}': unknown revision")), false)
    assert.equal(isWorktreeGone(failed(1, 'fatal: not a git repository')), false)
    assert.equal(isWorktreeGone(null), false)
    assert.equal(stdoutOf({ exitCode: 0, stdout: 'x\n', stderr: '' }), 'x\n')
    assert.equal(stdoutOf(failed(128, 'fatal')), null)
    assert.equal(stdoutOf(null), null)
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
  const env = fixtureEnv()
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

  // A German locale, as a translated git would answer under it; the mod's
  // GIT_ENV over it, as $.process.run lays env over the host's.
  const ask = path => {
    const ran = spawnSync('git', ['-C', path, ...DEFAULT_BRANCH_ARGS], { env: { ...env, LANG: 'de_DE.UTF-8', LANGUAGE: 'de', ...GIT_ENV }, encoding: 'utf8' })
    return { exitCode: ran.status ?? 1, stdout: ran.stdout, stderr: ran.stderr }
  }

  test('git\'s own words for a removed worktree, a directory left without its repository, and one that is there', () => {
    const outside = mkdtempSync(join(tmpdir(), 'factory-band-outside-'))
    try {
      mkdirSync(join(outside, 'pruned'))
      writeFileSync(join(outside, 'pruned', '.git'), `gitdir: ${join(outside, 'repo', '.git', 'worktrees', 'pruned')}\n`)
      assert.equal(isWorktreeGone(ask(join(outside, 'removed'))), true)
      assert.equal(isWorktreeGone(ask(join(outside, 'pruned'))), true)
      assert.equal(isWorktreeGone(ask(outside)), true)
      assert.equal(isWorktreeGone(ask(dir)), false)
    } finally {
      rmSync(outside, { recursive: true, force: true })
    }
  })

  test('a branch main never took stays ready', () => {
    git('checkout', '-q', 'open')
    const { facts, isIntegrated } = factsOfHead()
    assert.equal(facts?.ahead, 1)
    assert.equal(isIntegrated, false)
    assert.equal(workerState({ git: { ...facts, isIntegrated }, liveness: 'gone', lastKind: 'ready' }), 'ready')
  })
})

// A git call's result as $.process.run resolves it.
const gitRun = (exitCode, stdout = '', extra = {}) => ({ exitCode, stdout, stderr: exitCode === 0 ? '' : 'fatal', ...extra })

// A fake runner: answers each argv from `answers`, keyed by the joined argv,
// and records the calls; an argv it has no answer for never answers.
const fakeGit = answers => {
  const calls = []
  const runGit = async (args, stdin) => {
    calls.push(stdin === undefined ? args.join(' ') : `${args.join(' ')} < ${stdin.trim().split('\n').length} lines`)
    return answers[args.join(' ')] ?? null
  }
  return { runGit, calls }
}

const ORIGIN = { ref: 'origin/main', tree: 'aaff749' }
const commitLines = n => Array.from({ length: n }, (_, i) => `c${i} t${i}\n`).join('')

describe('first-parent walk', () => {
  test('the walk lists one commit past its limit, so a range too long shows, and no signature lines', () => {
    assert.deepEqual(walkArgs('f0f0', 'origin/main'), [
      'log',
      '--no-show-signature',
      '--first-parent',
      `--max-count=${WALK_LIMIT + 1}`,
      '--format=%H %T',
      'f0f0..origin/main',
    ])
    assert.deepEqual(mergeBaseArgs('origin/main'), ['merge-base', 'origin/main', 'HEAD'])
  })

  test('commits and trees from log; a range past the limit, a bad line or no answer is unknown', () => {
    assert.deepEqual(parseWalk('c2 t2\nc1 t1\n'), [
      { commit: 'c2', tree: 't2' },
      { commit: 'c1', tree: 't1' },
    ])
    assert.deepEqual(parseWalk(''), [])
    assert.equal(parseWalk(commitLines(WALK_LIMIT)).length, WALK_LIMIT)
    assert.equal(parseWalk(commitLines(WALK_LIMIT + 1)), null)
    assert.equal(parseWalk('c2\n'), null)
    assert.equal(parseWalk(null), null)
  })

  test('merge-tree reads one `<commit> HEAD` line per commit', () => {
    assert.equal(walkInput([{ commit: 'c2', tree: 't2' }, { commit: 'c1', tree: 't1' }]), 'c2 HEAD\nc1 HEAD\n')
    assert.deepEqual(WALK_MERGE_ARGS, ['merge-tree', '--write-tree', '--stdin', '--name-only', '--no-messages'])
  })

  test('merges from NUL-separated output: a conflict with its file names, then a clean merge', () => {
    assert.deepEqual(parseMerges('0\0t9\0notes.md\0b.md\0\0' + '1\0t1\0\0'), [
      { isClean: false, tree: 't9' },
      { isClean: true, tree: 't1' },
    ])
    assert.deepEqual(parseMerges(''), [])
    assert.equal(parseMerges('fatal: bad\0'), null)
  })

  test('landed when a clean merge writes its commit\'s own tree; a conflicted one never counts', () => {
    const commits = [{ commit: 'c2', tree: 't2' }, { commit: 'c1', tree: 't1' }]
    assert.equal(landedIn(commits, gitRun(0, '1\0x2\0\0' + '1\0t1\0\0')), true)
    assert.equal(landedIn(commits, gitRun(0, '1\0x2\0\0' + '0\0t1\0notes.md\0\0')), false)
    assert.equal(landedIn(commits, gitRun(0, '1\0x2\0\0' + '1\0x1\0\0')), false)
  })

  test('a merge-tree without --stdin is no proof; silence, a cut-off answer or a short one is unknown', () => {
    const commits = [{ commit: 'c1', tree: 't1' }]
    assert.equal(landedIn(commits, gitRun(129, '')), false)
    assert.equal(landedIn(commits, null), null)
    assert.equal(landedIn(commits, gitRun(0, '1\0t1\0\0', { isStdoutTruncated: true })), null)
    assert.equal(landedIn(commits, gitRun(0, '')), null)
  })

  test('no fork point is exit 1 with nothing printed; other failures are not', () => {
    assert.equal(isUnrelated(gitRun(1, '')), true)
    assert.equal(isUnrelated(gitRun(0, 'f0f0\n')), false)
    assert.equal(isUnrelated(gitRun(128, '')), false)
    assert.equal(isUnrelated(null), false)
  })

  test('a tip merge that proves it skips the walk', async () => {
    const { runGit, calls } = fakeGit({ 'merge-tree --write-tree origin/main HEAD': gitRun(0, 'aaff749\n') })
    assert.equal(await integratedOf(runGit, ORIGIN), true)
    assert.deepEqual(calls, ['merge-tree --write-tree origin/main HEAD'])
  })

  test('a walk past the limit gives up as unknown and merges nothing', async () => {
    const { runGit, calls } = fakeGit({
      'merge-tree --write-tree origin/main HEAD': gitRun(1, 'dd0dd0d\n100644 abc 1\tnotes.md\n'),
      'merge-base origin/main HEAD': gitRun(0, 'f0f0\n'),
      [walkArgs('f0f0', 'origin/main').join(' ')]: gitRun(0, commitLines(WALK_LIMIT + 1)),
    })
    assert.equal(await integratedOf(runGit, ORIGIN), null)
    assert.equal(calls.length, 3)
  })

  test('a tip that never answered is unknown; unrelated histories are not integrated', async () => {
    assert.equal(await integratedOf(fakeGit({}).runGit, ORIGIN), null)
    const unrelated = fakeGit({
      'merge-tree --write-tree origin/main HEAD': gitRun(128, ''),
      'merge-base origin/main HEAD': gitRun(1, ''),
    })
    assert.equal(await integratedOf(unrelated.runGit, ORIGIN), false)
  })
})

// A real repo where every branch forked from `base`. main squash-merged
// `landed`, then `drifted`, then changed the line next to drifted's;
// squash-merged `reverted` and reverted it; then moved on. `open` was never
// merged. The mod's own integratedOf, with git itself as the runner.
describe('first-parent walk, against a fixture repo', () => {
  const env = fixtureEnv()
  const id = ['-c', 'user.name=t', '-c', 'user.email=t@example.com', '-c', 'init.defaultBranch=main']
  let dir = ''
  const git = (...args) => execFileSync('git', ['-C', dir, ...id, ...args], { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
  const commitFile = (name, text, message) => {
    writeFileSync(join(dir, name), text)
    git('add', name)
    git('commit', '-q', '-m', message)
  }
  const squash = branch => {
    git('merge', '-q', '--squash', branch)
    git('commit', '-q', '-m', `${branch} (squash)`)
  }
  const runGit = async (args, stdin) => {
    const ran = spawnSync('git', ['-C', dir, ...args], { env: { ...env, ...GIT_ENV }, input: stdin, encoding: 'utf8' })
    return { exitCode: ran.status ?? 1, stdout: ran.stdout, stderr: ran.stderr }
  }
  // The mod's reading of a ready worker's clean tree on the branch checked out now.
  const readHead = async () => {
    const branch = parseDefaultBranch(git(...DEFAULT_BRANCH_ARGS))
    const facts = gitFactsFrom(git(...STATUS_ARGS), git(...aheadArgs(branch.ref)))
    const tip = await runGit(mergeTreeArgs(branch.ref))
    const isIntegrated = await integratedOf(runGit, branch)
    const read = { ...facts, isIntegrated }
    const tipProves = isIntegratedTree(stdoutOf(tip), branch.tree)
    return { ahead: facts?.ahead, tipExit: tip.exitCode, tipProves, isIntegrated, state: workerState({ git: read, liveness: 'gone', lastKind: 'ready' }) }
  }

  before(() => {
    dir = mkdtempSync(join(tmpdir(), 'factory-band-walk-'))
    git('init', '-q', '.')
    commitFile('notes.md', 'one\ntwo\nthree\nfour\n', 'base')
    for (const [branch, name, text] of [
      ['landed', 'landed.md', 'landed\n'],
      ['drifted', 'notes.md', 'one\nTWO\nthree\nfour\n'],
      ['reverted', 'reverted.md', 'reverted\n'],
      ['open', 'open.md', 'open\n'],
    ]) {
      git('checkout', '-q', '-b', branch, 'main')
      commitFile(name, text, branch)
    }
    git('checkout', '-q', 'main')
    squash('landed')
    squash('drifted')
    commitFile('notes.md', 'one\nTWO\nTHREE\nfour\n', 'the line next to drifted\'s')
    squash('reverted')
    git('revert', '--no-edit', 'HEAD')
    commitFile('later.md', 'later\n', 'later work on main')
    git('update-ref', 'refs/remotes/origin/main', 'main')
    git('symbolic-ref', 'refs/remotes/origin/HEAD', 'refs/remotes/origin/main')
  })

  after(() => rmSync(dir, { recursive: true, force: true }))

  test('squash-merged and untouched since: the tip merge proves it', async () => {
    git('checkout', '-q', 'landed')
    assert.deepEqual(await readHead(), { ahead: 1, tipExit: 0, tipProves: true, isIntegrated: true, state: 'integrated' })
  })

  test('squash-merged, then main changed the next line: the tip conflicts and the walk finds the squash', async () => {
    git('checkout', '-q', 'drifted')
    assert.deepEqual(await readHead(), { ahead: 1, tipExit: 1, tipProves: false, isIntegrated: true, state: 'integrated' })
  })

  test('squash-merged, then reverted on main: the tip merges clean to another tree and the walk finds the squash', async () => {
    git('checkout', '-q', 'reverted')
    assert.deepEqual(await readHead(), { ahead: 1, tipExit: 0, tipProves: false, isIntegrated: true, state: 'integrated' })
  })

  test('never merged: the walk finds nothing and the worker stays ready', async () => {
    git('checkout', '-q', 'open')
    assert.deepEqual(await readHead(), { ahead: 1, tipExit: 0, tipProves: false, isIntegrated: false, state: 'ready' })
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

  test('a worker whose worktree is gone is pruned once its loop ended, whatever it said last', () => {
    const w3 = { ...w2, agentId: 'a3', name: 'live-one' }
    const ledger = { facts: new Map(), denies: 0 }
    const agents = [{ id: 'a3', status: 'running' }]
    const probe = probeFrom({ workers: [w1, w2, w3], agents, gits: ['removed', 'removed', 'removed'], ledger, inbox: 0 })
    assert.deepEqual([...prunableIds([w1, w2, w3], probe, agents)], ['a1', 'a2'])
  })

  test('blocked and stopped workers whose worktree is still there stay', () => {
    const blocked = { ...w2, lastKind: 'blocked' }
    const ledger = { facts: new Map(), denies: 0 }
    const tree = { isDirty: true, ahead: 1, isIntegrated: false }
    const probe = probeFrom({ workers: [w1, blocked], agents: [], gits: [tree, tree], ledger, inbox: 0 })
    assert.deepEqual([probe.facts.a1?.state, probe.facts.a2?.state], ['in flight', 'blocked'])
    const stopped = probeFrom({ workers: [w2], agents: [], gits: [tree], ledger, inbox: 0 })
    assert.equal(stopped.facts.a2?.state, 'stopped')
    assert.equal(prunableIds([w1, blocked], probe, []).size + prunableIds([w2], stopped, []).size, 0)
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

describe('run', () => {
  const w1 = { agentId: 'a1', name: 'events', worktree: WT, model: null, lastKind: 'ready' }
  const w2 = { agentId: 'a2', name: 'csl', worktree: null, model: null, lastKind: null }
  const probeOf = states => ({
    facts: Object.fromEntries(Object.entries(states).map(([id, state]) => [id, { state, model: null, effort: null }])),
    ledgerDenies: 0,
    inbox: 0,
  })

  test('the first worker starts the run; a respawn keeps the start and adds no id', () => {
    const one = withWorker(NO_RUN, w1, 100)
    const again = withWorker(withWorker(one, w2, 200), { ...w1, name: 'again' }, 300)
    assert.equal(again.startedAt, 100)
    assert.deepEqual(again.workerIds, ['a1', 'a2'])
    assert.deepEqual(again.workers.map(worker => worker.name), ['csl', 'again'])
  })

  test('guard denials count for the run\'s workers only', () => {
    assert.equal(withGuardDeny(withGuardDeny(withWorker(NO_RUN, w1, 100), 'a1'), 'zz').guardDenies, 1)
    assert.equal(withGuardDeny(NO_RUN, 'a1'), NO_RUN)
  })

  test('a dropped worker keeps its id and denials in the run; the last one dropped closes it', () => {
    const both = withGuardDeny(withWorker(withWorker(NO_RUN, w1, 100), w2, 200), 'a1')
    const one = settleRun(both, probeOf({ a1: 'removed', a2: 'in flight' }), [{ id: 'a2', status: 'running' }])
    assert.deepEqual(one.workers.map(worker => worker.agentId), ['a2'])
    assert.deepEqual([one.workerIds, one.guardDenies, one.startedAt], [['a1', 'a2'], 1, 100])
    assert.equal(withGuardDeny(one, 'a1').guardDenies, 2)
    assert.equal(one.probe?.facts.a2?.state, 'in flight')
    assert.deepEqual(settleRun(one, probeOf({ a2: 'integrated' }), []), NO_RUN)
  })

  test('a worker spawned while the refresh ran is not in its probe, so it stays and keeps the run open', () => {
    const probe = probeOf({ a1: 'integrated' })
    const settled = settleRun(withWorker(withWorker(NO_RUN, w1, 100), w2, 200), probe, [])
    assert.deepEqual(settled.workers.map(worker => worker.agentId), ['a2'])
    assert.equal(settled.startedAt, 100)
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
