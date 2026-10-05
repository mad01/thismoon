export type SessionBandAgent = 'working' | 'waiting' | 'idle'

export type SessionBandPage = { id: string; url: string | null }

declare module 'claude-code' {
  interface PluginState {
    'session-band': {
      agent: SessionBandAgent
      worklogKey: string | null
      phase: string | null
      presentPage: SessionBandPage | null
      beltDenies: number
      eventsCursor: string | null
      isCompacted: boolean
    }
  }
}
