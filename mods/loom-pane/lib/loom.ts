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

/** Where the repo's checkouts are, as far as the mod knows. */
export type Layout = {
  canonical: string | null
  ownWorktree: string | null
  worktreeRoot: string | null
  /** Every worktree path git listed, nested ones under the canonical checkout included. */
  worktrees: readonly string[]
}

export type PathClass = 'own' | 'canonical' | 'other' | 'outside'

export type Detected = Omit<Layout, 'worktrees'> & { repo: string | null; role: Role }

/** How long one git call may take. */
export const GIT_BUDGET_MS = 2_000

/** The ref the ahead count compares against when origin names no HEAD. */
export const DEFAULT_BRANCH = 'main'

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

// The verbs whose path operands name what they change; the others take
// messages, refs and patch files, whose spelling says nothing about writes.
const OPERAND_VERBS = new Set(['add', 'rm', 'mv', 'checkout', 'restore', 'clean'])

// Flags that take the next word as a value, so that word is not a path.
const VALUE_FLAGS = new Set(['-m', '-F', '--message', '--file', '-b', '-B', '--reason', '-C'])

// Commands that run another command the tokenizer cannot see into.
const OPAQUE_COMMANDS = new Set(['sh', 'bash', 'zsh', 'eval', 'exec', 'xargs', 'sudo', 'env', 'popd'])

// Prefixes that run the command after them unchanged.
const PASSTHROUGH_COMMANDS = new Set(['command', 'time'])

const CONTROL_OPS = new Set(['&&', '||', ';', ';;', '|', '|&', '&', '\n'])

const ASSIGNMENT = /^([A-Za-z_][A-Za-z0-9_]*)=/

const GIT_ENV = new Set(['GIT_DIR', 'GIT_WORK_TREE', 'GIT_COMMON_DIR', 'GIT_INDEX_FILE'])

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

/**
 * Which checkout a path falls in: the longest root that holds it wins, so a
 * worktree nested under the canonical checkout counts as a worktree and the
 * own worktree wins over the `~/.worktrees/<repo>` root above it.
 */
export function classifyPath(path: string, layout: Layout): PathClass {
  const roots: Array<{ root: string; cls: PathClass }> = []
  if (layout.ownWorktree !== null) roots.push({ root: layout.ownWorktree, cls: 'own' })
  if (layout.canonical !== null) roots.push({ root: layout.canonical, cls: 'canonical' })
  if (layout.worktreeRoot !== null) roots.push({ root: layout.worktreeRoot, cls: 'other' })
  for (const worktree of layout.worktrees) {
    const cls: PathClass = isSamePath(worktree, layout.ownWorktree)
      ? 'own'
      : isSamePath(worktree, layout.canonical)
        ? 'canonical'
        : 'other'
    roots.push({ root: worktree, cls })
  }
  let best: { root: string; cls: PathClass } | null = null
  for (const candidate of roots) {
    if (!isUnder(path, candidate.root)) continue
    if (best === null || trimSlash(candidate.root).length > trimSlash(best.root).length) best = candidate
  }
  return best === null ? 'outside' : best.cls
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
 * The worktree a path claims in the loom layout: `~/.worktrees/<repo>/<slug>`
 * for a path at or below it, null for anything else.
 */
export function worktreeClaimedBy(path: string, worktreeRoot: string | null): string | null {
  if (worktreeRoot === null) return null
  const root = trimSlash(worktreeRoot)
  const candidate = trimSlash(path)
  if (candidate === root || !isUnder(candidate, root)) return null
  const slug = candidate.slice(root.length + 1).split('/')[0] ?? ''
  return slug === '' ? null : `${root}/${slug}`
}

/** The first worktree a command's `cd` or `git worktree add` claims in the layout. */
export function ownerClaimFrom(claims: readonly string[], worktreeRoot: string | null): string | null {
  for (const claim of claims) {
    const worktree = worktreeClaimedBy(claim, worktreeRoot)
    if (worktree !== null) return worktree
  }
  return null
}

/**
 * The repo around the session and its role in the loom: `weaver` anywhere in
 * the repo once a weave was asked for, `owner` inside
 * `~/.worktrees/<repo>/<slug>`, `none` anywhere else or outside git.
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
  if (input.weaveSeen) return { canonical, ownWorktree: null, worktreeRoot, repo, role: 'weaver' }
  if (worktreeRoot !== null && dirName(top) === worktreeRoot) {
    return { canonical, ownWorktree: top, worktreeRoot, repo, role: 'owner' }
  }
  return { canonical, ownWorktree: null, worktreeRoot, repo, role: 'none' }
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

export function worktreeAt(entries: readonly WorktreeEntry[], path: string): WorktreeEntry | null {
  return entries.find(entry => isSamePath(entry.path, path)) ?? null
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

/** The arguments of a `/loom ...` prompt, or null when the prompt is not one. */
export function loomArgsOf(text: string): string | null {
  const match = /^\/loom(?:\s+([\s\S]*))?$/.exec(text.trim())
  return match === null ? null : (match[1] ?? '').trim()
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

type Segment = Token[] | '(' | ')'

/** The simple commands in order, with the subshell parentheses kept as markers. */
function segmentsOf(tokens: readonly Token[]): Segment[] {
  const segments: Segment[] = []
  let current: Token[] = []
  const close = (): void => {
    if (current.length > 0) segments.push(current)
    current = []
  }
  for (const token of tokens) {
    if (token.kind === 'op' && (token.text === '(' || token.text === ')')) {
      close()
      segments.push(token.text)
      continue
    }
    if (token.kind === 'op' && CONTROL_OPS.has(token.text)) {
      close()
      continue
    }
    current.push(token)
  }
  close()
  return segments
}

type Parts = { argv: string[]; assignments: string[]; redirectTargets: string[] }

/** A simple command's argv, its leading assignments, and the files its redirections write. */
function partsOf(simple: readonly Token[]): Parts {
  const argv: string[] = []
  const assignments: string[] = []
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
  while (argv.length > 0) {
    const match = ASSIGNMENT.exec(argv[0] ?? '')
    if (match === null) break
    assignments.push(match[1] ?? '')
    argv.shift()
  }
  return { argv, assignments, redirectTargets }
}

/** The argv from `git` on, when the command is git by any of its spellings; else null. */
function gitArgvOf(argv: readonly string[]): readonly string[] | null {
  const head = argv[0]
  if (head === undefined) return null
  if (head === 'git' || (head.includes('/') && baseName(head) === 'git')) return argv
  return null
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

/** The operands of a verb with the flag values dropped. */
function operandsOf(args: readonly string[]): string[] {
  const operands: string[] = []
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i] ?? ''
    if (VALUE_FLAGS.has(arg)) {
      i += 1
      continue
    }
    if (arg.startsWith('-')) continue
    operands.push(arg)
  }
  return operands
}

function isMutating(call: GitCall): boolean {
  if (call.verb === null) return false
  if (call.verb === 'stash') {
    const sub = operandsOf(call.args)[0]
    return sub !== 'list' && sub !== 'show'
  }
  return MUTATING_GIT_VERBS.has(call.verb)
}

export type Removal =
  | { kind: 'branch'; name: string; location: string }
  | { kind: 'worktree'; path: string; isForce: boolean }

/** What a `git branch -D` or `git worktree remove` names; null when a path cannot be resolved. */
function removalsOf(call: GitCall, home: string | null): Removal[] | null {
  const flags = call.args.filter(arg => arg.startsWith('-'))
  const short = flags.filter(flag => /^-[A-Za-z]+$/.test(flag)).join('')
  const isForce = short.includes('f') || flags.includes('--force')
  if (call.verb === 'branch') {
    const isDelete = short.includes('d') || flags.includes('--delete')
    if (!(short.includes('D') || (isDelete && isForce))) return []
    return operandsOf(call.args).map(name => ({ kind: 'branch', name, location: call.location }))
  }
  if (call.verb === 'worktree' && call.args[0] === 'remove') {
    const removals: Removal[] = []
    for (const spelled of operandsOf(call.args.slice(1))) {
      const path = resolvePath(spelled, call.location, home)
      if (path === null) return null
      removals.push({ kind: 'worktree', path, isForce })
    }
    return removals
  }
  return []
}

export type BashAnalysis = {
  /** True when the command could not be followed; the caller lets it through. */
  isUncertain: boolean
  /** Where a mutating git verb acts (its repo, and path operands for the verbs that take them) and what redirections write. */
  writes: string[]
  /** The branches and worktrees the command force-deletes or removes. */
  removals: Removal[]
  /** Where the command goes: every `cd` target and `git worktree add` path, resolved. */
  claims: string[]
}

/**
 * Reads a Bash command for what it would change on disk: `cd` moves the
 * working directory for what follows (a subshell's `cd` ends with it),
 * `git -C` names the repo a verb acts on, `>` names a file. Anything it
 * cannot follow makes the whole reading uncertain rather than a guess.
 */
export function analyzeBash(command: string, env: { cwd: string; home: string | null }): BashAnalysis {
  const uncertain: BashAnalysis = { isUncertain: true, writes: [], removals: [], claims: [] }
  const { tokens, isUncertain } = tokenizeShell(command)
  if (isUncertain) return uncertain
  const analysis: BashAnalysis = { isUncertain: false, writes: [], removals: [], claims: [] }
  let cwd = normalizePath(env.cwd)
  const outer: string[] = []
  for (const segment of segmentsOf(tokens)) {
    if (segment === '(') {
      outer.push(cwd)
      continue
    }
    if (segment === ')') {
      cwd = outer.pop() ?? cwd
      continue
    }
    const { argv: words, assignments, redirectTargets } = partsOf(segment)
    for (const target of redirectTargets) {
      const path = resolvePath(target, cwd, env.home)
      if (path === null) return uncertain
      analysis.writes.push(path)
    }
    if (assignments.some(name => GIT_ENV.has(name))) return uncertain
    let argv: string[] = words
    while (argv.length > 0 && PASSTHROUGH_COMMANDS.has(argv[0] ?? '')) argv = argv.slice(1)
    const head = argv[0]
    if (head === undefined) continue
    if (OPAQUE_COMMANDS.has(head)) return uncertain
    if (head === 'cd' || head === 'pushd') {
      const spelled = argv.slice(1).find(arg => arg === '-' || !arg.startsWith('-'))
      const next = spelled === undefined ? env.home : resolvePath(spelled, cwd, env.home)
      if (next === null) return uncertain
      cwd = next
      analysis.claims.push(cwd)
      continue
    }
    const gitArgv = gitArgvOf(argv)
    if (gitArgv === null) continue
    const call = parseGit(gitArgv, cwd, env.home)
    if (call === null) return uncertain
    if (isMutating(call)) {
      analysis.writes.push(call.location)
      if (call.verb !== null && OPERAND_VERBS.has(call.verb)) {
        for (const operand of operandsOf(call.args)) {
          if (!operand.startsWith('/') && !operand.startsWith('~')) continue
          const path = resolvePath(operand, call.location, env.home)
          if (path !== null) analysis.writes.push(path)
        }
      }
    }
    if (call.verb === 'worktree' && call.args[0] === 'add') {
      const spelled = operandsOf(call.args.slice(1))[0]
      if (spelled !== undefined) {
        const path = resolvePath(spelled, call.location, env.home)
        if (path === null) return uncertain
        analysis.claims.push(path)
      }
    }
    const removals = removalsOf(call, env.home)
    if (removals === null) return uncertain
    analysis.removals.push(...removals)
  }
  return analysis
}

/**
 * Whether `git worktree remove` needs a word first: git itself refuses a
 * dirty tree without `--force`, so only a forced removal of a dirty tree, or
 * a detached head whose commits sit on no branch, can lose work.
 */
export function shouldAskWorktreeRemoval(input: {
  isForce: boolean
  isDirty: boolean
  isDetached: boolean
  ahead: number | null
}): boolean {
  if (input.isForce && input.isDirty) return true
  return input.isDetached && input.ahead !== null && input.ahead > 0
}

/**
 * Whether `git branch -D` needs a word first: when the branch's commits exist
 * nowhere else (ahead of the default branch and not on `origin/<branch>`,
 * `remoteAhead` null meaning no such upstream), or its worktree is dirty.
 */
export function shouldAskBranchDeletion(input: {
  ahead: number | null
  remoteAhead: number | null
  isWorktreeDirty: boolean
}): boolean {
  if (input.isWorktreeDirty) return true
  const isUnpushed = input.remoteAhead === null || input.remoteAhead > 0
  return isUnpushed && input.ahead !== null && input.ahead > 0
}

/**
 * The status line for a role, or undefined (clear) when the session has
 * none. The engine prefixes the line with the mod's name, so the text
 * never repeats it: `owner feat-pane`, `weaver · 3 worktrees`.
 */
export function statusText(role: Role, layout: Layout): string | undefined {
  if (role === 'owner') {
    return `owner ${layout.ownWorktree === null ? '?' : baseName(layout.ownWorktree)}`
  }
  if (role !== 'weaver') return undefined
  const linked = layout.worktrees.filter(path => !isSamePath(path, layout.canonical))
  if (linked.length === 0) return 'weaver'
  const noun = linked.length === 1 ? 'worktree' : 'worktrees'
  return `weaver · ${linked.length} ${noun}`
}

/** The question before a removal that would lose work. */
export function removalQuestion(label: string, ahead: number | null, isDirty: boolean): string {
  const risks: string[] = []
  if (ahead !== null && ahead > 0) risks.push(`${ahead} unmerged ${ahead === 1 ? 'commit' : 'commits'}`)
  if (isDirty) risks.push('uncommitted changes')
  return `${label} has ${risks.join(' and ')}. Remove anyway?`
}
