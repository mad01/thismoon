export type SessionBandPage = { id: string; url: string | null }

declare module 'claude-code' {
  interface PluginState {
    'session-band': {
      isWorking: boolean
      isWaiting: boolean
      worklogKey: string | null
      phase: string | null
      presentPage: SessionBandPage | null
      beltDenies: number
      isCompacted: boolean
    }
  }
}
