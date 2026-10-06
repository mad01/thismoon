// Pure helpers behind the session-band mod: no `$`, no I/O, so `node --test`
// covers them without the engine. register.tsx is the only caller.

// Every belt denial reason carries `belt[<guard>]: `; other settings hooks
// deny with their own words. The engine may wrap the reason before a mod
// sees it (`PreToolUse:Bash hook error: belt[git-push-main]: ...`), so the
// marker is searched for anywhere in the text, never anchored.
const BELT_REASON = /belt\[[^\]]+\]:/

/** How many characters of a decision's JSON the debug line keeps. */
const DECISION_JSON_CAP = 120

/**
 * Whether a `classic.PreToolUse` decision is a denial belt wrote. A string
 * `deny` is read as the typed shape; an allow or an ask is never a denial;
 * any other object is searched as JSON, so a shape the types do not name
 * (a nested `permissionDecisionReason`, a `block`) still counts.
 */
export function isBeltDeny(decision: unknown): boolean {
  if (decision === null || typeof decision !== 'object') return false
  const { deny, allow, ask } = decision as { deny?: unknown; allow?: unknown; ask?: unknown }
  if (typeof deny === 'string') return BELT_REASON.test(deny)
  if (allow === true || typeof ask === 'string') return false
  return BELT_REASON.test(stringify(decision))
}

function stringify(value: unknown): string {
  try {
    return JSON.stringify(value) ?? ''
  } catch {
    return ''
  }
}

/** One debug line's worth of a decision: its keys and the head of its JSON. */
export function decisionSummary(decision: unknown): string {
  if (decision === null || typeof decision !== 'object') return `${typeof decision} ${String(decision)}`
  const keys = Object.keys(decision).join(',')
  const json = stringify(decision)
  const head = json.length > DECISION_JSON_CAP ? `${json.slice(0, DECISION_JSON_CAP)}...` : json
  return `keys [${keys}] ${head}`
}

/**
 * The one line pinned under the prompt, or undefined (clear it) before the
 * first deny. The engine prefixes a status line with the mod's name, so the
 * text never repeats it.
 */
export function composeStatusText(beltDenies: number): string | undefined {
  return beltDenies > 0 ? `belt denies ${beltDenies}` : undefined
}
