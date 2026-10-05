// Pure helpers behind the panel-policy mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

export type Role = 'review' | 'other'

export type Member = {
  /** The subagent's engine id, or the Agent tool call's id when none came back. */
  id: string
  role: Role
  /** The first characters of the spawn's description. */
  label: string
  startedAt: number
  endedAt: number | null
  /** The main-loop turn the spawn happened in; null when none was seen yet. */
  turn: string | null
  /** True when `id` is the engine's agent id, so a `turn.complete` names it. */
  hasAgentId: boolean
}

export type PolicyConfig = {
  enforce: boolean
  reviewModel: string
  fanoutToast: number
  fanoutHold: number
}

export type PaneRow = {
  role: Role
  label: string
  status: 'running' | 'done'
  duration: string
}

/** How long a finished batch stays on the band after its last member returned. */
export const BAND_LINGER_MS = 60_000

/** How many characters of a prompt the classifier reads. */
export const PROMPT_HEAD_CHARS = 400

/** How many characters of a description a member keeps as its label. */
export const LABEL_CHARS = 60

/** How many members the state keeps; older ones fall off the pane. */
export const MEMBER_CAP = 100

const DEFAULTS: PolicyConfig = { enforce: true, reviewModel: 'opus', fanoutToast: 4, fanoutHold: 0 }

// The words that make a spawn a reviewer, matched on word boundaries so
// "verified" and "verification" (the brain-research promoter's bundle) and
// "critical" (every severity scale) stay out. Plurals and -ing forms of the
// review words are in because the skills use them ("reviewers", "reviews").
const REVIEW_WORDS =
  /\b(?:reviews?|reviewers?|reviewing|verify|verifies|verifying|verifiers?|critics?|panels?|audits?|auditors?|auditing|challengers?|second opinion)\b/i

// A description that announces building or research work settles the role
// as `other` before the prompt is read: the work-on impact agent's prompt
// says "name a verify method" and must not land on the review model.
const WORK_WORDS = /\b(?:research|explore|implement|build|promote|promoter|write|draft|fix|scaffold|refactor)\b/i

/**
 * Whether a spawn is a reviewer: its description decides first (a review
 * word wins, then a work word), and only then the head of its prompt.
 */
export function classifyRole(spawn: { description: string; prompt: string }): Role {
  if (REVIEW_WORDS.test(spawn.description)) return 'review'
  if (WORK_WORDS.test(spawn.description)) return 'other'
  return REVIEW_WORDS.test(spawn.prompt.slice(0, PROMPT_HEAD_CHARS)) ? 'review' : 'other'
}

/** True when `model` already names the review model, as an alias or a full id. */
export function isOnModel(model: string | undefined, reviewModel: string): boolean {
  if (model === undefined || model === '') return false
  return model === reviewModel || model.includes(reviewModel)
}

/**
 * The model to re-point a spawn to, or null to leave it alone: teammates and
 * forks are never touched (a fork inherits its parent's model whatever a
 * hook sets), and nothing moves while `enforce` is off.
 */
export function decideModel(
  spawn: { role: Role; model?: string; isTeammate?: boolean; fork?: boolean },
  config: Pick<PolicyConfig, 'enforce' | 'reviewModel'>,
): string | null {
  if (!config.enforce || spawn.role !== 'review') return null
  if (spawn.isTeammate === true || spawn.fork === true) return null
  return isOnModel(spawn.model, config.reviewModel) ? null : config.reviewModel
}

/** The spawn count at which the fan-out toast shows, once per turn. */
export function isFanoutToastDue(count: number, config: Pick<PolicyConfig, 'fanoutToast'>): boolean {
  return config.fanoutToast > 0 && count === config.fanoutToast
}

/** The spawn count at which the hold asks, once per turn; never while off. */
export function isFanoutHoldDue(count: number, config: Pick<PolicyConfig, 'fanoutHold'>): boolean {
  return config.fanoutHold > 0 && count === config.fanoutHold
}

/** The plugin's options with defaults filled in and types settled. */
export function readConfig(options: Readonly<Record<string, unknown>>): PolicyConfig {
  const bool = (value: unknown, fallback: boolean): boolean =>
    typeof value === 'boolean' ? value : value === 'true' ? true : value === 'false' ? false : fallback
  const text = (value: unknown, fallback: string): string =>
    typeof value === 'string' && value.trim() !== '' ? value.trim() : fallback
  const count = (value: unknown, fallback: number): number => {
    const n = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN
    return Number.isFinite(n) && n >= 0 ? Math.floor(n) : fallback
  }
  return {
    enforce: bool(options.enforce, DEFAULTS.enforce),
    reviewModel: text(options.reviewModel, DEFAULTS.reviewModel),
    fanoutToast: count(options.fanoutToast, DEFAULTS.fanoutToast),
    fanoutHold: count(options.fanoutHold, DEFAULTS.fanoutHold),
  }
}

/** The label a member shows: the head of the description, or the agent type. */
export function memberLabel(description: string, subagentType: string): string {
  const head = description.trim() === '' ? subagentType : description.trim()
  return head.slice(0, LABEL_CHARS)
}

/** The list with one more member, capped to the newest MEMBER_CAP. */
export function addMember(members: readonly Member[], member: Member): Member[] {
  return [...members, member].slice(-MEMBER_CAP)
}

/**
 * Marks the member a `turn.complete` names as done. An id no member carries
 * falls back to the oldest running member recorded without an agent id, the
 * order match the README describes; with none, the list is unchanged.
 */
export function completeMember(members: readonly Member[], agentId: string, now: number): Member[] {
  const byId = members.findIndex(m => m.id === agentId && m.endedAt === null)
  const index = byId >= 0 ? byId : members.findIndex(m => !m.hasAgentId && m.endedAt === null)
  if (index < 0) return [...members]
  return members.map((m, i) => (i === index ? { ...m, endedAt: now } : m))
}

/** The members of the latest batch: those spawned in the newest member's turn. */
export function currentBatch(members: readonly Member[]): Member[] {
  const last = members[members.length - 1]
  if (last === undefined) return []
  return members.filter(m => m.turn === last.turn)
}

/**
 * The band's one line, or null when nothing is in flight and nothing
 * returned within BAND_LINGER_MS: `panel: 2/5 returned · reviewers on opus`.
 */
export function bandText(
  members: readonly Member[],
  now: number,
  config: Pick<PolicyConfig, 'enforce' | 'reviewModel'>,
): string | null {
  const batch = currentBatch(members)
  if (batch.length === 0) return null
  const isRunning = batch.some(m => m.endedAt === null)
  const isRecent = batch.some(m => m.endedAt !== null && now - m.endedAt <= BAND_LINGER_MS)
  if (!isRunning && !isRecent) return null
  const done = batch.filter(m => m.endedAt !== null).length
  const suffix = config.enforce ? ` · reviewers on ${config.reviewModel}` : ''
  return `panel: ${done}/${batch.length} returned${suffix}`
}

/** `12 s` under a minute, `1m 05s` past it. */
export function formatDuration(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000))
  if (seconds < 60) return `${seconds} s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${String(seconds % 60).padStart(2, '0')}s`
}

/** The pane's rows, oldest first, at most `limit` of the newest members. */
export function paneRows(members: readonly Member[], now: number, limit = MEMBER_CAP): PaneRow[] {
  return members.slice(-Math.max(1, limit)).map(m => ({
    role: m.role,
    label: m.label,
    status: m.endedAt === null ? 'running' : 'done',
    duration: formatDuration((m.endedAt ?? now) - m.startedAt),
  }))
}

/** One pane row as a line: role, label, status and duration in columns. */
export function formatPaneRow(row: PaneRow, labelWidth = LABEL_CHARS): string {
  const role = row.role.padEnd(6)
  const label = row.label.padEnd(labelWidth).slice(0, labelWidth)
  const status = row.status.padEnd(7)
  return `${role}  ${label}  ${status}  ${row.duration}`
}
