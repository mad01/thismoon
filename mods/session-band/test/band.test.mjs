import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  beltEventsUrl,
  compactContextBlock,
  composeBandLines,
  countBeltDenies,
  eventsCursorAt,
  findPhaseMarker,
  parsePresentResult,
  parseWorklogList,
  phaseLabel,
  repoNameFrom,
} from '../lib/band.ts'

const idle = { agent: 'idle', worklogKey: null, phase: null, presentPage: null, beltDenies: 0 }

describe('findPhaseMarker', () => {
  test('takes the last phase line of the answer', () => {
    const answer = [
      '[Phase 4.unit.1/3] — scaffold done → tests',
      'some prose',
      '[Phase 4.unit.2/3] — tests done → docs',
    ].join('\n')
    assert.equal(findPhaseMarker(answer), '[Phase 4.unit.2/3] — tests done → docs')
  })

  test('ignores a marker that is not at a line start', () => {
    assert.equal(findPhaseMarker('see [Phase 1] above'), null)
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

describe('countBeltDenies', () => {
  const events = [
    { id: '00000000000000000003-aaaa', source: 'belt', title: 'blocked Bash (commit-guard)' },
    { id: '00000000000000000002-aaaa', source: 'belt', title: 'hinted Bash (prefer-csl)' },
    { id: '00000000000000000001-aaaa', source: 'deps', title: 'blocked something' },
    { id: '00000000000000000004-aaaa', source: 'belt', title: 'blocked Write (write-internal-names)' },
  ]

  test('counts only belt events whose title says blocked', () => {
    assert.equal(countBeltDenies(events).denies, 2)
  })

  test('reports the newest id as the next cursor', () => {
    assert.equal(countBeltDenies(events).newestId, '00000000000000000004-aaaa')
  })

  test('is empty for a non-array or junk entries', () => {
    assert.deepEqual(countBeltDenies(null), { denies: 0, newestId: null })
    assert.deepEqual(countBeltDenies([null, 1, 'x']), { denies: 0, newestId: null })
  })
})

describe('eventsCursorAt', () => {
  test('is a 20-digit nanosecond stamp with a zero suffix', () => {
    assert.equal(eventsCursorAt(1_700_000_000_000), '01700000000000000000-0000')
  })

  test('sorts after an id stamped earlier and before one stamped later', () => {
    const cursor = eventsCursorAt(1_700_000_000_000)
    assert.ok('01699999999999999999-ffff' < cursor)
    assert.ok('01700000000000000001-0000' > cursor)
  })
})

describe('beltEventsUrl', () => {
  test('filters to belt and carries the cursor', () => {
    assert.equal(
      beltEventsUrl('abc'),
      'http://localhost:7430/api/events?source=belt&limit=200&since=abc',
    )
  })

  test('omits since without a cursor', () => {
    assert.equal(beltEventsUrl(null), 'http://localhost:7430/api/events?source=belt&limit=200')
  })
})

describe('composeBandLines', () => {
  test('shows the idle default on one line', () => {
    assert.deepEqual(composeBandLines(idle), ['- idle | belt denies 0'])
  })

  test('names every known signal in order', () => {
    const full = {
      agent: 'working',
      worklogKey: 'MAD-379',
      phase: '[Phase 4.unit.2/3] — tests done → docs',
      presentPage: { id: '1e52d87017', url: null },
      beltDenies: 2,
    }
    assert.deepEqual(composeBandLines(full), [
      '* working | worklog MAD-379 | Phase 4.unit.2/3 | present 1e52d87017 | belt denies 2',
    ])
  })

  test('wraps to a second line when the width is short', () => {
    const lines = composeBandLines({ ...idle, agent: 'waiting', worklogKey: 'MAD-379' }, 32)
    assert.deepEqual(lines, ['? waiting for input', 'worklog MAD-379 | belt denies 0'])
  })

  test('never exceeds two lines', () => {
    const lines = composeBandLines(
      { agent: 'working', worklogKey: 'k', phase: '[Phase 1]', presentPage: { id: 'p', url: null }, beltDenies: 0 },
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
})

describe('repoNameFrom', () => {
  test('uses the directory holding .git for an absolute common dir', () => {
    assert.equal(repoNameFrom('/home/u/code/thismoon/.git\n', '/home/u/code/thismoon/.claude/worktrees/x'), 'thismoon')
  })

  test('resolves a relative common dir against the cwd', () => {
    assert.equal(repoNameFrom('.git', '/home/u/code/thismoon'), 'thismoon')
  })

  test('falls back to the cwd name outside a repo', () => {
    assert.equal(repoNameFrom(null, '/tmp/scratch'), 'scratch')
    assert.equal(repoNameFrom('', '/tmp/scratch/'), 'scratch')
  })
})

describe('parsePresentResult', () => {
  test('prefers structured content', () => {
    const result = { structuredContent: { id: 'abc', url: 'http://localhost:7423/p/abc' }, content: [] }
    assert.deepEqual(parsePresentResult(result, null), { id: 'abc', url: 'http://localhost:7423/p/abc' })
  })

  test('parses a JSON text block', () => {
    const result = { content: [{ type: 'text', text: '{"id":"def","url":"u"}' }] }
    assert.deepEqual(parsePresentResult(result, null), { id: 'def', url: 'u' })
  })

  test('falls back to the input id of an update', () => {
    assert.deepEqual(parsePresentResult({ content: [{ type: 'text', text: 'not json' }] }, 'ghi'), { id: 'ghi', url: null })
  })

  test('is null with nothing to go on', () => {
    assert.equal(parsePresentResult(undefined, null), null)
    assert.equal(parsePresentResult({ content: [] }, ''), null)
  })
})

describe('compactContextBlock', () => {
  test('lists every known signal and the checkpoint reminder', () => {
    const block = compactContextBlock({
      agent: 'idle',
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
