import { atom, read, update } from 'claude-code'
import type { Register } from 'claude-code'

import type { SessionBandPage } from '../types'
import {
  compactContextBlock,
  composeBandLines,
  decisionSummary,
  findPhaseMarker,
  isBeltDeny,
  parsePresentResult,
  parseWorklogList,
  repoNameFrom,
  shouldToastDone,
} from '../lib/band'
import type { BandSnapshot } from '../lib/band'

const WORKLOG_RUN_TIMEOUT_MS = 2_000

const isWaiting = atom({ plugin: 'session-band', key: 'isWaiting' } as const, false)
const worklogKey = atom({ plugin: 'session-band', key: 'worklogKey' } as const, null as string | null)
const phase = atom({ plugin: 'session-band', key: 'phase' } as const, null as string | null)
const presentPage = atom(
  { plugin: 'session-band', key: 'presentPage' } as const,
  null as SessionBandPage | null,
)
const beltDenies = atom({ plugin: 'session-band', key: 'beltDenies' } as const, 0)
const isCompacted = atom({ plugin: 'session-band', key: 'isCompacted' } as const, false)

type Logger = { ui: { log: (text: string, options: { to: 'debug' }) => void } }
type Failed<E, R> = ((e: E) => R) & { event: string; error: { kind: string; message?: string } }

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. The band never
// blocks a turn, a tool call or a draw.
const skipOnFailure = <E, R>($: Logger, e: E, next: Failed<E, R>): R => {
  const why = next.error.message === undefined ? next.error.kind : `${next.error.kind}: ${next.error.message}`
  $.ui.log(`session-band: ${next.event} skipped (${why})`, { to: 'debug' })
  return next(e)
}

const describe = (err: unknown): string => (err instanceof Error ? err.message : String(err))

type StateDollar = Parameters<typeof read>[0]

// Every signal the band and the compact block draw from, read in one go.
async function snapshot($: StateDollar): Promise<BandSnapshot> {
  return {
    isWaiting: await read($, isWaiting),
    worklogKey: await read($, worklogKey),
    phase: await read($, phase),
    presentPage: await read($, presentPage),
    beltDenies: await read($, beltDenies),
  }
}

// Writes the waiting flag only when it changes: every write redraws the band.
async function setWaiting($: StateDollar, value: boolean): Promise<void> {
  if ((await read($, isWaiting)) !== value) await update($, isWaiting, () => value)
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('session-band: loaded', { to: 'debug' })

    const detectWorklogKey = async (): Promise<void> => {
      if ((await read($, worklogKey)) !== null) return
      let commonDir: string | null = null
      try {
        const git = await $.process.run(
          ['git', 'rev-parse', '--path-format=absolute', '--git-common-dir'],
          { cwd: e.cwd, timeoutMs: WORKLOG_RUN_TIMEOUT_MS },
        )
        if (git.exitCode === 0) commonDir = git.stdout
      } catch {
        commonDir = null
      }
      const repo = repoNameFrom(commonDir, e.cwd)
      const home = await $.env.get('HOME')
      const installed = home === undefined ? null : `${home}/code/bin/worklog`
      const worklog = installed !== null && (await $.fs.exists(installed)) ? installed : 'worklog'
      const listed = await $.process.run([worklog, 'list', '--status', 'active', '--repo', repo], {
        cwd: e.cwd,
        timeoutMs: WORKLOG_RUN_TIMEOUT_MS,
      })
      if (listed.exitCode !== 0) return
      const key = parseWorklogList(listed.stdout)
      if (key !== null) await update($, worklogKey, () => key)
    }

    // Outlives this dispatch, so it starts from the clock rather than inside
    // the hook: the first prompt never waits on a child process.
    $.clock.after(0, () => {
      detectWorklogKey().catch(err => {
        $.ui.log(`session-band: worklog key not detected (${describe(err)})`, { to: 'debug' })
      })
    })

    return started
  }).catch(skipOnFailure)

  on('turn.complete', async ($, e, next) => {
    if (e.agentId === undefined) {
      await setWaiting($, false)
      const marker = findPhaseMarker(e.answer)
      if (marker !== null && marker !== (await read($, phase))) await update($, phase, () => marker)
      if (shouldToastDone(e.reason, e.durationMs)) {
        $.ui.toast(`done (${Math.round(e.durationMs / 1000)} s)`)
      }
    }
    return next(e)
  }).catch(skipOnFailure)

  // The person is being asked: a permission prompt or an MCP elicitation on
  // the main thread. A subagent's prompt is left to the engine's own spinner.
  on(
    'classic.Notification',
    { notification_type: ['permission_prompt', 'elicitation_dialog'] },
    async ($, e, next) => {
      if (e.agent_id === undefined) {
        await setWaiting($, true)
        if (e.notification_type === 'permission_prompt') $.ui.toast('needs input')
      }
      return next(e)
    },
  ).catch(skipOnFailure)

  // Answered: the tool ran, or the person refused it.
  on('classic.PostToolUse', async ($, e, next) => {
    if (e.agent_id === undefined) await setWaiting($, false)
    return next(e)
  }).catch(skipOnFailure)

  on('classic.PermissionDenied', async ($, e, next) => {
    if (e.agent_id === undefined) await setWaiting($, false)
    return next(e)
  }).catch(skipOnFailure)

  // belt answers PreToolUse as a settings hook beneath every mod; its deny
  // comes back from next(e) with a `belt[<guard>]: ` reason, possibly inside
  // the engine's own wrapping. Counted, never changed: the decision returns
  // exactly as belt made it. Every decision is described in the debug log so
  // its real shape can be read off `claude --debug`.
  on('classic.PreToolUse', async ($, e, next) => {
    const decision = await next(e)
    if (decision !== undefined) {
      $.ui.log(`session-band: PreToolUse ${e.tool} decision ${decisionSummary(decision)}`, { to: 'debug' })
    }
    if (isBeltDeny(decision)) await update($, beltDenies, n => n + 1)
    return decision
  }).catch(skipOnFailure)

  on('tool.call', { tool: 'mcp__worklog__worklog_checkpoint' }, async ($, e, next) => {
    const key = typeof e.key === 'string' ? e.key.trim() : ''
    if (key !== '' && key !== (await read($, worklogKey))) await update($, worklogKey, () => key)
    return next(e)
  }).catch(skipOnFailure)

  on(
    'tool.call',
    { tool: ['mcp__present__present_create', 'mcp__present__present_update'] },
    async ($, e, next) => {
      const answered = await next(e)
      if (answered.deny === undefined && answered.isError !== true) {
        const inputId = typeof e.id === 'string' ? e.id : null
        const page = parsePresentResult(answered.result, inputId, answered.text)
        if (page !== null) await update($, presentPage, () => page)
      }
      return answered
    },
  ).catch(skipOnFailure)

  // A compaction re-reads the first message's context, so the block goes in
  // through prompt.context alone; this only arms it.
  on('classic.SessionStart', { source: 'compact' }, async ($, e, next) => {
    if (!(await read($, isCompacted))) await update($, isCompacted, () => true)
    return next(e)
  }).catch(skipOnFailure)

  on('prompt.context', async ($, e, next) => {
    const below = await next(e)
    if (!(await read($, isCompacted))) return below
    const block = { name: 'session-band', text: compactContextBlock(await snapshot($)) }
    await update($, isCompacted, () => false)
    return { ...below, blocks: [...below.blocks, block] }
  }).catch(skipOnFailure)

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const below = await next(e)
    if (e.props.hasSurvey) return below
    const lines = composeBandLines(await snapshot($), e.props.isWorking, e.props.bodyColumns)
    const { Box, Text } = $.ui.resolve(e)
    return (
      <Box flexDirection="column">
        <Text dimColor wrap="truncate-end">
          {lines[0]}
        </Text>
        {lines[1] === undefined ? null : (
          <Text dimColor wrap="truncate-end">
            {lines[1]}
          </Text>
        )}
        {below}
      </Box>
    )
  }).catch(skipOnFailure)
}
