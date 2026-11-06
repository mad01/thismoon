import { atom, read, update } from 'claude-code'
import type { Caught, EngineInterface, Register } from 'claude-code'

import type { HumanizerGateDenied, HumanizerGateReport } from '../types'
import {
  CANCELLED_REASON,
  DETECT_BUDGET_MS,
  HOLD_OPTIONS,
  basename,
  extractCommitMessage,
  holdQuestion,
  isProseFile,
  judge,
  parseDetectOutput,
  readConfig,
  rememberDenial,
  statusText,
} from '../lib/gate'
import type { FindingsSummary, GateConfig, Severity } from '../lib/gate'

const lastReport = atom(
  { plugin: 'humanizer-gate', key: 'lastReport' } as const,
  null as HumanizerGateReport | null,
)

// The texts this session denied with findings. `/clear` resets it; the
// session id keeps an in-process `/resume` from carrying another session's
// keys over.
const denied = atom(
  { plugin: 'humanizer-gate', key: 'denied' } as const,
  { sessionId: null, keys: [] } as HumanizerGateDenied,
)

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. The gate never
// blocks a write, a PR or a commit on its own failure.
const skipOnFailure = <E, R>($: EngineInterface, e: E, next: ((e: E) => R) & Caught): R => {
  const { kind, message } = next.error
  const why = message === undefined ? kind : `${kind}: ${message}`
  $.ui.log(`humanizer-gate: hook skipped (${why})`, { to: 'debug' })
  return next(e)
}

const describe = (err: unknown): string => (err instanceof Error ? err.message : String(err))

type DetectSource = { file: string } | { text: string }

// One `humanizer detect --json` run under the budget: a file by path, or
// text on stdin. Null when it timed out, failed, or wrote no payload; the
// debug log says which. `timeoutMs` kills the child and rejects at the
// budget, so there is no second clock to race it against.
async function detect(
  $: EngineInterface,
  binary: string,
  source: DetectSource,
  level: Severity,
): Promise<FindingsSummary | null> {
  const argv = [binary, 'detect', '--json', '--min-severity', level]
  const init: { stdin?: string; timeoutMs: number } = { timeoutMs: DETECT_BUDGET_MS }
  if ('file' in source) argv.push(source.file)
  else init.stdin = source.text
  let ran: { exitCode: number; stdout: string; stderr: string }
  try {
    ran = await $.process.run(argv, init)
  } catch (err) {
    $.ui.log(`humanizer-gate: detect did not finish (${describe(err)})`, { to: 'debug' })
    return null
  }
  if (ran.exitCode !== 0) {
    const stderr = ran.stderr.trim().slice(0, 200)
    $.ui.log(`humanizer-gate: detect exit ${ran.exitCode} (${stderr})`, { to: 'debug' })
    return null
  }
  const summary = parseDetectOutput(ran.stdout)
  if (summary === null) $.ui.log('humanizer-gate: detect wrote no JSON payload', { to: 'debug' })
  return summary
}

// Pins the status line for `target` (clears it on a clean scan) and keeps
// the counts in state. A failed detect leaves both as they were.
async function report($: EngineInterface, summary: FindingsSummary | null, target: string): Promise<void> {
  if (summary === null) return
  $.ui.status(statusText(summary, target))
  const { total, errors, warnings, suggestions } = summary
  await update($, lastReport, () => ({ target, total, errors, warnings, suggestions }))
}

// True only when the person picked Cancel. A dialog nobody can answer (a
// `-p` run) or one dismissed lets the call through.
async function isCancelled($: EngineInterface, summary: FindingsSummary, target: string): Promise<boolean> {
  try {
    const answer = await $.ui.ask(holdQuestion(summary, target), { options: HOLD_OPTIONS, header: 'humanizer' })
    return answer === HOLD_OPTIONS[1]
  } catch (err) {
    $.ui.log(`humanizer-gate: hold not asked (${describe(err)}), proceeding`, { to: 'debug' })
    return false
  }
}

async function sessionId($: EngineInterface): Promise<string | null> {
  try {
    return await $.session.id()
  } catch {
    return null
  }
}

// The keys session `id` denied; another session's keys count as none.
const keysOf = (stored: HumanizerGateDenied, id: string | null): string[] =>
  stored.sessionId === id ? stored.keys : []

// Scans a PR body or commit message and answers the deny reason, or null to
// send it. Error-level findings with the hold on ask Proceed / Cancel as
// before; any other finding at warning or above is denied once with the
// findings as the reason, and the same text sent again passes.
async function gate(
  $: EngineInterface,
  binary: string,
  config: GateConfig,
  text: string,
  target: string,
  isHolding: boolean,
): Promise<string | null> {
  const summary = await detect($, binary, { text }, config.minSeverity)
  await report($, summary, target)
  const id = await sessionId($)
  const verdict = judge({
    summary,
    target,
    text,
    isHolding,
    isFeedback: config.feedbackWarnings,
    denied: keysOf(await read($, denied), id),
  })
  if (verdict.kind === 'hold') return (await isCancelled($, verdict.summary, target)) ? CANCELLED_REASON : null
  if (verdict.kind === 'resubmitted') {
    $.ui.log(`humanizer-gate: ${target} sent again unchanged, passing`, { to: 'debug' })
    return null
  }
  if (verdict.kind === 'pass') return null
  const { key } = verdict
  await update($, denied, stored => ({ sessionId: id, keys: rememberDenial(keysOf(stored, id), key) }))
  return verdict.reason
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
      const feedback = config.feedbackWarnings ? 'on' : 'off'
      $.ui.log(`humanizer-gate: loaded (min severity ${config.minSeverity}, feedback ${feedback})`, { to: 'debug' })
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
      const reason = await gate($, bin, config, body, 'PR body', config.holdOnError)
      return reason === null ? next(e) : { deny: reason }
    },
  ).catch(skipOnFailure)

  // A loose prefilter; extractCommitMessage decides whether a segment of
  // the command is really `git commit` and reads only that segment.
  on('tool.call', { tool: 'Bash', command: /\bgit\b[^|;&\n]*?\bcommit\b/ }, async ($, e, next) => {
    const bin = binary
    const message = extractCommitMessage(e.command)
    if (bin === null || message === null) return next(e)
    const reason = await gate($, bin, config, message, 'commit message', config.holdOnCommitError)
    return reason === null ? next(e) : { deny: reason }
  }).catch(skipOnFailure)
}
