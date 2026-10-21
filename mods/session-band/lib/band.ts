// Pure helpers behind the session-band mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx and the pi face import them.

// Every belt denial reason carries `belt[<guard>]: `, possibly behind the
// engine's own prefix on the same line.
const BELT_REASON = /belt\[[^\]]+\]:/

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * True when a tool call's result is a belt denial. A denial from a settings
 * hook reaches a plugin as an errored result whose text is the reason, so
 * only the first line counts: a failed command whose output mentions belt
 * does not.
 */
export function isBeltRefusal(result: unknown): boolean {
  if (!isRecord(result)) return false
  if (typeof result.deny === 'string') return BELT_REASON.test(result.deny)
  if (result.isError !== true || typeof result.text !== 'string') return false
  return BELT_REASON.test(result.text.trimStart().split('\n')[0] ?? '')
}

/**
 * The one line pinned under the prompt, or undefined (clear it) before the
 * first deny. The engine prefixes a status line with the mod's name, so the
 * text never repeats it.
 */
export function composeStatusText(beltDenies: number): string | undefined {
  return beltDenies > 0 ? `belt denies ${beltDenies}` : undefined
}
