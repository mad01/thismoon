// Pure helpers behind the session-band mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

export type AgentState = 'working' | 'waiting' | 'idle'

export type PageRef = { id: string; url: string | null }

export type BandSnapshot = {
  agent: AgentState
  worklogKey: string | null
  phase: string | null
  presentPage: PageRef | null
  beltDenies: number
}

/** How long a localhost probe may take before the band gives up on it. */
export const PROBE_BUDGET_MS = 400

/** The events service the belt deny counter polls. */
export const EVENTS_BASE_URL = 'http://localhost:7430'

/** A turn longer than this gets a "done" toast. */
export const LONG_TURN_MS = 60_000

/** How many characters of a phase line the state keeps. */
const PHASE_LINE_CAP = 120

/** How many lines the band may take above the prompt. */
const BAND_MAX_LINES = 2

const SEPARATOR = ' | '

// The work-on skill ends every interaction with a line that starts
// `[Phase X...]`; the last one in an answer is the current state.
const PHASE_MARKER = /^\[Phase [^\]]+\].*$/gm

/** The last work-on phase line in an answer, capped, or null. */
export function findPhaseMarker(answer: string): string | null {
  let last: string | null = null
  for (const match of answer.matchAll(PHASE_MARKER)) {
    last = match[0]
  }
  return last === null ? null : last.trim().slice(0, PHASE_LINE_CAP)
}

/** The text inside the leading brackets of a phase line: `Phase 4.unit.2/5`. */
export function phaseLabel(line: string): string {
  const close = line.indexOf(']')
  return close > 1 ? line.slice(1, close) : line
}

export type BeltTally = { denies: number; newestId: string | null }

type EventLike = { id?: unknown; source?: unknown; title?: unknown }

// belt emits `blocked <tool> (<guard>)` for a guard denial and
// `hinted ...` for advisory hints; only the first is a deny.
function isBeltDeny(event: EventLike): boolean {
  return (
    event.source === 'belt' &&
    typeof event.title === 'string' &&
    event.title.startsWith('blocked ')
  )
}

/** Counts belt denies in an events page and finds the cursor for the next poll. */
export function countBeltDenies(events: unknown): BeltTally {
  if (!Array.isArray(events)) return { denies: 0, newestId: null }
  let denies = 0
  let newestId: string | null = null
  for (const item of events) {
    if (item === null || typeof item !== 'object') continue
    const event = item as EventLike
    if (typeof event.id === 'string' && (newestId === null || event.id > newestId)) {
      newestId = event.id
    }
    if (isBeltDeny(event)) denies += 1
  }
  return { denies, newestId }
}

/**
 * An events cursor standing at `nowMs`: ids are `%020d-%04x` of unix
 * nanoseconds, so this sorts after everything emitted before now.
 */
export function eventsCursorAt(nowMs: number): string {
  const nanos = BigInt(Math.max(0, Math.floor(nowMs))) * 1_000_000n
  return `${nanos.toString().padStart(20, '0')}-0000`
}

/** The events query for belt activity after `cursor`. */
export function beltEventsUrl(cursor: string | null, baseUrl = EVENTS_BASE_URL): string {
  const query = new URLSearchParams({ source: 'belt', limit: '200' })
  if (cursor !== null) query.set('since', cursor)
  return `${baseUrl}/api/events?${query.toString()}`
}

const AGENT_LABEL: Record<AgentState, string> = {
  working: '* working',
  waiting: '? waiting for input',
  idle: '- idle',
}

/** The band's segments in display order; absent signals are left out. */
export function bandSegments(snapshot: BandSnapshot): string[] {
  const segments = [AGENT_LABEL[snapshot.agent]]
  if (snapshot.worklogKey !== null) segments.push(`worklog ${snapshot.worklogKey}`)
  if (snapshot.phase !== null) segments.push(phaseLabel(snapshot.phase))
  if (snapshot.presentPage !== null) segments.push(`present ${snapshot.presentPage.id}`)
  segments.push(`belt denies ${snapshot.beltDenies}`)
  return segments
}

/**
 * Packs the segments into at most two lines of `columns` cells. A segment
 * that fits nowhere is dropped rather than wrapped mid-word; with no width
 * known everything goes on one line.
 */
export function composeBandLines(snapshot: BandSnapshot, columns?: number): string[] {
  const segments = bandSegments(snapshot)
  if (columns === undefined || columns <= 0) return [segments.join(SEPARATOR)]
  const lines: string[] = []
  let current = ''
  for (const segment of segments) {
    const candidate = current === '' ? segment : `${current}${SEPARATOR}${segment}`
    if (candidate.length <= columns || current === '') {
      current = candidate
      continue
    }
    lines.push(current)
    current = segment
    if (lines.length === BAND_MAX_LINES) break
  }
  if (lines.length < BAND_MAX_LINES && current !== '') lines.push(current)
  return lines.slice(0, BAND_MAX_LINES)
}

/** The first KEY of a `worklog list` table, or null when it lists nothing. */
export function parseWorklogList(stdout: string): string | null {
  const rows = stdout.split('\n').map(line => line.trim()).filter(line => line !== '')
  for (const row of rows) {
    if (row.startsWith('KEY ')) continue
    const [key] = row.split(/\s+/)
    if (key !== undefined && key !== '') return key
  }
  return null
}

/**
 * The repo name behind a `git rev-parse --git-common-dir` answer: the
 * directory holding `.git`, resolved against `cwd` when git printed a
 * relative path (it does, from the main worktree). Falls back to the cwd's
 * own name outside a repo.
 */
export function repoNameFrom(gitCommonDir: string | null, cwd: string): string {
  const base = (path: string): string => {
    const parts = path.replace(/\/+$/, '').split('/')
    return parts[parts.length - 1] ?? ''
  }
  const trimmed = gitCommonDir?.trim() ?? ''
  if (trimmed === '') return base(cwd)
  const absolute = trimmed.startsWith('/') ? trimmed : `${cwd.replace(/\/+$/, '')}/${trimmed}`
  const withoutGit = absolute.replace(/\/\.git$/, '')
  return base(withoutGit === '' ? cwd : withoutGit)
}

type McpResultLike = {
  structuredContent?: unknown
  content?: unknown
}

function pageFromObject(value: unknown): PageRef | null {
  if (value === null || typeof value !== 'object') return null
  const { id, url } = value as { id?: unknown; url?: unknown }
  if (typeof id !== 'string' || id === '') return null
  return { id, url: typeof url === 'string' ? url : null }
}

/**
 * The page a present_create or present_update result names: its structured
 * content first, then the first text block parsed as JSON, then the id the
 * call was made with (an update's input names the page).
 */
export function parsePresentResult(result: unknown, inputId: string | null): PageRef | null {
  if (result !== null && typeof result === 'object') {
    const mcp = result as McpResultLike
    const structured = pageFromObject(mcp.structuredContent)
    if (structured !== null) return structured
    if (Array.isArray(mcp.content)) {
      for (const block of mcp.content) {
        const text = (block as { text?: unknown })?.text
        if (typeof text !== 'string') continue
        try {
          const parsed = pageFromObject(JSON.parse(text))
          if (parsed !== null) return parsed
        } catch {
          // not JSON: the next block may be
        }
      }
    }
  }
  return inputId !== null && inputId !== '' ? { id: inputId, url: null } : null
}

/** The Claude-only block injected after a compaction. */
export function compactContextBlock(snapshot: BandSnapshot): string {
  const lines = ['session-band: state carried across the compaction.']
  if (snapshot.worklogKey !== null) lines.push(`- active worklog key: ${snapshot.worklogKey}`)
  if (snapshot.phase !== null) lines.push(`- last phase marker: ${snapshot.phase}`)
  if (snapshot.presentPage !== null) {
    const url = snapshot.presentPage.url === null ? '' : ` (${snapshot.presentPage.url})`
    lines.push(`- present page id: ${snapshot.presentPage.id}${url}`)
  }
  lines.push('- checkpoint with worklog_checkpoint if work advanced since the last checkpoint')
  return lines.join('\n')
}
