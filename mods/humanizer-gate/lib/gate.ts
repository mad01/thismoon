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
