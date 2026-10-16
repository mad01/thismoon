// Pure helpers behind the factory-band mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

/** What the band shows for a field no source recorded. */
export const UNKNOWN = '-'

/** How long each probe (git per worker, each events query) may take. */
export const PROBE_BUDGET_MS = 400

/** The shortest gap between the starts of two refreshes. */
export const REFRESH_GAP_MS = 5_000

/** The local events service's query route. */
export const EVENTS_URL = 'http://localhost:7430/api/events'

/** The events source belt records lifecycle entries under. */
export const LEDGER_SOURCE = 'belt-ledger'

/** The events source items addressed to a session arrive under. */
export const INBOX_SOURCE = 'belt-inbox'

/** The ref the ahead count compares against when origin names no HEAD. */
export const DEFAULT_BRANCH = 'main'

// One page per query is plenty for one run: a factory of a few workers
// leaves tens of ledger entries, not hundreds.
const LEDGER_LIMIT = 200
const INBOX_LIMIT = 100

// The agent definitions the factory spawns its workers from, optionally
// under a plugin's `<plugin>:` prefix.
const WORKER_TYPE = /(?:^|:)factory-worker-(low|medium|high)$/

// The eight header lines of a factory message, in contract order.
const HEADER_KEYS = ['kind', 'unit', 'worktree', 'branch', 'model', 'effort', 'reply_to', 'needs_reply'] as const

// A header line, `key: value`; the value runs to the end of the line.
const HEADER_LINE = /^([a-z_]+):[ \t]*(.*?)[ \t]*$/

// The `kind:` line of a contract message, wherever the message starts: an
// answer handed back to its spawner can be led by the engine's warning.
const KIND_LINE = /^kind:[ \t]*(task|status|question|answer|ready|blocked)[ \t]*$/m

// Kinds after which the worker waits for the orchestrator.
const NEEDS_ANSWER = new Set(['blocked', 'question'])

// Agent statuses of a loop at work or about to be.
const LIVE = new Set(['pending', 'running', 'waiting'])

// Every belt denial reason carries `belt[<guard>]: `, possibly inside the
// engine's own wrapping, so the marker is searched for anywhere.
const BELT_REASON = /belt\[[^\]]+\]:/

export type Header = Record<(typeof HEADER_KEYS)[number], string>

/** One factory worker as its spawn recorded it. */
export type Worker = {
  agentId: string
  name: string
  /** The claimed worktree, absolute; null when neither the spawn nor the task named one. */
  worktree: string | null
  /** What the spawn result said the worker runs on. */
  model: string | null
  /** The kind of the last contract message the worker sent. */
  lastKind: string | null
}

/** One agent of this session, as `$.agent.list()` returns it. */
export type AgentLike = { id: string; status: string }

export type Liveness = 'live' | 'idle' | 'gone' | 'unknown'

/** What git says about a worker's worktree. */
export type GitFacts = { isDirty: boolean; ahead: number }

/** What belt's ledger recorded when a worker started. */
export type LedgerFacts = { model: string | null; effort: string | null }

/** The ledger read for one run: start facts per agent id and this session's denials. */
export type LedgerRead = { facts: Map<string, LedgerFacts>; denies: number | null }

/** What one refresh read for one worker. */
export type WorkerFacts = { state: string; model: string | null; effort: string | null }

/** What one refresh read for the run; null counts are sources that did not answer. */
export type Probe = { facts: Record<string, WorkerFacts>; ledgerDenies: number | null; parked: number | null }

/** What the band draws. */
export type BandView = { summary: string; rows: string[]; isAnyReady: boolean }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function textOf(value: unknown): string | null {
  return typeof value === 'string' && value !== '' ? value : null
}

// The events service's answer as an array, or null when it is not one.
function parseArray(body: string | null): unknown[] | null {
  if (body === null) return null
  try {
    const parsed: unknown = JSON.parse(body)
    return Array.isArray(parsed) ? parsed : null
  } catch {
    return null
  }
}

/** True for an agent type the factory spawns its workers from. */
export function isWorkerType(type: string): boolean {
  return WORKER_TYPE.test(type)
}

/** The eight contract header lines at the top of a message, or null when any is missing or out of order. */
export function parseHeader(text: string): Header | null {
  const lines = text.trimStart().split(/\r?\n/)
  if (lines.length < HEADER_KEYS.length) return null
  const header: Partial<Header> = {}
  for (const [i, key] of HEADER_KEYS.entries()) {
    const match = HEADER_LINE.exec(lines[i] ?? '')
    if (match === null || match[1] !== key || match[2] === '') return null
    header[key] = match[2]
  }
  return header as Header
}

/** True for a spawn prompt that opens with a `task` header. */
export function isTaskMessage(prompt: string): boolean {
  return parseHeader(prompt)?.kind === 'task'
}

/** The convenience guard's deny reason. */
export function guardReason(type: string): string {
  return `factory-band: the ${type} prompt lacks the eight-line task header (kind, unit, worktree, branch, model, effort, reply_to, needs_reply); see the factory skill's contract.md`
}

/** The kind a contract message names, or null for any other text. */
export function kindOf(text: string): string | null {
  return KIND_LINE.exec(text)?.[1] ?? null
}

/** `~/x` under `home`; an absolute path as given; null for a relative path or an unknown home. */
export function expandHome(path: string, home: string | null): string | null {
  if (path === '~' || path.startsWith('~/')) return home === null ? null : `${home}${path.slice(1)}`
  return path.startsWith('/') ? path : null
}

/** The worker a spawn started: the worktree from the spawn's cwd, else the task header. */
export function workerFrom(input: {
  agentId: string
  name?: string
  cwd?: string
  prompt: string
  model: string
  home: string | null
}): Worker {
  const header = parseHeader(input.prompt)
  const named = input.cwd ?? header?.worktree
  return {
    agentId: input.agentId,
    name: input.name ?? header?.unit ?? input.agentId,
    worktree: named === undefined ? null : expandHome(named, input.home),
    model: textOf(input.model),
    lastKind: null,
  }
}

/** The list with `worker` in its place, replacing an earlier record of the same agent. */
export function upsertWorker(list: readonly Worker[], worker: Worker): Worker[] {
  return [...list.filter(known => known.agentId !== worker.agentId), worker]
}

/** The list with the agent's last message kind set; unchanged when the agent is no worker. */
export function withKind(list: readonly Worker[], agentId: string, kind: string): Worker[] {
  return list.map(worker => (worker.agentId === agentId ? { ...worker, lastKind: kind } : worker))
}

/** An events id at `ms`: ids are unix nanoseconds, zero-padded to 20 digits, so they sort by time. */
export function idAt(ms: number): string {
  return `${Math.max(0, Math.floor(ms))}000000`.padStart(20, '0')
}

/** The query for one source's entries that mention this session. */
export function eventsUrl(source: string, sessionId: string): string {
  const limit = source === LEDGER_SOURCE ? LEDGER_LIMIT : INBOX_LIMIT
  return `${EVENTS_URL}?source=${encodeURIComponent(source)}&q=${encodeURIComponent(sessionId)}&limit=${limit}`
}

type Entry = { id: string; tags: Record<string, unknown> }

function entriesOf(body: string | null): Entry[] | null {
  const items = parseArray(body)
  if (items === null) return null
  const entries: Entry[] = []
  for (const item of items) {
    const id = isRecord(item) ? textOf(item.id) : null
    if (id !== null && isRecord(item) && isRecord(item.tags)) entries.push({ id, tags: item.tags })
  }
  return entries
}

/**
 * belt's ledger as one run sees it: the newest subagent-start facts per
 * agent id, and the auto-mode denials recorded for this session since the
 * run started. The answer comes newest first; denies is null when the
 * answer is not an event array.
 */
export function parseLedger(body: string | null, scope: { sessionId: string; sinceId: string }): LedgerRead {
  const facts = new Map<string, LedgerFacts>()
  const entries = entriesOf(body)
  if (entries === null) return { facts, denies: null }
  let denies = 0
  for (const { id, tags } of entries) {
    const agentId = textOf(tags.agent_id)
    if (tags.event === 'subagent-start' && agentId !== null && !facts.has(agentId)) {
      facts.set(agentId, { model: textOf(tags.model), effort: textOf(tags.effort) })
    }
    if (tags.event === 'permission-denied' && tags.session_id === scope.sessionId && id >= scope.sinceId) denies += 1
  }
  return { facts, denies }
}

/** How many inbox items are addressed to this session since the run started; null when the answer is not an event array. */
export function countParked(body: string | null, scope: { sessionId: string; sinceId: string }): number | null {
  const entries = entriesOf(body)
  if (entries === null) return null
  return entries.filter(({ id, tags }) => tags.to === scope.sessionId && id >= scope.sinceId).length
}

/** `origin/main` from `git symbolic-ref refs/remotes/origin/HEAD`; `main` when it failed. */
export function parseDefaultRef(stdout: string | null): string {
  const line = stdout?.trim().split('\n')[0] ?? ''
  const short = line.replace(/^refs\/remotes\//, '')
  return short === '' || short === line ? DEFAULT_BRANCH : short
}

/** The worktree's facts from `git status --porcelain` and `git rev-list --count`, or null when either failed. */
export function gitFactsFrom(status: string | null, count: string | null): GitFacts | null {
  const ahead = Number.parseInt(count?.trim() ?? '', 10)
  if (status === null || Number.isNaN(ahead)) return null
  return { isDirty: status.trim() !== '', ahead }
}

/** Where an agent's loop stands for the band; `gone` once it ended or left the list. */
export function livenessOf(agents: readonly AgentLike[] | null, agentId: string): Liveness {
  if (agents === null) return 'unknown'
  const status = agents.find(agent => agent.id === agentId)?.status
  if (status !== undefined && LIVE.has(status)) return 'live'
  return status === 'idle' ? 'idle' : 'gone'
}

/**
 * A worker's state in loom's words, plus `blocked` from the contract. A
 * loop at work is in flight. A clean tree whose worker sent ready is ready
 * while it is ahead of the default branch, integrated once it is not. A
 * worker whose last message waits for an answer is blocked. An idle worker
 * is still in flight; one that is gone left its worktree abandoned.
 */
export function workerState(input: { git: GitFacts | null; liveness: Liveness; lastKind: string | null }): string {
  const { git, liveness, lastKind } = input
  if (git === null || liveness === 'unknown') return UNKNOWN
  if (liveness === 'live') return 'in flight'
  if (!git.isDirty && lastKind === 'ready') return git.ahead > 0 ? 'ready' : 'integrated'
  if (lastKind !== null && NEEDS_ANSWER.has(lastKind)) return 'blocked'
  return liveness === 'idle' ? 'in flight' : 'abandoned'
}

/** One refresh's reading of every worker and of the run's counts. */
export function probeFrom(input: {
  workers: readonly Worker[]
  agents: readonly AgentLike[] | null
  gits: ReadonlyArray<GitFacts | null>
  ledger: LedgerRead
  parked: number | null
}): Probe {
  const facts: Record<string, WorkerFacts> = {}
  input.workers.forEach((worker, i) => {
    const recorded = input.ledger.facts.get(worker.agentId)
    const liveness = livenessOf(input.agents, worker.agentId)
    facts[worker.agentId] = {
      state: workerState({ git: input.gits[i] ?? null, liveness, lastKind: worker.lastKind }),
      model: recorded?.model ?? null,
      effort: recorded?.effort ?? null,
    }
  })
  return { facts, ledgerDenies: input.ledger.denies, parked: input.parked }
}

/** True when the decision `classic.PreToolUse` handed back is a belt denial. */
export function isBeltDeny(decision: unknown): boolean {
  if (!isRecord(decision)) return false
  if (typeof decision.deny === 'string') return BELT_REASON.test(decision.deny)
  if (decision.allow === true || typeof decision.ask === 'string') return false
  try {
    return BELT_REASON.test(JSON.stringify(decision) ?? '')
  } catch {
    return false
  }
}

/** True when no refresh started in the last `REFRESH_GAP_MS`. */
export function isRefreshDue(now: number, lastRefreshAt: number | null): boolean {
  return lastRefreshAt === null || now - lastRefreshAt >= REFRESH_GAP_MS
}

/** How long until the gap since the last refresh has passed: 0 when it already has. */
export function trailingDelay(now: number, lastRefreshAt: number | null): number {
  return lastRefreshAt === null ? 0 : Math.max(0, lastRefreshAt + REFRESH_GAP_MS - now)
}

type Row = [name: string, state: string, model: string, effort: string]

function rowOf(worker: Worker, facts: WorkerFacts | undefined): Row {
  return [worker.name, facts?.state ?? UNKNOWN, worker.model ?? facts?.model ?? UNKNOWN, facts?.effort ?? UNKNOWN]
}

// Each row's columns padded to the widest cell, two spaces between them.
function alignRows(rows: readonly Row[]): string[] {
  const width = (col: 0 | 1 | 2): number => Math.max(...rows.map(row => row[col].length))
  const [name, state, model] = [width(0), width(1), width(2)]
  return rows.map(row => [row[0].padEnd(name), row[1].padEnd(state), row[2].padEnd(model), row[3]].join('  '))
}

/**
 * The band: a summary line, then one aligned row per worker as name,
 * state, model and effort. The model comes from the spawn, then the
 * ledger; effort from the ledger alone. Denies add belt's guard denials
 * to the ledger's; parked is a dash while the inbox has not answered.
 */
export function bandView(input: { workers: readonly Worker[]; probe: Probe | null; guardDenies: number }): BandView {
  const rows = input.workers.map(worker => rowOf(worker, input.probe?.facts[worker.agentId]))
  const denies = input.guardDenies + (input.probe?.ledgerDenies ?? 0)
  const parked = input.probe?.parked ?? null
  const noun = rows.length === 1 ? 'worker' : 'workers'
  return {
    summary: `factory · ${rows.length} ${noun} · denies ${denies} · parked ${parked ?? UNKNOWN}`,
    rows: alignRows(rows),
    isAnyReady: rows.some(row => row[1] === 'ready'),
  }
}
