import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register, Timer } from 'claude-code'

import type { LoomPaneRole, LoomPaneWorktree } from '../types'
import {
  GIT_BUDGET_MS,
  analyzeBash,
  classifyPath,
  denyReason,
  detectLayout,
  isWeaveArgs,
  paneLines,
  parseCount,
  parseDefaultRef,
  parseWorktreeList,
  removalQuestion,
  statusText,
  worktreeOfBranch,
} from '../lib/loom'
import type { Layout, Removal, WorktreeEntry } from '../lib/loom'

const PANE = 'loom'
const COMMAND = 'loom-pane'
const DEFAULT_POLL_MS = 30_000
const MIN_POLL_MS = 1_000

const role = atom({ plugin: 'loom-pane', key: 'role' } as const, 'none' as LoomPaneRole)
const canonical = atom({ plugin: 'loom-pane', key: 'canonical' } as const, null as string | null)
const ownWorktree = atom({ plugin: 'loom-pane', key: 'ownWorktree' } as const, null as string | null)
const worktreeRoot = atom({ plugin: 'loom-pane', key: 'worktreeRoot' } as const, null as string | null)
const repo = atom({ plugin: 'loom-pane', key: 'repo' } as const, null as string | null)
const weaveSeen = atom({ plugin: 'loom-pane', key: 'weaveSeen' } as const, false)
const rows = atom({ plugin: 'loom-pane', key: 'rows' } as const, [] as LoomPaneWorktree[])

type Logger = { ui: { log: (text: string, options: { to: 'debug' }) => void } }
type Failed<E, R> = ((e: E) => R) & { event: string; error: { kind: string; message?: string } }

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. A guard that
// throws or times out lets the call through; belt stays the fail-closed one.
const skipOnFailure = <E, R>($: Logger, e: E, next: Failed<E, R>): R => {
  const why = next.error.message === undefined ? next.error.kind : `${next.error.kind}: ${next.error.message}`
  $.ui.log(`loom-pane: ${next.event} skipped (${why})`, { to: 'debug' })
  return next(e)
}

// Set while a survey runs, so a slow git never stacks surveys.
let isSurveying = false

// One git call under the budget: its stdout, or null when it failed, exited
// non-zero or outran the budget. Never throws.
async function git($: EngineInterface, args: readonly string[], cwd: string): Promise<string | null> {
  const outcome = await Promise.race([
    $.process.run(['git', ...args], { cwd, timeoutMs: GIT_BUDGET_MS })
      .then(ran => ({ kind: 'ran' as const, ran }))
      .catch(() => ({ kind: 'failed' as const })),
    $.clock.sleep(GIT_BUDGET_MS).then(() => ({ kind: 'timeout' as const })),
  ])
  if (outcome.kind !== 'ran' || outcome.ran.exitCode !== 0) return null
  return outcome.ran.stdout
}

async function layoutOf($: EngineInterface): Promise<Layout> {
  return {
    canonical: await read($, canonical),
    ownWorktree: await read($, ownWorktree),
    worktreeRoot: await read($, worktreeRoot),
  }
}

async function showStatus($: EngineInterface): Promise<void> {
  $.ui.status(statusText(await read($, role), await layoutOf($), await read($, rows)))
}

// Finds the repo around `cwd` and the session's role in its loom, writes
// both to the state and pins the status line.
async function detect($: EngineInterface, cwd: string): Promise<void> {
  const toplevel = await git($, ['rev-parse', '--show-toplevel'], cwd)
  let commonDir: string | null = null
  if (toplevel !== null) {
    // --path-format=absolute needs git 2.31; older gits answer the plain
    // form, relative to cwd, which detectLayout resolves.
    commonDir =
      (await git($, ['rev-parse', '--path-format=absolute', '--git-common-dir'], cwd)) ??
      (await git($, ['rev-parse', '--git-common-dir'], cwd))
  }
  const home = await $.env.get('HOME')
  const found = detectLayout({
    cwd,
    toplevel,
    commonDir,
    home: home ?? null,
    weaveSeen: await read($, weaveSeen),
  })
  await update($, canonical, () => found.canonical)
  await update($, ownWorktree, () => found.ownWorktree)
  await update($, worktreeRoot, () => found.worktreeRoot)
  await update($, repo, () => found.repo)
  await update($, role, () => found.role)
  await showStatus($)
  if (found.canonical === null) {
    $.ui.log(`loom-pane: no git repo at ${cwd}`, { to: 'debug' })
    return
  }
  $.ui.log(`loom-pane: repo ${found.repo} canonical ${found.canonical} role ${found.role}`, { to: 'debug' })
}

async function rowFor($: EngineInterface, entry: WorktreeEntry, defaultRef: string): Promise<LoomPaneWorktree> {
  const [status, count, age] = await Promise.all([
    git($, ['status', '--porcelain'], entry.path),
    git($, ['rev-list', '--count', `${defaultRef}..HEAD`], entry.path),
    git($, ['log', '-1', '--format=%cr'], entry.path),
  ])
  return {
    path: entry.path,
    branch: entry.branch,
    head: entry.head,
    isDirty: status === null ? null : status.trim() !== '',
    ahead: parseCount(count),
    age: age === null ? null : age.trim(),
  }
}

// Reads every worktree of the repo and stores the rows the pane draws.
async function survey($: EngineInterface): Promise<void> {
  if (isSurveying) return
  isSurveying = true
  try {
    const canon = await read($, canonical)
    if (canon === null) return
    const listed = await git($, ['worktree', 'list', '--porcelain'], canon)
    if (listed === null) {
      $.ui.log('loom-pane: worktree list failed', { to: 'debug' })
      return
    }
    const defaultRef = parseDefaultRef(await git($, ['symbolic-ref', 'refs/remotes/origin/HEAD'], canon))
    const pending: Promise<LoomPaneWorktree>[] = []
    for (const entry of parseWorktreeList(listed)) {
      if (!entry.isBare) pending.push(rowFor($, entry, defaultRef))
    }
    const surveyed = await Promise.all(pending)
    await update($, rows, () => surveyed)
    await showStatus($)
  } finally {
    isSurveying = false
  }
}

async function markWeave($: EngineInterface, cwd: string): Promise<void> {
  await update($, weaveSeen, () => true)
  await detect($, cwd)
  if ((await read($, role)) === 'weaver') {
    $.clock.after(0, () => {
      void survey($)
    })
  }
}

type RemovalTarget = { label: string; worktreePath: string | null; rev: string; revCwd: string }

function targetOf(removal: Removal, entries: readonly WorktreeEntry[], canon: string): RemovalTarget {
  if (removal.kind === 'worktree') {
    return { label: removal.path, worktreePath: removal.path, rev: 'HEAD', revCwd: removal.path }
  }
  const linked = worktreeOfBranch(entries, removal.name)
  return { label: removal.name, worktreePath: linked === null ? null : linked.path, rev: removal.name, revCwd: canon }
}

// True when the removal may go ahead: nothing would be lost, or the person
// chose Proceed. A git call that fails reads as nothing at risk.
async function confirmRemoval(
  $: EngineInterface,
  removal: Removal,
  entries: readonly WorktreeEntry[],
  canon: string,
  defaultRef: string,
): Promise<boolean> {
  const target = targetOf(removal, entries, canon)
  const status = target.worktreePath === null ? null : await git($, ['status', '--porcelain'], target.worktreePath)
  const isDirty = status !== null && status.trim() !== ''
  const ahead = parseCount(await git($, ['rev-list', '--count', `${defaultRef}..${target.rev}`], target.revCwd))
  if (!isDirty && (ahead === null || ahead === 0)) return true
  const answer = await $.ui.ask(removalQuestion(target.label, ahead, isDirty), ['Proceed', 'Cancel'])
  return answer === 'Proceed'
}

export const register: Register = (on, options) => {
  const isGuarding = options.guards !== false
  const pollMs =
    typeof options.pollMs === 'number' && options.pollMs >= MIN_POLL_MS ? options.pollMs : DEFAULT_POLL_MS
  // The pane's refresh timer; module-level on purpose, a reload drops it
  // with the pane.
  let refresh: Timer | null = null

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('loom-pane: loaded', { to: 'debug' })
    await $.command.register({ name: COMMAND, description: 'Open the loom worktree pane' })
    await detect($, e.cwd)
    return started
  }).catch(skipOnFailure)

  on('classic.SessionStart', { source: ['resume', 'clear'] }, async ($, e, next) => {
    await detect($, e.cwd)
    return next(e)
  }).catch(skipOnFailure)

  // `/loom weave` typed at the prompt makes a canonical-checkout session the
  // weaver. The skill runs as it always did; this only watches.
  on('prompt.submit', { text: /^\/loom\b.*\bweave\b/ }, async ($, e, next) => {
    await markWeave($, await $.session.cwd())
    return next(e)
  }).catch(skipOnFailure)

  on('command.run', { command: 'loom' }, async ($, e, next) => {
    if (isWeaveArgs(e.args)) await markWeave($, await $.session.cwd())
    return next(e)
  }).catch(skipOnFailure)

  on('command.run', { command: COMMAND }, async ($, e, next) => {
    if (e.args.trim() !== '') return next(e)
    const opened = await $.ui.open({ id: PANE, title: 'loom' })
    if (refresh === null) {
      refresh = $.clock.every(pollMs, () => {
        void survey($)
      })
    }
    $.clock.after(0, () => {
      void survey($)
    })
    return { text: opened.isPlaced ? 'loom pane opened.' : `loom pane waits: ${opened.reason}` }
  }).catch(skipOnFailure)

  on('ui.close', { id: PANE }, ($, e, next) => {
    if (refresh !== null) {
      refresh.cancel()
      refresh = null
    }
    return next(e)
  }).catch(skipOnFailure)

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text } = $.ui.resolve(e)
    const lines = paneLines(await read($, rows), await layoutOf($), e.props.bodyColumns)
    return (
      <Box flexDirection="column">
        {lines.length === 0 ? <Text dimColor>no worktrees surveyed yet</Text> : null}
        {lines.map(line => (
          <Text wrap="truncate-end">{line}</Text>
        ))}
      </Box>
    )
  }).catch(skipOnFailure)

  // Owner guards: an owner edits its own worktree and nothing else of the
  // repo. Paths outside the repo are none of the loom's business.
  on('tool.call', { tool: ['Write', 'Edit'] }, async ($, e, next) => {
    if (!isGuarding || (await read($, role)) !== 'owner') return next(e)
    const cls = classifyPath(e.file_path, await layoutOf($))
    if (cls === 'canonical' || cls === 'other') return { deny: denyReason(e.file_path, cls) }
    return next(e)
  }).catch(skipOnFailure)

  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    if (!isGuarding || (await read($, role)) !== 'owner') return next(e)
    const home = await $.env.get('HOME')
    const analysis = analyzeBash(e.command, { cwd: await $.session.cwd(), home: home ?? null })
    if (analysis.isUncertain) return next(e)
    const layout = await layoutOf($)
    for (const path of analysis.writes) {
      const cls = classifyPath(path, layout)
      if (cls === 'canonical' || cls === 'other') return { deny: denyReason(path, cls) }
    }
    return next(e)
  }).catch(skipOnFailure)

  // Any role: a force-delete or worktree removal that would lose commits or
  // uncommitted changes asks first. Cancel denies the call.
  on('tool.call', { tool: 'Bash', command: /\bgit\b[\s\S]*\b(branch|worktree)\b/ }, async ($, e, next) => {
    if (!isGuarding) return next(e)
    const canon = await read($, canonical)
    if (canon === null) return next(e)
    const home = await $.env.get('HOME')
    const analysis = analyzeBash(e.command, { cwd: await $.session.cwd(), home: home ?? null })
    if (analysis.isUncertain || analysis.removals.length === 0) return next(e)
    const listed = await git($, ['worktree', 'list', '--porcelain'], canon)
    const entries = listed === null ? [] : parseWorktreeList(listed)
    const defaultRef = parseDefaultRef(await git($, ['symbolic-ref', 'refs/remotes/origin/HEAD'], canon))
    for (const removal of analysis.removals) {
      if (await confirmRemoval($, removal, entries, canon, defaultRef)) continue
      const label = removal.kind === 'branch' ? removal.name : removal.path
      return { deny: `loom-pane: ${label} kept; the removal was cancelled at the prompt` }
    }
    return next(e)
  }).catch(skipOnFailure)
}
