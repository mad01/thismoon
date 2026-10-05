import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { LoomPaneRole } from '../types'
import {
  GIT_BUDGET_MS,
  analyzeBash,
  classifyPath,
  denyReason,
  detectLayout,
  isWeaveArgs,
  loomArgsOf,
  ownerClaimFrom,
  parseCount,
  parseDefaultRef,
  parseWorktreeList,
  removalQuestion,
  resolvePath,
  shouldAskBranchDeletion,
  shouldAskWorktreeRemoval,
  statusText,
  worktreeAt,
  worktreeOfBranch,
} from '../lib/loom'
import type { Layout, Removal, WorktreeEntry } from '../lib/loom'

const role = atom({ plugin: 'loom-pane', key: 'role' } as const, 'none' as LoomPaneRole)
const canonical = atom({ plugin: 'loom-pane', key: 'canonical' } as const, null as string | null)
const ownWorktree = atom({ plugin: 'loom-pane', key: 'ownWorktree' } as const, null as string | null)
const worktreeRoot = atom({ plugin: 'loom-pane', key: 'worktreeRoot' } as const, null as string | null)
const worktrees = atom({ plugin: 'loom-pane', key: 'worktrees' } as const, [] as string[])
const repo = atom({ plugin: 'loom-pane', key: 'repo' } as const, null as string | null)
const weaveSeen = atom({ plugin: 'loom-pane', key: 'weaveSeen' } as const, false)
const isInteractive = atom({ plugin: 'loom-pane', key: 'isInteractive' } as const, false)

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

// One git call under the budget: its stdout, or null when it failed, exited
// non-zero or outran the budget (the run rejects then). Never throws.
async function git($: EngineInterface, args: readonly string[], cwd: string): Promise<string | null> {
  try {
    const ran = await $.process.run(['git', ...args], { cwd, timeoutMs: GIT_BUDGET_MS })
    return ran.exitCode === 0 ? ran.stdout : null
  } catch {
    return null
  }
}

async function layoutOf($: EngineInterface): Promise<Layout> {
  return {
    canonical: await read($, canonical),
    ownWorktree: await read($, ownWorktree),
    worktreeRoot: await read($, worktreeRoot),
    worktrees: await read($, worktrees),
  }
}

async function showStatus($: EngineInterface): Promise<void> {
  $.ui.status(statusText(await read($, role), await layoutOf($)))
}

async function listWorktrees($: EngineInterface, canon: string): Promise<WorktreeEntry[] | null> {
  const listed = await git($, ['worktree', 'list', '--porcelain'], canon)
  return listed === null ? null : parseWorktreeList(listed).filter(entry => !entry.isBare)
}

// Re-reads the worktree list and stores the paths the guards judge by. Runs
// when the role is detected and when the session claims a worktree, never
// on a timer. A failed list keeps the paths already known.
async function refreshWorktrees($: EngineInterface): Promise<void> {
  const canon = await read($, canonical)
  if (canon === null) {
    await update($, worktrees, () => [])
    return
  }
  const entries = await listWorktrees($, canon)
  if (entries === null) {
    $.ui.log('loom-pane: worktree list failed', { to: 'debug' })
    return
  }
  await update($, worktrees, () => entries.map(entry => entry.path))
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
  await refreshWorktrees($)
  await showStatus($)
  $.ui.log(
    found.canonical === null
      ? `loom-pane: no git repo at ${cwd}`
      : `loom-pane: repo ${found.repo} canonical ${found.canonical} role ${found.role}`,
    { to: 'debug' },
  )
}

// A Bash command that enters a worktree of the loom layout, by `cd` or by
// `git worktree add`, makes this session its owner; a weaver stays a weaver.
async function claimOwner($: EngineInterface, claims: readonly string[]): Promise<void> {
  if (await read($, weaveSeen)) return
  const claimed = ownerClaimFrom(claims, await read($, worktreeRoot))
  if (claimed === null || claimed === (await read($, ownWorktree))) return
  await update($, ownWorktree, () => claimed)
  await update($, role, () => 'owner')
  await refreshWorktrees($)
  await showStatus($)
  $.ui.log(`loom-pane: owner of ${claimed}`, { to: 'debug' })
}

async function markWeave($: EngineInterface, cwd: string): Promise<void> {
  await update($, weaveSeen, () => true)
  await detect($, cwd)
}

// Asks before a removal that would lose work. A dismissed dialog keeps the
// target in an interactive session; headless (`-p`), nobody can be asked and
// the removal goes ahead.
async function askToRemove($: EngineInterface, label: string, ahead: number | null, isDirty: boolean): Promise<boolean> {
  try {
    const answer = await $.ui.ask(removalQuestion(label, ahead, isDirty), ['Proceed', 'Cancel'])
    return answer === 'Proceed'
  } catch (err) {
    const interactive = await read($, isInteractive)
    $.ui.log(`loom-pane: removal dialog closed (${err instanceof Error ? err.message : String(err)}); ${interactive ? 'kept' : 'headless, allowed'}`, { to: 'debug' })
    return !interactive
  }
}

// True when the removal may go ahead: nothing would be lost, or the person
// chose Proceed. A git call that fails reads as nothing at risk.
async function confirmRemoval(
  $: EngineInterface,
  removal: Removal,
  entries: readonly WorktreeEntry[],
  defaultRef: string,
): Promise<boolean> {
  if (removal.kind === 'worktree') {
    const entry = worktreeAt(entries, removal.path)
    const status = removal.isForce ? await git($, ['status', '--porcelain'], removal.path) : null
    const isDirty = status !== null && status.trim() !== ''
    const isDetached = entry?.isDetached === true
    const ahead = isDetached ? parseCount(await git($, ['rev-list', '--count', `${defaultRef}..HEAD`], removal.path)) : null
    if (!shouldAskWorktreeRemoval({ isForce: removal.isForce, isDirty, isDetached, ahead })) return true
    return askToRemove($, removal.path, ahead, isDirty)
  }
  const linked = worktreeOfBranch(entries, removal.name)
  const status = linked === null ? null : await git($, ['status', '--porcelain'], linked.path)
  const isWorktreeDirty = status !== null && status.trim() !== ''
  const ahead = parseCount(await git($, ['rev-list', '--count', `${defaultRef}..${removal.name}`], removal.location))
  const remoteAhead = parseCount(
    await git($, ['rev-list', '--count', `origin/${removal.name}..${removal.name}`], removal.location),
  )
  if (!shouldAskBranchDeletion({ ahead, remoteAhead, isWorktreeDirty })) return true
  return askToRemove($, removal.name, ahead, isWorktreeDirty)
}

export const register: Register = (on, options) => {
  const isGuarding = options.guards !== false

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('loom-pane: loaded', { to: 'debug' })
    await update($, isInteractive, () => e.isInteractive)
    await detect($, e.cwd)
    return started
  }).catch(skipOnFailure)

  on('classic.SessionStart', { source: ['resume', 'clear'] }, async ($, e, next) => {
    await detect($, e.cwd)
    return next(e)
  }).catch(skipOnFailure)

  // `/loom weave`, typed at the prompt or run as the skill, makes a session
  // of this repo the weaver. The skill runs as it always did; this only
  // watches.
  on('prompt.submit', { text: /^\/loom\b/ }, async ($, e, next) => {
    const args = loomArgsOf(e.text)
    if (args !== null && isWeaveArgs(args)) await markWeave($, await $.session.cwd())
    return next(e)
  }).catch(skipOnFailure)

  on('command.run', { command: 'loom' }, async ($, e, next) => {
    if (isWeaveArgs(e.args)) await markWeave($, await $.session.cwd())
    return next(e)
  }).catch(skipOnFailure)

  on('tool.call', { tool: 'Skill', skill: 'loom' }, async ($, e, next) => {
    if (e.agentId === undefined && isWeaveArgs(e.args ?? '')) await markWeave($, await $.session.cwd())
    return next(e)
  }).catch(skipOnFailure)

  // Owner guards, main loop only: an owner edits its own worktree and nothing
  // else of the repo. Paths outside the repo are none of the loom's business,
  // and a subagent in an isolation worktree is its own owner.
  on('tool.call', { tool: ['Write', 'Edit'] }, async ($, e, next) => {
    if (!isGuarding || e.agentId !== undefined || (await read($, role)) !== 'owner') return next(e)
    const home = await $.env.get('HOME')
    const path = resolvePath(e.file_path, await $.session.cwd(), home ?? null)
    if (path === null) return next(e)
    const cls = classifyPath(path, await layoutOf($))
    if (cls === 'canonical' || cls === 'other') return { deny: denyReason(path, cls) }
    return next(e)
  }).catch(skipOnFailure)

  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    if (e.agentId !== undefined) return next(e)
    const home = await $.env.get('HOME')
    const analysis = analyzeBash(e.command, { cwd: await $.session.cwd(), home: home ?? null })
    if (analysis.isUncertain) return next(e)
    await claimOwner($, analysis.claims)
    if (!isGuarding || (await read($, role)) !== 'owner') return next(e)
    const layout = await layoutOf($)
    for (const path of analysis.writes) {
      const cls = classifyPath(path, layout)
      if (cls === 'canonical' || cls === 'other') return { deny: denyReason(path, cls) }
    }
    return next(e)
  }).catch(skipOnFailure)

  // Any role, main loop only: a force-delete or worktree removal that would
  // lose commits or uncommitted changes asks first. Cancel denies the call.
  on('tool.call', { tool: 'Bash', command: /\bgit\b[\s\S]*\b(branch|worktree)\b/ }, async ($, e, next) => {
    if (!isGuarding || e.agentId !== undefined) return next(e)
    const canon = await read($, canonical)
    if (canon === null) return next(e)
    const home = await $.env.get('HOME')
    const analysis = analyzeBash(e.command, { cwd: await $.session.cwd(), home: home ?? null })
    if (analysis.isUncertain || analysis.removals.length === 0) return next(e)
    const layout = await layoutOf($)
    const entries = (await listWorktrees($, canon)) ?? []
    const defaultRef = parseDefaultRef(await git($, ['symbolic-ref', 'refs/remotes/origin/HEAD'], canon))
    for (const removal of analysis.removals) {
      // A branch of some other repo is not this loom's to judge.
      if (removal.kind === 'branch' && classifyPath(removal.location, layout) === 'outside') continue
      if (await confirmRemoval($, removal, entries, defaultRef)) continue
      const label = removal.kind === 'branch' ? removal.name : removal.path
      return { deny: `loom-pane: ${label} kept; the removal was not confirmed` }
    }
    return next(e)
  }).catch(skipOnFailure)
}
