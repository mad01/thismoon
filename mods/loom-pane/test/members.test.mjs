import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  MEMBERS_REFRESH_GAP_MS,
  UNKNOWN,
  agentWorktreeOf,
  composeStatus,
  isMemberWorktree,
  isRefreshDue,
  isolationWorktreeOf,
  linkedWorktreeOf,
  memberText,
  membersFrom,
  parseLedger,
  parseSessions,
  trailingDelay,
} from '../lib/loom.ts'

const CANONICAL = '/home/u/code/thismoon'
const ROOT = '/home/u/.worktrees/thismoon'
const OWN = `${ROOT}/feat-pane`
const OTHER = `${ROOT}/fix-band`
const QUIET = `${ROOT}/old-docs`
const NESTED = `${CANONICAL}/.claude/worktrees/agent-a1`
const STRAY = '/home/u/scratch/thismoon-wt'
const layout = {
  canonical: CANONICAL,
  ownWorktree: null,
  worktreeRoot: ROOT,
  worktrees: [CANONICAL, OWN, OTHER, QUIET, NESTED, STRAY],
}

// The shape `claude agents --json --all` prints: background rows carry
// `state`, interactive rows `status`, and every row its cwd.
const sessionsJson = JSON.stringify([
  { id: 'ec2f68c4', cwd: OTHER, kind: 'background', sessionId: 's-bg', name: 'band fixer', state: 'blocked' },
  { id: 'f00dcafe', cwd: QUIET, kind: 'background', sessionId: 's-done', name: 'docs pass', state: 'done' },
  { pid: 9110, cwd: CANONICAL, kind: 'interactive', sessionId: 's-lead', name: 'thismoon-6d', status: 'busy' },
  { pid: 9200, cwd: `${OWN}/mods`, kind: 'interactive', sessionId: 's-self', name: 'pane-owner', status: 'idle' },
  { pid: 9300, cwd: '/home/u/code/elsewhere', kind: 'interactive', sessionId: 's-out', name: 'other', status: 'idle' },
  { pid: 9400, cwd: STRAY, kind: 'interactive', sessionId: 's-stray', name: 'stray', status: 'idle' },
])

// The events service's answer for the belt-ledger source, newest first.
const ledgerJson = JSON.stringify([
  { id: '4', source: 'belt-ledger', title: 'subagent started (general-purpose)', tags: { event: 'subagent-start', agent_id: 'a4', worktree: OTHER } },
  { id: '3', source: 'belt-ledger', title: 'subagent started (general-purpose)', tags: { event: 'subagent-start', agent_id: 'a1', worktree: OTHER, model: 'opus', effort: 'high' } },
  { id: '2', source: 'belt-ledger', title: 'subagent stopped (Explore)', tags: { event: 'subagent-stop', agent_id: 'a2', model: 'haiku' } },
  { id: '1', source: 'belt-ledger', title: 'subagent started (general-purpose)', tags: { event: 'subagent-start', agent_id: 'a1', model: 'sonnet', effort: 'low' } },
])

describe('parseSessions', () => {
  test('reads session id, name, cwd and state from both row kinds', () => {
    const rows = parseSessions(sessionsJson)
    assert.equal(rows.length, 6)
    assert.deepEqual(rows[0], { sessionId: 's-bg', name: 'band fixer', cwd: OTHER, state: 'blocked' })
    assert.deepEqual(rows[2], { sessionId: 's-lead', name: 'thismoon-6d', cwd: CANONICAL, state: 'busy' })
  })

  test('falls back to the short id for a row with no name, and null for no state', () => {
    assert.deepEqual(parseSessions(JSON.stringify([{ id: 'ab12', cwd: OWN }])), [{ sessionId: null, name: 'ab12', cwd: OWN, state: null }])
  })

  test('is empty for output that is not a JSON array', () => {
    assert.deepEqual(parseSessions(null), [])
    assert.deepEqual(parseSessions(''), [])
    assert.deepEqual(parseSessions('error: unknown command agents'), [])
    assert.deepEqual(parseSessions('{"id":"x"}'), [])
    assert.deepEqual(parseSessions('[1, "x", null]'), [])
  })
})

describe('parseLedger', () => {
  test('keeps the newest subagent-start entry per agent', () => {
    const facts = parseLedger(ledgerJson)
    assert.deepEqual(facts.get('a1'), { worktree: OTHER, model: 'opus', effort: 'high' })
  })

  test('ignores other events and entries with no agent id', () => {
    const facts = parseLedger(ledgerJson)
    assert.equal(facts.has('a2'), false)
    assert.equal(parseLedger(JSON.stringify([{ tags: { event: 'subagent-start' } }])).size, 0)
  })

  test('reads absent tags as null rather than a value', () => {
    const facts = parseLedger(JSON.stringify([{ tags: { event: 'subagent-start', agent_id: 'a3', agent_type: 'Explore' } }]))
    assert.deepEqual(facts.get('a3'), { worktree: null, model: null, effort: null })
  })

  test('is empty when the service answered nothing usable', () => {
    assert.equal(parseLedger(null).size, 0)
    assert.equal(parseLedger('<html>bad gateway</html>').size, 0)
    assert.equal(parseLedger('[]').size, 0)
  })
})

describe('worktrees', () => {
  test('member worktrees are the loom layout and the isolation layout only', () => {
    assert.equal(isMemberWorktree(OWN, layout), true)
    assert.equal(isMemberWorktree(NESTED, layout), true)
    assert.equal(isMemberWorktree(STRAY, layout), false)
    assert.equal(isMemberWorktree(`${CANONICAL}/.claude/worktrees/scratch`, layout), false)
    assert.equal(isMemberWorktree(OWN, { ...layout, worktreeRoot: null }), false)
  })

  test('linkedWorktreeOf finds the innermost linked worktree and never the canonical checkout', () => {
    assert.equal(linkedWorktreeOf(`${OWN}/mods/loom-pane`, layout), OWN)
    assert.equal(linkedWorktreeOf(`${NESTED}/lib`, layout), NESTED)
    assert.equal(linkedWorktreeOf(`${CANONICAL}/mods`, layout), null)
    assert.equal(linkedWorktreeOf('/tmp', layout), null)
  })

  test('isolationWorktreeOf matches git\'s agent worktree by exact id', () => {
    assert.equal(isolationWorktreeOf('a1', layout), NESTED)
    assert.equal(isolationWorktreeOf('a', layout), null)
    assert.equal(isolationWorktreeOf('a1', { ...layout, canonical: null }), null)
  })

  test('the isolation worktree wins over the ledger tag', () => {
    assert.equal(agentWorktreeOf('a1', OTHER, layout), NESTED)
  })

  test('the ledger tag counts only for a member worktree that is not this session\'s own', () => {
    assert.equal(agentWorktreeOf('a4', OTHER, layout), OTHER)
    assert.equal(agentWorktreeOf('a4', OWN, { ...layout, ownWorktree: OWN }), null)
    assert.equal(agentWorktreeOf('a4', STRAY, layout), null)
    assert.equal(agentWorktreeOf('a4', null, layout), null)
  })
})

describe('membersFrom', () => {
  const sessions = parseSessions(sessionsJson)
  const ledger = parseLedger(ledgerJson)

  test('an agent in an isolation worktree takes model and effort from the ledger', () => {
    const agents = [{ id: 'a1', name: 'w1', type: 'general-purpose', status: 'running' }]
    const [member] = membersFrom({ agents, sessions: [], ledger, layout, selfSessionId: null })
    assert.deepEqual(member, { name: 'w1', role: 'owner', worktree: 'agent-a1', model: 'opus', effort: 'high', state: null })
  })

  test('with no ledger entry model and effort are dashes, never read off the type', () => {
    const agents = [{ id: 'a1', name: 'w1', type: 'general-purpose', status: 'idle' }]
    const [member] = membersFrom({ agents, sessions: [], ledger: new Map(), layout, selfSessionId: null })
    assert.deepEqual(member, { name: 'w1', role: 'owner', worktree: 'agent-a1', model: UNKNOWN, effort: UNKNOWN, state: null })
  })

  test('an agent in an isolation or loom worktree is an owner', () => {
    const agents = [
      { id: 'a1', type: 'general-purpose', status: 'running' },
      { id: 'a4', name: 'band agent', type: 'general-purpose', status: 'running' },
    ]
    const found = membersFrom({ agents, sessions: [], ledger, layout, selfSessionId: null })
    assert.deepEqual(found.map(member => [member.name, member.role, member.worktree]), [
      ['a1', 'owner', 'agent-a1'],
      ['band agent', 'owner', 'fix-band'],
    ])
  })

  test('plain named teammates, unnamed helpers and finished agents are left out', () => {
    const agents = [
      { id: 'a1', name: 'done', type: 'general-purpose', status: 'completed' },
      { id: 'a6', type: 'Explore', status: 'running' },
      { id: 'a7', name: 'implementer', type: 'general-purpose', status: 'running' },
    ]
    assert.deepEqual(membersFrom({ agents, sessions: [], ledger: new Map(), layout, selfSessionId: null }), [])
  })

  test('sessions count in a member worktree with their state, done ones and this one left out', () => {
    const found = membersFrom({ agents: [], sessions, ledger, layout, selfSessionId: 's-self' })
    assert.deepEqual(found, [{ name: 'band fixer', role: 'owner', worktree: 'fix-band', model: UNKNOWN, effort: UNKNOWN, state: 'blocked' }])
  })

  test('without a session id every live session in a member worktree counts', () => {
    const found = membersFrom({ agents: [], sessions, ledger, layout, selfSessionId: null })
    assert.deepEqual(found.map(member => member.name), ['band fixer', 'pane-owner'])
  })

  test('no sources, no members', () => {
    assert.deepEqual(membersFrom({ agents: [], sessions: [], ledger: new Map(), layout, selfSessionId: null }), [])
  })
})

describe('the refresh throttle', () => {
  test('the first refresh is always due', () => {
    assert.equal(isRefreshDue(0, null), true)
    assert.equal(trailingDelay(0, null), 0)
  })

  test('a refresh is due only a full gap after the last one started', () => {
    assert.equal(MEMBERS_REFRESH_GAP_MS, 5_000)
    assert.equal(isRefreshDue(14_999, 10_000), false)
    assert.equal(isRefreshDue(15_000, 10_000), true)
  })

  test('the trailing refresh waits for the rest of the gap', () => {
    assert.equal(trailingDelay(12_000, 10_000), 3_000)
    assert.equal(trailingDelay(10_000, 10_000), 5_000)
    assert.equal(trailingDelay(16_000, 10_000), 0)
  })
})

describe('composeStatus', () => {
  const w1 = { name: 'w1', role: 'owner', worktree: 'feat-x', model: 'opus', effort: 'high', state: null }
  const fixer = { name: 'fixer', role: 'owner', worktree: 'fix-band', model: UNKNOWN, effort: UNKNOWN, state: 'idle' }

  test('memberText lists the five fields in order, a session\'s state after them', () => {
    assert.equal(memberText(w1), 'w1 (owner, feat-x, opus, high)')
    assert.equal(memberText(fixer), 'fixer (owner, fix-band, -, -) idle')
  })

  test('members follow the role text', () => {
    assert.equal(composeStatus('weaver · 2 worktrees', [w1, fixer]), 'weaver · 2 worktrees · members w1 (owner, feat-x, opus, high), fixer (owner, fix-band, -, -) idle')
  })

  test('the role text alone when nobody else works the loom', () => {
    assert.equal(composeStatus('owner feat-pane', []), 'owner feat-pane')
  })

  test('the members alone for a session with no loom role, and clear with neither', () => {
    assert.equal(composeStatus(undefined, [w1]), 'members w1 (owner, feat-x, opus, high)')
    assert.equal(composeStatus(undefined, []), undefined)
  })
})
