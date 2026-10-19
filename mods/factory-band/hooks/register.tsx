import { atom, read, update } from 'claude-code'
import type { AgentSpawnInput, EngineInterface, Register } from 'claude-code'

import {
  DEFAULT_BRANCH_ARGS,
  GIT_ENV,
  INBOX_SOURCE,
  LEDGER_SOURCE,
  NO_RUN,
  PROBE_BUDGET_MS,
  STATUS_ARGS,
  aheadArgs,
  bandView,
  countInbox,
  eventsUrl,
  gitFactsFrom,
  guardReason,
  idAt,
  integratedOf,
  isBeltRefusal,
  isReadyCandidate,
  isRefreshDue,
  isTaskMessage,
  isWorkerType,
  isWorktreeGone,
  kindOf,
  parseDefaultBranch,
  parseLedger,
  probeFrom,
  settleRun,
  stdoutOf,
  trailingDelay,
  withGuardDeny,
  withKind,
  withWorker,
  workerFrom,
} from '../lib/band'
import type { AgentLike, GitRun, LedgerRead, Run, Worker, WorktreeRead } from '../lib/band'

// The whole run in one atom: every change below is one update, so a spawn
// and the refresh that closes the run never see each other half done.
const run = atom({ plugin: 'factory-band', key: 'run' } as const, NO_RUN)

type Logger = { ui: { log: (text: string, options: { to: 'debug' }) => void } }
type Failed<E, R> = ((e: E) => R) & { event: string; error: { kind: string; message?: string } }

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. The band never
// blocks a turn, and the guard lets a spawn through when it fails.
const skipOnFailure = <E, R>($: Logger, e: E, next: Failed<E, R>): R => {
  const why = next.error.message === undefined ? next.error.kind : `${next.error.kind}: ${next.error.message}`
  $.ui.log(`factory-band: ${next.event} skipped (${why})`, { to: 'debug' })
  return next(e)
}

const errorText = (err: unknown): string => (err instanceof Error ? err.message : String(err))

// What `work` answers, or null when it fails or the 400 ms probe budget
// runs out first.
async function withinBudget<T>($: EngineInterface, work: Promise<T | null>): Promise<T | null> {
  const timedOut = $.clock
    .sleep(PROBE_BUDGET_MS)
    .then(() => null)
    .catch(() => null)
  return Promise.race([work.catch(() => null), timedOut])
}

// One git call in a worktree, with stdin when given, or null when it
// outran the budget (the child is killed at the same budget, which rejects
// the call).
async function git($: EngineInterface, worktree: string, args: readonly string[], stdin?: string): Promise<GitRun | null> {
  const init = stdin === undefined ? { timeoutMs: PROBE_BUDGET_MS, env: GIT_ENV } : { timeoutMs: PROBE_BUDGET_MS, env: GIT_ENV, stdin }
  const ran = $.process.run(['git', '-C', worktree, ...args], init)
  return withinBudget($, ran)
}

// Whether the worktree is dirty, how far it is ahead of the default branch
// and, for a ready worker's clean tree only, whether the default branch
// already holds its changes: lib's integratedOf, the tip merge and then the
// first-parent walk, unknown when that walk runs past its limit. `removed`
// when the first call finds the worktree gone; null when git could not
// say. A merge-tree that fails (git before 2.38, or 2.40 for the walk)
// leaves it not integrated.
async function gitFactsOf($: EngineInterface, worktree: string, lastKind: string | null): Promise<WorktreeRead> {
  const head = await git($, worktree, DEFAULT_BRANCH_ARGS)
  if (isWorktreeGone(head)) return 'removed'
  const branch = parseDefaultBranch(stdoutOf(head))
  const [status, count] = await Promise.all([git($, worktree, STATUS_ARGS), git($, worktree, aheadArgs(branch.ref))])
  const facts = gitFactsFrom(stdoutOf(status), stdoutOf(count))
  if (facts === null || branch.tree === null || !isReadyCandidate(facts, lastKind)) return facts
  return { ...facts, isIntegrated: await integratedOf((args, stdin) => git($, worktree, args, stdin), branch) }
}

// One events query, or null when the service is down, answers an error or
// outruns the budget.
async function fetchText($: EngineInterface, url: string): Promise<string | null> {
  const fetched = $.http.fetch(url).then(response => (response.ok ? response.text : null))
  return withinBudget($, fetched)
}

// This session's agents, or null when the engine cannot list them.
async function listAgents($: EngineInterface): Promise<AgentLike[] | null> {
  try {
    return await $.agent.list()
  } catch {
    return null
  }
}

async function sessionIdOf($: EngineInterface): Promise<string | null> {
  try {
    return await $.session.id()
  } catch {
    return null
  }
}

type EventsRead = { ledger: LedgerRead; inbox: number | null }

// belt's ledger and inbox for this run, or nothing read when the events
// service is switched off, down or this session has no id. Denials count
// for every worker the run spawned, so a dropped worker's stay counted.
async function readEvents($: EngineInterface, isReading: boolean, current: Run): Promise<EventsRead> {
  const sessionId = isReading ? await sessionIdOf($) : null
  if (sessionId === null) return { ledger: { facts: new Map(), denies: null }, inbox: null }
  const workerIds = new Set(current.workerIds)
  const sinceId = idAt(current.startedAt ?? 0)
  const [ledgerBody, inboxBody] = await Promise.all([
    fetchText($, eventsUrl(LEDGER_SOURCE, sessionId)),
    fetchText($, eventsUrl(INBOX_SOURCE, sessionId)),
  ])
  return { ledger: parseLedger(ledgerBody, { sessionId, workerIds }), inbox: countInbox(inboxBody, { sessionId, sinceId }) }
}

// One worker's worktree as git reports it, within the probe budget.
function readWorktree($: EngineInterface, worker: Worker): Promise<WorktreeRead> {
  return worker.worktree === null ? Promise.resolve(null) : withinBudget($, gitFactsOf($, worker.worktree, worker.lastKind))
}

// Asks every source at once, then stores what they said and drops the
// workers that are done in one update; the last one dropped closes the
// run. Every source that fails reads as unknown, so a field turns to a
// dash and nothing errors.
async function readProbe($: EngineInterface, isReading: boolean): Promise<void> {
  const current = await read($, run)
  if (current.workers.length === 0) return
  const [agents, events, gits] = await Promise.all([
    listAgents($),
    readEvents($, isReading, current),
    Promise.all(current.workers.map(worker => readWorktree($, worker))),
  ])
  const found = probeFrom({ workers: current.workers, agents, gits, ledger: events.ledger, inbox: events.inbox })
  const settled = await update($, run, latest => settleRun(latest, found, agents))
  if (settled.workers.length === 0) $.ui.log('factory-band: every worker done; run closed', { to: 'debug' })
}

// The refresh throttle: one refresh at a time, and none within
// REFRESH_GAP_MS of the last one's start, so a chatty run never queues
// probes. A refresh the throttle skips marks the band stale, and one
// trailing refresh at the gap boundary catches it up. Module variables, so
// a hot reload starts it over, which costs one extra refresh at most.
let isRefreshing = false
let isStale = false
let isTrailingScheduled = false
let lastRefreshAt: number | null = null

// Re-reads the run, unless the throttle says no. A recheck also leaves one
// trailing refresh behind, for a read that may come too early (an agent
// still listed as running at its own last turn end). Nothing here throws.
async function refreshBand($: EngineInterface, isReading: boolean, isRecheck = false): Promise<void> {
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
    isStale = isRecheck
    await readProbe($, isReading)
  } catch (err) {
    $.ui.log(`factory-band: band not refreshed (${errorText(err)})`, { to: 'debug' })
  } finally {
    isRefreshing = false
    if (isStale && now !== null) scheduleTrailing($, now, isReading)
  }
}

// Schedules the one trailing refresh at the gap boundary, unless one is
// already waiting.
function scheduleTrailing($: EngineInterface, now: number, isReading: boolean): void {
  if (isTrailingScheduled) return
  isTrailingScheduled = true
  try {
    $.clock.after(trailingDelay(now, lastRefreshAt), () => {
      isTrailingScheduled = false
      void refreshBand($, isReading)
    })
  } catch (err) {
    isTrailingScheduled = false
    $.ui.log(`factory-band: trailing refresh not scheduled (${errorText(err)})`, { to: 'debug' })
  }
}

// Records a started worker; the first one starts the run.
async function recordWorker($: EngineInterface, e: AgentSpawnInput, agentId: string, model: string): Promise<void> {
  const home = (await $.env.get('HOME')) ?? null
  const worker = workerFrom({ agentId, name: e.name, cwd: e.cwd, prompt: e.prompt, model, home })
  const now = await $.clock.now()
  await update($, run, current => withWorker(current, worker, now))
  $.ui.log(`factory-band: worker ${worker.name} in ${worker.worktree ?? 'no worktree'}`, { to: 'debug' })
}

const isWorker = (list: readonly Worker[], agentId: string): boolean => list.some(worker => worker.agentId === agentId)

// A worker's turn ended or it sent a message: keep the kind of any
// contract message in it, then re-read the band. After a turn end the
// band reads again at the gap boundary, once the agent list has caught up.
async function noteWorker($: EngineInterface, agentId: string, text: string, isReading: boolean, isTurnEnd: boolean): Promise<void> {
  if (!isWorker((await read($, run)).workers, agentId)) return
  const kind = kindOf(text)
  if (kind !== null) await update($, run, current => ({ ...current, workers: withKind(current.workers, agentId, kind) }))
  await refreshBand($, isReading, isTurnEnd)
}

// The one human action: `/loom weave`, queued as if typed, run once the
// session is idle. A command that cannot start says why in a toast,
// because the person pressed for it.
async function weave($: EngineInterface): Promise<void> {
  try {
    await $.command.run({ command: 'loom', args: 'weave' })
  } catch (err) {
    $.ui.toast(`factory-band: /loom weave did not start (${errorText(err)})`)
  }
}

export const register: Register = (on, options) => {
  const isReading = options.events !== false
  const isGuarding = options.contractGuard === true

  // A hot reload keeps the run in $.state; read it again so the band is
  // current from the first draw.
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('factory-band: loaded', { to: 'debug' })
    if ((await read($, run)).workers.length > 0) await refreshBand($, isReading)
    return started
  }).catch(skipOnFailure)

  // A factory worker spawn: the convenience guard (off by default) judges
  // the task header first; then the started worker is recorded with the
  // model the spawn resolved. A resumed worker is already known.
  on('agent.spawn', async ($, e, next) => {
    if (!isWorkerType(e.subagentType)) return next(e)
    if (isGuarding && e.resume === undefined && !isTaskMessage(e.prompt)) return { deny: guardReason(e.subagentType) }
    const spawned = await next(e)
    if (spawned.deny !== undefined || spawned.agentId === undefined) return spawned
    try {
      if (e.resume === undefined) await recordWorker($, e, spawned.agentId, spawned.model)
      await refreshBand($, isReading)
    } catch (err) {
      $.ui.log(`factory-band: worker not recorded (${errorText(err)})`, { to: 'debug' })
    }
    return spawned
  }).catch(skipOnFailure)

  // A worker's turn end carries its final answer, which for an unnamed
  // worker is its contract message; a main-loop turn end re-reads the band.
  on('turn.complete', async ($, e, next) => {
    const completed = await next(e)
    if (e.agentId !== undefined) await noteWorker($, e.agentId, e.answer, isReading, true)
    else if ((await read($, run)).workers.length > 0) await refreshBand($, isReading)
    return completed
  }).catch(skipOnFailure)

  // A named worker reports over SendMessage. Observed after delivery,
  // passed on unchanged.
  on('session.send', async ($, e, next) => {
    const sent = await next(e)
    if (e.agentId !== undefined && sent.isDelivered) await noteWorker($, e.agentId, e.text, isReading, false)
    return sent
  }).catch(skipOnFailure)

  // belt's guard denials of the workers' own tool calls, read off the
  // result and returned exactly as it came. The orchestrator's denials are
  // session-band's to count.
  on('tool.call', async ($, e, next) => {
    const ran = await next(e)
    const agentId = e.agentId
    if (agentId === undefined || !isBeltRefusal(ran)) return ran
    try {
      await update($, run, current => withGuardDeny(current, agentId))
    } catch (err) {
      $.ui.log(`factory-band: deny not counted (${errorText(err)})`, { to: 'debug' })
    }
    return ran
  }).catch(skipOnFailure)

  // The band, from the first worker on; it yields to a survey and keeps
  // whatever the hooks beneath drew under it.
  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const below = await next(e)
    const current = await read($, run)
    if (current.workers.length === 0 || e.props.hasSurvey) return below
    const view = bandView(current)
    const { Box, Button, Text } = $.ui.resolve(e)
    return (
      <Box flexDirection="column">
        <Box flexDirection="row" gap={1}>
          <Text wrap="truncate-end">{view.summary}</Text>
          <Button key="weave" label="weave now" hotkey="w" variant={view.isAnyReady ? 'primary' : 'secondary'} onPress={() => weave($)} />
        </Box>
        {view.rows.map(row => (
          <Text wrap="truncate-end">{row}</Text>
        ))}
        {below}
      </Box>
    )
  }).catch(skipOnFailure)
}
