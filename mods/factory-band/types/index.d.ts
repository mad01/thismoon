/** One factory worker as its spawn recorded it; `lastKind` is the kind of its last contract message. */
export type FactoryBandWorker = {
  agentId: string
  name: string
  worktree: string | null
  model: string | null
  lastKind: string | null
}

/** What one refresh read for one worker. */
export type FactoryBandFacts = {
  state: string
  model: string | null
  effort: string | null
}

/** What one refresh read for the run; a null count is a source that did not answer. */
export type FactoryBandProbe = {
  facts: Record<string, FactoryBandFacts>
  ledgerDenies: number | null
  inbox: number | null
}

/** One factory run; `workerIds` keeps every worker it spawned, dropped ones included. */
export type FactoryBandRun = {
  workers: FactoryBandWorker[]
  startedAt: number | null
  workerIds: string[]
  guardDenies: number
  probe: FactoryBandProbe | null
}

declare module 'claude-code' {
  interface PluginState {
    'factory-band': {
      run: FactoryBandRun
    }
  }
}
