import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  agentState,
  compactContextBlock,
  composeBandLines,
  findPhaseMarker,
  isBeltDeny,
  parsePresentResult,
  parseWorklogList,
  phaseLabel,
  repoNameFrom,
  shouldToastDone,
} from '../lib/band.ts'

const idle = { isWaiting: false, worklogKey: null, phase: null, presentPage: null, beltDenies: 0 }

describe('findPhaseMarker', () => {
  test('takes the last phase line of the answer', () => {
    const answer = [
      '[Phase 4.unit.1/3] — scaffold done → tests',
      'some prose',
      '[Phase 4.unit.2/3] — tests done → docs',
    ].join('\n')
    assert.equal(findPhaseMarker(answer), '[Phase 4.unit.2/3] — tests done → docs')
  })

  test('takes the keyed form of ticketed work, key included', () => {
    const answer = 'Research done.\n[MAD-123 Phase 2.1] — evaluate done → plan'
    assert.equal(findPhaseMarker(answer), '[MAD-123 Phase 2.1] — evaluate done → plan')
    assert.equal(phaseLabel('[MAD-123 Phase 2.1] — evaluate done → plan'), 'MAD-123 Phase 2.1')
  })

  test('ignores a marker that is not at a line start', () => {
    assert.equal(findPhaseMarker('see [Phase 1] above'), null)
  })

  test('ignores a bracket that is not a phase marker', () => {
    assert.equal(findPhaseMarker('[WARN] disk is full\n[MAD-123] ticket moved → Done'), null)
    assert.equal(findPhaseMarker('[Phases are over] — wrap up'), null)
  })

  test('is null for an answer with no marker', () => {
    assert.equal(findPhaseMarker('nothing here'), null)
    assert.equal(findPhaseMarker(''), null)
  })

  test('caps a long line', () => {
    const line = `[Phase 5] ${'x'.repeat(300)}`
    assert.equal(findPhaseMarker(line)?.length, 120)
  })
})

describe('phaseLabel', () => {
  test('is the bracketed head of the line', () => {
    assert.equal(phaseLabel('[Phase 4.unit.2/3] — tests done → docs'), 'Phase 4.unit.2/3')
  })

  test('falls back to the whole line without brackets', () => {
    assert.equal(phaseLabel('Phase 2'), 'Phase 2')
  })
})

describe('shouldToastDone', () => {
  test('toasts an answered turn past a minute', () => {
    assert.equal(shouldToastDone('answer', 60_001), true)
  })

  test('stays quiet for a short answer', () => {
    assert.equal(shouldToastDone('answer', 60_000), false)
  })

  test('stays quiet for an aborted or failed turn however long', () => {
    assert.equal(shouldToastDone('aborted', 600_000), false)
    assert.equal(shouldToastDone('error', 600_000), false)
    assert.equal(shouldToastDone('refusal', 600_000), false)
  })
})

describe('isBeltDeny', () => {
  test('is true for a denial with a belt reason', () => {
    assert.equal(isBeltDeny({ deny: 'belt[commit-guard]: main is PR-only' }), true)
  })

  test('is false for another hook, an allow, an ask, or no decision', () => {
    assert.equal(isBeltDeny({ deny: 'policy: no' }), false)
    assert.equal(isBeltDeny({ allow: true }), false)
    assert.equal(isBeltDeny({ ask: 'belt[x]: sure?' }), false)
    assert.equal(isBeltDeny({}), false)
    assert.equal(isBeltDeny(undefined), false)
  })
})

describe('agentState', () => {
  test('waiting wins over working', () => {
    assert.equal(agentState(true, true), 'waiting')
    assert.equal(agentState(true, false), 'waiting')
  })

  test('working and idle follow the render prop', () => {
    assert.equal(agentState(false, true), 'working')
    assert.equal(agentState(false, false), 'idle')
  })
})

describe('composeBandLines', () => {
  test('shows the idle default on one line', () => {
    assert.deepEqual(composeBandLines(idle, false), ['- idle | belt denies 0'])
  })

  test('names every known signal in order', () => {
    const full = {
      isWaiting: false,
      worklogKey: 'MAD-379',
      phase: '[Phase 4.unit.2/3] — tests done → docs',
      presentPage: { id: '1e52d87017', url: null },
      beltDenies: 2,
    }
    assert.deepEqual(composeBandLines(full, true), [
      '* working | worklog MAD-379 | Phase 4.unit.2/3 | present 1e52d87017 | belt denies 2',
    ])
  })

  test('wraps to a second line when the width is short', () => {
    const lines = composeBandLines({ ...idle, isWaiting: true, worklogKey: 'MAD-379' }, true, 32)
    assert.deepEqual(lines, ['? waiting for input', 'worklog MAD-379 | belt denies 0'])
  })

  test('never exceeds two lines', () => {
    const lines = composeBandLines(
      { isWaiting: false, worklogKey: 'k', phase: '[Phase 1]', presentPage: { id: 'p', url: null }, beltDenies: 0 },
      true,
      12,
    )
    assert.equal(lines.length, 2)
  })
})

describe('parseWorklogList', () => {
  test('returns the first key below the header', () => {
    const table = [
      'KEY       STATUS  UPDATED           REPOS',
      'MAD-379   active  2026-10-05 17:37  thismoon',
      'MAD-290   active  2026-09-01 20:15  dotfiles',
    ].join('\n')
    assert.equal(parseWorklogList(table), 'MAD-379')
  })

  test('is null for a header-only or empty table', () => {
    assert.equal(parseWorklogList('KEY  STATUS  UPDATED  REPOS\n'), null)
    assert.equal(parseWorklogList(''), null)
  })

  test('is null for the no items message of an empty store', () => {
    assert.equal(parseWorklogList('no items\n'), null)
  })
})

describe('repoNameFrom', () => {
  test('uses the directory holding .git for an absolute common dir', () => {
    assert.equal(repoNameFrom('/home/u/code/thismoon/.git\n', '/home/u/code/thismoon/.claude/worktrees/x'), 'thismoon')
  })

  test('resolves a relative common dir against the cwd', () => {
    assert.equal(repoNameFrom('.git', '/home/u/code/thismoon'), 'thismoon')
  })

  test('resolves a parent-relative common dir from a subdirectory', () => {
    assert.equal(repoNameFrom('../../.git\n', '/home/u/code/thismoon/services/present'), 'thismoon')
  })

  test('falls back to the cwd name outside a repo', () => {
    assert.equal(repoNameFrom(null, '/tmp/scratch'), 'scratch')
    assert.equal(repoNameFrom('', '/tmp/scratch/'), 'scratch')
  })
})

describe('parsePresentResult', () => {
  test('parses the JSON string a present tool returns', () => {
    const result = '{"id":"c1b321f2c4","url":"http://localhost:7423/p/c1b321f2c4","version":1}'
    assert.deepEqual(parsePresentResult(result, null), { id: 'c1b321f2c4', url: 'http://localhost:7423/p/c1b321f2c4' })
  })

  test('prefers structured content on an MCP record', () => {
    const result = { structuredContent: { id: 'abc', url: 'http://localhost:7423/p/abc' }, content: [] }
    assert.deepEqual(parsePresentResult(result, null), { id: 'abc', url: 'http://localhost:7423/p/abc' })
  })

  test('parses a JSON text block', () => {
    const result = { content: [{ type: 'text', text: '{"id":"def","url":"u"}' }] }
    assert.deepEqual(parsePresentResult(result, null), { id: 'def', url: 'u' })
  })

  test('falls back to the text the model reads', () => {
    assert.deepEqual(parsePresentResult({ content: [] }, null, '{"id":"ghi","url":"v"}'), { id: 'ghi', url: 'v' })
  })

  test('falls back to the input id of an update', () => {
    assert.deepEqual(parsePresentResult('not json', 'jkl', 'not json either'), { id: 'jkl', url: null })
  })

  test('is null with nothing to go on', () => {
    assert.equal(parsePresentResult(undefined, null), null)
    assert.equal(parsePresentResult({ content: [] }, ''), null)
  })
})

describe('compactContextBlock', () => {
  test('lists every known signal and the checkpoint reminder', () => {
    const block = compactContextBlock({
      isWaiting: false,
      worklogKey: 'MAD-379',
      phase: '[Phase 4.unit.2/3] — tests done → docs',
      presentPage: { id: 'p1', url: 'http://localhost:7423/p/p1' },
      beltDenies: 1,
    })
    assert.match(block, /active worklog key: MAD-379/)
    assert.match(block, /last phase marker: \[Phase 4.unit.2\/3\]/)
    assert.match(block, /present page id: p1 \(http:\/\/localhost:7423\/p\/p1\)/)
    assert.match(block, /checkpoint with worklog_checkpoint if work advanced since the last checkpoint/)
  })

  test('keeps only the reminder when nothing is known', () => {
    const lines = compactContextBlock(idle).split('\n')
    assert.equal(lines.length, 2)
  })
})
