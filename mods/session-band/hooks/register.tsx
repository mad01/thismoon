import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import { composeStatusText, decisionSummary, isBeltDeny } from '../lib/band'

const beltDenies = atom({ plugin: 'session-band', key: 'beltDenies' } as const, 0)

type Logger = { ui: { log: (text: string, options: { to: 'debug' }) => void } }
type Failed<E, R> = ((e: E) => R) & { event: string; error: { kind: string; message?: string } }

// The one .catch every hook below carries: say why in the debug log, then
// let the chain beneath answer as if the hook were absent. The line never
// blocks a turn or a tool call.
const skipOnFailure = <E, R>($: Logger, e: E, next: Failed<E, R>): R => {
  const why = next.error.message === undefined ? next.error.kind : `${next.error.kind}: ${next.error.message}`
  $.ui.log(`session-band: ${next.event} skipped (${why})`, { to: 'debug' })
  return next(e)
}

// The text this module instance last pinned; null before its first pin. A
// hot reload starts over and pins again, and the engine keeps one status
// line per plugin either way.
let pinned: string | undefined | null = null

// Recomposes the line and pins it when it changed; undefined clears it.
async function refreshStatus($: EngineInterface): Promise<void> {
  const text = composeStatusText(await read($, beltDenies))
  if (pinned !== null && text === pinned) return
  pinned = text
  $.ui.status(text)
}

export const register: Register = on => {
  // The count lives in $.state and outlives a reload; the pin does not, so
  // a count carried across a hot reload is put back on screen here.
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    $.ui.log('session-band: loaded', { to: 'debug' })
    await refreshStatus($)
    return started
  }).catch(skipOnFailure)

  // belt answers PreToolUse as a settings hook beneath every mod; its deny
  // comes back from next(e) with a `belt[<guard>]: ` reason, possibly inside
  // the engine's own wrapping. Counted, never changed: the decision returns
  // exactly as belt made it. Every decision is described in the debug log so
  // its real shape can be read off `claude --debug`. The line is refreshed
  // on every call, not only on a deny, so a /clear that zeroed the count
  // takes the stale line down with it.
  on('classic.PreToolUse', async ($, e, next) => {
    const decision = await next(e)
    if (decision !== undefined) {
      $.ui.log(`session-band: PreToolUse ${e.tool} decision ${decisionSummary(decision)}`, { to: 'debug' })
    }
    if (isBeltDeny(decision)) await update($, beltDenies, n => n + 1)
    await refreshStatus($)
    return decision
  }).catch(skipOnFailure)
}
