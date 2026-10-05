export type HumanizerGateReport = {
  /** What was scanned: a file's basename, `PR body`, or `commit message`. */
  target: string
  total: number
  errors: number
  warnings: number
  suggestions: number
}

declare module 'claude-code' {
  interface PluginState {
    'humanizer-gate': {
      lastReport: HumanizerGateReport | null
    }
  }
}
