/** How many belt denials this session has seen; zero keeps the line clear. */
export type SessionBandDenies = number

declare module 'claude-code' {
  interface PluginState {
    'session-band': {
      beltDenies: SessionBandDenies
    }
  }
}
