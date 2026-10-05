import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { PanelPolicyMember, PanelPolicyTurn } from '../types'
import {
  addMember,
  bandText,
  classifyRole,
  completeMember,
  countsTowardFanout,
  decideModel,
  formatDuration,
  formatPaneRow,
  isBatchActive,
  isFanoutHoldDue,
  isFanoutToastDue,
  memberLabel,
  paneRows,
  readConfig,
  staleAfterMs,
} from '../lib/policy'

const PANE = 'panel'
const HOLD_PROCEED = 'Proceed'
const HOLD_CANCEL = 'Cancel'

/** How often the band and pane redraw while a batch is on screen. */
const TICK_MS = 1_000

const members = atom({ plugin: 'panel-policy', key: 'members' } as const, [] as PanelPolicyMember[])
const turn = atom({ plugin: 'panel-policy', key: 'turn' } as const, { id: null, spawns: 0 } as PanelPolicyTurn)

type Logger = { ui: { log: (text: string, options: { to: 'debug' }) => void } }
type Failed<E, R> = ((e: E) => R) & { event: string; error: { kind: string; message?: string } }

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. A spawn is never
// denied by a failure here, a draw never blocked.
const skipOnFailure = <E, R>($: Logger, e: E, next: Failed<E, R>): R => {
  const why = next.error.message === undefined ? next.error.kind : `${next.error.kind}: ${next.error.message}`
  $.ui.log(`panel-policy: ${next.event} skipped (${why})`, { to: 'debug' })
  return next(e)
}

const describe = (err: unknown): string => (err instanceof Error ? err.message : String(err))

// Marks the member `id` names as returned, from whichever signal came first:
// the subagent's turn.complete or the classic SubagentStop event. The second
// one finds the member already returned and only says so in the debug log.
async function markReturned($: EngineInterface, id: string, signal: string): Promise<void> {
  const now = await $.clock.now()
  const known = (await read($, members)).find(m => m.id === id)
  if (known === undefined) {
    const running = (await read($, members)).filter(m => m.endedAt === null).map(m => m.id)
    $.ui.log(`panel-policy: ${signal} for ${id} matched no member (running: ${running.join(',') || 'none'})`, { to: 'debug' })
    return
  }
  if (known.endedAt !== null) {
    $.ui.log(`panel-policy: ${signal} for ${id} found "${known.label}" already returned`, { to: 'debug' })
    return
  }
  await update($, members, list => completeMember(list, id, now))
  $.ui.log(
    `panel-policy: ${known.role} "${known.label}" returned on ${signal} (${id}, ${formatDuration(now - known.startedAt)})`,
    { to: 'debug' },
  )
}

export const register: Register = (on, options) => {
  const config = readConfig(options)
  const staleMs = staleAfterMs(config)

  // Starts the redraw ticker that keeps durations and the linger moving;
  // the ticker cancels itself once the batch is off the band. Set in
  // session.start, which runs again on every reload.
  let startTicking: () => void = () => {}

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    await $.command.register({ name: PANE, description: 'open the panel members pane', immediate: true })

    let ticker: { cancel: () => void } | null = null
    const tick = async (): Promise<void> => {
      if (isBatchActive(await read($, members), await $.clock.now(), staleMs)) {
        $.ui.invalidate('ui.render')
        return
      }
      ticker?.cancel()
      ticker = null
    }
    startTicking = () => {
      if (ticker === null) ticker = $.clock.every(TICK_MS, () => { void tick() })
    }
    // A batch may be mid-flight after a reload; the first tick settles it.
    startTicking()

    $.ui.log(
      `panel-policy: loaded (enforce ${config.enforce}, reviewModel ${config.reviewModel}, overrideFrom ${config.overrideFrom.join(',')}, fanoutToast ${config.fanoutToast}, fanoutHold ${config.fanoutHold}, staleMinutes ${config.staleMinutes})`,
      { to: 'debug' },
    )
    return started
  }).catch(skipOnFailure)

  on('turn.start', async ($, e, next) => {
    await update($, turn, () => ({ id: e.turnId, spawns: 0 }))
    return next(e)
  }).catch(skipOnFailure)

  on('agent.spawn', async ($, e, next) => {
    if (countsTowardFanout(e)) {
      const counted = await update($, turn, t => ({ id: t.id, spawns: t.spawns + 1 }))
      const count = counted.spawns
      if (isFanoutHoldDue(count, config)) {
        let answer = HOLD_PROCEED
        try {
          answer = await $.ui.ask(`${count} agents spawned this turn. Proceed?`, [HOLD_PROCEED, HOLD_CANCEL])
        } catch (err) {
          $.ui.log(`panel-policy: fan-out hold not asked (${describe(err)}); proceeding`, { to: 'debug' })
        }
        if (answer === HOLD_CANCEL) return { deny: 'panel-policy: spawn cancelled at the fan-out hold' }
      } else if (isFanoutToastDue(count, config)) {
        $.ui.toast(`${count} agents spawned this turn`)
      }
    }

    const role = classifyRole(e)
    const model = decideModel(
      { role, model: e.model, subagentType: e.subagentType, isTeammate: e.isTeammate, fork: e.fork },
      config,
    )
    if (model !== null) {
      $.ui.log(
        `panel-policy: ${e.subagentType} "${e.description}" is a reviewer; model ${e.model ?? 'inherit'} -> ${model}`,
        { to: 'debug' },
      )
    }
    const started = model === null ? await next(e) : await next({ ...e, model })

    if (started.deny !== undefined || e.isTeammate === true) return started
    if (started.agentId === undefined) {
      $.ui.log(`panel-policy: spawn "${e.description}" resolved without an agentId; not tracked`, { to: 'debug' })
      return started
    }
    const member: PanelPolicyMember = {
      id: started.agentId,
      role,
      label: memberLabel(e.description, e.subagentType),
      startedAt: await $.clock.now(),
      endedAt: null,
      turn: (await read($, turn)).id,
    }
    await update($, members, list => addMember(list, member))
    startTicking()
    return started
  }).catch(skipOnFailure)

  on('turn.complete', async ($, e, next) => {
    if (e.agentId !== undefined) await markReturned($, e.agentId, 'turn.complete')
    return next(e)
  }).catch(skipOnFailure)

  // The settings-hook view of the same end: a background subagent's stop
  // reaches the main session here with the id spelled `agent_id`.
  on('classic.SubagentStop', async ($, e, next) => {
    await markReturned($, e.agent_id, 'SubagentStop')
    return next(e)
  }).catch(skipOnFailure)

  on('command.run', { command: PANE }, async $ => {
    const opened = await $.ui.open({ id: PANE, title: 'Panel members', focus: true, closeOnEscape: true })
    return {
      text: opened.isPlaced ? 'panel members pane open; Esc closes it' : 'panel members pane waits for a wider terminal',
    }
  }).catch(skipOnFailure)

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const below = await next(e)
    if (e.props.hasSurvey) return below
    const line = bandText(await read($, members), await $.clock.now(), config)
    if (line === null) return below
    const { Box, Text } = $.ui.resolve(e)
    return (
      <Box flexDirection="column">
        <Text dimColor wrap="truncate-end">
          {line}
        </Text>
        {below}
      </Box>
    )
  }).catch(skipOnFailure)

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text } = $.ui.resolve(e)
    const room = Math.max(1, (e.viewport?.rows ?? 24) - 4)
    const rows = paneRows(await read($, members), await $.clock.now(), room, staleMs)
    const labelWidth = Math.max(12, Math.min(60, e.props.bodyColumns - 28))
    return (
      <Box flexDirection="column">
        {rows.length === 0 && <Text dimColor>No agents spawned yet.</Text>}
        {rows.map(row => (
          <Text dimColor={row.status === 'done'} wrap="truncate-end">
            {formatPaneRow(row, labelWidth)}
          </Text>
        ))}
      </Box>
    )
  }).catch(skipOnFailure)
}
