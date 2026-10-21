import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import { composeStatusText, isBeltRefusal } from '../lib/band'

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

  // belt runs as a settings PreToolUse hook beneath every mod's tool.call
  // hook, so a belt deny comes back from next(e) as an errored result whose
  // text starts with the `belt[<guard>]: ` reason. Every caller counts: the
  // main loop and each subagent. Counted, never changed: the result returns
  // exactly as it came. The line is refreshed on every call, not only on a
  // deny, so a /clear that zeroed the count takes the stale line down with it.
  on('tool.call', async ($, e, next) => {
    const ran = await next(e)
    if (isBeltRefusal(ran)) {
      const count = await update($, beltDenies, n => n + 1)
      $.ui.log(`session-band: belt deny ${count} counted on ${e.tool}`, { to: 'debug' })
    }
    await refreshStatus($)
    return ran
  }).catch(skipOnFailure)
}
