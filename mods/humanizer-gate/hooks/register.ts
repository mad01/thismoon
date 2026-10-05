import { atom, update } from 'claude-code'
import type { Register } from 'claude-code'

import type { HumanizerGateReport } from '../types'
import {
  CANCELLED_REASON,
  DETECT_BUDGET_MS,
  HOLD_OPTIONS,
  basename,
  extractCommitMessage,
  holdQuestion,
  isProseFile,
  parseDetectOutput,
  readConfig,
  statusText,
} from '../lib/gate'
import type { FindingsSummary, Severity } from '../lib/gate'

const lastReport = atom(
  { plugin: 'humanizer-gate', key: 'lastReport' } as const,
  null as HumanizerGateReport | null,
)

type Logger = { ui: { log: (text: string, options: { to: 'debug' }) => void } }
type Failed<E, R> = ((e: E) => R) & { event: string; error: { kind: string; message?: string } }

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. The gate never
// blocks a write, a PR or a commit on its own failure.
const skipOnFailure = <E, R>($: Logger, e: E, next: Failed<E, R>): R => {
  const why = next.error.message === undefined ? next.error.kind : `${next.error.kind}: ${next.error.message}`
  $.ui.log(`humanizer-gate: ${next.event} skipped (${why})`, { to: 'debug' })
  return next(e)
}

const describe = (err: unknown): string => (err instanceof Error ? err.message : String(err))

type Detector = Logger & {
  process: {
    run: (
      argv: readonly string[],
      init?: { stdin?: string; timeoutMs?: number },
    ) => Promise<{ exitCode: number; stdout: string; stderr: string }>
  }
  clock: { sleep: (ms: number) => Promise<void> }
}

type Reporter = Parameters<typeof update>[0] & { ui: { status: (text: string | undefined) => void } }

type Asker = Logger & {
  ui: {
    ask: (
      question: string,
      options?: readonly string[] | { options?: readonly string[]; header?: string },
    ) => Promise<string>
  }
}

type DetectSource = { file: string } | { text: string }

// One `humanizer detect --json` run under the budget: a file by path, or
// text on stdin. Null when it timed out, failed, or wrote no payload; the
// debug log says which.
async function detect(
  $: Detector,
  binary: string,
  source: DetectSource,
  level: Severity,
): Promise<FindingsSummary | null> {
  const argv = [binary, 'detect', '--json', '--min-severity', level]
  const init: { stdin?: string; timeoutMs: number } = { timeoutMs: DETECT_BUDGET_MS }
  if ('file' in source) argv.push(source.file)
  else init.stdin = source.text
  const outcome = await Promise.race([
    $.process.run(argv, init).then(ran => ({ kind: 'ran' as const, ran })),
    $.clock.sleep(DETECT_BUDGET_MS).then(() => ({ kind: 'timeout' as const })),
  ])
  if (outcome.kind === 'timeout') {
    $.ui.log(`humanizer-gate: detect past ${DETECT_BUDGET_MS} ms, no report`, { to: 'debug' })
    return null
  }
  if (outcome.ran.exitCode !== 0) {
    const stderr = outcome.ran.stderr.trim().slice(0, 200)
    $.ui.log(`humanizer-gate: detect exit ${outcome.ran.exitCode} (${stderr})`, { to: 'debug' })
    return null
  }
  const summary = parseDetectOutput(outcome.ran.stdout)
  if (summary === null) $.ui.log('humanizer-gate: detect wrote no JSON payload', { to: 'debug' })
  return summary
}

// Pins the status line for `target` (clears it on a clean scan) and keeps
// the counts in state. A failed detect leaves both as they were.
async function report($: Reporter, summary: FindingsSummary | null, target: string): Promise<void> {
  if (summary === null) return
  $.ui.status(statusText(summary, target))
  const { total, errors, warnings, suggestions } = summary
  await update($, lastReport, () => ({ target, total, errors, warnings, suggestions }))
}

// True only when the person picked Cancel. A dialog nobody can answer (a
// `-p` run) or one dismissed lets the call through.
async function isCancelled($: Asker, summary: FindingsSummary, target: string): Promise<boolean> {
  try {
    const answer = await $.ui.ask(holdQuestion(summary, target), { options: HOLD_OPTIONS, header: 'humanizer' })
    return answer === HOLD_OPTIONS[1]
  } catch (err) {
    $.ui.log(`humanizer-gate: hold not asked (${describe(err)}), proceeding`, { to: 'debug' })
    return false
  }
}

export const register: Register = (on, options) => {
  const config = readConfig(options)
  // The binary's path once session.start found it. Null until then and when
  // it is missing, and every hook below is then a pass-through.
  let binary: string | null = null

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const home = await $.env.get('HOME')
    const candidate = `${home ?? '~'}/code/bin/humanizer`
    binary = home !== undefined && (await $.fs.exists(candidate)) ? candidate : null
    if (binary === null) {
      $.ui.log(`humanizer-gate: ${candidate} missing, every hook passes through`, { to: 'debug' })
    } else {
      $.ui.log(`humanizer-gate: loaded (min severity ${config.minSeverity})`, { to: 'debug' })
    }
    return started
  }).catch(skipOnFailure)

  // The write happens first; the scan only reports on what landed. (This
  // build registers no MultiEdit tool: its types name Write and Edit only.)
  on('tool.call', { tool: ['Write', 'Edit'] }, async ($, e, next) => {
    const answered = await next(e)
    const bin = binary
    if (bin === null || answered.deny !== undefined || answered.isError === true) return answered
    if (!isProseFile(e.file_path)) return answered
    await report($, await detect($, bin, { file: e.file_path }, config.minSeverity), basename(e.file_path))
    return answered
  }).catch(skipOnFailure)

  on(
    'tool.call',
    { tool: ['mcp__gh_com__create_pull_request', 'mcp__gh_com__update_pull_request'] },
    async ($, e, next) => {
      const bin = binary
      const body = typeof e.body === 'string' ? e.body : ''
      if (bin === null || body.trim() === '') return next(e)
      const summary = await detect($, bin, { text: body }, config.minSeverity)
      await report($, summary, 'PR body')
      const held = summary !== null && summary.errors > 0 && config.holdOnError
      if (held && (await isCancelled($, summary, 'PR body'))) return { deny: CANCELLED_REASON }
      return next(e)
    },
  ).catch(skipOnFailure)

  on('tool.call', { tool: 'Bash', command: /\bgit\b[^|;&\n]*?\bcommit\b/ }, async ($, e, next) => {
    const bin = binary
    const message = extractCommitMessage(e.command)
    if (bin === null || message === null) return next(e)
    const summary = await detect($, bin, { text: message }, config.minSeverity)
    await report($, summary, 'commit message')
    const held = summary !== null && summary.errors > 0 && config.holdOnCommitError
    if (held && (await isCancelled($, summary, 'commit message'))) return { deny: CANCELLED_REASON }
    return next(e)
  }).catch(skipOnFailure)
}
