// Pure helpers behind the session-band mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

export type AgentState = 'working' | 'waiting' | 'idle'

export type PageRef = { id: string; url: string | null }

export type BandSnapshot = {
  isWaiting: boolean
  worklogKey: string | null
  phase: string | null
  presentPage: PageRef | null
  beltDenies: number
}

/** A turn longer than this gets a "done" toast. */
export const LONG_TURN_MS = 60_000

/** How many characters of a phase line the state keeps. */
const PHASE_LINE_CAP = 120

/** How many lines the band may take above the prompt. */
const BAND_MAX_LINES = 2

const SEPARATOR = ' | '

// The work-on skill ends every interaction with a line that starts
// `[Phase X...]`, or `[<key> Phase X...]` for ticketed work; the last one
// in an answer is the current state.
const PHASE_MARKER = /^\[(?:\S+ )?Phase [^\]]+\].*$/gm

/** The last work-on phase line in an answer, capped, or null. */
export function findPhaseMarker(answer: string): string | null {
  let last: string | null = null
  for (const match of answer.matchAll(PHASE_MARKER)) {
    last = match[0]
  }
  return last === null ? null : last.trim().slice(0, PHASE_LINE_CAP)
}

/** The text inside the leading brackets of a phase line: `Phase 4.unit.2/5` or `MAD-123 Phase 2.1`. */
export function phaseLabel(line: string): string {
  const close = line.indexOf(']')
  return close > 1 ? line.slice(1, close) : line
}

/** Whether a finished turn earns the "done" toast: answered, and long. */
export function shouldToastDone(reason: string, durationMs: number): boolean {
  return reason === 'answer' && durationMs > LONG_TURN_MS
}

// Every belt denial reason starts `belt[<guard>]: `; other settings hooks
// deny with their own words.
const BELT_REASON = /^belt\[/

/** Whether a `classic.PreToolUse` decision is a denial belt wrote. */
export function isBeltDeny(decision: unknown): boolean {
  if (decision === null || typeof decision !== 'object') return false
  const { deny } = decision as { deny?: unknown }
  return typeof deny === 'string' && BELT_REASON.test(deny)
}

/** What the band says about the agent, from the render prop and the waiting flag. */
export function agentState(isWaiting: boolean, isWorking: boolean): AgentState {
  if (isWaiting) return 'waiting'
  return isWorking ? 'working' : 'idle'
}

const AGENT_LABEL: Record<AgentState, string> = {
  working: '* working',
  waiting: '? waiting for input',
  idle: '- idle',
}

/** The band's segments in display order; absent signals are left out. */
export function bandSegments(snapshot: BandSnapshot, isWorking: boolean): string[] {
  const segments = [AGENT_LABEL[agentState(snapshot.isWaiting, isWorking)]]
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
export function composeBandLines(snapshot: BandSnapshot, isWorking: boolean, columns?: number): string[] {
  const segments = bandSegments(snapshot, isWorking)
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

/**
 * The first KEY of a `worklog list` table, or null. Rows count only below
 * the `KEY` header: an empty store prints `no items` with no header at all.
 */
export function parseWorklogList(stdout: string): string | null {
  let isBelowHeader = false
  for (const line of stdout.split('\n')) {
    const row = line.trim()
    if (row === '') continue
    if (!isBelowHeader) {
      isBelowHeader = row.startsWith('KEY ')
      continue
    }
    const [key] = row.split(/\s+/)
    if (key !== undefined && key !== '') return key
  }
  return null
}

/** `a/b/../c` → `a/c`, with `.` segments dropped; keeps the leading slash. */
function normalizePath(path: string): string {
  const out: string[] = []
  for (const segment of path.split('/')) {
    if (segment === '' || segment === '.') continue
    if (segment === '..') {
      out.pop()
      continue
    }
    out.push(segment)
  }
  return (path.startsWith('/') ? '/' : '') + out.join('/')
}

/**
 * The repo name behind a `git rev-parse --git-common-dir` answer: the
 * directory holding `.git`. register.tsx asks git for an absolute path, and
 * a relative one (`.git`, `../../.git`) is still resolved against `cwd`.
 * Falls back to the cwd's own name outside a repo.
 */
export function repoNameFrom(gitCommonDir: string | null, cwd: string): string {
  const base = (path: string): string => {
    const parts = normalizePath(path).split('/')
    return parts[parts.length - 1] ?? ''
  }
  const trimmed = gitCommonDir?.trim() ?? ''
  if (trimmed === '') return base(cwd)
  const absolute = trimmed.startsWith('/') ? trimmed : `${cwd}/${trimmed}`
  const withoutGit = normalizePath(absolute).replace(/\/\.git$/, '')
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

function pageFromJson(text: string): PageRef | null {
  try {
    return pageFromObject(JSON.parse(text))
  } catch {
    return null
  }
}

/**
 * The page a present_create or present_update result names. The result is
 * usually one JSON string (`{"id":..,"url":..,"version":..}`); an MCP record
 * with `structuredContent` or text blocks is read too, then `text` as the
 * model sees it, then the id the call was made with (an update's input
 * names the page).
 */
export function parsePresentResult(result: unknown, inputId: string | null, text?: string): PageRef | null {
  if (typeof result === 'string') {
    const parsed = pageFromJson(result)
    if (parsed !== null) return parsed
  }
  if (result !== null && typeof result === 'object') {
    const mcp = result as McpResultLike
    const structured = pageFromObject(mcp.structuredContent)
    if (structured !== null) return structured
    if (Array.isArray(mcp.content)) {
      for (const block of mcp.content) {
        const blockText = (block as { text?: unknown })?.text
        if (typeof blockText !== 'string') continue
        const parsed = pageFromJson(blockText)
        if (parsed !== null) return parsed
      }
    }
  }
  if (text !== undefined) {
    const parsed = pageFromJson(text)
    if (parsed !== null) return parsed
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
