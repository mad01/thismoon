// Pure helpers behind the humanizer-gate mod: no `$`, no I/O, so `node --test`
// covers them without the engine. hooks/register.ts and pi/index.ts call them.

export type Severity = 'suggestion' | 'warning' | 'error'

const SEVERITIES: readonly Severity[] = ['suggestion', 'warning', 'error']

/** How long one detect run may take before the gate gives up on it. */
export const DETECT_BUDGET_MS = 5_000

/** How many matched texts a question quotes. */
const QUOTED_MATCHES = 3

/** How many findings a feedback deny lists; the rest are counted. */
const LISTED_FINDINGS = 20

/** How long a matched text a feedback deny quotes before it is clipped. */
const QUOTED_MATCH_CHARS = 60

/** How many denied texts a session remembers, so an unchanged resubmission passes. */
const REMEMBERED_DENIALS = 32

/** The labels of the hold dialog; only the second one stops the call. */
export const HOLD_OPTIONS = ['Proceed', 'Cancel'] as const

/** The deny text a cancelled call carries back to the model. */
export const CANCELLED_REASON = 'humanizer-gate: cancelled by user'

export type GateConfig = {
  holdOnError: boolean
  holdOnCommitError: boolean
  feedbackWarnings: boolean
  minSeverity: Severity
}

export const DEFAULT_CONFIG: GateConfig = {
  holdOnError: true,
  holdOnCommitError: false,
  feedbackWarnings: true,
  minSeverity: 'warning',
}

type ConfigFlag = 'holdOnError' | 'holdOnCommitError' | 'feedbackWarnings'

function isSeverity(value: unknown): value is Severity {
  return typeof value === 'string' && (SEVERITIES as readonly string[]).includes(value)
}

/** The mod's userConfig with defaults filled in; a value of the wrong kind falls back. */
export function readConfig(options: Readonly<Record<string, unknown>>): GateConfig {
  const flag = (key: ConfigFlag): boolean => {
    const value = options[key]
    return typeof value === 'boolean' ? value : DEFAULT_CONFIG[key]
  }
  return {
    holdOnError: flag('holdOnError'),
    holdOnCommitError: flag('holdOnCommitError'),
    feedbackWarnings: flag('feedbackWarnings'),
    minSeverity: isSeverity(options.minSeverity) ? options.minSeverity : DEFAULT_CONFIG.minSeverity,
  }
}

const PROSE_EXTENSION = /\.mdx?$/i
const SKIPPED_SEGMENTS = new Set(['node_modules', '.git'])

/**
 * Whether a Write or Edit target is prose the gate scans: a `.md` or `.mdx`
 * file outside `node_modules` and `.git`. Dot directories are scanned, so
 * markdown under `.claude/worktrees` and `~/.worktrees` counts.
 */
export function isProseFile(path: string): boolean {
  if (!PROSE_EXTENSION.test(path)) return false
  return !path.split('/').some(segment => SKIPPED_SEGMENTS.has(segment))
}

/** The last path segment. */
export function basename(path: string): string {
  const parts = path.replace(/\/+$/, '').split('/')
  return parts[parts.length - 1] ?? path
}

type Context = 'top' | 'sub' | 'dq' | 'sq'

const HEREDOC_OPEN = /^<<-?\s*(['"]?)(\w+)\1/

/**
 * Splits a shell command into its top-level segments: the text between
 * `&&`, `||`, `;`, `|`, `&` and newlines that sit outside quotes, `$(...)`
 * and parentheses. A heredoc body stays inside the segment that opened it,
 * so a script fed to `cat` or `python3` never becomes a segment of its own.
 */
export function splitSegments(command: string): string[] {
  const segments: string[] = []
  const stack: Context[] = ['top']
  const pending: string[] = []
  let current = ''
  let i = 0
  const flush = (): void => {
    if (current.trim() !== '') segments.push(current)
    current = ''
  }
  // Called at the newline that ends the line holding the openers: each
  // pending heredoc's body runs to the line holding its delimiter alone.
  const consumeHeredocs = (): void => {
    while (pending.length > 0) {
      const delimiter = pending.shift()
      while (i < command.length) {
        const end = command.indexOf('\n', i)
        const line = end === -1 ? command.slice(i) : command.slice(i, end + 1)
        current += line
        i = end === -1 ? command.length : end + 1
        if (line.trim() === delimiter) break
      }
    }
  }
  while (i < command.length) {
    const context = stack[stack.length - 1]
    const ch = command[i] ?? ''
    if (context === 'sq') {
      current += ch
      i += 1
      if (ch === "'") stack.pop()
      continue
    }
    if (ch === '\\') {
      current += command.slice(i, i + 2)
      i += 2
      continue
    }
    if (ch === '\n') {
      current += ch
      i += 1
      if (pending.length > 0) consumeHeredocs()
      if (context === 'top') flush()
      continue
    }
    if (context === 'dq') {
      current += ch
      i += 1
      if (ch === '"') stack.pop()
      else if (ch === '$' && command[i] === '(') {
        current += '('
        i += 1
        stack.push('sub')
      }
      continue
    }
    if (ch === "'" || ch === '"') {
      stack.push(ch === "'" ? 'sq' : 'dq')
    } else if (ch === '(') {
      stack.push('sub')
    } else if (ch === ')') {
      if (context === 'sub') stack.pop()
    } else if (ch === '<' && command[i + 1] === '<') {
      const open = HEREDOC_OPEN.exec(command.slice(i))
      if (open !== null) {
        pending.push(open[2] ?? '')
        current += open[0]
        i += open[0].length
        continue
      }
    } else if (context === 'top' && (ch === ';' || ch === '|' || ch === '&')) {
      flush()
      i += ch !== ';' && command[i + 1] === ch ? 2 : 1
      continue
    }
    current += ch
    i += 1
  }
  flush()
  return segments
}

// A segment whose command is git's `commit` subcommand, after any leading
// variable assignments and git's own options (`-C dir`, `-c key=val`, `--x`).
const COMMIT_SEGMENT = /^\s*(?:\w+=\S*\s+)*git(?:\s+-[cC]\s+\S+|\s+--\S+)*\s+commit\b/
// `-m "..."` with backslash escapes, or `-m '...'` with the `'\''` escape; a
// short-flag cluster ending in m (`-am`), no separator (`-m"x"`), and
// `--message` or `--message=` the same way.
const MESSAGE_FLAG = /(?:^|\s)(?:-[a-zA-Z]*m|--message)(?:\s*|=)(?:"((?:[^"\\]|\\[\s\S])*)"|'((?:[^']|'\\'')*)')/g

function unescapeDoubleQuoted(text: string): string {
  return text.replace(/\\([\\"$`\n])/g, '$1')
}

function unescapeSingleQuoted(text: string): string {
  return text.replace(/'\\''/g, "'")
}

// The body of the first heredoc in a segment: from the line after the
// `<<DELIM` opener to the line holding the delimiter alone. Null when there
// is none or it is unterminated.
function heredocBody(segment: string): string | null {
  const open = /<<-?\s*(['"]?)(\w+)\1/.exec(segment)
  if (open === null) return null
  const delimiter = open[2] ?? ''
  const bodyStart = segment.indexOf('\n', open.index)
  if (bodyStart === -1) return null
  const body: string[] = []
  for (const line of segment.slice(bodyStart + 1).split('\n')) {
    if (line.trim() === delimiter) return body.join('\n')
    body.push(line)
  }
  return null
}

/**
 * The message of the last `git commit` segment in a shell command, or null
 * when there is none or it carries no message the gate can read (a message
 * from a file or an editor, an unquoted word). Only that segment is read, so
 * a script in a heredoc elsewhere in the command is never scanned, and of
 * two chained commits the last one (the one that lands) is checked. Inside
 * the segment a heredoc wins (`-F - <<'EOF'`, `-m "$(cat <<'EOF' ...)"`);
 * otherwise every quoted `-m` in order, joined as git joins them.
 */
export function extractCommitMessage(command: string): string | null {
  const commits = splitSegments(command).filter(segment => COMMIT_SEGMENT.test(segment))
  const segment = commits[commits.length - 1]
  if (segment === undefined) return null
  const heredoc = heredocBody(segment)
  if (heredoc !== null) return heredoc.trim() === '' ? null : heredoc
  const parts: string[] = []
  for (const match of segment.matchAll(MESSAGE_FLAG)) {
    const text =
      match[1] !== undefined ? unescapeDoubleQuoted(match[1]) : unescapeSingleQuoted(match[2] ?? '')
    if (text.trim() !== '') parts.push(text)
  }
  return parts.length === 0 ? null : parts.join('\n\n')
}

/** One detect finding, as a feedback deny lists it. Line and column are 1-based; 0 when detect gave none. */
export type Finding = {
  ruleId: string
  severity: Severity | null
  line: number
  column: number
  match: string
  message: string
}

export type FindingsSummary = {
  total: number
  errors: number
  warnings: number
  suggestions: number
  /** The first three distinct matched texts, errors first. */
  matches: string[]
  /** Every finding, in text order. */
  findings: Finding[]
}

type FindingLike = {
  rule_id?: unknown
  severity?: unknown
  line?: unknown
  column?: unknown
  match?: unknown
  message?: unknown
}

function orderedBySeverity(findings: FindingLike[]): FindingLike[] {
  const rank = (finding: FindingLike): number =>
    isSeverity(finding.severity) ? SEVERITIES.indexOf(finding.severity) : -1
  return [...findings].sort((a, b) => rank(b) - rank(a))
}

const stringOf = (value: unknown): string => (typeof value === 'string' ? value : '')
const positionOf = (value: unknown): number => (typeof value === 'number' && Number.isFinite(value) ? value : 0)

function toFinding(item: FindingLike): Finding {
  return {
    ruleId: stringOf(item.rule_id),
    severity: isSeverity(item.severity) ? item.severity : null,
    line: positionOf(item.line),
    column: positionOf(item.column),
    match: stringOf(item.match),
    message: stringOf(item.message),
  }
}

function inTextOrder(a: Finding, b: Finding): number {
  return a.line - b.line || a.column - b.column
}

/** Counts a detect payload's findings by severity, quotes the first few, and keeps them all in text order. */
export function summarizeFindings(payload: unknown): FindingsSummary {
  const summary: FindingsSummary = { total: 0, errors: 0, warnings: 0, suggestions: 0, matches: [], findings: [] }
  const findings = (payload as { findings?: unknown } | null)?.findings
  if (!Array.isArray(findings)) return summary
  const objects = findings.filter((f): f is FindingLike => f !== null && typeof f === 'object')
  const seen = new Set<string>()
  for (const item of orderedBySeverity(objects)) {
    summary.total += 1
    if (item.severity === 'error') summary.errors += 1
    else if (item.severity === 'warning') summary.warnings += 1
    else if (item.severity === 'suggestion') summary.suggestions += 1
    const match = typeof item.match === 'string' ? item.match.trim() : ''
    if (match !== '' && !seen.has(match) && summary.matches.length < QUOTED_MATCHES) {
      seen.add(match)
      summary.matches.push(match)
    }
  }
  summary.findings = objects.map(toFinding).sort(inTextOrder)
  return summary
}

/** The summary of a `humanizer detect --json` run, or null when stdout is not its payload. */
export function parseDetectOutput(stdout: string): FindingsSummary | null {
  try {
    const parsed: unknown = JSON.parse(stdout)
    if (parsed === null || typeof parsed !== 'object' || !('findings' in parsed)) return null
    return summarizeFindings(parsed)
  } catch {
    return null
  }
}

function count(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`
}

/**
 * The status line for a scanned target, or undefined (clear it) with
 * nothing found. The engine prefixes the line with the mod's name, so the
 * text never repeats it.
 */
export function statusText(summary: FindingsSummary, target: string): string | undefined {
  if (summary.total === 0) return undefined
  const parts: string[] = []
  if (summary.errors > 0) parts.push(count(summary.errors, 'error'))
  if (summary.warnings > 0) parts.push(count(summary.warnings, 'warning'))
  if (summary.suggestions > 0) parts.push(count(summary.suggestions, 'suggestion'))
  return `${parts.join(', ')} in ${target}`
}

/** The question the hold dialog asks: the error count and the first matches. */
export function holdQuestion(summary: FindingsSummary, target: string): string {
  const quoted = summary.matches.map(match => `"${match}"`).join(', ')
  const detail = quoted === '' ? '' : ` (${quoted})`
  return `humanizer found ${count(summary.errors, 'error-level finding')} in the ${target}${detail}. Send it anyway?`
}

const isWarningOrAbove = (finding: Finding): boolean => finding.severity === 'warning' || finding.severity === 'error'

// A matched text on one line, clipped: a finding may match a newline or a
// whole sentence.
function clip(match: string): string {
  const flat = match.replace(/\s+/g, ' ').trim()
  return flat.length <= QUOTED_MATCH_CHARS ? flat : `${flat.slice(0, QUOTED_MATCH_CHARS - 3)}...`
}

function findingLine(finding: Finding): string {
  const quoted = clip(finding.match)
  const match = quoted === '' ? '' : ` "${quoted}"`
  return `- ${finding.ruleId} ${finding.line}:${finding.column}${match}: ${finding.message}`
}

/**
 * The deny text that hands warning-level and error-level findings back to
 * the model: one line per finding (rule id, line:column in the text, the
 * matched text, the message), at most LISTED_FINDINGS of them, then what to
 * do next.
 */
export function feedbackReason(summary: FindingsSummary, target: string): string {
  const due = summary.findings.filter(isWarningOrAbove)
  const lines = due.slice(0, LISTED_FINDINGS).map(findingLine)
  if (due.length > LISTED_FINDINGS) lines.push(`- and ${due.length - LISTED_FINDINGS} more`)
  const found = `humanizer detect found ${count(due.length, 'finding')} in the ${target}`
  return [
    `humanizer-gate: not sent. ${found} (line:column in that text):`,
    ...lines,
    `Fix them and send the call again. If they are false positives, send the same ${target} unchanged and it goes through.`,
  ].join('\n')
}

/**
 * A short key for a scanned text, so the session can remember which texts
 * it denied without keeping them. cyrb53, a 53-bit non-cryptographic hash:
 * a collision only lets a changed text through unchecked, which fails open.
 */
export function textKey(target: string, text: string): string {
  const input = `${target}\u0000${text}`
  let h1 = 0xdeadbeef
  let h2 = 0x41c6ce57
  for (let i = 0; i < input.length; i += 1) {
    const ch = input.charCodeAt(i)
    h1 = Math.imul(h1 ^ ch, 2654435761)
    h2 = Math.imul(h2 ^ ch, 1597334677)
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909)
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(16)
}

/** `keys` with `key` added last, dropping the oldest past REMEMBERED_DENIALS. */
export function rememberDenial(keys: readonly string[], key: string): string[] {
  return [...keys.filter(k => k !== key), key].slice(-REMEMBERED_DENIALS)
}

/** What the gate does with a scanned PR body or commit message. */
export type GateVerdict =
  /** No finding to act on, a failed scan, or the feature off: send it. */
  | { kind: 'pass' }
  /** Error-level findings with the hold on: ask Proceed / Cancel. */
  | { kind: 'hold'; summary: FindingsSummary }
  /** The same text was denied before in this session: send it. */
  | { kind: 'resubmitted' }
  /** Deny with `reason` and remember `key`, so the same text passes next time. */
  | { kind: 'feedback'; reason: string; key: string }

export type GateInput = {
  /** The detect summary; null when the scan failed. */
  summary: FindingsSummary | null
  /** `PR body` or `commit message`. */
  target: string
  text: string
  /** holdOnError for a PR body, holdOnCommitError for a commit message. */
  isHolding: boolean
  /** feedbackWarnings. */
  isFeedback: boolean
  /** The keys of the texts this session already denied. */
  denied: readonly string[]
}

/**
 * Decides what happens to a scanned text. Error-level findings with the
 * hold on go to the dialog, as they always have. Otherwise a text with a
 * finding at warning or above is denied once with the findings as the
 * reason; the same text sent again passes, and a changed text is judged
 * afresh.
 */
export function judge(input: GateInput): GateVerdict {
  const { summary, target, text } = input
  if (summary === null) return { kind: 'pass' }
  if (summary.errors > 0 && input.isHolding) return { kind: 'hold', summary }
  if (!input.isFeedback || !summary.findings.some(isWarningOrAbove)) return { kind: 'pass' }
  const key = textKey(target, text)
  if (input.denied.includes(key)) return { kind: 'resubmitted' }
  return { kind: 'feedback', reason: feedbackReason(summary, target), key }
}
