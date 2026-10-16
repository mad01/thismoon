import { atom, read, update } from 'claude-code'
import type { AgentSpawnInput, EngineInterface, Register } from 'claude-code'

import type { FactoryBandProbe, FactoryBandWorker } from '../types'
import {
  INBOX_SOURCE,
  LEDGER_SOURCE,
  PROBE_BUDGET_MS,
  bandView,
  countParked,
  eventsUrl,
  gitFactsFrom,
  guardReason,
  idAt,
  isBeltDeny,
  isRefreshDue,
  isTaskMessage,
  isWorkerType,
  kindOf,
  parseDefaultRef,
  parseLedger,
  probeFrom,
  trailingDelay,
  upsertWorker,
  withKind,
  workerFrom,
} from '../lib/band'
import type { AgentLike, GitFacts, LedgerRead } from '../lib/band'

const workers = atom({ plugin: 'factory-band', key: 'workers' } as const, [] as FactoryBandWorker[])
const runStartedAt = atom({ plugin: 'factory-band', key: 'runStartedAt' } as const, null as number | null)
const guardDenies = atom({ plugin: 'factory-band', key: 'guardDenies' } as const, 0)
const probe = atom({ plugin: 'factory-band', key: 'probe' } as const, null as FactoryBandProbe | null)

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

// One git call in a worktree: its stdout, or null when it failed, exited
// non-zero or outran the budget (the child is killed at the same budget).
async function git($: EngineInterface, worktree: string, args: readonly string[]): Promise<string | null> {
  const ran = $.process
    .run(['git', '-C', worktree, ...args], { timeoutMs: PROBE_BUDGET_MS })
    .then(result => (result.exitCode === 0 ? result.stdout : null))
  return withinBudget($, ran)
}

// Whether the worktree is dirty and how far it is ahead of the default
// branch, or null when git could not say.
async function gitFactsOf($: EngineInterface, worktree: string): Promise<GitFacts | null> {
  const defaultRef = parseDefaultRef(await git($, worktree, ['symbolic-ref', 'refs/remotes/origin/HEAD']))
  const [status, count] = await Promise.all([
    git($, worktree, ['status', '--porcelain']),
    git($, worktree, ['rev-list', '--count', `${defaultRef}..HEAD`]),
  ])
  return gitFactsFrom(status, count)
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

// belt's ledger and inbox for this run, or nothing read when the events
// service is switched off, down or this session has no id.
async function readEvents($: EngineInterface, isReading: boolean): Promise<{ ledger: LedgerRead; parked: number | null }> {
  const sessionId = isReading ? await sessionIdOf($) : null
  if (sessionId === null) return { ledger: { facts: new Map(), denies: null }, parked: null }
  const scope = { sessionId, sinceId: idAt((await read($, runStartedAt)) ?? 0) }
  const [ledgerBody, inboxBody] = await Promise.all([
    fetchText($, eventsUrl(LEDGER_SOURCE, sessionId)),
    fetchText($, eventsUrl(INBOX_SOURCE, sessionId)),
  ])
  return { ledger: parseLedger(ledgerBody, scope), parked: countParked(inboxBody, scope) }
}

// Asks every source at once and stores what they said. Every source that
// fails reads as unknown, so a field turns to a dash and nothing errors.
async function readProbe($: EngineInterface, isReading: boolean): Promise<void> {
  const known = await read($, workers)
  if (known.length === 0) return
  const [agents, events, gits] = await Promise.all([
    listAgents($),
    readEvents($, isReading),
    Promise.all(known.map(worker => (worker.worktree === null ? null : withinBudget($, gitFactsOf($, worker.worktree))))),
  ])
  const found = probeFrom({ workers: known, agents, gits, ledger: events.ledger, parked: events.parked })
  await update($, probe, () => found)
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

// Re-reads the run, unless the throttle says no. Nothing here throws.
async function refreshBand($: EngineInterface, isReading: boolean): Promise<void> {
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

// Records a started worker and, at the first one, the run's start.
async function recordWorker($: EngineInterface, e: AgentSpawnInput, agentId: string, model: string): Promise<void> {
  const home = (await $.env.get('HOME')) ?? null
  const worker = workerFrom({ agentId, name: e.name, cwd: e.cwd, prompt: e.prompt, model, home })
  await update($, workers, list => upsertWorker(list, worker))
  if ((await read($, runStartedAt)) === null) {
    const now = await $.clock.now()
    await update($, runStartedAt, () => now)
  }
  $.ui.log(`factory-band: worker ${worker.name} in ${worker.worktree ?? 'no worktree'}`, { to: 'debug' })
}

// A worker's turn ended or it sent a message: keep the kind of any
// contract message in it, then re-read the band.
async function noteWorker($: EngineInterface, agentId: string, text: string, isReading: boolean): Promise<void> {
  const known = await read($, workers)
  if (!known.some(worker => worker.agentId === agentId)) return
  const kind = kindOf(text)
  if (kind !== null) await update($, workers, list => withKind(list, agentId, kind))
  await refreshBand($, isReading)
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
    if ((await read($, workers)).length > 0) await refreshBand($, isReading)
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
    if (e.agentId !== undefined) await noteWorker($, e.agentId, e.answer, isReading)
    else if ((await read($, workers)).length > 0) await refreshBand($, isReading)
    return completed
  }).catch(skipOnFailure)

  // A named worker reports over SendMessage. Observed after delivery,
  // passed on unchanged.
  on('session.send', async ($, e, next) => {
    const sent = await next(e)
    if (e.agentId !== undefined && sent.isDelivered) await noteWorker($, e.agentId, e.text, isReading)
    return sent
  }).catch(skipOnFailure)

  // belt's guard denials once the run started, read off the decision belt
  // handed back and returned exactly as it came.
  on('classic.PreToolUse', async ($, e, next) => {
    const decision = await next(e)
    if (isBeltDeny(decision) && (await read($, runStartedAt)) !== null) await update($, guardDenies, n => n + 1)
    return decision
  }).catch(skipOnFailure)

  // The band, from the first worker on; it yields to a survey and keeps
  // whatever the hooks beneath drew under it.
  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const below = await next(e)
    const known = await read($, workers)
    if (known.length === 0 || e.props.hasSurvey) return below
    const view = bandView({ workers: known, probe: await read($, probe), guardDenies: await read($, guardDenies) })
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
