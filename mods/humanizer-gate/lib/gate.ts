// Pure helpers behind the humanizer-gate mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.ts is the only caller.

export type Severity = 'suggestion' | 'warning' | 'error'

const SEVERITIES: readonly Severity[] = ['suggestion', 'warning', 'error']

/** How long one detect run may take before the gate gives up on it. */
export const DETECT_BUDGET_MS = 5_000

/** How many matched texts a question quotes. */
const QUOTED_MATCHES = 3

/** The labels of the hold dialog; only the second one stops the call. */
export const HOLD_OPTIONS = ['Proceed', 'Cancel'] as const

/** The deny text a cancelled call carries back to the model. */
export const CANCELLED_REASON = 'humanizer-gate: cancelled by user'

export type GateConfig = {
  holdOnError: boolean
  holdOnCommitError: boolean
  minSeverity: Severity
}

export const DEFAULT_CONFIG: GateConfig = {
  holdOnError: true,
  holdOnCommitError: false,
  minSeverity: 'warning',
}

function isSeverity(value: unknown): value is Severity {
  return typeof value === 'string' && (SEVERITIES as readonly string[]).includes(value)
}

/** The mod's userConfig with defaults filled in; a value of the wrong kind falls back. */
export function readConfig(options: Readonly<Record<string, unknown>>): GateConfig {
  const flag = (key: 'holdOnError' | 'holdOnCommitError'): boolean => {
    const value = options[key]
    return typeof value === 'boolean' ? value : DEFAULT_CONFIG[key]
  }
  return {
    holdOnError: flag('holdOnError'),
    holdOnCommitError: flag('holdOnCommitError'),
    minSeverity: isSeverity(options.minSeverity) ? options.minSeverity : DEFAULT_CONFIG.minSeverity,
  }
}

const PROSE_EXTENSION = /\.mdx?$/i
const SKIPPED_SEGMENTS = new Set(['node_modules', '.git'])
const VISIBLE_DOT_SEGMENTS = new Set(['.github', '.', '..'])

/**
 * Whether a Write or Edit target is prose the gate scans: a `.md` or `.mdx`
 * file outside `node_modules`, `.git`, and any dot directory but `.github`.
 */
export function isProseFile(path: string): boolean {
  if (!PROSE_EXTENSION.test(path)) return false
  for (const segment of path.split('/')) {
    if (SKIPPED_SEGMENTS.has(segment)) return false
    if (segment.startsWith('.') && !VISIBLE_DOT_SEGMENTS.has(segment)) return false
  }
  return true
}

/** The last path segment. */
export function basename(path: string): string {
  const parts = path.replace(/\/+$/, '').split('/')
  return parts[parts.length - 1] ?? path
}

// `git commit` with any git options between them (`git -C dir commit`),
// inside one shell command: a separator ends the search.
const GIT_COMMIT = /\bgit\b[^|;&\n]*?\bcommit\b/
const HEREDOC_OPEN = /<<-?\s*(['"]?)(\w+)\1/
// `-m "..."` with backslash escapes, or `-m '...'`; `--message` the same way.
const MESSAGE_FLAG = /(?:^|\s)(?:-m|--message)(?:\s+|=)(?:"((?:[^"\\]|\\[\s\S])*)"|'([^']*)')/g

function unescapeDoubleQuoted(text: string): string {
  return text.replace(/\\([\\"$`\n])/g, '$1')
}

// The body of the first heredoc in the command: from the line after the
// `<<DELIM` opener to the line holding the delimiter alone. Null when the
// heredoc is unterminated.
function heredocBody(command: string): string | null {
  const open = HEREDOC_OPEN.exec(command)
  if (open === null) return null
  const delimiter = open[2] ?? ''
  const bodyStart = command.indexOf('\n', open.index)
  if (bodyStart === -1) return null
  const body: string[] = []
  for (const line of command.slice(bodyStart + 1).split('\n')) {
    if (line.trim() === delimiter) return body.join('\n')
    body.push(line)
  }
  return null
}

/**
 * The commit message a shell command carries, or null when it carries none
 * the gate can read (a message from a file or an editor, an unquoted word).
 * A heredoc wins (`-F - <<'EOF'`, or `-m "$(cat <<'EOF' ...)"`); otherwise
 * every quoted `-m` in order, joined as git joins them.
 */
export function extractCommitMessage(command: string): string | null {
  if (!GIT_COMMIT.test(command)) return null
  const heredoc = heredocBody(command)
  if (heredoc !== null) return heredoc.trim() === '' ? null : heredoc
  const parts: string[] = []
  for (const match of command.matchAll(MESSAGE_FLAG)) {
    const text = match[1] !== undefined ? unescapeDoubleQuoted(match[1]) : (match[2] ?? '')
    if (text.trim() !== '') parts.push(text)
  }
  return parts.length === 0 ? null : parts.join('\n\n')
}

export type FindingsSummary = {
  total: number
  errors: number
  warnings: number
  suggestions: number
  /** The first three distinct matched texts, errors first. */
  matches: string[]
}

type FindingLike = { severity?: unknown; match?: unknown }

function orderedBySeverity(findings: FindingLike[]): FindingLike[] {
  const rank = (finding: FindingLike): number =>
    isSeverity(finding.severity) ? SEVERITIES.indexOf(finding.severity) : -1
  return [...findings].sort((a, b) => rank(b) - rank(a))
}

/** Counts a detect payload's findings by severity and quotes the first few. */
export function summarizeFindings(payload: unknown): FindingsSummary {
  const summary: FindingsSummary = { total: 0, errors: 0, warnings: 0, suggestions: 0, matches: [] }
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

/** The status line for a scanned target, or undefined (clear it) with nothing found. */
export function statusText(summary: FindingsSummary, target: string): string | undefined {
  if (summary.total === 0) return undefined
  const parts: string[] = []
  if (summary.errors > 0) parts.push(count(summary.errors, 'error'))
  if (summary.warnings > 0) parts.push(count(summary.warnings, 'warning'))
  if (summary.suggestions > 0) parts.push(count(summary.suggestions, 'suggestion'))
  return `humanizer: ${parts.join(', ')} in ${target}`
}

/** The question the hold dialog asks: the error count and the first matches. */
export function holdQuestion(summary: FindingsSummary, target: string): string {
  const quoted = summary.matches.map(match => `"${match}"`).join(', ')
  const detail = quoted === '' ? '' : ` (${quoted})`
  return `humanizer found ${count(summary.errors, 'error-level finding')} in the ${target}${detail}. Send it anyway?`
}
