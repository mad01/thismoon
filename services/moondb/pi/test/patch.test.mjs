import assert from 'node:assert/strict'
import test from 'node:test'

import { diffHunks, parseUnifiedPatch } from '../lib/patch.ts'

// apply is moondb's patch.Apply in miniature: every context and removed
// line must sit where its hunk says, and a '\' line drops the newline of
// the line before it.
function apply(before, hunks) {
  const lines = before === '' ? [] : before.split(/(?<=\n)/)
  const out = []
  let at = 0
  for (const h of hunks) {
    const start = h.oldLines > 0 ? h.oldStart - 1 : h.oldStart
    while (at < start) out.push(lines[at++])
    for (let i = 0; i < h.lines.length; i++) {
      const line = h.lines[i]
      const last = h.lines[i + 1]?.[0] === '\\'
      const text = line.slice(1) + (last ? '' : '\n')
      if (line[0] === '\\') continue
      if (line[0] !== '+') {
        assert.equal(lines[at], text, `hunk at ${h.oldStart}: line ${at + 1}`)
        at++
      }
      if (line[0] !== '-') out.push(text)
    }
  }
  while (at < lines.length) out.push(lines[at++])
  return out.join('')
}

// counts checks every hunk's header against its lines, as moondb's
// patch.Parse does.
function counts(hunks) {
  for (const h of hunks) {
    const old = h.lines.filter((l) => l[0] === ' ' || l[0] === '-').length
    const neu = h.lines.filter((l) => l[0] === ' ' || l[0] === '+').length
    assert.deepEqual([old, neu], [h.oldLines, h.newLines])
    assert.ok(h.lines.every((l) => l !== ''))
  }
}

test('parseUnifiedPatch reads the hunks of pi\'s edit patch', () => {
  const patch = [
    '--- a.md',
    '+++ a.md',
    '@@ -1,3 +1,3 @@',
    ' # title',
    '-old',
    '+new',
    ' ',
    '@@ -9 +9,2 @@',
    '-tail',
    '\\ No newline at end of file',
    '+tail',
    '+--- not a header',
    '',
  ].join('\n')
  assert.deepEqual(parseUnifiedPatch(patch), [
    { oldStart: 1, oldLines: 3, newStart: 1, newLines: 3, lines: [' # title', '-old', '+new', ' '] },
    {
      oldStart: 9, oldLines: 1, newStart: 9, newLines: 2,
      lines: ['-tail', '\\ No newline at end of file', '+tail', '+--- not a header'],
    },
  ])
})

test('parseUnifiedPatch refuses what moondb would reject', () => {
  assert.equal(parseUnifiedPatch(undefined), undefined)
  assert.equal(parseUnifiedPatch('--- a\n+++ a\n'), undefined)
  assert.equal(parseUnifiedPatch('@@ -1,2 +1,2 @@\n-a\n+b\n'), undefined)
  assert.equal(parseUnifiedPatch('@@ -1 +1 @@\n*a\n'), undefined)
})

test('diffHunks gives the hunks diff -u would', () => {
  assert.deepEqual(diffHunks('same\n', 'same\n'), [])
  assert.deepEqual(diffHunks('', 'a\nb\n'), [{ oldStart: 0, oldLines: 0, newStart: 1, newLines: 2, lines: ['+a', '+b'] }])
  assert.deepEqual(diffHunks('a\nb\n', ''), [{ oldStart: 1, oldLines: 2, newStart: 0, newLines: 0, lines: ['-a', '-b'] }])
  assert.deepEqual(diffHunks('a\nb', 'a\nb\n'), [
    { oldStart: 1, oldLines: 2, newStart: 1, newLines: 2, lines: [' a', '-b', '\\ No newline at end of file', '+b'] },
  ])
  const before = Array.from({ length: 20 }, (_, i) => `line ${i + 1}\n`).join('')
  const after = before.replace('line 2\n', 'line two\n').replace('line 18\n', '')
  assert.deepEqual(diffHunks(before, after), [
    { oldStart: 1, oldLines: 5, newStart: 1, newLines: 5, lines: [' line 1', '-line 2', '+line two', ' line 3', ' line 4', ' line 5'] },
    { oldStart: 15, oldLines: 6, newStart: 15, newLines: 5, lines: [' line 15', ' line 16', ' line 17', '-line 18', ' line 19', ' line 20'] },
  ])
})

test('diffHunks round-trips through apply for random edits', () => {
  let seed = 7
  const rand = (n) => {
    seed = (seed * 1103515245 + 12345) % 2 ** 31
    return seed % n
  }
  const words = ['a', 'b', 'c', 'd', '', 'e']
  const text = (n) => Array.from({ length: n }, () => `${words[rand(words.length)]}\n`).join('')
  for (let i = 0; i < 300; i++) {
    const before = text(rand(30))
    const lines = before.split(/(?<=\n)/).filter(Boolean)
    for (let j = rand(6); j > 0; j--) {
      const at = rand(lines.length + 1)
      if (rand(2)) lines.splice(at, 1)
      else lines.splice(at, 0, `${words[rand(words.length)]}\n`)
    }
    let after = lines.join('')
    if (rand(5) === 0) after = after.replace(/\n$/, '')
    const hunks = diffHunks(before, after)
    counts(hunks)
    assert.equal(apply(before, hunks), after, JSON.stringify({ before, after }))
  }
})

test('diffHunks past its edit distance bound still gives a valid patch', () => {
  const before = Array.from({ length: 3000 }, (_, i) => `old ${i}\n`).join('')
  const after = `head\n${Array.from({ length: 3000 }, (_, i) => `new ${i}\n`).join('')}`
  const hunks = diffHunks(before, after)
  counts(hunks)
  assert.equal(apply(before, hunks), after)
})
