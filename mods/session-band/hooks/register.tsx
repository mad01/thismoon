import { atom, read, update } from 'claude-code'
import type { Register } from 'claude-code'

import type { SessionBandAgent, SessionBandPage } from '../types'
import {
  LONG_TURN_MS,
  PROBE_BUDGET_MS,
  beltEventsUrl,
  compactContextBlock,
  composeBandLines,
  countBeltDenies,
  eventsCursorAt,
  findPhaseMarker,
  parsePresentResult,
  parseWorklogList,
  repoNameFrom,
} from '../lib/band'
import type { BandSnapshot } from '../lib/band'

const POLL_EVERY_MS = 15_000
const WORKLOG_RUN_TIMEOUT_MS = 2_000

const agent = atom({ plugin: 'session-band', key: 'agent' } as const, 'idle' as SessionBandAgent)
const worklogKey = atom({ plugin: 'session-band', key: 'worklogKey' } as const, null as string | null)
const phase = atom({ plugin: 'session-band', key: 'phase' } as const, null as string | null)
const presentPage = atom(
  { plugin: 'session-band', key: 'presentPage' } as const,
  null as SessionBandPage | null,
)
const beltDenies = atom({ plugin: 'session-band', key: 'beltDenies' } as const, 0)
const eventsCursor = atom({ plugin: 'session-band', key: 'eventsCursor' } as const, null as string | null)
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

// Every signal the band and the compact block draw from, read in one go.
async function snapshot($: Parameters<typeof read>[0]): Promise<BandSnapshot> {
  return {
    agent: await read($, agent),
    worklogKey: await read($, worklogKey),
    phase: await read($, phase),
    presentPage: await read($, presentPage),
    beltDenies: await read($, beltDenies),
  }
}

export const register: Register = on => {
  // Set while a belt poll is in flight, so a slow events service never
  // stacks requests; reset on every settle.
  let isPolling = false
  let wasEventsDown = false

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('session-band: loaded', { to: 'debug' })

    if ((await read($, eventsCursor)) === null) {
      const now = await $.clock.now()
      await update($, eventsCursor, () => eventsCursorAt(now))
    }

    const detectWorklogKey = async (): Promise<void> => {
      if ((await read($, worklogKey)) !== null) return
      const home = await $.env.get('HOME')
      if (home === undefined) return
      let commonDir: string | null = null
      try {
        const git = await $.process.run(['git', 'rev-parse', '--git-common-dir'], {
          cwd: e.cwd,
          timeoutMs: WORKLOG_RUN_TIMEOUT_MS,
        })
        if (git.exitCode === 0) commonDir = git.stdout
      } catch {
        commonDir = null
      }
      const repo = repoNameFrom(commonDir, e.cwd)
      const listed = await $.process.run(
        [`${home}/code/bin/worklog`, 'list', '--status', 'active', '--repo', repo],
        { cwd: e.cwd, timeoutMs: WORKLOG_RUN_TIMEOUT_MS },
      )
      if (listed.exitCode !== 0) return
      const key = parseWorklogList(listed.stdout)
      if (key !== null) await update($, worklogKey, () => key)
    }

    const pollBeltDenies = async (): Promise<void> => {
      if (isPolling) return
      isPolling = true
      try {
        const url = beltEventsUrl(await read($, eventsCursor))
        const outcome = await Promise.race([
          $.http.fetch(url).then(response => ({ kind: 'response' as const, response })),
          $.clock.sleep(PROBE_BUDGET_MS).then(() => ({ kind: 'timeout' as const })),
        ])
        if (outcome.kind === 'timeout' || !outcome.response.ok) {
          wasEventsDown = true
          return
        }
        if (wasEventsDown) {
          wasEventsDown = false
          $.ui.log('session-band: events service reachable again', { to: 'debug' })
        }
        const tally = countBeltDenies(JSON.parse(outcome.response.text))
        if (tally.newestId !== null) {
          const newest = tally.newestId
          await update($, eventsCursor, () => newest)
        }
        if (tally.denies > 0) await update($, beltDenies, n => n + tally.denies)
      } catch (err) {
        if (!wasEventsDown) {
          $.ui.log(`session-band: belt poll off (${describe(err)})`, { to: 'debug' })
        }
        wasEventsDown = true
      } finally {
        isPolling = false
      }
    }

    // Both outlive this dispatch, so they start from the clock rather than
    // inside the hook: the first prompt never waits on a child process.
    $.clock.after(0, () => {
      detectWorklogKey().catch(err => {
        $.ui.log(`session-band: worklog key not detected (${describe(err)})`, { to: 'debug' })
      })
    })
    $.clock.every(POLL_EVERY_MS, () => {
      void pollBeltDenies()
    })

    return started
  }).catch(skipOnFailure)

  on('turn.start', async ($, e, next) => {
    await update($, agent, () => 'working')
    return next(e)
  }).catch(skipOnFailure)

  on('turn.complete', async ($, e, next) => {
    if (e.agentId === undefined) {
      await update($, agent, () => 'idle')
      const marker = findPhaseMarker(e.answer)
      if (marker !== null) await update($, phase, () => marker)
      if (e.durationMs > LONG_TURN_MS) {
        $.ui.toast(`done (${Math.round(e.durationMs / 1000)} s)`)
      }
    }
    return next(e)
  }).catch(skipOnFailure)

  on(
    'classic.Notification',
    { notification_type: ['permission_prompt', 'elicitation_dialog'] },
    async ($, e, next) => {
      await update($, agent, () => 'waiting')
      if (e.notification_type === 'permission_prompt') $.ui.toast('needs input')
      return next(e)
    },
  ).catch(skipOnFailure)

  // A tool call means the model is working, before the permission prompt
  // and again once it was answered and the tool ran.
  on('tool.call', async ($, e, next) => {
    await update($, agent, () => 'working')
    const answered = await next(e)
    await update($, agent, () => 'working')
    return answered
  }).catch(skipOnFailure)

  on('tool.call', { tool: 'mcp__worklog__worklog_checkpoint' }, async ($, e, next) => {
    const key = typeof e.key === 'string' ? e.key.trim() : ''
    if (key !== '') await update($, worklogKey, () => key)
    return next(e)
  }).catch(skipOnFailure)

  on(
    'tool.call',
    { tool: ['mcp__present__present_create', 'mcp__present__present_update'] },
    async ($, e, next) => {
      const answered = await next(e)
      if (answered.deny === undefined) {
        const page = parsePresentResult(answered.result, typeof e.id === 'string' ? e.id : null)
        if (page !== null) await update($, presentPage, () => page)
      }
      return answered
    },
  ).catch(skipOnFailure)

  on('classic.SessionStart', { source: 'compact' }, async ($, e, next) => {
    const below = (await next(e)) ?? {}
    await update($, isCompacted, () => true)
    const block = compactContextBlock(await snapshot($))
    return { ...below, additionalContext: [...(below.additionalContext ?? []), block] }
  }).catch(skipOnFailure)

  on('prompt.context', async ($, e, next) => {
    const below = await next(e)
    if (!(await read($, isCompacted))) return below
    const block = { name: 'session-band', text: compactContextBlock(await snapshot($)) }
    return { ...below, blocks: [...below.blocks, block] }
  }).catch(skipOnFailure)

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const below = await next(e)
    if (e.props.hasSurvey) return below
    const lines = composeBandLines(await snapshot($), e.props.bodyColumns)
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
