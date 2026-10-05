export type LoomPaneRole = 'owner' | 'weaver' | 'none'

export type LoomPaneWorktree = {
  path: string
  branch: string | null
  head: string | null
  isDirty: boolean | null
  ahead: number | null
  age: string | null
}

declare module 'claude-code' {
  interface PluginState {
    'loom-pane': {
      role: LoomPaneRole
      canonical: string | null
      ownWorktree: string | null
      worktreeRoot: string | null
      worktrees: string[]
      repo: string | null
      weaveSeen: boolean
      isInteractive: boolean
      rows: LoomPaneWorktree[]
    }
  }
}
