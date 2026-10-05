// Pure helpers behind the panel-policy mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

export type Role = 'review' | 'other'

export type Member = {
  /** The subagent's engine id: `agentId` on its `turn.complete`, `agent_id` on `classic.SubagentStop`. */
  id: string
  role: Role
  /** The first characters of the spawn's description. */
  label: string
  startedAt: number
  endedAt: number | null
  /** The main-loop turn the spawn happened in; null when none was seen yet. */
  turn: string | null
}

export type PolicyConfig = {
  enforce: boolean
  reviewModel: string
  /** Models a reviewer is moved off when the caller set one of them. */
  overrideFrom: string[]
  fanoutToast: number
  fanoutHold: number
  /** Minutes a member may stay in flight before it counts as stale; 0 never. */
  staleMinutes: number
}

/** Returned, still in flight, or in flight past the stale budget. */
export type MemberStatus = 'running' | 'done' | 'stale'

/** How long a finished batch stays on the band after its last member returned. */
export const BAND_LINGER_MS = 60_000

/** How many characters of the prompt's first line the classifier reads. */
export const PROMPT_LEAD_CHARS = 200

/** How many characters of a description a member keeps as its label. */
export const LABEL_CHARS = 60

/** How many members the state keeps; older ones are forgotten. */
export const MEMBER_CAP = 100

/** The agent type whose model the mod may pick when the caller set none. */
export const GENERAL_PURPOSE = 'general-purpose'

const DEFAULTS: PolicyConfig = {
  enforce: true,
  reviewModel: 'opus',
  overrideFrom: ['sonnet'],
  fanoutToast: 4,
  fanoutHold: 0,
  staleMinutes: 30,
}

const MINUTE_MS = 60_000

// The description's review vocabulary, on word boundaries so "verified",
// "verification" (the brain-research promoter's bundle) and "critical"
// (every severity scale) stay out. Plurals and -ing forms are in because the
// skills use them ("reviewers", "reviews"); analyst, assessor, adherence and
// validation name the sre-panel and work-on agents whose labels say nothing
// of reviewing.
const REVIEW_WORDS =
  /\b(?:reviews?|reviewers?|reviewing|verify|verifies|verifying|verifiers?|critics?|panels?|audits?|auditors?|auditing|challengers?|second opinion|analysts?|assessors?|adherence|validation)\b/i

// A description that opens with a work verb is work, whatever it goes on to
// name: "Implement review comment threading", "Fix cosign verify step",
// "Build the panel-policy mod". The verb is followed by a space or a colon,
// so a compound such as "Build-gate verifier" is not a lead.
const WORK_VERB_LEAD =
  /^(?:implement|build|fix|update|find|write|rewrite|scaffold|refactor|research|explore|promote|promoter|add|create|draft|run|migrate)(?:\s|:)/i

// The prompt's first line, where a reviewer prompt names its role or opens
// with the imperative; the rest of the prompt is the artifact and is never
// read. The role clause stops at sentence punctuation, so "You are a fresh
// reader. Answer about the reviewer tool" does not match.
const ROLE_LEAD =
  /^(?:You are [^.:;!?]*?\b(?:reviewer|critic|auditor|verifier|analyst|assessor)s?\b|Review\b|Audit\b|Verify\b|\**Role:?\**:?\s)/i

/** The prompt's first non-empty line, capped. */
export function promptLead(prompt: string): string {
  for (const line of prompt.split('\n')) {
    const trimmed = line.trim()
    if (trimmed !== '') return trimmed.slice(0, PROMPT_LEAD_CHARS)
  }
  return ''
}

/**
 * Whether a spawn is a reviewer. The description decides first: a leading
 * work verb settles it as other, then a review word makes it a reviewer.
 * Otherwise the prompt's first line decides, by a role or imperative opener.
 */
export function classifyRole(spawn: { description: string; prompt: string }): Role {
  const description = spawn.description.trim()
  if (WORK_VERB_LEAD.test(description)) return 'other'
  if (REVIEW_WORDS.test(description)) return 'review'
  return ROLE_LEAD.test(promptLead(spawn.prompt)) ? 'review' : 'other'
}

/** True when `model` already names the review model, as an alias or a full id. */
export function isOnModel(model: string | undefined, reviewModel: string): boolean {
  if (model === undefined || model === '') return false
  return model === reviewModel || model.includes(reviewModel)
}

/** True for a spawn whose type lets the parent's model decide. */
export function isGeneralPurpose(subagentType: string | undefined): boolean {
  const type = subagentType?.trim() ?? ''
  return type === '' || type === GENERAL_PURPOSE
}

/**
 * The model to re-point a spawn to, or null to leave it alone. A reviewer
 * moves when the caller set a model `overrideFrom` lists, or set none (or
 * `inherit`) on a general-purpose spawn. A model the caller chose that the
 * key does not list stays, as does an agent type with a model of its own
 * (Explore, Plan, a plugin's agent) unless the caller set a listed model.
 * Teammates and forks are never touched: a fork inherits whatever a hook
 * sets, and nothing moves while `enforce` is off.
 */
export function decideModel(
  spawn: { role: Role; model?: string; subagentType?: string; isTeammate?: boolean; fork?: boolean },
  config: Pick<PolicyConfig, 'enforce' | 'reviewModel' | 'overrideFrom'>,
): string | null {
  if (!config.enforce || spawn.role !== 'review') return null
  if (spawn.isTeammate === true || spawn.fork === true) return null
  const model = spawn.model?.trim() ?? ''
  if (isOnModel(model, config.reviewModel)) return null
  const isListed = config.overrideFrom.some(m => m.toLowerCase() === model.toLowerCase())
  if (isListed) return config.reviewModel
  const isUnset = model === '' || model.toLowerCase() === 'inherit'
  return isUnset && isGeneralPurpose(spawn.subagentType) ? config.reviewModel : null
}

/** True for a spawn the fan-out count takes: the main loop's own, no teammate. */
export function countsTowardFanout(spawn: { isTeammate?: boolean; parentAgentId?: string }): boolean {
  return spawn.isTeammate !== true && spawn.parentAgentId === undefined
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
  const list = (value: unknown, fallback: string[]): string[] => {
    const items = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : null
    if (items === null) return fallback
    return items.filter((item): item is string => typeof item === 'string').map(item => item.trim()).filter(item => item !== '')
  }
  return {
    enforce: bool(options.enforce, DEFAULTS.enforce),
    reviewModel: text(options.reviewModel, DEFAULTS.reviewModel),
    overrideFrom: list(options.overrideFrom, DEFAULTS.overrideFrom),
    fanoutToast: count(options.fanoutToast, DEFAULTS.fanoutToast),
    fanoutHold: count(options.fanoutHold, DEFAULTS.fanoutHold),
    staleMinutes: count(options.staleMinutes, DEFAULTS.staleMinutes),
  }
}

/** How long a member may be in flight before it is stale, in ms; 0 means never. */
export function staleAfterMs(config: Pick<PolicyConfig, 'staleMinutes'>): number {
  return config.staleMinutes * MINUTE_MS
}

/** What a member is at `now`; a member never returned goes stale past `staleMs`. */
export function memberStatus(member: Member, now: number, staleMs: number): MemberStatus {
  if (member.endedAt !== null) return 'done'
  return staleMs > 0 && now - member.startedAt > staleMs ? 'stale' : 'running'
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

/** Marks the running member `agentId` names as done; unchanged when none is. */
export function completeMember(members: readonly Member[], agentId: string, now: number): Member[] {
  return members.map(m => (m.id === agentId && m.endedAt === null ? { ...m, endedAt: now } : m))
}

/** The members of the latest batch: those spawned in the newest member's turn. */
export function currentBatch(members: readonly Member[]): Member[] {
  const last = members[members.length - 1]
  if (last === undefined) return []
  return members.filter(m => m.turn === last.turn)
}

/**
 * True while the latest batch has a member in flight (not yet stale) or one
 * returned within BAND_LINGER_MS. A stale member holds nothing open.
 */
export function isBatchActive(members: readonly Member[], now: number, staleMs = 0): boolean {
  const batch = currentBatch(members)
  if (batch.length === 0) return false
  const isRunning = batch.some(m => memberStatus(m, now, staleMs) === 'running')
  const isRecent = batch.some(m => m.endedAt !== null && now - m.endedAt <= BAND_LINGER_MS)
  return isRunning || isRecent
}

/**
 * The band's one line, or null while the batch is neither in flight nor
 * fresh: `panel: 2/5 returned · reviewers on opus`, with `, 1 stale` after
 * the count when a member outran the stale budget. The suffix names the
 * review model only when the batch holds a reviewer and `enforce` is on.
 */
export function bandText(
  members: readonly Member[],
  now: number,
  config: Pick<PolicyConfig, 'enforce' | 'reviewModel' | 'staleMinutes'>,
): string | null {
  const staleMs = staleAfterMs(config)
  if (!isBatchActive(members, now, staleMs)) return null
  const batch = currentBatch(members)
  const done = batch.filter(m => m.endedAt !== null).length
  const stale = batch.filter(m => memberStatus(m, now, staleMs) === 'stale').length
  const staleNote = stale > 0 ? `, ${stale} stale` : ''
  const hasReviewer = batch.some(m => m.role === 'review')
  const suffix = config.enforce && hasReviewer ? ` · reviewers on ${config.reviewModel}` : ''
  return `panel: ${done}/${batch.length} returned${staleNote}${suffix}`
}

/** `12 s` under a minute, `1m 05s` past it; the debug log's duration of a returned member. */
export function formatDuration(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000))
  if (seconds < 60) return `${seconds} s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${String(seconds % 60).padStart(2, '0')}s`
}
