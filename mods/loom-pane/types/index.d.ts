export type LoomPaneRole = 'owner' | 'weaver' | 'none'

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
    }
  }
}
