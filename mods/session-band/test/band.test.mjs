import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { composeStatusText, decisionSummary, isBeltDeny } from '../lib/band.ts'

describe('isBeltDeny', () => {
  test('is true for a denial with a belt reason', () => {
    assert.equal(isBeltDeny({ deny: 'belt[commit-guard]: main is PR-only' }), true)
  })

  test('is true when the engine wraps the reason in its hook-error prefix', () => {
    assert.equal(
      isBeltDeny({ deny: 'PreToolUse:Bash hook error: belt[git-push-main]: pushing to "main" is blocked' }),
      true,
    )
  })

  test('is true for a shape the types do not name, searched as JSON', () => {
    assert.equal(isBeltDeny({ block: 'belt[git-push-main]: pushing to "main" is blocked' }), true)
    assert.equal(
      isBeltDeny({ hookSpecificOutput: { permissionDecision: 'deny', permissionDecisionReason: 'belt[x]: no' } }),
      true,
    )
  })

  test('is false for another hook, an allow, an ask, or no decision', () => {
    assert.equal(isBeltDeny({ deny: 'policy: no' }), false)
    assert.equal(isBeltDeny({ deny: 'hook error: belt denied it' }), false)
    assert.equal(isBeltDeny({ allow: true }), false)
    assert.equal(isBeltDeny({ ask: 'belt[x]: sure?' }), false)
    assert.equal(isBeltDeny({ block: 'policy: no' }), false)
    assert.equal(isBeltDeny({}), false)
    assert.equal(isBeltDeny(undefined), false)
    assert.equal(isBeltDeny('belt[x]: a bare string'), false)
  })
})

describe('decisionSummary', () => {
  test('names the keys and the head of the JSON', () => {
    assert.equal(decisionSummary({ deny: 'belt[x]: no' }), 'keys [deny] {"deny":"belt[x]: no"}')
    assert.equal(decisionSummary({ allow: true, updatedInput: { a: 1 } }), 'keys [allow,updatedInput] {"allow":true,"updatedInput":{"a":1}}')
  })

  test('caps a long JSON at 120 characters', () => {
    const summary = decisionSummary({ deny: 'x'.repeat(300) })
    assert.equal(summary, `keys [deny] ${JSON.stringify({ deny: 'x'.repeat(300) }).slice(0, 120)}...`)
  })

  test('describes a non-object', () => {
    assert.equal(decisionSummary(null), 'object null')
    assert.equal(decisionSummary('x'), 'string x')
  })
})

describe('composeStatusText', () => {
  test('clears the line before the first deny', () => {
    assert.equal(composeStatusText(0), undefined)
  })

  test('shows the count from the first deny on', () => {
    assert.equal(composeStatusText(1), 'belt denies 1')
    assert.equal(composeStatusText(12), 'belt denies 12')
  })

  test('never repeats the mod name, which the engine prefixes', () => {
    assert.doesNotMatch(composeStatusText(3), /session-band/)
  })
})
