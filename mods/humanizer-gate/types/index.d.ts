export type HumanizerGateReport = {
  /** What was scanned: a file's basename, `PR body`, or `commit message`. */
  target: string
  total: number
  errors: number
  warnings: number
  suggestions: number
}

/** The texts a session denied with findings, so the same text sent again passes. */
export type HumanizerGateDenied = {
  /** The session the keys belong to; null when the engine gave no id. */
  sessionId: string | null
  /** textKey of each denied PR body or commit message, oldest first, at most 32. */
  keys: string[]
}

declare module 'claude-code' {
  interface PluginState {
    'humanizer-gate': {
      lastReport: HumanizerGateReport | null
      denied: HumanizerGateDenied
    }
  }
}
