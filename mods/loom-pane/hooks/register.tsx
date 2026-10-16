import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { LoomPaneMember, LoomPaneRole } from '../types'
import {
  AGENTS_ARGV,
  GIT_BUDGET_MS,
  LEDGER_URL,
  PROBE_BUDGET_MS,
  analyzeBash,
  classifyPath,
  composeStatus,
  denyReason,
  detectLayout,
  isIntegratedTree,
  isLive,
  isRefreshDue,
  isWeaveArgs,
  loomArgsOf,
  membersFrom,
  mergeTreeArgs,
  ownerClaimFrom,
  parseCount,
  parseDefaultRef,
  parseLedger,
  parseSessions,
  parseWorktreeList,
  removalQuestion,
  resolvePath,
  shouldAskBranchDeletion,
  shouldAskWorktreeRemoval,
  statusText,
  trailingDelay,
  worktreeAt,
  worktreeOfBranch,
} from '../lib/loom'
import type { AgentLike, Layout, LedgerFacts, Removal, WorktreeEntry } from '../lib/loom'

const role = atom({ plugin: 'loom-pane', key: 'role' } as const, 'none' as LoomPaneRole)
const canonical = atom({ plugin: 'loom-pane', key: 'canonical' } as const, null as string | null)
const ownWorktree = atom({ plugin: 'loom-pane', key: 'ownWorktree' } as const, null as string | null)
const worktreeRoot = atom({ plugin: 'loom-pane', key: 'worktreeRoot' } as const, null as string | null)
const worktrees = atom({ plugin: 'loom-pane', key: 'worktrees' } as const, [] as string[])
const repo = atom({ plugin: 'loom-pane', key: 'repo' } as const, null as string | null)
const weaveSeen = atom({ plugin: 'loom-pane', key: 'weaveSeen' } as const, false)
const isInteractive = atom({ plugin: 'loom-pane', key: 'isInteractive' } as const, false)
const members = atom({ plugin: 'loom-pane', key: 'members' } as const, [] as LoomPaneMember[])

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
  $.ui.status(composeStatus(statusText(await read($, role), await layoutOf($)), await read($, members)))
}

// This session's agents, or none when the engine cannot list them.
async function listAgents($: EngineInterface): Promise<AgentLike[]> {
  try {
    return await $.agent.list()
  } catch {
    return []
  }
}

async function sessionId($: EngineInterface): Promise<string | null> {
  try {
    return await $.session.id()
  } catch {
    return null
  }
}

// What `work` answers, or null when it fails or the 400 ms probe budget
// runs out first.
async function withinBudget<T>($: EngineInterface, work: Promise<T | null>): Promise<T | null> {
  const timedOut = $.clock
    .sleep(PROBE_BUDGET_MS)
    .then(() => null)
    .catch(() => null)
  return Promise.race([work.catch(() => null), timedOut])
}

// The engine's session list, or null when `claude` is missing, fails or
// outruns the budget (the child is killed at the same budget).
async function listSessions($: EngineInterface): Promise<string | null> {
  const ran = $.process
    .run(AGENTS_ARGV, { timeoutMs: PROBE_BUDGET_MS })
    .then(result => (result.exitCode === 0 ? result.stdout : null))
  return withinBudget($, ran)
}

// belt's subagent-start entries from the events service, or null when it
// is down, answers an error or outruns the budget.
async function fetchLedger($: EngineInterface): Promise<string | null> {
  const fetched = $.http.fetch(LEDGER_URL).then(response => (response.ok ? response.text : null))
  return withinBudget($, fetched)
}

// The member refresh throttle: one refresh at a time, and none within
// MEMBERS_REFRESH_GAP_MS of the last one's start, so a chatty session never
// queues probes. A refresh the throttle skips marks the line stale, and one
// trailing refresh at the gap boundary catches it up; further skips fold
// into that one. Module variables, so a hot reload starts it over (and
// cancels the timer), which costs one extra refresh at most.
let isRefreshing = false
let isStale = false
let isTrailingScheduled = false
let lastRefreshAt: number | null = null

const errorText = (err: unknown): string => (err instanceof Error ? err.message : String(err))

// Re-reads who works beside this session, unless the throttle says no.
// Nothing here throws.
async function refreshMembers($: EngineInterface): Promise<void> {
  if (isRefreshing) {
    isStale = true
    return
  }
  isRefreshing = true
  let now: number | null = null
  try {
    now = await $.clock.now()
    if (!isRefreshDue(now, lastRefreshAt)) {
      isStale = true
      return
    }
    lastRefreshAt = now
    isStale = false
    await readMembers($)
  } catch (err) {
    $.ui.log(`loom-pane: members not refreshed (${errorText(err)})`, { to: 'debug' })
  } finally {
    isRefreshing = false
    if (isStale && now !== null) scheduleTrailing($, now)
  }
}

// Schedules the one trailing refresh at the gap boundary, unless one is
// already waiting.
function scheduleTrailing($: EngineInterface, now: number): void {
  if (isTrailingScheduled) return
  isTrailingScheduled = true
  try {
    $.clock.after(trailingDelay(now, lastRefreshAt), () => {
      isTrailingScheduled = false
      void refreshMembers($)
    })
  } catch (err) {
    isTrailingScheduled = false
    $.ui.log(`loom-pane: trailing refresh not scheduled (${errorText(err)})`, { to: 'debug' })
  }
}

// Asks every source, stores the members and pins the status line again.
// Every source that fails reads as empty, so the line degrades to the role
// alone.
async function readMembers($: EngineInterface): Promise<void> {
  const layout = await layoutOf($)
  const agents = layout.canonical === null ? [] : await listAgents($)
  const hasLinked = layout.worktrees.some(path => path !== layout.canonical)
  const sessions = hasLinked ? parseSessions(await listSessions($)) : []
  const ledger: Map<string, LedgerFacts> = agents.some(isLive) ? parseLedger(await fetchLedger($)) : new Map()
  const found = membersFrom({ agents, sessions, ledger, layout, selfSessionId: await sessionId($) })
  await update($, members, () => found)
  await showStatus($)
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
  // One round, all at once: run one after another behind the worktree
  // listing and the default ref, these calls could outlast the engine's
  // 10 s hook limit, and a skipped hook lets the delete through.
  const [status, aheadOut, remoteAheadOut, isIntegrated] = await Promise.all([
    linked === null ? null : git($, ['status', '--porcelain'], linked.path),
    git($, ['rev-list', '--count', `${defaultRef}..${removal.name}`], removal.location),
    git($, ['rev-list', '--count', `origin/${removal.name}..${removal.name}`], removal.location),
    isIntegratedBranch($, removal.name, removal.location, defaultRef),
  ])
  const isWorktreeDirty = status !== null && status.trim() !== ''
  const ahead = parseCount(aheadOut)
  const remoteAhead = parseCount(remoteAheadOut)
  if (!shouldAskBranchDeletion({ ahead, remoteAhead, isIntegrated, isWorktreeDirty })) return true
  return askToRemove($, removal.name, ahead, isWorktreeDirty)
}

// True when the default branch already holds the branch's changes: a merge
// would write the default branch's own tree. Unlike the counts beside it, a
// failure here (git before 2.38, a conflict, a call past its budget) keeps
// the question.
async function isIntegratedBranch($: EngineInterface, branch: string, cwd: string, defaultRef: string): Promise<boolean> {
  const [merged, tree] = await Promise.all([
    git($, mergeTreeArgs(defaultRef, branch), cwd),
    git($, ['rev-parse', `${defaultRef}^{tree}`], cwd),
  ])
  return isIntegratedTree(merged, tree)
}

export const register: Register = (on, options) => {
  const isGuarding = options.guards !== false
  const isListing = options.members !== false

  // A session start (a hot reload after `members` was switched off included)
  // drops members a past refresh left in the state, so the line never shows
  // them with the listing off.
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('loom-pane: loaded', { to: 'debug' })
    await update($, isInteractive, () => e.isInteractive)
    if (!isListing) await update($, members, () => [])
    await detect($, e.cwd)
    if (isListing) await refreshMembers($)
    return started
  }).catch(skipOnFailure)

  on('classic.SessionStart', { source: ['resume', 'clear'] }, async ($, e, next) => {
    if (!isListing) await update($, members, () => [])
    await detect($, e.cwd)
    if (isListing) await refreshMembers($)
    return next(e)
  }).catch(skipOnFailure)

  // The member line follows the agents: re-read once a spawn has started
  // its agent, and when a main-loop turn ends (a member reported, stopped
  // or went idle). Observers only; the results pass on unchanged.
  on('agent.spawn', async ($, e, next) => {
    const spawned = await next(e)
    if (isListing) await refreshMembers($)
    return spawned
  }).catch(skipOnFailure)

  on('turn.complete', async ($, e, next) => {
    const completed = await next(e)
    if (isListing && e.agentId === undefined) await refreshMembers($)
    return completed
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
    if (isGuarding && (await read($, role)) === 'owner') {
      const layout = await layoutOf($)
      for (const path of analysis.writes) {
        const cls = classifyPath(path, layout)
        if (cls === 'canonical' || cls === 'other') return { deny: denyReason(path, cls) }
      }
    }
    // A claim counts once the command ran: a call this guard, belt or the
    // person denied leaves the role where it was. A failure from here on is
    // logged, never handed to .catch, which would run the command again.
    const ran = await next(e)
    if (ran.deny !== undefined || ran.isError === true) return ran
    try {
      await claimOwner($, analysis.claims)
    } catch (err) {
      $.ui.log(`loom-pane: claim not recorded (${errorText(err)})`, { to: 'debug' })
    }
    return ran
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
