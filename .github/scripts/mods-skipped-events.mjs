// Fails when a mod's hooks module registers an event that Claude Code's
// built-in guard mod, when seated, forwards past every user-tier mod, so the
// hook never runs (mods/CLAUDE.md, Rules). Run it from the repo root, or pass
// the root as the one argument:
//
//   node .github/scripts/mods-skipped-events.mjs [root]
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

const SKIPPED =
  'classic\\.[A-Za-z]+|prompt\\.(?:compose|context|section|attachment)|skill\\.prompt|session\\.(?:compact|append)|settings\\.read|attribution\\.text'

// `\s` spans newlines, so `on(\n  'classic.PreToolUse', ...)` matches too.
const HOOKED = new RegExp(`\\bon\\(\\s*(['"\`])(${SKIPPED})\\1`, 'g')

// A `/` after one of these, or at the start, opens a regex literal rather
// than a division.
const BEFORE_REGEX = new Set('(,=:[!&|?{};+-*%<>~^')

// The source with every `//` and block comment blanked to spaces. Newlines
// stay, so line numbers still match. Strings, template literals and regex
// literals are copied unchanged, so a `//` inside one is not a comment.
function stripComments(src) {
  const blank = text => text.replace(/[^\n]/g, ' ')
  let out = ''
  let last = '' // the previous character outside whitespace and comments
  let i = 0
  while (i < src.length) {
    const c = src[i]
    const pair = src.slice(i, i + 2)
    let stop
    if (pair === '//') {
      stop = src.indexOf('\n', i)
      if (stop === -1) stop = src.length
      out += blank(src.slice(i, stop))
    } else if (pair === '/*') {
      stop = src.indexOf('*/', i + 2)
      stop = stop === -1 ? src.length : stop + 2
      out += blank(src.slice(i, stop))
    } else {
      if (c === "'" || c === '"' || c === '`') stop = skipQuoted(src, i)
      else if (c === '/' && (last === '' || BEFORE_REGEX.has(last))) stop = skipRegex(src, i)
      else stop = i + 1
      out += src.slice(i, stop)
      if (!/\s/.test(c)) last = c
    }
    i = stop
  }
  return out
}

// The index just past the string or template literal opened at `start`. A
// template's `${...}` is not parsed: a backtick nested in one ends it early.
function skipQuoted(src, start) {
  const quote = src[start]
  let i = start + 1
  while (i < src.length && src[i] !== quote) {
    if (src[i] === '\n' && quote !== '`') return i
    i += src[i] === '\\' ? 2 : 1
  }
  return Math.min(i + 1, src.length)
}

// The index just past the regex literal opened at `start`, flags included.
function skipRegex(src, start) {
  let inClass = false
  let i = start + 1
  while (i < src.length && src[i] !== '\n') {
    const c = src[i]
    if (c === '/' && !inClass) break
    if (c === '[') inClass = true
    if (c === ']') inClass = false
    i += c === '\\' ? 2 : 1
  }
  i++
  while (i < src.length && /[a-z]/.test(src[i])) i++
  return i
}

// Each skipped event the source registers, with the line its name is on.
function findHits(src) {
  const code = stripComments(src)
  return [...code.matchAll(HOOKED)].map(m => {
    const at = m.index + m[0].length - m[2].length - 1
    return { event: m[2], line: code.slice(0, at).split('\n').length }
  })
}

// mods/*/hooks/*.ts and *.tsx, relative to root, leaving out test files.
function hooksModules(root) {
  const files = []
  const mods = readdirSync(join(root, 'mods'), { withFileTypes: true }).filter(d => d.isDirectory())
  for (const mod of mods.map(d => d.name).sort()) {
    const dir = join('mods', mod, 'hooks')
    let names = []
    try {
      names = readdirSync(join(root, dir)).sort()
    } catch {
      continue
    }
    for (const name of names) {
      if (/\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name)) files.push(join(dir, name))
    }
  }
  return files
}

function main(root) {
  const files = hooksModules(root)
  if (files.length === 0) {
    console.log('::error::no hooks module under mods/*/hooks/')
    return 1
  }
  let found = 0
  for (const file of files) {
    for (const { event, line } of findHits(readFileSync(join(root, file), 'utf8'))) {
      found++
      console.log(
        `::error file=${file},line=${line}::${file}:${line} hooks ${event}, which the built-in guard skips for user-tier mods`,
      )
    }
  }
  if (found === 0) console.log(`no skipped event hooked in ${files.length} hooks modules`)
  return found === 0 ? 0 : 1
}

process.exitCode = main(process.argv[2] ?? '.')
