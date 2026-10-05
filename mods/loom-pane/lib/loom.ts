// Pure helpers behind the loom-pane mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

export type Role = 'owner' | 'weaver' | 'none'

/** One entry of `git worktree list --porcelain`. */
export type WorktreeEntry = {
  path: string
  head: string | null
  branch: string | null
  isBare: boolean
  isDetached: boolean
}

/** One pane row: a worktree and what the survey learned; null is a failed call. */
export type WorktreeRow = {
  path: string
  branch: string | null
  head: string | null
  isDirty: boolean | null
  ahead: number | null
  age: string | null
}

/** Where the repo's checkouts are, as far as the mod knows. */
export type Layout = {
  canonical: string | null
  ownWorktree: string | null
  worktreeRoot: string | null
}

export type PathClass = 'own' | 'canonical' | 'other' | 'outside'

export type Detected = Layout & { repo: string | null; role: Role }

/** How long one git call may take. */
export const GIT_BUDGET_MS = 2_000

/** The ref the ahead count compares against when origin names no HEAD. */
export const DEFAULT_BRANCH = 'main'

/** The widest a branch column gets in the pane. */
const LABEL_WIDTH_CAP = 36

// The git verbs that change a working tree, its index or its refs. The loom
// skill's list, plus the verbs that do the same under another name.
const MUTATING_GIT_VERBS = new Set([
  'commit',
  'add',
  'checkout',
  'switch',
  'reset',
  'rebase',
  'merge',
  'push',
  'rm',
  'mv',
  'restore',
  'stash',
  'cherry-pick',
  'revert',
  'clean',
  'pull',
  'apply',
  'am',
])

// Commands that run another command the tokenizer cannot see into.
const OPAQUE_COMMANDS = new Set(['sh', 'bash', 'zsh', 'eval', 'exec', 'xargs', 'sudo', 'env', 'popd'])

const CONTROL_OPS = new Set(['&&', '||', ';', ';;', '|', '|&', '&', '(', ')', '\n'])

const ASSIGNMENT = /^[A-Za-z_][A-Za-z0-9_]*=/

function trimSlash(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  return trimmed === '' ? '/' : trimmed
}

/** Collapses `.`, `..` and repeated slashes in an absolute path. */
export function normalizePath(path: string): string {
  const parts: string[] = []
  for (const part of path.split('/')) {
    if (part === '' || part === '.') continue
    if (part === '..') {
      parts.pop()
      continue
    }
    parts.push(part)
  }
  return `/${parts.join('/')}`
}

export function baseName(path: string): string {
  const parts = trimSlash(path).split('/')
  return parts[parts.length - 1] ?? ''
}

export function dirName(path: string): string {
  const trimmed = trimSlash(path)
  const cut = trimmed.lastIndexOf('/')
  return cut <= 0 ? '/' : trimmed.slice(0, cut)
}

function isSamePath(a: string | null, b: string | null): boolean {
  return a !== null && b !== null && trimSlash(a) === trimSlash(b)
}

/** True when `path` is `root` or lies below it, by spelling (not by inode). */
export function isUnder(path: string, root: string): boolean {
  const p = trimSlash(path)
  const r = trimSlash(root)
  return p === r || p.startsWith(r === '/' ? '/' : `${r}/`)
}

/**
 * The absolute path a shell word names, or null when it cannot be known
 * without running the shell: a variable or substitution, `~user`, `-`, or
 * `~` with no home.
 */
export function resolvePath(word: string, cwd: string, home: string | null): string | null {
  if (word === '' || word === '-' || /[$`]/.test(word)) return null
  if (word.startsWith('/')) return normalizePath(word)
  if (word === '~' || word.startsWith('~/')) {
    return home === null ? null : normalizePath(`${home}${word.slice(1)}`)
  }
  if (word.startsWith('~')) return null
  return normalizePath(`${cwd}/${word}`)
}

/** Which checkout a path falls in. Own wins over the worktree root it sits under. */
export function classifyPath(path: string, layout: Layout): PathClass {
  if (layout.ownWorktree !== null && isUnder(path, layout.ownWorktree)) return 'own'
  if (layout.canonical !== null && isUnder(path, layout.canonical)) return 'canonical'
  if (layout.worktreeRoot !== null && isUnder(path, layout.worktreeRoot)) return 'other'
  return 'outside'
}

export function denyReason(path: string, cls: 'canonical' | 'other'): string {
  const holder = cls === 'canonical' ? 'the canonical checkout' : 'another worktree'
  return `loom-pane: ${path} belongs to ${holder}; owners edit only their own worktree`
}

/**
 * The canonical checkout behind `git rev-parse --git-common-dir`: the parent
 * of the common git dir, which git prints relative to `cwd` from the main
 * worktree and absolute from a linked one.
 */
export function canonicalFrom(commonDir: string | null, cwd: string): string | null {
  const trimmed = commonDir?.trim() ?? ''
  if (trimmed === '') return null
  const absolute = trimmed.startsWith('/') ? normalizePath(trimmed) : normalizePath(`${cwd}/${trimmed}`)
  return dirName(absolute)
}

/** `~/.worktrees/<repo>`, the loom's layout for one repo. */
export function worktreeRootFor(home: string | null, repo: string): string | null {
  return home === null || home === '' ? null : `${trimSlash(home)}/.worktrees/${repo}`
}

/**
 * The repo around the session and its role in the loom: `owner` inside
 * `~/.worktrees/<repo>/<slug>`, `weaver` in the canonical checkout once a
 * weave was asked for, `none` anywhere else or outside git.
 */
export function detectLayout(input: {
  cwd: string
  toplevel: string | null
  commonDir: string | null
  home: string | null
  weaveSeen: boolean
}): Detected {
  const none: Detected = { canonical: null, ownWorktree: null, worktreeRoot: null, repo: null, role: 'none' }
  const toplevel = input.toplevel?.trim() ?? ''
  const canonical = canonicalFrom(input.commonDir, input.cwd)
  if (toplevel === '' || canonical === null) return none
  const repo = baseName(canonical)
  const worktreeRoot = worktreeRootFor(input.home, repo)
  const top = trimSlash(toplevel)
  if (worktreeRoot !== null && dirName(top) === worktreeRoot) {
    return { canonical, ownWorktree: top, worktreeRoot, repo, role: 'owner' }
  }
  const role: Role = input.weaveSeen && isSamePath(top, canonical) ? 'weaver' : 'none'
  return { canonical, ownWorktree: null, worktreeRoot, repo, role }
}

/** The entries of `git worktree list --porcelain`, in git's order. */
export function parseWorktreeList(porcelain: string): WorktreeEntry[] {
  const entries: WorktreeEntry[] = []
  let current: WorktreeEntry | null = null
  for (const raw of porcelain.split('\n')) {
    const line = raw.trimEnd()
    if (line.startsWith('worktree ')) {
      if (current !== null) entries.push(current)
      current = { path: line.slice('worktree '.length), head: null, branch: null, isBare: false, isDetached: false }
      continue
    }
    if (current === null) continue
    if (line.startsWith('HEAD ')) current.head = line.slice('HEAD '.length)
    else if (line.startsWith('branch ')) current.branch = line.slice('branch '.length).replace(/^refs\/heads\//, '')
    else if (line === 'bare') current.isBare = true
    else if (line === 'detached') current.isDetached = true
  }
  if (current !== null) entries.push(current)
  return entries
}

export function worktreeOfBranch(entries: readonly WorktreeEntry[], branch: string): WorktreeEntry | null {
  return entries.find(entry => entry.branch === branch) ?? null
}

/** `origin/main` from `git symbolic-ref refs/remotes/origin/HEAD`; `main` when it failed. */
export function parseDefaultRef(stdout: string | null): string {
  const line = stdout?.trim().split('\n')[0] ?? ''
  const short = line.replace(/^refs\/remotes\//, '')
  return short === '' || short === line ? DEFAULT_BRANCH : short
}

export function parseCount(stdout: string | null): number | null {
  const n = Number.parseInt(stdout?.trim() ?? '', 10)
  return Number.isNaN(n) ? null : n
}

/** The `/loom` arguments that make the session the weaver: `weave` as the first word. */
export function isWeaveArgs(args: string): boolean {
  return /^\s*weave(\s|$)/.test(args)
}

type Token = { kind: 'word'; text: string } | { kind: 'op'; text: string }

type Tokenized = { tokens: Token[]; isUncertain: boolean }

/**
 * Splits a shell command into words and operators the way a POSIX shell
 * would, quotes removed. Gives up (`isUncertain`) on what it cannot follow
 * without running it: substitutions, heredocs, an unbalanced quote.
 */
export function tokenizeShell(command: string): Tokenized {
  const tokens: Token[] = []
  const uncertain: Tokenized = { tokens, isUncertain: true }
  let word = ''
  let hasWord = false
  const flush = (): void => {
    if (!hasWord) return
    tokens.push({ kind: 'word', text: word })
    word = ''
    hasWord = false
  }
  const push = (text: string): void => {
    flush()
    tokens.push({ kind: 'op', text })
  }
  const n = command.length
  let i = 0
  while (i < n) {
    const c = command[i] ?? ''
    if (c === "'") {
      const end = command.indexOf("'", i + 1)
      if (end < 0) return uncertain
      word += command.slice(i + 1, end)
      hasWord = true
      i = end + 1
      continue
    }
    if (c === '"') {
      let j = i + 1
      let closed = false
      while (j < n) {
        const d = command[j] ?? ''
        if (d === '\\' && j + 1 < n) {
          word += command[j + 1]
          j += 2
          continue
        }
        if (d === '"') {
          closed = true
          break
        }
        if (d === '`') return uncertain
        word += d
        j += 1
      }
      if (!closed) return uncertain
      hasWord = true
      i = j + 1
      continue
    }
    if (c === '\\') {
      const escaped = command[i + 1]
      if (escaped !== undefined && escaped !== '\n') {
        word += escaped
        hasWord = true
      }
      i += 2
      continue
    }
    if (c === '`' || (c === '$' && command[i + 1] === '(')) return uncertain
    if (c === '#' && !hasWord) {
      const end = command.indexOf('\n', i)
      i = end < 0 ? n : end
      continue
    }
    if (c === ' ' || c === '\t' || c === '\r') {
      flush()
      i += 1
      continue
    }
    if (c === '\n') {
      push('\n')
      i += 1
      continue
    }
    if (c === '<') {
      if (command[i + 1] === '<') return uncertain
      push('<')
      i += 1
      continue
    }
    if (c === '>') {
      let op = ''
      if (hasWord && /^\d$/.test(word)) {
        op = word
        word = ''
        hasWord = false
      }
      flush()
      op += '>'
      i += 1
      if (command[i] === '>') {
        op += '>'
        i += 1
      }
      if (command[i] === '&') {
        op += '&'
        i += 1
      } else if (command[i] === '|') {
        i += 1
      }
      push(op)
      continue
    }
    if (c === '&') {
      if (command[i + 1] === '&') {
        push('&&')
        i += 2
        continue
      }
      if (command[i + 1] === '>') {
        let op = '&>'
        i += 2
        if (command[i] === '>') {
          op += '>'
          i += 1
        }
        push(op)
        continue
      }
      push('&')
      i += 1
      continue
    }
    if (c === '|') {
      const two = command[i + 1] === '|' ? '||' : command[i + 1] === '&' ? '|&' : null
      push(two ?? '|')
      i += two === null ? 1 : 2
      continue
    }
    if (c === ';') {
      const two = command[i + 1] === ';'
      push(two ? ';;' : ';')
      i += two ? 2 : 1
      continue
    }
    if (c === '(' || c === ')') {
      push(c)
      i += 1
      continue
    }
    word += c
    hasWord = true
    i += 1
  }
  flush()
  return { tokens, isUncertain: false }
}

function splitSimple(tokens: readonly Token[]): Token[][] {
  const commands: Token[][] = []
  let current: Token[] = []
  for (const token of tokens) {
    if (token.kind === 'op' && CONTROL_OPS.has(token.text)) {
      if (current.length > 0) commands.push(current)
      current = []
      continue
    }
    current.push(token)
  }
  if (current.length > 0) commands.push(current)
  return commands
}

/** A simple command's argv (leading assignments dropped) and the files its redirections write. */
function partsOf(simple: readonly Token[]): { argv: string[]; redirectTargets: string[] } {
  const argv: string[] = []
  const redirectTargets: string[] = []
  for (let i = 0; i < simple.length; i += 1) {
    const token = simple[i]
    if (token === undefined) break
    if (token.kind === 'word') {
      argv.push(token.text)
      continue
    }
    const target = simple[i + 1]
    i += 1
    if (token.text.endsWith('&') || token.text === '<') continue
    if (target?.kind === 'word') redirectTargets.push(target.text)
  }
  while (argv.length > 0 && ASSIGNMENT.test(argv[0] ?? '')) argv.shift()
  return { argv, redirectTargets }
}

type GitCall = { location: string; verb: string | null; args: string[] }

/** `git [-C p] [-c k=v] [--flag] verb args...`; null when the repo it acts on cannot be known. */
function parseGit(argv: readonly string[], cwd: string, home: string | null): GitCall | null {
  let location = cwd
  let i = 1
  while (i < argv.length) {
    const arg = argv[i] ?? ''
    if (arg === '-C' || (arg.startsWith('-C') && arg.length > 2)) {
      const spelled = arg === '-C' ? argv[i + 1] : arg.slice(2)
      if (spelled === undefined) return null
      const resolved = resolvePath(spelled, location, home)
      if (resolved === null) return null
      location = resolved
      i += arg === '-C' ? 2 : 1
      continue
    }
    if (arg === '-c') {
      i += 2
      continue
    }
    if (/^--(work-tree|git-dir)(=|$)/.test(arg)) return null
    if (arg.startsWith('-')) {
      i += 1
      continue
    }
    return { location, verb: arg, args: argv.slice(i + 1) }
  }
  return { location, verb: null, args: [] }
}

export type Removal =
  | { kind: 'branch'; name: string; location: string }
  | { kind: 'worktree'; path: string }

/** What a `git branch -D` or `git worktree remove` names; null when a path cannot be resolved. */
function removalsOf(call: GitCall, home: string | null): Removal[] | null {
  const flags = call.args.filter(arg => arg.startsWith('-'))
  const operands = call.args.filter(arg => !arg.startsWith('-'))
  if (call.verb === 'branch') {
    const short = flags.filter(flag => /^-[A-Za-z]+$/.test(flag)).join('')
    const isDelete = short.includes('d') || flags.includes('--delete')
    const isForce = short.includes('f') || flags.includes('--force')
    if (!(short.includes('D') || (isDelete && isForce))) return []
    return operands.map(name => ({ kind: 'branch', name, location: call.location }))
  }
  if (call.verb === 'worktree' && call.args[0] === 'remove') {
    const removals: Removal[] = []
    for (const spelled of operands.slice(1)) {
      const path = resolvePath(spelled, call.location, home)
      if (path === null) return null
      removals.push({ kind: 'worktree', path })
    }
    return removals
  }
  return []
}

export type BashAnalysis = {
  /** True when the command could not be followed; the caller lets it through. */
  isUncertain: boolean
  /** Where a mutating git verb acts (its repo and absolute operands) and what redirections write. */
  writes: string[]
  /** The branches and worktrees the command force-deletes or removes. */
  removals: Removal[]
}

/**
 * Reads a Bash command for what it would change on disk: `cd` moves the
 * working directory for what follows, `git -C` names the repo a verb acts
 * on, `>` names a file. Anything it cannot follow makes the whole reading
 * uncertain rather than a guess.
 */
export function analyzeBash(command: string, env: { cwd: string; home: string | null }): BashAnalysis {
  const uncertain: BashAnalysis = { isUncertain: true, writes: [], removals: [] }
  const { tokens, isUncertain } = tokenizeShell(command)
  if (isUncertain) return uncertain
  const analysis: BashAnalysis = { isUncertain: false, writes: [], removals: [] }
  let cwd = normalizePath(env.cwd)
  for (const simple of splitSimple(tokens)) {
    const { argv, redirectTargets } = partsOf(simple)
    for (const target of redirectTargets) {
      const path = resolvePath(target, cwd, env.home)
      if (path === null) return uncertain
      analysis.writes.push(path)
    }
    const head = argv[0]
    if (head === undefined) continue
    if (OPAQUE_COMMANDS.has(head)) return uncertain
    if (head === 'cd' || head === 'pushd') {
      const spelled = argv.slice(1).find(arg => arg === '-' || !arg.startsWith('-'))
      const next = spelled === undefined ? env.home : resolvePath(spelled, cwd, env.home)
      if (next === null) return uncertain
      cwd = next
      continue
    }
    if (head !== 'git') continue
    const call = parseGit(argv, cwd, env.home)
    if (call === null) return uncertain
    if (call.verb !== null && MUTATING_GIT_VERBS.has(call.verb)) {
      analysis.writes.push(call.location)
      for (const arg of call.args) {
        if (!arg.startsWith('/') && !arg.startsWith('~')) continue
        const path = resolvePath(arg, call.location, env.home)
        if (path !== null) analysis.writes.push(path)
      }
    }
    const removals = removalsOf(call, env.home)
    if (removals === null) return uncertain
    analysis.removals.push(...removals)
  }
  return analysis
}

/** The branch column of a row: the branch, or the detached head's short sha. */
export function rowLabel(row: WorktreeRow): string {
  if (row.branch !== null) return row.branch
  return row.head === null ? 'detached' : `detached ${row.head.slice(0, 7)}`
}

/** One pane line: a mark (`>` own, `=` canonical), branch, dirty state, ahead count, age. */
export function formatRow(row: WorktreeRow, layout: Layout, labelWidth: number): string {
  const mark = isSamePath(row.path, layout.ownWorktree) ? '>' : isSamePath(row.path, layout.canonical) ? '=' : ' '
  const state = row.isDirty === null ? '?' : row.isDirty ? 'dirty' : 'clean'
  const ahead = row.ahead === null ? '?' : `+${row.ahead}`
  const age = row.age ?? '?'
  return `${mark} ${rowLabel(row).padEnd(labelWidth)}  ${state.padEnd(5)}  ${ahead.padStart(4)}  ${age}`
}

/** The pane's lines, one per worktree, cut to `columns` when a width is known. */
export function paneLines(rows: readonly WorktreeRow[], layout: Layout, columns?: number): string[] {
  if (rows.length === 0) return []
  const widest = Math.max(...rows.map(row => rowLabel(row).length))
  const labelWidth = Math.min(LABEL_WIDTH_CAP, widest)
  return rows.map(row => {
    const line = formatRow(row, layout, labelWidth)
    return columns === undefined || columns <= 0 || line.length <= columns ? line : line.slice(0, columns)
  })
}

/** The status line for a role, or undefined (clear) when the session has none. */
export function statusText(role: Role, layout: Layout, rows: readonly WorktreeRow[]): string | undefined {
  if (role === 'owner') {
    return `loom: owner ${layout.ownWorktree === null ? '?' : baseName(layout.ownWorktree)}`
  }
  if (role !== 'weaver') return undefined
  if (rows.length === 0) return 'loom: weaver'
  const linked = rows.filter(row => !isSamePath(row.path, layout.canonical))
  const dirty = linked.filter(row => row.isDirty === true).length
  const noun = linked.length === 1 ? 'worktree' : 'worktrees'
  return `loom: weaver · ${linked.length} ${noun}, ${dirty} dirty`
}

/** The question before a removal that would lose work. */
export function removalQuestion(label: string, ahead: number | null, isDirty: boolean): string {
  const risks: string[] = []
  if (ahead !== null && ahead > 0) risks.push(`${ahead} unmerged ${ahead === 1 ? 'commit' : 'commits'}`)
  if (isDirty) risks.push('uncommitted changes')
  return `${label} has ${risks.join(' and ')}. Remove anyway?`
}
