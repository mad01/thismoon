import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { composeStatusText, isBeltRefusal } from '../lib/band.ts'

describe('isBeltRefusal', () => {
  test('a belt reason on the first line of an errored result counts', () => {
    assert.equal(isBeltRefusal({ isError: true, text: 'belt[git-push-main]: pushes to main are denied', result: undefined }), true)
    assert.equal(isBeltRefusal({ isError: true, text: 'PreToolUse:Bash hook error: belt[x]: no\nmore', result: undefined }), true)
    assert.equal(isBeltRefusal({ isError: true, text: '  belt[x]: after leading blanks', result: undefined }), true)
    assert.equal(isBeltRefusal({ deny: 'belt[x]: no' }), true)
  })

  test('other results do not count, belt in the output included', () => {
    assert.equal(isBeltRefusal({ isError: true, text: 'Exit code 1\nbelt[x]: quoted from a log', result: undefined }), false)
    assert.equal(isBeltRefusal({ isError: true, text: 'settings rule denied the call', result: undefined }), false)
    assert.equal(isBeltRefusal({ isError: true, text: 'hook error: belt denied it', result: undefined }), false)
    assert.equal(isBeltRefusal({ result: { stdout: 'belt[x]: no' }, text: 'belt[x]: no' }), false)
    assert.equal(isBeltRefusal({ deny: 'loom-pane: outside your worktree' }), false)
    assert.equal(isBeltRefusal({ isError: true, result: undefined }), false)
    assert.equal(isBeltRefusal({}), false)
    assert.equal(isBeltRefusal(undefined), false)
    assert.equal(isBeltRefusal(null), false)
    assert.equal(isBeltRefusal(['belt[x]: in an array']), false)
    assert.equal(isBeltRefusal('belt[x]: a bare string'), false)
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
